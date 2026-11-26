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
	"slices"
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

type modelSelection struct{ model, effort string }

func TestNativeSessionModelOptionsThroughBridge(t *testing.T) {
	b := newNativeSessionModelBridge(t, "low")
	initial := b.pid()
	override, profile := modelSelection{"model-b", "high"}, modelSelection{"model-a", "low"}
	b.turn("a", override, 100, override)
	b.turn("b", modelSelection{}, 100, profile)
	b.turn("a", modelSelection{}, 100, profile)
	b.turn("b", modelSelection{}, 100, profile)
	if b.pid() != initial {
		t.Fatal("selecting an unlisted model restarted the App Server")
	}
}

func TestNativeSessionModelCompactionThroughBridge(t *testing.T) {
	for _, tc := range []struct {
		name    string
		next    modelSelection
		restart bool
	}{
		{name: "profile_default"},
		{name: "explicit_model", next: modelSelection{model: "model-a"}},
		{name: "after_restart", restart: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newNativeSessionModelBridge(t, "none")
			override, profile := modelSelection{"model-b", "high"}, modelSelection{"model-a", "none"}
			b.turn("a", override, 40000, override)
			b.turn("b", modelSelection{}, 100, profile)
			if tc.restart {
				initial := b.pid()
				if _, err := b.controller.Stop(b.ctx, b.agentID); err != nil {
					t.Fatal(err)
				}
				if _, err := b.controller.Start(b.ctx, b.agentID); err != nil {
					t.Fatal(err)
				}
				if b.pid() == initial {
					t.Fatal("App Server did not restart")
				}
			}
			pid := b.pid()
			b.turn("a", tc.next, 100, override, profile)
			b.turn("b", modelSelection{}, 100, profile)
			if b.pid() != pid {
				t.Fatal("compaction restarted the App Server")
			}
		})
	}
}

func TestNativeSessionModelUpdatedWindowThroughBridge(t *testing.T) {
	b := newNativeSessionModelBridge(t, "none")
	override, profile := modelSelection{"model-b", "high"}, modelSelection{"model-a", "none"}
	b.turn("a", modelSelection{}, 100, profile)
	b.turn("b", modelSelection{}, 100, profile)
	setWindow := func(window int64) {
		cfg := b.providers.Normalized()
		provider := cfg.Providers["fixture"]
		provider.Models = []string{"model-a", "model-b"}
		provider.ModelOverrides = map[string]modelcap.Metadata{"model-b": {ContextWindow: window}}
		cfg.Providers["fixture"] = provider
		b.controller.Models().SetLLMConfig(cfg)
	}
	setWindow(32000)
	b.turn("a", override, 40000, override)
	pid := b.pid()
	b.turn("a", override, 100, override, override)
	b.turn("b", modelSelection{}, 100, profile)
	if b.pid() != pid {
		t.Fatal("unchanged window restarted the App Server")
	}
	setWindow(128000)
	b.turn("a", override, 40000, override)
	b.turn("a", override, 100, override)
}

type sessionModelBridgePayload struct {
	Model     string            `json:"model"`
	Stream    bool              `json:"stream"`
	Tools     []json.RawMessage `json:"tools"`
	Reasoning struct {
		Effort string `json:"effort"`
	} `json:"reasoning"`
}

type nativeSessionModelBridge struct {
	t                  *testing.T
	ctx                context.Context
	server             *httptest.Server
	controller         *agent.Controller
	runtime            *codex.Runtime
	agentID, runtimeID string
	providers          config.LLMConfig
	bindings           *agentsession.Store
	threads            map[string]string
	mu                 sync.Mutex
	upstream, native   []sessionModelBridgePayload
	inputTokens        int
}

func newNativeSessionModelBridge(t *testing.T, defaultEffort string) *nativeSessionModelBridge {
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

	b := &nativeSessionModelBridge{t: t, threads: make(map[string]string)}
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
		b.mu.Lock()
		b.upstream = append(b.upstream, payload)
		responseID := fmt.Sprintf("fixture-%d", len(b.upstream))
		inputTokens := b.inputTokens
		b.mu.Unlock()
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
	t.Cleanup(upstream.Close)

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
		"fixture": {BaseURL: upstream.URL + "/v1", APIKey: "fixture-key", APIProtocol: "responses", Models: []string{"model-a"}, ReasoningEffort: defaultEffort, ModelMetadata: map[string]modelcap.Metadata{"model-a": {ContextWindow: 32000}}},
	}}
	server := httptest.NewUnstartedServer(nil)
	t.Cleanup(server.Close)
	controller, err := agent.NewControllerWithLLM(providers, config.ServerConfig{ListenAddr: server.Listener.Addr().String(), AccessToken: "fixture-token"}, "", statePath, runtimewiring.WithCodexRuntime())
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { controller.Close() })
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
	t.Cleanup(func() { handler.CloseConnectors() })
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
				b.mu.Lock()
				b.native = append(b.native, payload)
				b.mu.Unlock()
			}
		}
		routes.ServeHTTP(w, r)
	})
	server.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	started, err := controller.Start(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	runtimeImpl, err := controller.Runtime(agent.RuntimeKindCodex)
	if err != nil {
		t.Fatal(err)
	}
	b.ctx, b.server, b.controller = ctx, server, controller
	b.runtime, b.runtimeID, b.agentID = runtimeImpl.(*codex.Runtime), started.RuntimeID, item.ID
	b.providers, b.bindings = providers, bindings
	initialProfile, err := controller.ResolvedAgentProfile(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		profile, err := controller.ResolvedAgentProfile(item.ID)
		if err != nil || !reflect.DeepEqual(profile, initialProfile) {
			t.Errorf("agent defaults changed: model=%q effort=%q error=%v", profile.ModelID, profile.ReasoningEffort, err)
		}
	})
	return b
}

func (b *nativeSessionModelBridge) pid() int {
	b.t.Helper()
	live, err := b.runtime.SessionManager().LiveSession(codex.SessionHandle{RuntimeID: b.runtimeID})
	if err != nil || live.ProcessID <= 0 {
		b.t.Fatalf("live session=%+v error=%v", live, err)
	}
	return live.ProcessID
}

func (b *nativeSessionModelBridge) turn(session string, request modelSelection, tokens int, want ...modelSelection) {
	t := b.t
	t.Helper()
	b.mu.Lock()
	beforeUpstream, beforeNative := len(b.upstream), len(b.native)
	b.inputTokens = tokens
	b.mu.Unlock()
	payload := map[string]any{"input": "Reply done."}
	if request.model != "" {
		payload["model"] = request.model
	}
	if request.effort != "" {
		payload["reasoning_effort"] = request.effort
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(b.ctx, http.MethodPost, b.server.URL+"/api/v1/agents/"+b.agentID+"/sessions/"+session+"/responses", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer fixture-token")
	response, err := b.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var result agentSessionResponse
	if err := json.Unmarshal(body, &result); err != nil || response.StatusCode != http.StatusOK || result.Status != "completed" {
		t.Fatalf("session %s: HTTP %d body=%s error=%v", session, response.StatusCode, body, err)
	}

	b.mu.Lock()
	upstream := slices.Clone(b.upstream[beforeUpstream:])
	native := slices.Clone(b.native[beforeNative:])
	b.mu.Unlock()
	for _, traffic := range []struct {
		name  string
		calls []sessionModelBridgePayload
	}{{"native", native}, {"upstream", upstream}} {
		if len(traffic.calls) != len(want) {
			t.Fatalf("session %s %s calls=%d, want %v", session, traffic.name, len(traffic.calls), want)
		}
		for i, call := range traffic.calls {
			answer := i == len(want)-1
			if call.Model != want[i].model || (len(call.Tools) > 0) != answer || ((answer || traffic.name == "upstream") && call.Reasoning.Effort != want[i].effort) {
				t.Errorf("session %s %s call %d: model=%q effort=%q tools=%d, want %+v answer=%t", session, traffic.name, i, call.Model, call.Reasoning.Effort, len(call.Tools), want[i], answer)
			}
		}
	}
	binding, ok := b.bindings.Lookup(b.agentID, session)
	if !ok {
		t.Fatal("missing session binding")
	}
	thread, exists, err := b.runtime.ExistingEngineSession(b.ctx, b.runtimeID, binding.ConversationKey)
	if err != nil || !exists || thread == "" {
		t.Fatalf("native thread=%q exists=%t error=%v", thread, exists, err)
	}
	for key, previous := range b.threads {
		if (key == session) != (previous == thread) {
			t.Fatalf("session %s uses thread %s; previous bindings %v", session, thread, b.threads)
		}
	}
	b.threads[session] = thread
}
