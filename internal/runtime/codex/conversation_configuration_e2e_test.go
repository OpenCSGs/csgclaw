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

func TestNativeCodexRoomTurnsOverlapWithSeparateRoutes(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "host"))
	t.Setenv("CSGCLAW_EXECUTION_MODE", "standard")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	type inference struct{ route, model, effort string }
	started := make(chan inference, 4)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Model     string                  `json:"model"`
			Reasoning struct{ Effort string } `json:"reasoning"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		select {
		case started <- inference{r.URL.Path, input.Model, input.Reasoning.Effort}:
		case <-r.Context().Done():
			return
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		case <-ctx.Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"room-test\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"id\":\"msg-1\",\"content\":[{\"type\":\"output_text\",\"text\":\"done\"}]}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"room-test\",\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\n")
	}))
	defer endpoint.Close()

	base := agentruntime.Profile{Provider: "api", ModelID: "fixture-a", BaseURL: endpoint.URL + "/llm", APIKey: "fixture", ModelMetadata: modelcap.Resolved{ContextWindow: 32000, ContextSource: "provider"}}
	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: binary}, EventSink: NewEventSink(),
		AgentHome: func(string) (string, error) { return filepath.Join(home, "agent"), nil },
		ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "alice", Name: "alice", Profile: base, RuntimeOptions: map[string]any{memoryModeOptionKey: MemoryModeDisabled}}, nil
		},
		ResolveRoomProfile: func(_, room string, cfg taskconfig.Config) (agentruntime.Profile, error) {
			profile := base
			profile.BaseURL = endpoint.URL + "/tasks/" + room + "/llm"
			profile.ModelID, profile.ReasoningEffort = cfg.ModelID, cfg.ReasoningEffort
			return profile, nil
		},
		ModelCatalog: func(string) []codexmodel.Profile {
			return []codexmodel.Profile{{Provider: base.Provider, ModelID: "fixture-b", Metadata: base.ModelMetadata}}
		},
	})
	defer rt.Close()
	defer unblock()
	handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", AgentName: "alice", Profile: base})
	if err != nil {
		t.Fatal(err)
	}
	manager := rt.sessionManager().(*appServerManager)
	original := manager.liveSession(handle.RuntimeID)
	results := make(chan contract.TurnResult, 2)
	rooms := []struct{ id, model, effort string }{{"a", "fixture-a", "high"}, {"b", "fixture-b", "low"}}
	var requestA contract.TurnRequest
	for _, room := range rooms {
		dir := filepath.Join(home, "room-"+room.id)
		request := contract.TurnRequest{
			ID: contract.TurnID(room.id), ConversationKey: contract.ConversationKey("room:" + room.id),
			Input:             []contract.InputPart{{Kind: contract.InputPartText, Text: "Reply done."}},
			RoomConfiguration: &contract.RoomConfiguration{RoomID: room.id, Configuration: taskconfig.Config{WorkspaceDir: dir, ModelID: room.model, ReasoningEffort: room.effort}},
		}
		if room.id == "a" {
			requestA = request
		}
		go func() {
			results <- rt.Conversation(handle.RuntimeID).Run(ctx, request, nil)
		}()
	}
	want := map[inference]bool{
		{"/tasks/a/llm/responses", "fixture-a", "high"}: true,
		{"/tasks/b/llm/responses", "fixture-b", "low"}:  true,
	}
	for range rooms {
		select {
		case got := <-started:
			if !want[got] {
				t.Fatalf("unexpected or duplicate inference %+v", got)
			}
			delete(want, got)
		case result := <-results:
			t.Fatalf("turn returned before both inferences overlapped: %+v", result)
		case <-ctx.Done():
			t.Fatal("both rooms did not reach inference while the first was blocked")
		}
	}
	if live := manager.liveSession(handle.RuntimeID); live != original {
		t.Fatal("room turns replaced the app-server")
	}
	for _, room := range rooms {
		original.mu.Lock()
		threadID := original.conversationSessions["room:"+room.id]
		original.mu.Unlock()
		raw, err := original.appClient.request(ctx, "thread/read", map[string]any{"threadId": threadID})
		if err != nil {
			t.Fatal(err)
		}
		var response struct{ Thread struct{ Cwd string } }
		if err := json.Unmarshal(raw, &response); err != nil {
			t.Fatal(err)
		}
		expected, err := filepath.EvalSymlinks(filepath.Join(home, "room-"+room.id))
		if err != nil {
			t.Fatal(err)
		}
		actual, err := filepath.EvalSymlinks(response.Thread.Cwd)
		if err != nil || actual != expected {
			t.Fatalf("room %s thread cwd = %q, want %q (%v)", room.id, response.Thread.Cwd, expected, err)
		}
	}
	unblock()
	for range rooms {
		select {
		case result := <-results:
			if result.Status != contract.TurnSucceeded {
				t.Fatalf("room turn failed: %+v", result)
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for both turns to complete")
		}
	}

	original.mu.Lock()
	originalThread := original.conversationSessions["room:a"]
	original.mu.Unlock()
	if _, err := rt.Stop(ctx, handle); err != nil {
		t.Fatal(err)
	}
	requestA.ID = "follow-up-a"
	if result := rt.Conversation(handle.RuntimeID).Run(ctx, requestA, nil); result.Status != contract.TurnSucceeded {
		t.Fatalf("follow-up after restart failed: %+v", result)
	}
	live := manager.liveSession(handle.RuntimeID)
	live.mu.Lock()
	resumedThread := live.conversationSessions["room:a"]
	live.mu.Unlock()
	if resumedThread != originalThread {
		t.Fatalf("room A resumed %q, want %q", resumedThread, originalThread)
	}
	select {
	case got := <-started:
		if expected := (inference{"/tasks/a/llm/responses", "fixture-a", "high"}); got != expected {
			t.Fatalf("resumed room inference = %+v, want %+v", got, expected)
		}
	default:
		t.Fatal("resumed turn did not reach inference")
	}
}
