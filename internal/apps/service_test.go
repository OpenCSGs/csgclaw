package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/localstore"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newTestService(t *testing.T, options Options) *Service {
	t.Helper()
	s, err := NewService(filepath.Join(t.TempDir(), "state.json"), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func upstreamServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, &mcp.ServerOptions{Logger: quietLogger})
	server.AddTool(&mcp.Tool{Name: "inspect", Title: "Inspect resource", Description: "Read an authorized resource", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}}, OutputSchema: map[string]any{"type": "object"}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}, Meta: mcp.Meta{"fixture": true}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		credential := req.Extra.Header.Get("Authorization")
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: credential}, &mcp.ImageContent{MIMEType: "image/png", Data: []byte{1, 2, 3}}}, StructuredContent: map[string]any{"credential": credential}, Meta: mcp.Meta{"fixture": true}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true, Logger: quietLogger})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)
	return httpServer
}

func gatewayClient(t *testing.T, s *Service, agentID string) *mcp.ClientSession {
	t.Helper()
	httpServer := httptest.NewServer(s.Handler(agentID))
	t.Cleanup(httpServer.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-codex", Version: "1"}, &mcp.ClientOptions{Logger: quietLogger, Capabilities: &mcp.ClientCapabilities{}})
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func addHTTP(t *testing.T, s *Service, agentID, appID, name, endpoint, token string) Installation {
	t.Helper()
	item, err := s.Create(context.Background(), agentID, CreateRequest{AppID: appID, Name: name, Config: Config{Transport: "http", URL: endpoint, AuthMode: "bearer"}, Credentials: Credentials{Token: token}, Connect: true})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func TestHTTPAppsGatewayIsolationAndRevocation(t *testing.T) {
	upstream := upstreamServer(t)
	var changes atomic.Int32
	s := newTestService(t, Options{OnCatalogChanged: func(string, uint64) { changes.Add(1) }})
	a := addHTTP(t, s, "agent-a", "gitlab", "Work", upstream.URL, "work-token")
	b := addHTTP(t, s, "agent-a", "gitlab", "Personal", upstream.URL, "personal-token")
	_ = addHTTP(t, s, "agent-b", "llm-wiki", "Wiki", upstream.URL, "wiki-token")
	client := gatewayClient(t, s, "agent-a")
	other := gatewayClient(t, s, "agent-b")
	listed, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 2 {
		t.Fatalf("got %d tools", len(listed.Tools))
	}
	otherTools, err := other.ListTools(context.Background(), nil)
	if err != nil || len(otherTools.Tools) != 1 {
		t.Fatalf("other agent tools: %v %v", otherTools, err)
	}
	name := toolName(a.InstallationID, "inspect")
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{"query": "resource"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content[0].(*mcp.TextContent).Text != "Bearer work-token" || len(result.Content) != 2 || result.StructuredContent == nil {
		t.Fatalf("lost upstream content: %#v", result)
	}
	var found *mcp.Tool
	for _, tool := range listed.Tools {
		if tool.Name == name {
			found = tool
		}
	}
	if found == nil || found.Title != "Inspect resource" || found.OutputSchema == nil || !found.Annotations.ReadOnlyHint {
		t.Fatal("lost tool metadata")
	}
	if _, err := other.CallTool(context.Background(), &mcp.CallToolParams{Name: name}); err == nil {
		t.Fatal("other Agent invoked an unowned tool")
	}
	if _, err := s.Get(context.Background(), "agent-b", a.InstallationID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-agent installation lookup succeeded")
	}
	if _, err := s.Disconnect(context.Background(), "agent-a", a.InstallationID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: name}); err == nil {
		t.Fatal("disconnected tool remained callable")
	}
	result, err = client.CallTool(context.Background(), &mcp.CallToolParams{Name: toolName(b.InstallationID, "inspect")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content[0].(*mcp.TextContent).Text != "Bearer personal-token" {
		t.Fatal("wrong per-instance credential")
	}
	if changes.Load() < 4 {
		t.Fatal("missing catalog revision notifications")
	}
	if err := s.Delete(context.Background(), "agent-a", b.InstallationID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: toolName(b.InstallationID, "inspect")}); err == nil {
		t.Fatal("removed tool remained callable")
	}
}

func TestPersistedDisconnectAndProtectedConfiguration(t *testing.T) {
	upstream := upstreamServer(t)
	s := newTestService(t, Options{})
	item, err := s.Create(context.Background(), "agent", CreateRequest{AppID: "gitlab", Name: "GitLab", Config: Config{URL: upstream.URL, AuthMode: "none", Headers: map[string]string{"Authorization": "Bearer hidden"}, Env: map[string]string{"EXTRA_SECRET": "hidden-env"}}, Connect: true})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(item)
	if bytes.Contains(data, []byte("hidden")) {
		t.Fatal("secret leaked through App view")
	}
	info, err := os.Stat(s.path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("state permissions: %v %v", info, err)
	}
	if _, err := s.Disconnect(context.Background(), "agent", item.InstallationID); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	restored, err := NewService(s.path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.RestoreAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	current, err := restored.Get(context.Background(), "agent", item.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if !current.Disconnected || current.Status != "disconnected" || len(current.Tools) != 0 {
		t.Fatalf("disconnect intent was lost: %+v", current)
	}
	data, _ = os.ReadFile(s.path)
	if !bytes.Contains(data, []byte("hidden")) {
		t.Fatal("disconnect removed credentials owned by the global resource")
	}
}

func TestRestoreEnabledAndDisabledIntent(t *testing.T) {
	upstream := upstreamServer(t)
	s := newTestService(t, Options{})
	a := addHTTP(t, s, "agent", "gitlab", "Enabled", upstream.URL, "enabled")
	b := addHTTP(t, s, "agent", "gitlab", "Disabled", upstream.URL, "disabled")
	enabled := false
	if _, err := s.Update(context.Background(), "agent", b.InstallationID, UpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	restored, err := NewService(s.path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.RestoreAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	one, _ := restored.Get(context.Background(), "agent", a.InstallationID)
	two, _ := restored.Get(context.Background(), "agent", b.InstallationID)
	if one.Status != "connected" || two.Status != "disabled" || len(two.Tools) != 0 {
		t.Fatalf("wrong restoration state: %s / %s", one.Status, two.Status)
	}
	enabled = true
	if _, err := restored.Update(context.Background(), "agent", b.InstallationID, UpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	two, _ = restored.Get(context.Background(), "agent", b.InstallationID)
	if two.Status != "connected" {
		t.Fatalf("reenable did not reconnect: %s", two.Status)
	}
}

func TestConnectCompletionCannotRestoreRemovedApp(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	upstream := upstreamServer(t)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		<-release
		upstream.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	s := newTestService(t, Options{})
	item, err := s.Create(context.Background(), "agent", CreateRequest{AppID: "gitlab", Name: "Delayed", Config: Config{URL: proxy.URL}, Credentials: Credentials{Token: "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Connect(context.Background(), "agent", item.InstallationID); done <- err }()
	<-started
	if err := s.Delete(context.Background(), "agent", item.InstallationID); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("late connection restored deleted App")
	}
	items, _ := s.List(context.Background(), "agent")
	if len(items) != 0 {
		t.Fatal("removed installation was recreated")
	}
}

func TestStateUpdatesPreserveOtherSectionsAndInstallations(t *testing.T) {
	s := newTestService(t, Options{})
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.Create(context.Background(), "agent", CreateRequest{AppID: "gitlab", Name: string(rune('A' + i))})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := localstore.UpdateObjectSection(s.path, "other", func(values map[string]json.RawMessage) error { values["kept"] = json.RawMessage(`true`); return nil }); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	reloaded, err := NewService(s.path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	items, _ := reloaded.List(context.Background(), "agent")
	if len(items) != 12 {
		t.Fatalf("lost installations: %d", len(items))
	}
	var other map[string]bool
	if _, err := localstore.ReadSection(s.path, "other", &other); err != nil || !other["kept"] {
		t.Fatal("lost unrelated state")
	}
}

func TestAgentMCPSessionsCannotCrossPaths(t *testing.T) {
	s := newTestService(t, Options{})
	mux := http.NewServeMux()
	mux.Handle("/a", s.Handler("a"))
	mux.Handle("/b", s.Handler("b"))
	server := httptest.NewServer(mux)
	defer server.Close()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	sessionID := response.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("missing legacy MCP session ID")
	}
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/b", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", sessionID)
	req.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	response, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-agent session status = %d", response.StatusCode)
	}
}

// This subprocess runs a real stdio MCP server. It intentionally reports only
// fixture credentials and whether an unrelated host secret reached its env.
func TestAppStdioHelper(t *testing.T) {
	if os.Getenv("CSGCLAW_APP_TEST_HELPER") != "1" {
		return
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "stdio-fixture", Version: "1"}, &mcp.ServerOptions{Logger: quietLogger})
	if os.Getenv("CSGCLAW_APP_PATHS_HELPER") == "1" {
		cwd, _ := os.Getwd()
		paths, _ := json.Marshal(map[string]string{"home": os.Getenv("HOME"), "data": os.Getenv("PLUGIN_DATA"), "plugin": os.Getenv("PLUGIN_ROOT"), "cwd": cwd})
		server.AddTool(&mcp.Tool{Name: "directories", Description: string(paths), InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(paths)}}}, nil
		})
	}
	server.AddTool(&mcp.Tool{Name: "environment", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: os.Getenv("FEISHU_APP_ID") + ":" + os.Getenv("FEISHU_APP_SECRET") + ":" + os.Getenv("UNRELATED_HOST_SECRET")}}}, nil
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestFeishuStdioChannelRotationAndManualDisconnect(t *testing.T) {
	t.Setenv("UNRELATED_HOST_SECRET", "must-not-inherit")
	var mu sync.Mutex
	value := FeishuCredentials{AppID: "fixture-id", AppSecret: "fixture-one"}
	s := newTestService(t, Options{ResolveFeishu: func(context.Context, string) (FeishuCredentials, error) {
		mu.Lock()
		defer mu.Unlock()
		return value, nil
	}})
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	item, err := s.Create(context.Background(), "agent", CreateRequest{AppID: "feishu", Name: "Feishu", Config: Config{Transport: "stdio", Command: executable, Args: []string{"-test.run=^TestAppStdioHelper$"}, AuthMode: "feishu", CredentialSource: "feishu_channel", Env: map[string]string{"CSGCLAW_APP_TEST_HELPER": "1"}}, Connect: true})
	if err != nil {
		t.Fatal(err)
	}
	client := gatewayClient(t, s, "agent")
	name := toolName(item.InstallationID, "environment")
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Content[0].(*mcp.TextContent).Text; got != "fixture-id:fixture-one:" {
		t.Fatalf("credential/env isolation failed: %q", got)
	}
	data, _ := os.ReadFile(s.path)
	if bytes.Contains(data, []byte("fixture-one")) {
		t.Fatal("channel secret was copied into installation")
	}
	mu.Lock()
	value.AppSecret = "fixture-two"
	mu.Unlock()
	if err := s.RefreshCredentials(context.Background(), "agent"); err != nil {
		t.Fatal(err)
	}
	result, err = client.CallTool(context.Background(), &mcp.CallToolParams{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Content[0].(*mcp.TextContent).Text; got != "fixture-id:fixture-two:" {
		t.Fatalf("stale channel credential: %q", got)
	}
	if _, err := s.Disconnect(context.Background(), "agent", item.InstallationID); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	value.AppSecret = "fixture-three"
	mu.Unlock()
	if err := s.RefreshCredentials(context.Background(), "agent"); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(context.Background(), "agent", item.InstallationID)
	if !current.Disconnected || current.Status != "disconnected" {
		t.Fatal("channel update restored manually disconnected App")
	}
}

func TestUnsupportedOAuthAndProbeDoesNotPersist(t *testing.T) {
	s := newTestService(t, Options{})
	_, err := s.Create(context.Background(), "agent", CreateRequest{AppID: "gitlab", Name: "OAuth", Config: Config{AuthMode: "oauth2"}})
	if !errors.Is(err, ErrUnsupportedOAuth) {
		t.Fatalf("OAuth not rejected: %v", err)
	}
	upstream := upstreamServer(t)
	result, err := s.Probe(context.Background(), "agent", ProbeRequest{AppID: "llm-wiki", Config: Config{URL: upstream.URL}, Credentials: Credentials{Token: "probe-token"}})
	if err != nil || !result.Connected || len(result.Tools) != 1 {
		t.Fatalf("probe: %+v %v", result, err)
	}
	items, _ := s.List(context.Background(), "agent")
	if len(items) != 0 {
		t.Fatal("probe saved an installation")
	}
}

func TestAddKeepsFailedConnectionAsEditableInstallation(t *testing.T) {
	s := newTestService(t, Options{})
	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "bad credential", http.StatusUnauthorized) }))
	defer unauthorized.Close()
	item, err := s.Create(context.Background(), "agent", CreateRequest{AppID: "gitlab", Name: "Failed connection", Config: Config{URL: unauthorized.URL}, Credentials: Credentials{Token: "fixture"}, Connect: true})
	if err != nil || item.InstallationID == "" || item.Status != "authorization_required" || item.LastErrorCode != "app_platform_unauthorized" || item.LastErrorHTTPStatus != 401 || item.LastError == "" {
		t.Fatalf("missing repairable installation: %+v %v", item, err)
	}
	items, _ := s.List(context.Background(), "agent")
	if len(items) != 1 {
		t.Fatal("missing saved installation")
	}
}

func TestCallbacksRunOutsideServiceLock(t *testing.T) {
	upstream := upstreamServer(t)
	var s *Service
	s = newTestService(t, Options{OnCatalogChanged: func(agentID string, _ uint64) { _, _ = s.List(context.Background(), agentID); _ = s.Revision(agentID) }})
	done := make(chan struct{})
	go func() { defer close(done); addHTTP(t, s, "agent", "gitlab", "App", upstream.URL, "fixture") }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("callback ran under service lock")
	}
}

func TestAuthorizationRevocationRemovesTools(t *testing.T) {
	upstream := upstreamServer(t)
	var revoked atomic.Bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if revoked.Load() {
			http.Error(w, "expired token", http.StatusUnauthorized)
			return
		}
		upstream.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	s := newTestService(t, Options{})
	item := addHTTP(t, s, "agent", "gitlab", "GitLab", proxy.URL, "fixture")
	client := gatewayClient(t, s, "agent")
	revoked.Store(true)
	_, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: toolName(item.InstallationID, "inspect")})
	if err == nil {
		t.Fatal("revoked credentials remained callable")
	}
	current, _ := s.Get(context.Background(), "agent", item.InstallationID)
	if current.Status != "authorization_required" {
		t.Fatalf("status after revocation: %s", current.Status)
	}
	listed, err := client.ListTools(context.Background(), nil)
	if err != nil || len(listed.Tools) != 0 {
		t.Fatal("revoked App tools remained in catalog")
	}
}

func TestUpstreamToolCatalogChangesRefreshAgent(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "changing", Version: "1"}, &mcp.ServerOptions{Logger: quietLogger})
	handle := func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
	}
	server.AddTool(&mcp.Tool{Name: "first", InputSchema: map[string]any{"type": "object"}}, handle)
	upstream := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true, Logger: quietLogger}))
	t.Cleanup(upstream.Close)
	s := newTestService(t, Options{})
	item := addHTTP(t, s, "agent", "gitlab", "GitLab", upstream.URL, "fixture")
	client := gatewayClient(t, s, "agent")
	server.AddTool(&mcp.Tool{Name: "second", InputSchema: map[string]any{"type": "object"}}, handle)
	deadline := time.Now().Add(5 * time.Second)
	for {
		listed, err := client.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(listed.Tools) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tools/list_changed did not refresh App tools")
		}
		time.Sleep(10 * time.Millisecond)
	}
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: toolName(item.InstallationID, "second")})
	if err != nil || result.IsError {
		t.Fatalf("new tool call failed: %v", err)
	}
}

func TestGatewayAppIdentityAndRenameRefresh(t *testing.T) {
	upstream := upstreamServer(t)
	s := newTestService(t, Options{})
	work := addHTTP(t, s, "agent", "gitlab", "Work GitLab", upstream.URL, "work-token")
	personal := addHTTP(t, s, "agent", "gitlab", "Personal GitLab", upstream.URL, "personal-token")
	client := gatewayClient(t, s, "agent")
	check := func(want string) {
		t.Helper()
		result, err := client.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Tools) != 2 {
			t.Fatal("expected both installations")
		}
		for _, tool := range result.Tools {
			expected := want
			id := work.InstallationID
			if tool.Name == toolName(personal.InstallationID, "inspect") {
				expected = "Personal GitLab"
				id = personal.InstallationID
			}
			for _, part := range []string{expected, "service: gitlab", id, "Read an authorized resource"} {
				if !strings.Contains(tool.Description, part) {
					t.Errorf("missing source metadata %q", part)
				}
			}
			if strings.Contains(tool.Description, "work-token") || strings.Contains(tool.Description, "personal-token") {
				t.Fatal("secret in tool discovery")
			}
		}
	}
	check("Work GitLab")
	before := s.Revision("agent")
	updated := "Company GitLab"
	_, err := s.Update(context.Background(), "", work.ResourceID, UpdateRequest{Name: &updated})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(context.Background(), "agent", work.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "connected" || s.Revision("agent") <= before {
		t.Fatal("rename did not refresh live catalog")
	}
	check(updated)
	if got.Tools[0].Description != "Read an authorized resource" {
		t.Fatal("upstream schema was mutated")
	}
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: toolName(work.InstallationID, "inspect"), Arguments: map[string]any{"query": "resource"}})
	if err != nil || result.IsError {
		t.Fatal("renaming broke stable tool identity")
	}
}
