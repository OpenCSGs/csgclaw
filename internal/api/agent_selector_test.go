package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"csgclaw/internal/modelprovider"
	agentruntime "csgclaw/internal/runtime"
)

func TestAgentSelectorUpdateResolvesEffectiveModel(t *testing.T) {
	for _, tc := range []struct {
		name, provider string
		complete       bool
		sameSelector   bool
	}{
		{name: "configured", provider: "configured", complete: true},
		{name: "incomplete", provider: "configured"},
		{name: "different provider", provider: "alternate", complete: true},
		{name: "same selector", provider: "configured", complete: true, sameSelector: true},
	} {
		for _, operation := range []string{"PATCH", "recreate", "Controller Update", "Controller Recreate"} {
			t.Run(tc.name+"/"+operation, func(t *testing.T) {
				profile := agent.AgentProfile{
					Name: "worker model", Description: "model settings", ReasoningEffort: "high", EnableFastMode: true,
					RequestOptions: map[string]any{"temperature": 0.2}, Env: map[string]string{"KEEP": "yes"},
					ImageGeneration: &modelprovider.ImageGenerationConfig{ProviderID: "configured", ModelID: "image-model"},
					VideoGeneration: &modelprovider.VideoGenerationConfig{ProviderID: "configured", ModelID: "video-model"},
				}
				selector, status := "", "profile_incomplete"
				if tc.complete {
					selector, status = "configured.previous-model", "stopped"
					profile.Provider = agent.ProviderAPI
					profile.ModelProviderID = "configured"
					profile.ModelID = "previous-model"
					profile.ProfileComplete = true
				}
				nextSelector := tc.provider + ".next-model"
				if tc.sameSelector {
					selector = nextSelector
				}
				runtimeCreates := 0
				svc, _ := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{{
					ID: "agent-worker", Name: "worker", Description: "original", Role: agent.RoleWorker,
					RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "runtime-worker", BoxID: "box-worker",
					Status: status, Profile: selector, ProfileComplete: tc.complete, AgentProfile: profile,
				}}, agent.WithRuntime(fakeCompatRuntime{
					kind: agent.RuntimeKindCodex,
					new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
						runtimeCreates++
						return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "new-runtime"}, nil
					},
				}))
				providers := map[string]config.ProviderConfig{
					"configured": {BaseURL: "https://provider.test/v1", APIKey: "test-key", Models: []string{"previous-model", "next-model"}, ImageModels: []string{"image-model"}, VideoModels: []string{"video-model"}},
					"alternate":  {BaseURL: "https://alternate.test/v1", APIKey: "alternate-key", Models: []string{"next-model"}},
				}
				svc.Models().SetLLMConfig(config.LLMConfig{Providers: providers})
				before, ok := svc.Agent("agent-worker")
				if !ok {
					t.Fatal("initial Agent not found")
				}
				if strings.HasPrefix(operation, "Controller") {
					request := agentengine.AgentUpdateRequest{
						Spec:      agentengine.AgentSpec{Model: agentengine.ModelSpec{Selector: nextSelector}},
						FieldMask: []string{"model.selector"},
					}
					var err error
					if operation == "Controller Update" {
						_, err = svc.Update(t.Context(), before.ID, request)
					} else {
						_, err = svc.Recreate(t.Context(), before.ID, agentengine.AgentRecreateOptions{Update: &request})
					}
					if err != nil {
						t.Fatal(err)
					}
				} else {
					method, path, body := http.MethodPatch, "/api/v1/agents/agent-worker", `{"model_config":{"selector":"`+nextSelector+`"}}`
					if operation == "recreate" {
						method, path, body = http.MethodPost, path+"/recreate", `{"configuration":{"model_config":{"selector":"`+nextSelector+`"}},"field_mask":["model_config.selector"]}`
					}
					handler := &Handler{svc: svc, agentEngine: agentengine.New(svc), agentModels: svc.Models(), workspace: svc.Workspace()}
					recorder := httptest.NewRecorder()
					handler.Routes().ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader(body)))
					if recorder.Code != http.StatusOK {
						t.Fatalf("%s = %d: %s", operation, recorder.Code, recorder.Body.String())
					}
					if !strings.Contains(recorder.Body.String(), `"model_id":"next-model"`) {
						t.Fatalf("response does not contain the selected model: %s", recorder.Body.String())
					}
				}
				after, ok := svc.Agent(before.ID)
				if !ok || after.Profile != nextSelector || after.AgentProfile.ModelProviderID != tc.provider || after.AgentProfile.ModelID != "next-model" || !after.ProfileComplete {
					t.Fatalf("selected model was not applied: %+v", after)
				}
				previousSettings, nextSettings := before.AgentProfile, after.AgentProfile
				for _, settings := range []*agent.AgentProfile{&previousSettings, &nextSettings} {
					settings.Provider, settings.ModelProviderID, settings.ModelID = "", "", ""
					settings.ProfileComplete, settings.EnvRestartRequired = false, false
				}
				if !reflect.DeepEqual(previousSettings, nextSettings) || after.Description != before.Description || !reflect.DeepEqual(after.RuntimeOptions, before.RuntimeOptions) {
					t.Fatalf("selector update changed retained settings: before=%+v, after=%+v", before, after)
				}
				resolved, err := svc.Models().ResolvedModelConfig(before.ID)
				if err != nil || resolved.ModelID != "next-model" || resolved.BaseURL != providers[tc.provider].BaseURL || resolved.APIKey != providers[tc.provider].APIKey {
					t.Fatalf("effective model = %+v, error = %v", resolved, err)
				}
				if strings.Contains(strings.ToLower(operation), "recreate") && runtimeCreates != 1 {
					t.Fatalf("runtime creations = %d, want 1", runtimeCreates)
				}
				if err := svc.Reload(); err != nil {
					t.Fatal(err)
				}
				saved, ok := svc.Agent(before.ID)
				if !ok || saved.Profile != nextSelector || saved.AgentProfile.ModelID != "next-model" {
					t.Fatalf("selected model was not saved: %+v", saved)
				}
			})
		}
	}
}

func TestAgentPatchSelectorWithExplicitModelConfiguration(t *testing.T) {
	svc := mustNewSeededService(t, []agent.Agent{{
		ID: "agent-worker", Name: "worker", Role: agent.RoleWorker,
		RuntimeKind: agent.RuntimeKindCodex, Status: "stopped", Profile: "configured.previous-model", ProfileComplete: true,
		AgentProfile: agent.AgentProfile{
			Name: "worker", Provider: agent.ProviderAPI, ModelProviderID: "configured", ModelID: "previous-model", ProfileComplete: true,
		},
	}})
	svc.Models().SetLLMConfig(config.LLMConfig{Providers: map[string]config.ProviderConfig{
		"configured": {BaseURL: "https://provider.test/v1", APIKey: "test-key", Models: []string{"previous-model", "next-model"}},
	}})
	handler := &Handler{svc: svc, agentEngine: agentengine.New(svc), agentModels: svc.Models(), workspace: svc.Workspace()}
	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, "/api/v1/agents/agent-worker", strings.NewReader(
		`{"model_config":{"selector":"configured.next-model","name":"worker","provider":"api","model_provider_id":"configured","model_id":"previous-model"}}`,
	)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("PATCH = %d: %s", recorder.Code, recorder.Body.String())
	}
	after, ok := svc.Agent("agent-worker")
	if !ok || after.AgentProfile.ModelProviderID != "configured" || after.AgentProfile.ModelID != "previous-model" {
		t.Fatalf("explicit model configuration was not retained: %+v", after)
	}
}
