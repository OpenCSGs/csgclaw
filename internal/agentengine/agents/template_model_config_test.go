package agents

import (
	"context"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	hub "csgclaw/internal/template"
)

func TestCreateWorkerFromTemplateModelDefaults(t *testing.T) {
	for _, runtimeKind := range []string{RuntimeKindCodex, RuntimeKindDSH} {
		for _, tc := range []struct {
			name       string
			defaults   *apitypes.TemplateModelConfig
			profile    AgentProfile
			selector   string
			wantModel  string
			wantEffort string
		}{
			{name: "legacy", wantModel: "platform-model", wantEffort: "auto"},
			{name: "model only", defaults: &apitypes.TemplateModelConfig{ModelID: "template-model"}, wantModel: "template-model", wantEffort: "auto"},
			{name: "effort only", defaults: &apitypes.TemplateModelConfig{ReasoningEffort: "high"}, wantModel: "platform-model", wantEffort: "high"},
			{name: "both", defaults: &apitypes.TemplateModelConfig{ModelID: "template-model", ReasoningEffort: "high"}, wantModel: "template-model", wantEffort: "high"},
			{name: "request model", defaults: &apitypes.TemplateModelConfig{ModelID: "template-model", ReasoningEffort: "high"}, profile: AgentProfile{ModelProviderID: "remote", ModelID: "request-model"}, wantModel: "request-model", wantEffort: "high"},
			{name: "request effort", defaults: &apitypes.TemplateModelConfig{ModelID: "template-model", ReasoningEffort: "high"}, profile: AgentProfile{ReasoningEffort: "medium"}, wantModel: "template-model", wantEffort: "medium"},
			{name: "request auto", defaults: &apitypes.TemplateModelConfig{ModelID: "template-model", ReasoningEffort: "high"}, profile: AgentProfile{ReasoningEffort: "auto"}, wantModel: "template-model", wantEffort: "auto"},
			{name: "request selector", defaults: &apitypes.TemplateModelConfig{ModelID: "template-model", ReasoningEffort: "high"}, selector: "remote.request-model", wantModel: "request-model", wantEffort: "high"},
			{name: "selector precedence", defaults: &apitypes.TemplateModelConfig{ModelID: "template-model"}, selector: "remote.request-model", profile: AgentProfile{ModelID: "other-model"}, wantModel: "request-model", wantEffort: "auto"},
		} {
			t.Run(runtimeKind+"/"+tc.name, func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				hubSvc := mustNewLocalTemplateHubService(t, "worker", hub.Template{
					Name: "worker", RuntimeKind: runtimeKind, ModelConfig: tc.defaults,
				})
				var runtimeProfile agentruntime.Profile
				svc, err := NewControllerWithLLM(config.LLMConfig{
					Default: "remote.platform-model",
					Providers: map[string]config.ProviderConfig{
						"remote": {Status: "connected", BaseURL: "https://gateway.example/v1", APIKey: "test-platform-key", Models: []string{"platform-model"}},
					},
				}, config.ServerConfig{}, "", "", WithHubService(hubSvc), WithSandboxProvider(fakeProvider{name: "csghub"}), WithRuntime(fakeAgentRuntime{
					kind: runtimeKind,
					new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
						runtimeProfile = spec.Profile
						return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "template-worker"}, nil
					},
				}))
				if err != nil {
					t.Fatal(err)
				}
				defer svc.Close()
				created, err := svc.CreateRecord(context.Background(), CreateRequest{Spec: CreateAgentSpec{
					ID: "template-worker", Name: "template-worker", FromTemplate: "local.worker", AgentProfile: tc.profile, Profile: tc.selector,
				}})
				if err != nil {
					t.Fatal(err)
				}
				if created.AgentProfile.ModelProviderID != "remote" || created.AgentProfile.ModelID != tc.wantModel || created.AgentProfile.ReasoningEffort != tc.wantEffort {
					t.Fatalf("created model = %q/%q/%q, want remote/%s/%s", created.AgentProfile.ModelProviderID, created.AgentProfile.ModelID, created.AgentProfile.ReasoningEffort, tc.wantModel, tc.wantEffort)
				}
				if runtimeProfile.ModelID != tc.wantModel || runtimeProfile.ReasoningEffort != tc.wantEffort {
					t.Fatal("runtime did not receive the selected model and reasoning effort")
				}
				providerProfile := svc.hydrateProfileFromCatalog(created.AgentProfile)
				if providerProfile.BaseURL != "https://gateway.example/v1" || providerProfile.APIKey != "test-platform-key" {
					t.Fatal("template model lost the existing provider configuration")
				}
				published, err := svc.HubPublishSpec(created.ID, false)
				if err != nil {
					t.Fatal(err)
				}
				if published.ModelConfig == nil || published.ModelConfig.ModelID != tc.wantModel || published.ModelConfig.ReasoningEffort != tc.wantEffort {
					t.Fatalf("published model defaults = %+v", published.ModelConfig)
				}
			})
		}
	}
}

func TestTemplateModelDefaultsRetainDetectedProvider(t *testing.T) {
	for _, tc := range []struct {
		name       string
		defaults   apitypes.TemplateModelConfig
		requested  string
		wantEffort string
	}{
		{name: "model only", defaults: apitypes.TemplateModelConfig{ModelID: "template-model"}, wantEffort: "medium"},
		{name: "template auto", defaults: apitypes.TemplateModelConfig{ModelID: "template-model", ReasoningEffort: "auto"}, wantEffort: "auto"},
		{name: "request auto", defaults: apitypes.TemplateModelConfig{ModelID: "template-model", ReasoningEffort: "high"}, requested: "auto", wantEffort: "auto"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			svc, err := NewControllerWithLLM(config.LLMConfig{
				DefaultProfile: "remote",
				Profiles: map[string]config.ModelConfig{
					"remote": {Provider: config.ProviderLLMAPI, BaseURL: "https://gateway.example/v1", APIKey: "test-platform-key", ModelID: "platform-model", ReasoningEffort: "medium"},
				},
			}, config.ServerConfig{}, "", "")
			if err != nil {
				t.Fatal(err)
			}
			defer svc.Close()
			profile, err := svc.profileForCreateRequest(context.Background(), &CreateAgentSpec{
				Name: "worker", RuntimeKind: RuntimeKindCodex, TemplateModelConfig: &tc.defaults,
				AgentProfile: AgentProfile{ReasoningEffort: tc.requested},
			})
			if err != nil {
				t.Fatal(err)
			}
			if profile.ModelProviderID != "remote" || profile.ModelID != "template-model" || profile.ReasoningEffort != tc.wantEffort {
				t.Fatalf("model = %q/%q/%q, want remote/template-model/%s", profile.ModelProviderID, profile.ModelID, profile.ReasoningEffort, tc.wantEffort)
			}
			runtimeProfile := svc.hydrateProfileFromCatalog(profile)
			if runtimeProfile.BaseURL != "https://gateway.example/v1" || runtimeProfile.APIKey != "test-platform-key" {
				t.Fatal("template model lost the detected provider configuration")
			}
		})
	}
}
