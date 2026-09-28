package agents

import (
	"path/filepath"
	"testing"

	"csgclaw/internal/config"
)

func TestSetLLMConfigReconcilesOpenCSGCreationDefaults(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		models   []string
		want     string
	}{
		{"environment switch", ModelProviderIDOpenCSG, []string{"staging-model"}, "staging-model"},
		{"still available", ModelProviderIDOpenCSG, []string{"staging-model", "production-model"}, "production-model"},
		{"empty or failed catalog", ModelProviderIDOpenCSG, nil, ""},
		{"another provider", ModelProviderIDCodex, []string{"staging-model"}, "production-model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			statePath := filepath.Join(t.TempDir(), "agents.json")
			svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
			if err != nil {
				t.Fatal(err)
			}
			defer svc.Close()
			profile := normalizeProfile(AgentProfile{ModelProviderID: tc.provider, ModelID: "production-model", ReasoningEffort: "high"}, "scorer", "")
			svc.profileDefaults = profile
			svc.agents["agent-existing"] = Agent{ID: "agent-existing", Name: "existing", RuntimeKind: RuntimeKindCodex, AgentProfile: profile}
			if err := svc.saveLocked(); err != nil {
				t.Fatal(err)
			}
			llm := config.LLMConfig{Default: "opencsg.missing-model", Providers: map[string]config.ProviderConfig{
				ModelProviderIDOpenCSG: {Models: tc.models},
			}}
			svc.SetLLMConfig(llm)
			if got := svc.ProfileDefaultsView(); got.ModelID != tc.want || got.ReasoningEffort != "high" || got.ProfileComplete != (tc.want != "") {
				t.Fatalf("creation defaults = %+v, want model %q with preserved reasoning", got, tc.want)
			}
			if got, _ := svc.Agent("agent-existing"); got.AgentProfile.ModelID != "production-model" {
				t.Fatalf("existing agent model changed to %q", got.AgentProfile.ModelID)
			}
			reloaded, err := NewControllerWithLLM(llm, config.ServerConfig{}, "", statePath)
			if err != nil {
				t.Fatal(err)
			}
			defer reloaded.Close()
			if got := reloaded.ProfileDefaultsView(); got.ModelID != tc.want {
				t.Fatalf("defaults after restart = %q, want %q", got.ModelID, tc.want)
			}
		})
	}
}
