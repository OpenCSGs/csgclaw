package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentengine/enginetest"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/modelprovider"
	agentruntime "csgclaw/internal/runtime"
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

func TestAgentRecreateValidatesModelBeforeMutation(t *testing.T) {
	image := &modelprovider.ImageGenerationConfig{ProviderID: "configured", ModelID: "image-model"}
	video := &modelprovider.VideoGenerationConfig{ProviderID: "configured", ModelID: "video-model"}
	for _, tc := range []struct {
		name   string
		model  agentengine.ModelSpec
		masked bool
		valid  bool
	}{
		{name: "image only", model: agentengine.ModelSpec{ImageGeneration: image}},
		{name: "video only", model: agentengine.ModelSpec{VideoGeneration: video}},
		{name: "masked video only", model: agentengine.ModelSpec{VideoGeneration: video}, masked: true},
		{name: "primary and multimodal", model: agentengine.ModelSpec{ProviderID: "configured", ModelID: "next-model", ImageGeneration: image, VideoGeneration: video}, valid: true},
	} {
		for _, entry := range []string{"HTTP", "Controller"} {
			t.Run(tc.name+"/"+entry, func(t *testing.T) {
				runtimeCalls := 0
				runtime := fakeCompatRuntime{
					kind: agent.RuntimeKindCodex,
					new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
						runtimeCalls++
						return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "new-runtime"}, nil
					},
					start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
						runtimeCalls++
						return agentruntime.StateRunning, nil
					},
					stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
						runtimeCalls++
						return agentruntime.StateStopped, nil
					},
					del: func(context.Context, agentruntime.Handle) error {
						runtimeCalls++
						return nil
					},
				}
				svc, statePath := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{{
					ID: "agent-worker", Name: "worker", Description: "original", Role: agent.RoleWorker,
					RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "runtime-worker", BoxID: "box-worker", Status: "stopped",
					Profile: "configured.previous-model", ProfileComplete: true,
					AgentProfile: agent.AgentProfile{
						Provider: agent.ProviderAPI, ModelProviderID: "configured", ModelID: "previous-model", APIKey: "test-key",
						Env: map[string]string{"PREVIOUS_SETTING": "enabled"}, ProfileComplete: true,
					},
				}}, agent.WithRuntime(runtime))
				svc.Models().SetLLMConfig(config.LLMConfig{Providers: map[string]config.ProviderConfig{
					"configured": {BaseURL: "https://provider.test/v1", APIKey: "test-key", Models: []string{"previous-model", "next-model"}, ImageModels: []string{"image-model"}, VideoModels: []string{"video-model"}},
				}})
				before, err := svc.Get(context.Background(), "agent-worker", agentengine.AgentGetOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if before.Spec.Model.ModelID != "previous-model" {
					t.Fatalf("initial model = %q, want previous-model", before.Spec.Model.ModelID)
				}
				beforeFile, err := os.ReadFile(statePath)
				if err != nil {
					t.Fatal(err)
				}
				if entry == "HTTP" {
					request := apitypes.RecreateAgentRequest{Configuration: &apitypes.RecreateAgentConfiguration{
						Name: "worker", Description: "replacement", ModelConfig: &apitypes.CreateAgentProfile{
							ModelProviderID: tc.model.ProviderID, ModelID: tc.model.ModelID, ImageGeneration: tc.model.ImageGeneration, VideoGeneration: tc.model.VideoGeneration,
						},
					}}
					if tc.masked {
						request.FieldMask = []string{"model_config"}
					}
					body, err := json.Marshal(request)
					if err != nil {
						t.Fatal(err)
					}
					handler := &Handler{svc: svc, agentEngine: agentengine.New(svc), agentModels: svc.Models(), workspace: svc.Workspace()}
					recorder := httptest.NewRecorder()
					handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-worker/recreate", bytes.NewReader(body)))
					wantStatus := http.StatusBadRequest
					if tc.valid {
						wantStatus = http.StatusOK
					}
					if recorder.Code != wantStatus {
						t.Fatalf("recreate = %d: %s, want %d", recorder.Code, recorder.Body.String(), wantStatus)
					}
					if !tc.valid && !strings.Contains(recorder.Body.String(), "profile is incomplete") {
						t.Fatalf("unexpected validation error: %s", recorder.Body.String())
					}
				} else {
					request := &agentengine.AgentUpdateRequest{
						Spec:            agentengine.AgentSpec{Name: "worker", Description: "replacement", Role: before.Spec.Role, Runtime: before.Spec.Runtime, Model: tc.model},
						ResourceVersion: before.ResourceVersion,
					}
					if tc.masked {
						request.FieldMask = []string{"model"}
					}
					_, err := svc.Recreate(context.Background(), before.ID, agentengine.AgentRecreateOptions{Update: request})
					if tc.valid && err != nil {
						t.Fatal(err)
					}
					if !tc.valid && (err == nil || !strings.Contains(err.Error(), "profile is incomplete")) {
						t.Fatalf("Recreate() error = %v, want incomplete profile", err)
					}
				}
				after, err := svc.Get(context.Background(), before.ID, agentengine.AgentGetOptions{})
				if err != nil {
					t.Fatal(err)
				}
				afterFile, err := os.ReadFile(statePath)
				if err != nil {
					t.Fatal(err)
				}
				if !tc.valid {
					if !reflect.DeepEqual(after, before) || !bytes.Equal(afterFile, beforeFile) {
						t.Fatal("rejected recreate changed Agent memory or its saved file")
					}
					if runtimeCalls != 0 {
						t.Fatalf("rejected recreate performed %d runtime operations", runtimeCalls)
					}
					return
				}
				if after.Spec.Model.ModelID != "next-model" || after.Spec.Model.ProviderID != "configured" || after.Spec.Description != "replacement" || len(after.Spec.Model.Env) != 0 {
					t.Fatalf("replacement configuration was not applied: %+v", after.Spec)
				}
				if !reflect.DeepEqual(after.Spec.Model.ImageGeneration, image) || !reflect.DeepEqual(after.Spec.Model.VideoGeneration, video) || runtimeCalls == 0 {
					t.Fatalf("multimodal configuration or runtime recreation missing: %+v, calls=%d", after.Spec.Model, runtimeCalls)
				}
				if err := svc.Reload(); err != nil {
					t.Fatal(err)
				}
				saved, err := svc.Get(context.Background(), before.ID, agentengine.AgentGetOptions{})
				if err != nil || !reflect.DeepEqual(saved.Spec.Model, after.Spec.Model) {
					t.Fatalf("saved model configuration differs after reload: %+v, %v", saved.Spec.Model, err)
				}
			})
		}
	}
}
