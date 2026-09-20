package apps

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReadOnlyFiltersAppAndPlatformToolsAndRejectsStaleCalls(t *testing.T) {
	var readonly atomic.Bool
	var writes atomic.Int32
	readonly.Store(true)
	server := mcp.NewServer(&mcp.Implementation{Name: "readonly-fixture", Version: "1"}, &mcp.ServerOptions{Logger: quietLogger})
	handler := func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "read"}}}, nil
	}
	server.AddTool(&mcp.Tool{Name: "read", InputSchema: map[string]any{"type": "object"}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, handler)
	server.AddTool(&mcp.Tool{Name: "write", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		writes.Add(1)
		return &mcp.CallToolResult{}, nil
	})
	upstream := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true, Logger: quietLogger}))
	t.Cleanup(upstream.Close)
	s := newTestService(t, Options{ReadOnly: func(string) bool { return readonly.Load() }})
	item := addHTTP(t, s, "agent", "gitlab", "GitLab", upstream.URL, "fixture")
	s.RegisterTool("agent", &mcp.Tool{Name: "platform_write", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		writes.Add(1)
		return &mcp.CallToolResult{}, nil
	})
	client := gatewayClient(t, s, "agent")
	listed, err := client.ListTools(context.Background(), nil)
	if err != nil || len(listed.Tools) != 1 || listed.Tools[0].Name != toolName(item.InstallationID, "read") {
		t.Fatalf("readonly catalog: %+v %v", listed, err)
	}
	if _, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: toolName(item.InstallationID, "read")}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: toolName(item.InstallationID, "write")}); err == nil {
		t.Fatal("readonly write was admitted")
	}
	readonly.Store(false)
	listed, err = client.ListTools(context.Background(), nil)
	if err != nil || len(listed.Tools) != 3 {
		t.Fatalf("read-write catalog was not restored: %+v %v", listed, err)
	}
	if _, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: toolName(item.InstallationID, "write")}); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	generation := s.entries[item.InstallationID].generation
	s.mu.Unlock()
	readonly.Store(true)
	// Exercise the invocation gate without going through a new tools/list or
	// HTTP request, simulating an already-held tool name and handler.
	_, err = s.call(context.Background(), "agent", item.InstallationID, generation, "write", &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: []byte(`{}`)}})
	if err == nil || writes.Load() != 1 {
		t.Fatal("stale write handler bypassed read-only policy")
	}
	if _, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "platform_write"}); err == nil {
		t.Fatal("platform write bypassed read-only policy")
	}
}

func TestAppUploadUsesOwnedCredentialsAndSameOriginPUT(t *testing.T) {
	var received atomic.Int32
	upstream := upstreamServer(t)
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/upload" {
			upstream.Config.Handler.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodPut || r.Header.Get("Authorization") != "Bearer fixture-upload" || r.Header.Get("Content-Type") != "application/pdf" || r.ContentLength != 7 {
			t.Errorf("wrong upload request: %s %s %d", r.Method, r.Header.Get("Content-Type"), r.ContentLength)
			http.Error(w, "invalid", 400)
			return
		}
		data, _ := io.ReadAll(r.Body)
		if string(data) != "PDFDATA" {
			t.Error("upload body changed")
		}
		received.Add(1)
		_, _ = io.WriteString(w, `{"token":"fixture-upload","signed_url":"secret-query"}`)
	}))
	t.Cleanup(service.Close)
	s := newTestService(t, Options{})
	item := addHTTP(t, s, "agent-a", "llm-wiki", "Wiki", service.URL+"/mcp", "fixture-upload")
	result, err := s.Upload(context.Background(), "agent-a", item.InstallationID, "/upload", "file.pdf", "application/pdf", 7, strings.NewReader("PDFDATA-excess"))
	if err != nil {
		t.Fatal(err)
	}
	if received.Load() != 1 || bytes.Contains(result, []byte("fixture-upload")) || bytes.Contains(result, []byte("signed_url")) {
		t.Fatalf("unsafe upload result: %s", result)
	}
	if _, err := s.Upload(context.Background(), "agent-b", item.InstallationID, "/upload", "file.pdf", "application/pdf", 7, strings.NewReader("PDFDATA")); err == nil {
		t.Fatal("cross-agent upload allowed")
	}
	if _, err := s.Upload(context.Background(), "agent-a", item.InstallationID, "https://other.example/upload", "file.pdf", "application/pdf", 7, strings.NewReader("PDFDATA")); err == nil {
		t.Fatal("cross-origin upload allowed")
	}
	if _, err := s.Disconnect(context.Background(), "agent-a", item.InstallationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upload(context.Background(), "agent-a", item.InstallationID, "/upload", "file.pdf", "application/pdf", 7, strings.NewReader("PDFDATA")); err == nil {
		t.Fatal("disconnected upload allowed")
	}
}

func TestAppUploadRejectsRedirectAndSupportsEmptyFiles(t *testing.T) {
	var leaked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer other.Close()
	upstream := upstreamServer(t)
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, other.URL+"/stolen", http.StatusTemporaryRedirect)
		case "/empty":
			if r.ContentLength != 0 {
				t.Error("empty upload has unknown length")
			}
			if data, _ := io.ReadAll(r.Body); len(data) != 0 {
				t.Error("empty upload included bytes")
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			upstream.Config.Handler.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(service.Close)
	s := newTestService(t, Options{})
	item := addHTTP(t, s, "agent", "gitlab", "App", service.URL+"/mcp", "fixture")
	if _, err := s.Upload(context.Background(), "agent", item.InstallationID, "/redirect", "empty.txt", "", 0, nil); err == nil {
		t.Fatal("upload redirect allowed")
	}
	if leaked.Load() != 0 {
		t.Fatal("credentials or file sent to redirect target")
	}
	if _, err := s.Upload(context.Background(), "agent", item.InstallationID, "/empty", "empty.txt", "text/plain", 0, nil); err != nil {
		t.Fatal(err)
	}
}

func TestAppUploadCancellationAndReadOnly(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	upstream := upstreamServer(t)
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/slow" {
			upstream.Config.Handler.ServeHTTP(w, r)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	t.Cleanup(service.Close)
	var readonly atomic.Bool
	s := newTestService(t, Options{ReadOnly: func(string) bool { return readonly.Load() }})
	item := addHTTP(t, s, "agent", "gitlab", "App", service.URL+"/mcp", "fixture")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := s.Upload(ctx, "agent", item.InstallationID, "/slow", "file.txt", "text/plain", 4, strings.NewReader("data"))
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled upload succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("upload ignored cancellation")
	}
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream request was not cancelled")
	}
	readonly.Store(true)
	if _, err := s.Upload(context.Background(), "agent", item.InstallationID, "/slow", "file.txt", "text/plain", 4, strings.NewReader("data")); err == nil {
		t.Fatal("readonly upload allowed")
	}
}

func TestAppUploadRejectsStdio(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s := newTestService(t, Options{})
	item, err := s.Create(context.Background(), "agent", CreateRequest{AppID: "feishu", Name: "Stdio", Config: Config{Transport: "stdio", Command: executable, Args: []string{"-test.run=^TestAppStdioHelper$"}, AuthMode: "none", Env: map[string]string{"CSGCLAW_APP_TEST_HELPER": "1"}}, Connect: true})
	if err != nil || item.Status != "connected" {
		t.Fatalf("stdio setup failed: %v %s", err, item.LastError)
	}
	if _, err := s.Upload(context.Background(), "agent", item.InstallationID, "/upload", "file.txt", "text/plain", 0, nil); err == nil {
		t.Fatal("stdio upload allowed")
	}
}
