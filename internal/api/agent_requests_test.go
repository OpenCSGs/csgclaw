package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/enginetest"
	"csgclaw/internal/modelprovider"
)

func TestAgentPatchCanonicalFieldsPreserveOmissionAndClearCollections(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		clear      bool
	}{
		{"omitted", `{}`, false},
		{"null", `{"description":null,"runtime":{"image":null,"options":null},"model_config":null}`, false},
		{"empty collections", `{"description":"","runtime":{"options":{}},"mcp_servers":{}}`, true},
		{"null MCP", `{"description":"","runtime":{"options":{}},"mcp_servers":null}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seed := agentengine.Agent{ID: "agent-worker", ResourceVersion: "3", Spec: agentengine.AgentSpec{Name: "worker", Description: "keep", Role: agentengine.AgentRoleWorker, Runtime: agentengine.RuntimeSpec{Adapter: "dsh", Image: "worker:1", Options: map[string]any{"workspace": "/tmp"}}, Model: agentengine.ModelSpec{Selector: "configured", ModelID: "model-1", BaseURL: "https://example.test/v1"}, MCPServers: map[string]agentengine.MCPServerConfig{"docs": {"url": "https://example.test/mcp"}}}}
			client := enginetest.NewMemoryClient(seed)
			handler := &Handler{agentEngine: client}
			recorder := httptest.NewRecorder()
			handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, "/api/v1/agents/agent-worker", strings.NewReader(tc.body)))
			if recorder.Code != http.StatusOK {
				t.Fatalf("PATCH = %d: %s", recorder.Code, recorder.Body.String())
			}
			got, err := client.Agents().Get(context.Background(), seed.ID, agentengine.AgentGetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Spec.Name != seed.Spec.Name || got.Spec.Runtime.Image != seed.Spec.Runtime.Image || !reflect.DeepEqual(got.Spec.Model, seed.Spec.Model) {
				t.Fatalf("omitted configuration changed: %+v", got.Spec)
			}
			if tc.clear {
				if got.Spec.Description != "" || len(got.Spec.Runtime.Options) != 0 || len(got.Spec.MCPServers) != 0 {
					t.Fatalf("clear request retained configuration: %+v", got.Spec)
				}
			} else if got.Spec.Description != seed.Spec.Description || !reflect.DeepEqual(got.Spec.Runtime.Options, seed.Spec.Runtime.Options) || !reflect.DeepEqual(got.Spec.MCPServers, seed.Spec.MCPServers) {
				t.Fatalf("omitted collections changed: %+v", got.Spec)
			}
		})
	}
}

func TestAgentControlRejectsUnsupportedFieldsAndAmbiguousMasks(t *testing.T) {
	client := enginetest.NewMemoryClient(agentengine.Agent{ID: "agent-worker", Spec: agentengine.AgentSpec{Name: "worker", Role: agentengine.AgentRoleWorker, Runtime: agentengine.RuntimeSpec{Adapter: "dsh"}}})
	handler := &Handler{agentEngine: client}
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/agents", `null`},
		{http.MethodPatch, "/api/v1/agents/agent-worker", `null`},
		{http.MethodPost, "/api/v1/agents/agent-worker/recreate", `null`},
		{http.MethodPost, "/api/v1/agents", `{"name":"new","replace":true}`},
		{http.MethodPost, "/api/v1/agents", `{"name":"new","field_mask":[]}`},
		{http.MethodPost, "/api/v1/agents", `{"name":"new","runtime_name":"dsh"}`},
		{http.MethodPost, "/api/v1/agents", `{"name":"new","runtime":{"state":"running"}}`},
		{http.MethodPatch, "/api/v1/agents/agent-worker", `{"name":"changed","field_mask":["name"]}`},
		{http.MethodPatch, "/api/v1/agents/agent-worker", `{"model_config":"selector"}`},
		{http.MethodPatch, "/api/v1/agents/agent-worker", `{"runtime":{"credentials":{"token":"secret"}}}`},
		{http.MethodPost, "/api/v1/agents/agent-worker/recreate", `{"field_mask":["name"]}`},
		{http.MethodPost, "/api/v1/agents/agent-worker/recreate", `{"configuration":{},"field_mask":["runtime","runtime.options"]}`},
		{http.MethodPost, "/api/v1/agents/agent-worker/recreate", `{"configuration":{},"field_mask":["model_config","model_config.selector"]}`},
		{http.MethodPost, "/api/v1/agents/agent-worker/recreate", `{"configuration":{},"field_mask":["role"]}`},
		{http.MethodPost, "/api/v1/agents/agent-worker/recreate", `{"configuration":{"from_template":"builtin.worker"}}`},
	} {
		t.Run(tc.body, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.Routes().ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("%s %s = %d: %s", tc.method, tc.path, recorder.Code, recorder.Body.String())
			}
		})
	}
	got, err := client.Agents().Get(context.Background(), "agent-worker", agentengine.AgentGetOptions{})
	if err != nil || got.Spec.Name != "worker" {
		t.Fatalf("rejected request changed Agent: %+v, %v", got, err)
	}
}

func TestAgentProfileAndCanonicalResponseIncludeVideoConfiguration(t *testing.T) {
	video := &modelprovider.VideoGenerationConfig{ProviderID: "video-provider", ModelID: "video-model"}
	seed := agentengine.Agent{ID: "agent-worker", Spec: agentengine.AgentSpec{Name: "worker", Role: agentengine.AgentRoleWorker, Runtime: agentengine.RuntimeSpec{Adapter: "dsh", Image: "worker:1"}, Model: agentengine.ModelSpec{VideoGeneration: video}, MCPServers: map[string]agentengine.MCPServerConfig{"docs": {"url": "https://example.test/mcp"}}}}
	handler := &Handler{agentEngine: enginetest.NewMemoryClient(seed)}
	for _, path := range []string{"/api/v1/agents/agent-worker", "/api/v1/agents/agent-worker/profile"} {
		recorder := httptest.NewRecorder()
		handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, recorder.Code, recorder.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		profile := payload
		if path == "/api/v1/agents/agent-worker" {
			for _, field := range []string{"runtime_name", "sandbox_enabled", "image", "mcpServers", "agent_profile"} {
				if _, exists := payload[field]; exists {
					t.Fatalf("unexpected response field %s", field)
				}
			}
			runtime := payload["runtime"].(map[string]any)
			if runtime["image"] != "worker:1" || payload["mcp_servers"] == nil {
				t.Fatalf("canonical fields missing: %+v", payload)
			}
			profile = payload["model_config"].(map[string]any)
		}
		actual, ok := profile["video_generation"].(map[string]any)
		if !ok || actual["provider_id"] != video.ProviderID || actual["model_id"] != video.ModelID {
			t.Fatalf("video configuration = %#v", profile["video_generation"])
		}
	}
}

func TestAgentRecreateAcceptsVideoOnlyModelConfiguration(t *testing.T) {
	seed := agentengine.Agent{ID: "agent-worker", ResourceVersion: "1", Spec: agentengine.AgentSpec{Name: "worker", Role: agentengine.AgentRoleWorker, Runtime: agentengine.RuntimeSpec{Adapter: "dsh"}, Model: agentengine.ModelSpec{ModelID: "previous-model"}}}
	client := enginetest.NewMemoryClient(seed)
	handler := &Handler{agentEngine: client}
	recorder := httptest.NewRecorder()
	request := `{"configuration":{"name":"worker","model_config":{"video_generation":{"provider_id":"video-provider","model_id":"video-model"}}}}`
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-worker/recreate", strings.NewReader(request)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("recreate = %d: %s", recorder.Code, recorder.Body.String())
	}
	got, err := client.Agents().Get(context.Background(), seed.ID, agentengine.AgentGetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Model.VideoGeneration == nil || got.Spec.Model.VideoGeneration.ModelID != "video-model" || got.Spec.Model.ModelID != "" {
		t.Fatalf("model configuration was not replaced: %+v", got.Spec.Model)
	}
	if got.Spec.Runtime.Adapter != seed.Spec.Runtime.Adapter {
		t.Fatalf("runtime = %+v, want existing configuration", got.Spec.Runtime)
	}
}
