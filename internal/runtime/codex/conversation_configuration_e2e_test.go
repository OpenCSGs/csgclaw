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
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/codexmodel"
	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/taskconfig"
)

func TestNativeRoomProfileCompactionAfterModelChange(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "host"))
	type selection struct{ model, effort string }
	var mu sync.Mutex
	var requests []selection
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Model     string `json:"model"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, selection{payload.Model, payload.Reasoning.Effort})
		n := len(requests)
		mu.Unlock()
		tokens := 100
		if n == 1 {
			tokens = 40000
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(v any) {
			data, _ := json.Marshal(v)
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": fmt.Sprint(n)}})
		emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": fmt.Sprint(n), "content": []any{map[string]any{"type": "output_text", "text": "done"}}}})
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": fmt.Sprint(n), "usage": map[string]any{"input_tokens": tokens, "output_tokens": 20, "total_tokens": tokens + 20}}})
	}))
	defer endpoint.Close()
	profile := agentruntime.Profile{Provider: "api", ModelID: "fixture-a", BaseURL: endpoint.URL + "/v1", APIKey: "fixture", ReasoningEffort: "low", ModelMetadata: modelcap.Resolved{ContextWindow: 32000, ContextSource: "provider"}}
	rt := New(Dependencies{
		EventSink: NewEventSink(), BinaryProvider: fakeBinaryProvider{path: binary},
		AgentHome: func(string) (string, error) { return filepath.Join(home, "agent"), nil },
		ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "alice", Name: "alice", Profile: profile, RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
		},
		ResolveRoomProfile: func(_, _ string, cfg taskconfig.Config) (agentruntime.Profile, error) {
			selected := profile
			selected.ModelID, selected.ReasoningEffort = cfg.ModelID, cfg.ReasoningEffort
			if cfg.ModelID == "fixture-b" {
				selected.ModelMetadata = modelcap.Resolved{ContextWindow: 64000, ContextSource: "user"}
			}
			return selected, nil
		},
		ModelCatalog: func(string) []codexmodel.Profile {
			return []codexmodel.Profile{{Provider: profile.Provider, ModelID: profile.ModelID, Metadata: profile.ModelMetadata}}
		},
	})
	defer rt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", AgentName: "alice", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	var previousPID int
	var previousThread string
	for i, step := range []struct {
		model, effort string
		window        int64
	}{{"fixture-b", "high", 64000}, {"fixture-a", "medium", 32000}} {
		request := contract.TurnRequest{
			ID: contract.TurnID(fmt.Sprint(i)), ConversationKey: "room:one",
			Input:             []contract.InputPart{{Kind: contract.InputPartText, Text: "Reply done."}},
			RoomConfiguration: &contract.RoomConfiguration{RoomID: "one", Configuration: taskconfig.Config{ModelID: step.model, ReasoningEffort: step.effort}},
		}
		result := rt.Conversation(handle.RuntimeID).Run(ctx, request, contract.EventSinkFunc(func(context.Context, contract.TurnEvent) error { return nil }))
		if result.Status != contract.TurnSucceeded {
			t.Fatalf("step %d: %+v", i, result)
		}
		live := rt.sessionManager().(*appServerManager).liveSession(handle.RuntimeID)
		live.mu.Lock()
		thread := live.conversationSessions["room:one"]
		usage := live.contextUsage[thread]
		live.mu.Unlock()
		if thread == "" || (previousThread != "" && previousThread != thread) {
			t.Fatalf("native history changed: %q -> %q", previousThread, thread)
		}
		if usage.ContextWindow != step.window {
			t.Fatalf("context window = %d, want %d", usage.ContextWindow, step.window)
		}
		pid := live.cmd.Process.Pid
		if previousPID == pid {
			t.Fatal("room Profile change did not refresh the runtime")
		}
		previousPID, previousThread = pid, thread
	}
	mu.Lock()
	got := append([]selection(nil), requests...)
	mu.Unlock()
	want := []selection{{"fixture-b", "high"}, {"fixture-a", "medium"}, {"fixture-a", "medium"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("answer/compaction models = %v, want %v", got, want)
	}
}
