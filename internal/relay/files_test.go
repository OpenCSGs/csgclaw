package relay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"
	"github.com/gorilla/websocket"
)

func TestRelayContentClient(t *testing.T) {
	data := []byte("generated artifact")
	file := File{Name: "report.txt", MediaType: "text/plain", Size: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}
	scope := fileScope{ResponseID: "resp_test", AgentID: "agt_test", InstanceID: "instance"}
	var uploaded bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("X-CSG-Client-ID") != "test-device" {
			t.Error("missing Relay credentials")
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/relay/v1/files":
			var request struct {
				File
				fileScope
				Transfer string
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Transfer != "relay" || request.fileScope != scope || r.Header.Get("Idempotency-Key") != "stable-key" {
				t.Errorf("upload request: %+v, %v", request, err)
			}
			result := file
			result.ID, result.Status = "file_test", "uploading"
			if uploaded {
				result.Status = "available"
			}
			_ = json.NewEncoder(w).Encode(result)
		case r.Method == "PUT" && r.URL.Path == "/api/v1/relay/v1/files/file_test/content":
			body, err := io.ReadAll(r.Body)
			if err != nil || !bytes.Equal(body, data) || r.ContentLength != int64(len(data)) {
				t.Errorf("upload content: %q, %v", body, err)
			}
			uploaded = true
			result := file
			result.ID, result.Status = "file_test", "available"
			_ = json.NewEncoder(w).Encode(result)
		case r.Method == "POST" && r.URL.Path == "/api/v1/relay/v1/files/file_test/content":
			var request fileScope
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request != scope {
				t.Errorf("download scope: %+v, %v", request, err)
			}
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1/relay", "test-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.WithClientID("test-device")
	var opens int
	source := func() (io.ReadCloser, error) {
		opens++
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	first, err := client.uploadFile(context.Background(), scope, "stable-key", file, source)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.uploadFile(context.Background(), scope, "stable-key", file, source)
	if err != nil || second != first || opens != 1 {
		t.Fatalf("upload replay: %+v, opened %d times, %v", second, opens, err)
	}
	target, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if err := client.downloadFile(context.Background(), scope, first, target); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(target)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("download: %q, %v", got, err)
	}
}

func TestExecutorReplaysFileExecutionPhaseAfterDisconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	disconnected := make(chan struct{})
	replayed := make(chan Frame, 2)
	var connections, executions atomic.Int32
	u := websocket.Upgrader{Subprotocols: []string{Subprotocol}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := u.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		number := connections.Add(1)
		var f Frame
		if conn.ReadJSON(&f) != nil || conn.WriteJSON(Frame{Type: "hello.ack", HeartbeatIntervalSeconds: 15}) != nil {
			return
		}
		if number == 1 {
			if conn.WriteJSON(Frame{Type: "request", ResponseID: "resp_reconnect", SessionID: "sess_reconnect",
				Input: textInput("hello"), InputFiles: []File{{ID: "file_input"}}, Deadline: time.Now().Add(time.Minute)}) == nil &&
				conn.ReadJSON(&f) == nil && f.Type == "request.accepted" {
				_ = conn.Close()
				close(disconnected)
			}
			return
		}
		for conn.ReadJSON(&f) == nil {
			if f.Type == "request.accepted" || f.Type == "request.started" {
				select {
				case replayed <- f:
				case <-ctx.Done():
					return
				}
			}
		}
	}))
	defer server.Close()
	e := &Executor{Client: testClient(t, server.URL, "secret"), AgentID: "agt_reconnect",
		StatePath: filepath.Join(t.TempDir(), "state.json"), Handler: handlerFunc{run: func(runCtx context.Context, frame Frame) Frame {
			executions.Add(1)
			select {
			case <-disconnected:
			case <-runCtx.Done():
				return Frame{Type: "response.cancelled"}
			}
			executionCtx, stop := frame.start()
			defer stop()
			<-executionCtx.Done()
			return Frame{Type: "response.cancelled"}
		}}}
	startExecutor(t, e)
	for _, want := range []string{"request.accepted", "request.started"} {
		select {
		case f := <-replayed:
			if f.Type != want || f.ResponseID != "resp_reconnect" {
				t.Fatalf("unexpected replay: %+v, want %s", f, want)
			}
		case <-ctx.Done():
			t.Fatalf("missing replay of %s", want)
		}
	}
	if executions.Load() != 1 {
		t.Fatalf("executions = %d, want 1", executions.Load())
	}
}

func TestExecutorRestartReplaysDeliveryAndRejectsOversizedResult(t *testing.T) {
	const id = "resp_oversized"
	path := filepath.Join(t.TempDir(), "state.json")
	state := executorState{Version: 1, Records: map[string]executionRecord{
		id: {InstanceID: "previous-incarnation", Result: Frame{Type: "request.accepted", ResponseID: id},
			Delivery: &fileDelivery{SessionID: "sess_oversized", Text: strings.Repeat("x", 2<<20),
				Deadline: time.Now().Add(time.Minute), Attachments: []apitypes.MessageAttachment{{ID: "snapshot"}}}},
	}, Sessions: map[string]executionSession{}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	received := make(chan Frame, 4)
	u := websocket.Upgrader{Subprotocols: []string{Subprotocol}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agents/local" {
			writeTestJSON(w, http.StatusOK, map[string]any{"id": "local", "status": "running", "runtime": map[string]string{"name": "codex"}})
			return
		}
		if r.URL.Path == "/v1/files" {
			writeTestJSON(w, http.StatusOK, File{ID: "file_delivered", Status: "available"})
			return
		}
		conn, err := u.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var f Frame
		if conn.ReadJSON(&f) != nil || conn.WriteJSON(Frame{Type: "hello.ack", HeartbeatIntervalSeconds: 15}) != nil {
			return
		}
		for conn.ReadJSON(&f) == nil {
			if f.ResponseID != id {
				continue
			}
			select {
			case received <- f:
			default:
			}
			if f.Type == "response.failed" {
				_ = conn.WriteJSON(Frame{Type: "result.ack", ResponseID: id})
			}
		}
	}))
	defer server.Close()
	e := &Executor{Client: testClient(t, server.URL, "secret"), AgentID: "agt_restart", StatePath: path, Handler: &LocalExecutor{Client: apiclient.New(server.URL, "local-token", server.Client()), AgentID: "local"}}
	startExecutor(t, e)
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	for _, want := range []string{"request.accepted", "request.started", "response.delivering", "response.failed"} {
		select {
		case f := <-received:
			if f.Type != want || (want == "response.failed" && (f.Error == nil || f.Error.Code != "output_too_large" || f.Sequence != 1)) {
				t.Fatalf("unexpected recovery frame %s, error %v, sequence %d, want %s", f.Type, f.Error, f.Sequence, want)
			}
		case <-ctx.Done():
			t.Fatalf("missing recovery frame %s", want)
		}
	}
}

func TestExecutorDeliversRemainingFilesAfterAnUploadFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var file File
		if err := json.NewDecoder(r.Body).Decode(&file); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if file.Name == "failed.txt" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		writeTestJSON(w, http.StatusOK, File{ID: "file_" + file.Name, Status: "available"})
	}))
	defer server.Close()
	e := Executor{Client: testClient(t, server.URL, "secret"), Handler: &LocalExecutor{}}
	result := e.deliverFiles(t.Context(), fileScope{ResponseID: "resp_partial"}, &fileDelivery{
		Text: "Original reply", Deadline: time.Now().Add(time.Minute),
		Attachments: []apitypes.MessageAttachment{
			{ID: "before", Name: "before.txt"},
			{ID: "failed", Name: "failed.txt"},
			{ID: "after", Name: "after.txt"},
		},
	})
	if result.Type != "response.completed" || result.OutputText != "Original reply\n\nOutput attachment delivery failed for 1 of 3 files." {
		t.Fatalf("unexpected partial result: %+v", result)
	}
	if len(result.Files) != 2 || result.Files[0].ID != "file_before.txt" || result.Files[1].ID != "file_after.txt" {
		t.Fatalf("unexpected delivered files: %+v", result.Files)
	}
}

func TestExecutorRetriesInputDownloadWithoutBreakingSession(t *testing.T) {
	for _, tc := range []struct {
		name         string
		downloadable bool
		wantAttempts int32
		wantTip      string
	}{
		{name: "retry replaces partial file", downloadable: true, wantAttempts: 2, wantTip: "resp_current"},
		{name: "failed download preserves previous turn", wantAttempts: 3, wantTip: "resp_previous"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const content = "verified input"
			var attempts, runs atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/api/v1/agents/local":
					writeTestJSON(w, http.StatusOK, map[string]any{"id": "local", "status": "running", "runtime": map[string]string{"name": "codex"}})
				case r.URL.Path == "/v1/files/file_input/content":
					if attempt := attempts.Add(1); tc.downloadable && attempt > 1 {
						_, _ = io.WriteString(w, content)
					} else {
						_, _ = io.WriteString(w, "partial")
					}
				case strings.HasSuffix(r.URL.Path, "/attachments"):
					file, header, err := r.FormFile("files")
					if err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					defer file.Close()
					if got := header.Header.Get("Content-Type"); got != "image/png" {
						t.Errorf("session image media type = %q", got)
					}
					data, err := io.ReadAll(file)
					if err != nil || string(data) != content {
						t.Errorf("session input was not verified: %q, %v", data, err)
					}
					writeTestJSON(w, http.StatusCreated, map[string]any{"attachments": []apitypes.MessageAttachment{{ID: "local_input"}}})
				case strings.HasSuffix(r.URL.Path, "/responses"):
					runs.Add(1)
					writeTestJSON(w, http.StatusOK, Response{Status: "completed"})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			e := &Executor{Client: testClient(t, server.URL, "secret"), AgentID: "agt_test", instance: "test-instance",
				StatePath: filepath.Join(t.TempDir(), "state.json"),
				Handler:   &LocalExecutor{Client: apiclient.New(server.URL, "local-token", server.Client()), AgentID: "local"},
				records:   make(map[string]executionRecord), sessions: map[string]executionSession{"sess_test": {Tip: "resp_previous"}}}
			frame := Frame{Type: "request", ResponseID: "resp_current", SessionID: "sess_test", PreviousResponseID: "resp_previous",
				Input: textInput("read attachment"), Deadline: time.Now().Add(5 * time.Second),
				InputFiles: []File{{ID: "file_input", Name: "input.png", MediaType: "image/png", Size: int64(len(content)),
					SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(content)))}}}
			if err := e.accept(t.Context(), frame, func(Frame) error { return nil }); err != nil {
				t.Fatal(err)
			}
			e.workers.Wait()
			if got := attempts.Load(); got != tc.wantAttempts {
				t.Fatalf("download attempts = %d, want %d", got, tc.wantAttempts)
			}
			if got := e.sessions[frame.SessionID]; got.Dirty || got.Tip != tc.wantTip {
				t.Fatalf("session = %+v, want clean tip %q", got, tc.wantTip)
			}
			result := e.records[frame.ResponseID].Result
			if tc.downloadable {
				if runs.Load() != 1 || result.Type != "response.completed" {
					t.Fatalf("want one execution, got %d and %+v", runs.Load(), result)
				}
			} else if runs.Load() != 0 || result.Error == nil || result.Error.Code != "input_file_unavailable" {
				t.Fatalf("want preparation failure before execution, got %d and %+v", runs.Load(), result)
			}
		})
	}
}

func TestExecutorReservesTimeForPartialDeliveryResult(t *testing.T) {
	const window = 500 * time.Millisecond
	start := time.Now()
	deadline := deliveryUploadDeadline(start, window)
	if reserve := start.Add(window).Sub(deadline); reserve != window/5 {
		t.Fatalf("delivery reserve = %s, want %s", reserve, window/5)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var file File
		if err := json.NewDecoder(r.Body).Decode(&file); err != nil {
			t.Error(err)
			return
		}
		if file.Name == "ready.txt" {
			writeTestJSON(w, http.StatusOK, File{ID: "file_ready", Status: "available"})
			return
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	e := Executor{Client: testClient(t, server.URL, "secret"), Handler: &LocalExecutor{}}
	result := e.deliverFiles(t.Context(), fileScope{ResponseID: "resp_partial"}, &fileDelivery{
		Text: "Completed reply", Deadline: deadline,
		Attachments: []apitypes.MessageAttachment{{ID: "ready", Name: "ready.txt"}, {ID: "blocked", Name: "blocked.txt"}},
	})
	if result.Type != "response.completed" || len(result.Files) != 1 || !strings.HasPrefix(result.OutputText, "Completed reply") {
		t.Fatalf("partial result was lost: %+v", result)
	}
	if !time.Now().Before(start.Add(window)) {
		t.Fatal("no time remains to publish the partial result")
	}
}

func TestAttachmentSupportRejectsLegacyExecutorBeforeUpload(t *testing.T) {
	for _, capable := range []bool{false, true} {
		t.Run(fmt.Sprint(capable), func(t *testing.T) {
			var uploads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/agents/agt_test":
					agent := Agent{ID: "agt_test"}
					if capable {
						agent.Capabilities = []string{CapabilityInputFiles}
					}
					writeTestJSON(w, http.StatusOK, agent)
				case "/v1/files":
					uploads.Add(1)
					writeTestJSON(w, http.StatusOK, File{ID: "file_input", Status: "available"})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			manager, err := NewManager(t.Context(), t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			if err := manager.Configure(server.URL, "test-token"); err != nil {
				t.Fatal(err)
			}
			manager.manifest.Agents["agt_test"] = PublishRequest{Visibility: "public"}
			_, err = manager.UploadAttachment(t.Context(), "agt_test", manager.manifest.ConnectionID, "message",
				apitypes.MessageAttachment{ID: "input", Name: "input.txt"}, func() (io.ReadCloser, error) {
					t.Fatal("available upload was reopened")
					return nil, nil
				})
			if capable {
				if err != nil || uploads.Load() != 1 {
					t.Fatalf("capable executor upload = %d, %v", uploads.Load(), err)
				}
			} else if !errors.Is(err, ErrExecutorFilesUnsupported) || uploads.Load() != 0 {
				t.Fatalf("legacy executor upload = %d, %v", uploads.Load(), err)
			}
		})
	}
}
