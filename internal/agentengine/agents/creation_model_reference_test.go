package agents

import (
	"context"
	"testing"

	"csgclaw/internal/config"
)

func TestCreateModelOnlyUsesConnectedProvider(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	service := &Controller{}
	service.ModelConfiguration.Repository = &service.Repository
	service.llm = config.LLMConfig{Providers: map[string]config.ProviderConfig{
		"opencsg": {Status: "connected", Models: []string{"auto", "qwen-test"}},
		"other":   {Status: "connected", BaseURL: "https://other.example/v1", APIKey: "fixture", Models: []string{"qwen-test"}},
		"failed":  {Status: "failed", Models: []string{"unavailable"}},
	}}
	service.profileDefaults = AgentProfile{Provider: ProviderCSGHubLite, ModelID: "stale-local-model"}
	for _, tc := range []struct {
		name, model, wantProvider string
		preferred                 *CreationModelDefaults
		wantError                 bool
	}{
		{name: "official model", model: "qwen-test", wantProvider: "opencsg"},
		{name: "auto", model: "auto", wantProvider: "opencsg"},
		{name: "explicit default provider", model: "qwen-test", wantProvider: "other", preferred: &CreationModelDefaults{Agent: &ModelDefaultRef{ProviderID: "other", ModelID: "qwen-test"}}},
		{name: "disconnected model", model: "unavailable", wantError: true},
		{name: "unknown model", model: "not-listed", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service.creationDefaults = tc.preferred
			spec := CreateAgentSpec{Name: "worker", RuntimeKind: RuntimeKindCodex, AgentProfile: AgentProfile{ModelID: tc.model, Env: map[string]string{"WORKER_OPTION": "preserved"}}}
			got, err := service.profileForCreateRequest(context.Background(), &spec)
			if tc.wantError {
				if err == nil {
					t.Fatalf("accepted unavailable model: provider=%q model=%q", got.ModelProviderID, got.ModelID)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ModelProviderID != tc.wantProvider || got.ModelID != tc.model {
				t.Fatalf("provider=%q model=%q; want %q/%q", got.ModelProviderID, got.ModelID, tc.wantProvider, tc.model)
			}
			if got.Env["WORKER_OPTION"] != "preserved" {
				t.Fatal("lost supplied environment")
			}
		})
	}
}

func TestCreateModelReferencePreservesExplicitProvider(t *testing.T) {
	service := &ModelConfiguration{Repository: &Repository{}}
	for _, profile := range []AgentProfile{
		{ModelProviderID: "explicit", ModelID: "private-model"},
		{Provider: ProviderAPI, BaseURL: "https://private.example/v1", APIKey: "fixture", ModelID: "private-model"},
		{ModelID: "private-model", Headers: map[string]string{"X-Provider": "explicit"}},
	} {
		got, err := service.resolveCreateModelReference(profile)
		if err != nil {
			t.Fatal(err)
		}
		if got.ModelProviderID != profile.ModelProviderID || got.Provider != profile.Provider || got.BaseURL != profile.BaseURL || got.APIKey != profile.APIKey || got.ModelID != profile.ModelID {
			t.Fatal("explicit provider was replaced")
		}
	}
}

func TestCreateWithoutModelUsesConnectedDefault(t *testing.T) {
	service := &Controller{}
	service.ModelConfiguration.Repository = &service.Repository
	service.llm = config.LLMConfig{Providers: map[string]config.ProviderConfig{"opencsg": {Status: "connected", Models: []string{"auto"}}}}
	service.profileDefaults = AgentProfile{Provider: ProviderCSGHubLite, ModelID: "stale-local-model"}
	spec := CreateAgentSpec{Name: "worker", RuntimeKind: RuntimeKindCodex}
	got, err := service.profileForCreateRequest(context.Background(), &spec)
	if err != nil {
		t.Fatal(err)
	}
	if got.ModelProviderID != "opencsg" || got.ModelID != "auto" {
		t.Fatalf("wrong default provider=%q model=%q", got.ModelProviderID, got.ModelID)
	}
}
