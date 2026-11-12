package api

import (
	"bytes"
	"context"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentmemory"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeMemoryCompatRuntime struct {
	fakeCompatRuntime
}

func (f fakeMemoryCompatRuntime) ReadMemoryDocument(_ context.Context, _ string, options map[string]any) (agentruntime.MemoryDocument, error) {
	return agentruntime.MemoryDocument{
		Enabled:  options["memory_mode"] != "disabled",
		Ready:    true,
		Name:     "MEMORY.md",
		Location: "$RUNTIME_HOME/MEMORY.md",
		Content:  "# Readable memory\n",
	}, nil
}

func (f fakeMemoryCompatRuntime) ConfigureMemory(options map[string]any, enabled bool) (map[string]any, error) {
	next := make(map[string]any, len(options)+1)
	for key, value := range options {
		next[key] = value
	}
	if enabled {
		next["memory_mode"] = "enabled"
	} else {
		next["memory_mode"] = "disabled"
	}
	return next, nil
}

func TestAgentMemoryEndpointsExposeCapabilityDocumentAndToggle(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{{
		ID:             "agent-alice",
		Name:           "alice",
		RuntimeID:      "rt-agent-alice",
		RuntimeKind:    agent.RuntimeKindCodex,
		RuntimeOptions: map[string]any{"execution_mode": "standard", "memory_mode": "enabled"},
		Role:           agent.RoleWorker,
		Status:         string(agentruntime.StateStopped),
		CreatedAt:      time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}
	svc, err := agent.NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath,
		agent.WithRuntime(fakeMemoryCompatRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	handler := &Handler{svc: svc, agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc}

	agentRecorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(agentRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-alice", nil))
	if agentRecorder.Code != http.StatusOK {
		t.Fatalf("GET agent status = %d; body=%s", agentRecorder.Code, agentRecorder.Body.String())
	}
	var presented agentResponse
	if err := json.NewDecoder(agentRecorder.Body).Decode(&presented); err != nil {
		t.Fatal(err)
	}
	if !presented.MemorySupported {
		t.Fatal("agent response memory_supported = false")
	}

	getRecorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(getRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-alice/memory", nil))
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("GET memory status = %d; body=%s", getRecorder.Code, getRecorder.Body.String())
	}
	var before agent.MemoryDocument
	if err := json.NewDecoder(getRecorder.Body).Decode(&before); err != nil {
		t.Fatal(err)
	}
	if !before.Enabled || !before.Ready || before.Name != "MEMORY.md" || before.Location != "$RUNTIME_HOME/MEMORY.md" || before.Content != "# Readable memory\n" {
		t.Fatalf("GET memory = %#v", before)
	}

	putBody, err := json.Marshal(map[string]any{"enabled": false})
	if err != nil {
		t.Fatal(err)
	}
	putRecorder := httptest.NewRecorder()
	putRequest := httptest.NewRequest(http.MethodPut, "/api/v1/agents/agent-alice/memory", bytes.NewReader(putBody))
	putRequest.Header.Set("Content-Type", "application/json")
	handler.Routes().ServeHTTP(putRecorder, putRequest)
	if putRecorder.Code != http.StatusOK {
		t.Fatalf("PUT memory status = %d; body=%s", putRecorder.Code, putRecorder.Body.String())
	}
	var after agent.MemoryDocument
	if err := json.NewDecoder(putRecorder.Body).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.Enabled || !after.Ready || after.Name != "MEMORY.md" || after.Content != "# Readable memory\n" {
		t.Fatalf("PUT memory = %#v", after)
	}
	updated, ok := svc.Agent("agent-alice")
	if !ok || updated.RuntimeOptions["memory_mode"] != "disabled" || updated.RuntimeOptions["execution_mode"] != "standard" {
		t.Fatalf("updated runtime options = %#v", updated.RuntimeOptions)
	}
}

func TestManagedMemoryRequiresTrustedLocalAccess(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{{
		ID: "agent-alice", Name: "alice", RuntimeID: "rt-agent-alice", RuntimeKind: agent.RuntimeKindCodex,
		Role: agent.RoleWorker, Status: string(agentruntime.StateStopped),
	}}); err != nil {
		t.Fatal(err)
	}
	controller, err := agent.NewController(config.ModelConfig{}, config.ServerConfig{}, "", statePath,
		agent.WithRuntime(fakeMemoryCompatRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}}))
	if err != nil {
		t.Fatal(err)
	}
	store, err := agentmemory.Open(filepath.Join(root, "agents"), filepath.Join(root, "state.json"), "local")
	if err != nil {
		t.Fatal(err)
	}
	controller.SetManagedMemory(store.DeleteAgent, store.SeedTemplate)
	engine := agentengine.New(controller)
	engine.SetMemory(store)
	handler := &Handler{svc: controller, agentEngine: engine, serverNoAuth: true}
	handler.SetAgentMemory(store, "local")
	router := handler.Routes()

	request := func(method, address, body string, headers map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://127.0.0.1:18080/api/v1/agents/agent-alice/memory", strings.NewReader(body))
		r.RemoteAddr = address
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		result := httptest.NewRecorder()
		router.ServeHTTP(result, r)
		return result
	}
	body := `{"revision":0,"enabled":true}`
	for _, test := range []struct {
		name, address string
		headers       map[string]string
	}{
		{"remote", "192.0.2.1:5678", nil},
		{"proxy", "127.0.0.1:5678", map[string]string{"X-Forwarded-For": "192.0.2.1"}},
		{"cross-origin", "127.0.0.1:5678", map[string]string{"Origin": "https://another.example"}},
		{"agent", "127.0.0.1:5678", map[string]string{"Authorization": "Bearer agent.fixture"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := request(http.MethodPut, test.address, body, test.headers); got.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", got.Code, got.Body.String())
			}
		})
	}
	if got := request(http.MethodPut, "127.0.0.1:5678", body, nil); got.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPut, "127.0.0.1:5678", body, nil); got.Code != http.StatusConflict {
		t.Fatalf("stale revision status=%d body=%s", got.Code, got.Body.String())
	}
	handler.SetAgentMemory(store, "disabled")
	if got := request(http.MethodGet, "127.0.0.1:5678", "", nil); got.Code != http.StatusOK ||
		!strings.Contains(got.Body.String(), `"available":false`) {
		t.Fatalf("shared memory status=%d body=%s", got.Code, got.Body.String())
	}
}
