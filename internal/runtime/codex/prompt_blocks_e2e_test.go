package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/instructions"
)

func TestAppServerPromptInputPreservesTextBoundaries(t *testing.T) {
	got, err := appServerPromptInput(PromptRequest{Prompt: []PromptContentBlock{
		TextBlock("curl http://127.0.0.1:8888/"),
		TextBlock("Managed resource selection"),
		{LocalImage: &PromptLocalImageBlock{Path: "/workspace/image.png"}},
		{Resource: &PromptResourceBlock{Text: "Attachment context"}},
		TextBlock("  Keep these spaces.  "),
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{
		{"type": "text", "text": "curl http://127.0.0.1:8888/\n\nManaged resource selection"},
		{"type": "localImage", "path": "/workspace/image.png"},
		{"type": "text", "text": "Attachment context\n\n  Keep these spaces.  "},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("native input = %#v, want %#v", got, want)
	}
}

// Check what the real Codex process sends to the model, rather than assuming
// that separate native text inputs retain their boundaries.
func TestPromptBlockBoundariesBundledCodexE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	binary, _ = filepath.Abs(binary)
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("CODEX_HOME", filepath.Join(root, "host"))
	const command = "curl http://127.0.0.1:8888/"
	var observed atomic.Bool
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Input []struct {
				Role    string
				Content []struct{ Text string }
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		for _, item := range payload.Input {
			if item.Role != "user" {
				continue
			}
			var text strings.Builder
			for _, content := range item.Content {
				text.WriteString(content.Text)
			}
			if strings.Contains(text.String(), command) {
				observed.Store(true)
				if !strings.Contains(text.String(), command+"\n\n"+instructions.ResourceSelectionInstructions) {
					t.Errorf("model received joined user command and managed context: %q", text.String())
				}
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []any{
			map[string]any{"type": "response.created", "response": map[string]any{"id": "boundary"}},
			map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": "answer", "content": []any{map[string]any{"type": "output_text", "text": "OK"}}}},
			map[string]any{"type": "response.completed", "response": map[string]any{"id": "boundary", "usage": map[string]any{"input_tokens": 100, "output_tokens": 1, "total_tokens": 101}}},
		} {
			raw, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
		}
	}))
	defer endpoint.Close()
	profile := agentruntime.Profile{Provider: "api", ModelID: "fixture-boundary", BaseURL: endpoint.URL + "/v1", APIKey: "fixture", ModelMetadata: modelcap.Resolved{ContextWindow: 32768, ContextSource: "provider"}}
	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: binary}, EventSink: NewEventSink(),
		AgentHome: func(string) (string, error) { return filepath.Join(root, "agent"), nil },
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "alice", Name: "alice", RuntimeID: h.RuntimeID, Profile: profile, RuntimeOptions: map[string]any{"memory_mode": MemoryModeDisabled}}, nil
		},
	})
	defer rt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", AgentName: "alice", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := rt.EnsureEngineSession(ctx, handle.RuntimeID, "room:boundary")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.PromptTurn(ctx, handle.RuntimeID, PromptRequest{SessionID: thread, Prompt: []PromptContentBlock{TextBlock(command), TextBlock(instructions.ResourceSelectionInstructions)}}); err != nil {
		t.Fatal(err)
	}
	if !observed.Load() {
		t.Fatal("user command did not reach the model fixture")
	}
}
