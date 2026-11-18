package relay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
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
	"github.com/google/uuid"
)

func TestLiveCloudAttachmentsResumeDelivery(t *testing.T) {
	endpoint, publisher, caller, _ := liveRelayClients(t)
	publication := testPublish(t, publisher, "file-delivery", "public")
	content := []byte("artifact")
	sum := fmt.Sprintf("%x", sha256.Sum256(content))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	input, err := caller.uploadFile(ctx, fileScope{}, "input", File{Name: "input.txt", MediaType: "text/plain", Size: int64(len(content)), SHA256: sum}, func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(content)), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 1)
	var allowDelivery atomic.Bool
	var executed atomic.Int32
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/agents/local":
			writeTestJSON(w, 200, map[string]any{"id": "local", "status": "running", "runtime": map[string]any{"name": "codex"}})
		case strings.HasSuffix(r.URL.Path, "/attachments") && r.Method == http.MethodPost:
			source, _, err := r.FormFile("files")
			if err != nil {
				t.Errorf("session input: %v", err)
				w.WriteHeader(400)
				return
			}
			defer source.Close()
			got, err := io.ReadAll(source)
			if err != nil || !bytes.Equal(got, content) {
				t.Errorf("session input bytes: %q, %v", got, err)
			}
			writeTestJSON(w, 201, map[string]any{"attachments": []apitypes.MessageAttachment{{ID: "local-input"}}})
		case strings.HasSuffix(r.URL.Path, "/responses"):
			executed.Add(1)
			var payload struct {
				AttachmentIDs []string `json:"attachment_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || len(payload.AttachmentIDs) != 1 || payload.AttachmentIDs[0] != "local-input" {
				t.Errorf("session input references: %+v, %v", payload, err)
			}
			writeTestJSON(w, 200, Response{Status: "completed", Output: []Output{{
				Content:     []Content{{Type: "output_text", Text: "ready"}},
				Attachments: []apitypes.MessageAttachment{{ID: "snapshot", Name: "output.txt", MediaType: "text/plain", SizeBytes: int64(len(content)), SHA256: sum}},
			}}})
		case r.URL.Path == "/api/v1/attachments/snapshot":
			if !allowDelivery.Load() {
				select {
				case started <- struct{}{}:
				default:
				}
				<-r.Context().Done()
				return
			}
			_, _ = w.Write(content)
		default:
			http.NotFound(w, r)
		}
	}))
	defer local.Close()
	state := filepath.Join(t.TempDir(), "executor.json")
	newExecutor := func() *Executor {
		return &Executor{Client: publisher, AgentID: publication.Agent.ID, StatePath: state, Handler: &LocalExecutor{Client: apiclient.New(local.URL, "local-token", local.Client()), AgentID: "local", Scope: endpoint}}
	}
	execCtx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- newExecutor().Run(execCtx) }()
	waitOnline(t, caller, publication.Agent.ID)
	wireInput := fmt.Sprintf(`[{"role":"user","content":[{"type":"input_file","file_id":%q}]}]`, input.ID)
	key := uuid.NewString()
	type outcome struct {
		result Response
		err    error
	}
	called := make(chan outcome, 1)
	go func() {
		result, err := caller.Call(ctx, key, CallRequest{Model: publication.Agent.ID, Input: json.RawMessage(wireInput)})
		called <- outcome{result, err}
	}()
	select {
	case <-started:
	case result := <-called:
		t.Fatalf("task stopped before delivery: %+v, %v", result.result, result.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	allowDelivery.Store(true)
	executor := newExecutor()
	startExecutor(t, executor)
	var response Response
	select {
	case result := <-called:
		if result.err != nil {
			t.Fatal(result.err)
		}
		response = result.result
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if response.Status != "completed" || response.Text() != "ready" || executed.Load() != 1 {
		t.Fatalf("response = %+v, executions = %d", response, executed.Load())
	}
	var output *File
	for _, part := range response.Output[0].Content {
		if part.Type == "output_file" {
			output = part.File
		}
	}
	if output == nil {
		t.Fatal("response omitted its delivered attachment")
	}
	target, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if err := caller.downloadFile(ctx, fileScope{}, *output, target); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(target)
	if err != nil || !bytes.Equal(content, got) {
		t.Fatalf("output = %q, %v", got, err)
	}
	waitFor(t, func() bool {
		executor.mu.Lock()
		defer executor.mu.Unlock()
		return executor.records[response.ID].Acked
	})
}
