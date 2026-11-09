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
	"reflect"
	"strings"
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
	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/codex"
)

func TestNativeSessionModelOptionsThroughBridge(t *testing.T) {
	runNativeSessionModelBridge(t, 100, false)
}

func TestNativeSessionModelCompactionThroughBridge(t *testing.T) {
	for _, explicitModel := range []bool{true, false} {
		name := "profile_default"
		if explicitModel {
			name = "explicit_model"
		}
		t.Run(name, func(t *testing.T) {
			runNativeSessionModelBridge(t, 40000, explicitModel)
		})
	}
}

type sessionModelBridgePayload struct {
	Model     string            `json:"model"`
	Stream    bool              `json:"stream"`
	Tools     []json.RawMessage `json:"tools"`
	Reasoning struct {
		Effort string `json:"effort"`
	} `json:"reasoning"`
}

func runNativeSessionModelBridge(t *testing.T, firstInputTokens int, explicitDefaultModel bool) {
	t.Helper()
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
	defaultEffort := "low"
	if firstInputTokens == 40000 {
		defaultEffort = "none"
	}
	var mu sync.Mutex
	var observed []sessionModelBridgePayload
	var native []sessionModelBridgePayload
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
		var payload sessionModelBridgePayload
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
		observed = append(observed, payload)
		responseID := fmt.Sprintf("fixture-%d", len(observed))
		inputTokens := 100
		if len(observed) == 1 {
			inputTokens = firstInputTokens
		}
		mu.Unlock()
		if payload.Model == "model-b" && payload.Reasoning.Effort != "high" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"model-b requires high reasoning effort","type":"invalid_request_error","param":"reasoning.effort"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(value any) {
			data, _ := json.Marshal(value)
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": responseID}})
		emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": responseID, "content": []any{map[string]any{"type": "output_text", "text": "Done."}}}})
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": responseID, "status": "completed", "usage": map[string]any{"input_tokens": inputTokens, "output_tokens": 10, "total_tokens": inputTokens + 10}}})
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
	item.AgentProfile = agent.AgentProfile{Provider: agent.ProviderAPI, ModelProviderID: "fixture", ModelID: "model-a", ReasoningEffort: defaultEffort, ProfileComplete: true}
	item.RuntimeOptions = map[string]any{"memory_mode": "disabled", "local_workspace_dir": workspace}
	statePath := filepath.Join(root, "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{item}); err != nil {
		t.Fatal(err)
	}
	providers := config.LLMConfig{Default: "fixture.model-a", Providers: map[string]config.ProviderConfig{
		"fixture": {BaseURL: upstream.URL + "/v1", APIKey: "fixture-key", APIProtocol: "responses", Models: []string{"model-a", "model-b"}, ReasoningEffort: defaultEffort, ModelMetadata: map[string]modelcap.Metadata{"model-a": {ContextWindow: 32000}, "model-b": {ContextWindow: 64000}}},
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
	routes := handler.Routes()
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/llm/") {
			body, err := io.ReadAll(r.Body)
			r.Body.Close()
			if err != nil {
				t.Error(err)
				http.Error(w, "read model request", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			var payload sessionModelBridgePayload
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Error(err)
			} else if payload.Stream {
				mu.Lock()
				native = append(native, payload)
				mu.Unlock()
			}
		}
		routes.ServeHTTP(w, r)
	})
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
	if initial.ProcessID <= 0 {
		t.Fatalf("invalid runtime PID: %d", initial.ProcessID)
	}
	initialProfile, err := controller.ResolvedAgentProfile(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	finalBody := map[string]any{"input": "Reply done again."}
	if explicitDefaultModel {
		finalBody["model"] = "model-a"
	}
	threads := map[string]string{}
	for stepIndex, step := range []struct {
		session string
		body    map[string]any
		want    selection
	}{
		{"session-a", map[string]any{"input": "Reply done.", "model": "model-b", "reasoning_effort": "high"}, selection{"model-b", "high"}},
		{"session-b", map[string]any{"input": "Reply done."}, selection{"model-a", defaultEffort}},
		{"session-a", finalBody, selection{"model-a", defaultEffort}},
		{"session-b", map[string]any{"input": "Reply done again."}, selection{"model-a", defaultEffort}},
	} {
		mu.Lock()
		beforeUpstream, beforeNative := len(observed), len(native)
		mu.Unlock()
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
		upstreamCalls := append([]sessionModelBridgePayload(nil), observed[beforeUpstream:]...)
		nativeCalls := append([]sessionModelBridgePayload(nil), native[beforeNative:]...)
		mu.Unlock()
		wantModels := []string{step.want.model}
		compaction := firstInputTokens == 40000 && stepIndex == 2
		if compaction {
			wantModels = []string{"model-b", "model-a"}
		}
		for _, traffic := range []struct {
			name  string
			calls []sessionModelBridgePayload
		}{{"native", nativeCalls}, {"upstream", upstreamCalls}} {
			models := make([]string, 0, len(traffic.calls))
			for _, call := range traffic.calls {
				models = append(models, call.Model)
			}
			if !reflect.DeepEqual(models, wantModels) {
				t.Errorf("session %s %s models=%v, want %v", step.session, traffic.name, models, wantModels)
			}
			if len(traffic.calls) != len(wantModels) {
				continue
			}
			if compaction && len(traffic.calls[0].Tools) != 0 {
				t.Errorf("%s compaction request unexpectedly includes tools", traffic.name)
			}
			if compaction && traffic.name == "upstream" && traffic.calls[0].Reasoning.Effort != "high" {
				t.Errorf("compaction effort=%q, want the previous model's accepted high effort", traffic.calls[0].Reasoning.Effort)
			}
			last := traffic.calls[len(traffic.calls)-1]
			if last.Reasoning.Effort != step.want.effort || len(last.Tools) == 0 {
				t.Errorf("session %s %s answer effort=%q tools=%d, want effort=%q and tools", step.session, traffic.name, last.Reasoning.Effort, len(last.Tools), step.want.effort)
			}
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
	if err != nil || !reflect.DeepEqual(profile, initialProfile) {
		t.Fatalf("agent defaults changed: model=%q effort=%q error=%v", profile.ModelID, profile.ReasoningEffort, err)
	}
	mu.Lock()
	defer mu.Unlock()
	wantCalls := 4
	if firstInputTokens == 40000 {
		wantCalls++
	}
	if len(observed) != wantCalls || len(native) != wantCalls {
		t.Fatalf("upstream calls=%d native calls=%d, want %d", len(observed), len(native), wantCalls)
	}
}
