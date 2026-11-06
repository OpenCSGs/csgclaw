package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentsession"
	"csgclaw/internal/app/runtimewiring"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/codex"
)

func TestNativeSessionModelOptionsThroughBridge(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Production wiring resolves the bundled CLI beside the Go test executable.
	bundled := filepath.Join(filepath.Dir(executable), "codex")
	if err := os.Symlink(binary, bundled); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(bundled) })

	type selection struct{ model, effort string }
	var mu sync.Mutex
	var observed []selection
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"model-a"},{"id":"model-b"}]}`)
			return
		}
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Model     string `json:"model"`
			Stream    bool   `json:"stream"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if !payload.Stream {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"probe","object":"response","status":"completed","output":[]}`)
			return
		}
		mu.Lock()
		observed = append(observed, selection{payload.Model, payload.Reasoning.Effort})
		responseID := fmt.Sprintf("fixture-%d", len(observed))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(value any) {
			data, _ := json.Marshal(value)
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": responseID}})
		emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": responseID, "content": []any{map[string]any{"type": "output_text", "text": "Done."}}}})
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": responseID, "status": "completed", "usage": map[string]any{"input_tokens": 100, "output_tokens": 10, "total_tokens": 110}}})
	}))
	defer upstream.Close()

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	item := sessionCodexAgent("agent-native", "Native")
	item.Status = string(agentruntime.StateStopped)
	item.DesiredState = agent.DesiredStateRunning
	item.AgentProfile = agent.AgentProfile{Provider: agent.ProviderAPI, ModelProviderID: "fixture", ModelID: "model-a", ReasoningEffort: "low", ProfileComplete: true}
	item.RuntimeOptions = map[string]any{"memory_mode": "disabled", "local_workspace_dir": workspace}
	statePath := filepath.Join(root, "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{item}); err != nil {
		t.Fatal(err)
	}
	providers := config.LLMConfig{Default: "fixture.model-a", Providers: map[string]config.ProviderConfig{
		"fixture": {BaseURL: upstream.URL + "/v1", APIKey: "fixture-key", APIProtocol: "responses", Models: []string{"model-a", "model-b"}, ReasoningEffort: "low"},
	}}
	server := httptest.NewUnstartedServer(nil)
	defer server.Close()
	controller, err := agent.NewControllerWithLLM(providers, config.ServerConfig{ListenAddr: server.Listener.Addr().String(), AccessToken: "fixture-token"}, "", statePath, runtimewiring.WithCodexRuntime())
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	defer controller.Close()
	engine := agentengine.New(controller)
	bindings, err := agentsession.NewStore(filepath.Join(root, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	messages, err := im.NewServiceFromPath(filepath.Join(root, "im", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandlerWithAuth(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, engine, messages, nil, im.NewParticipantBridge(""), nil, llm.NewService(config.ModelConfig{}, controller), "fixture-token", true)
	if err := handler.EnableConnectors(filepath.Join(root, "apps.json")); err != nil {
		t.Fatal(err)
	}
	defer handler.CloseConnectors()
	handler.SetAgentEngine(engine, bindings)
	server.Config.Handler = handler.Routes()
	server.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	started, err := controller.Start(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	runtimeImpl, err := controller.Runtime(agent.RuntimeKindCodex)
	if err != nil {
		t.Fatal(err)
	}
	runtime := runtimeImpl.(*codex.Runtime)
	handle := codex.SessionHandle{RuntimeID: started.RuntimeID}
	initial, err := runtime.SessionManager().LiveSession(handle)
	if err != nil {
		t.Fatal(err)
	}
	threads := map[string]string{}
	for _, step := range []struct {
		session string
		body    map[string]any
		want    selection
	}{
		{"session-a", map[string]any{"input": "Reply done.", "model": "model-b", "reasoning_effort": "high"}, selection{"model-b", "high"}},
		{"session-b", map[string]any{"input": "Reply done."}, selection{"model-a", "low"}},
		{"session-a", map[string]any{"input": "Reply done again."}, selection{"model-a", "low"}},
	} {
		payload, err := json.Marshal(step.body)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/agents/"+item.ID+"/sessions/"+step.session+"/responses", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer fixture-token")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		var result agentSessionResponse
		decodeErr := json.Unmarshal(body, &result)
		if readErr != nil || decodeErr != nil || response.StatusCode != http.StatusOK || result.Status != "completed" {
			t.Fatalf("session %s: HTTP %d body=%s read=%v decode=%v", step.session, response.StatusCode, body, readErr, decodeErr)
		}
		mu.Lock()
		count := len(observed)
		var last selection
		if count > 0 {
			last = observed[count-1]
		}
		mu.Unlock()
		if last != step.want {
			t.Fatalf("session %s upstream selection=%+v, want %+v", step.session, last, step.want)
		}
		binding, ok := bindings.Lookup(item.ID, step.session)
		if !ok {
			t.Fatal("missing session binding")
		}
		thread, exists, err := runtime.ExistingEngineSession(ctx, started.RuntimeID, binding.ConversationKey)
		if err != nil || !exists || thread == "" {
			t.Fatalf("native thread=%q exists=%t error=%v", thread, exists, err)
		}
		if previous := threads[step.session]; previous != "" && previous != thread {
			t.Fatalf("session %s changed thread from %s to %s", step.session, previous, thread)
		}
		threads[step.session] = thread
		live, err := runtime.SessionManager().LiveSession(handle)
		if err != nil || live.ProcessID != initial.ProcessID {
			t.Fatalf("runtime restarted: initial PID=%d session=%+v error=%v", initial.ProcessID, live, err)
		}
	}
	if threads["session-a"] == threads["session-b"] {
		t.Fatal("sessions share a native thread")
	}
	profile, err := controller.ResolvedAgentProfile(item.ID)
	if err != nil || profile.ModelID != "model-a" || profile.ReasoningEffort != "low" {
		t.Fatalf("agent defaults changed: profile=%+v error=%v", profile, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(observed) != 3 {
		t.Fatalf("upstream calls=%d, want 3", len(observed))
	}
}
