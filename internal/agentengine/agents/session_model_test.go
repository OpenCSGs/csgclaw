package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"csgclaw/internal/config"
	"csgclaw/internal/modelcap"
)

func TestSessionModelOptionsUseIndependentProfileDefaults(t *testing.T) {
	state := filepath.Join(t.TempDir(), "agents.json")
	data, err := json.Marshal(map[string]any{"agents": []Agent{{ID: "agent-test", Name: "test", RuntimeKind: RuntimeKindCodex, Role: RoleWorker, AgentProfile: AgentProfile{Provider: ProviderAPI, ModelProviderID: "example", ModelID: "first", ReasoningEffort: "low"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, data, 0600); err != nil {
		t.Fatal(err)
	}
	controller, err := NewControllerWithLLM(config.LLMConfig{Default: "example.first", Providers: map[string]config.ProviderConfig{
		"example": {BaseURL: "https://models.example/v1", APIKey: "test-key", Models: []string{"first", "second", "image"}, ImageModels: []string{"image"}, ReasoningEffort: "medium", ModelMetadata: map[string]modelcap.Metadata{"first": {ContextWindow: 32000}, "second": {ContextWindow: 64000}}},
		"other":   {BaseURL: "https://other.example/v1", APIKey: "other-key", Models: []string{"other-model"}},
	}}, config.ServerConfig{ListenAddr: "127.0.0.1:18080"}, "manager:test", state)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := controller.Agent("agent-test")
	for _, test := range []struct {
		model, effort, wantModel, wantEffort string
		window                               int64
	}{
		{"second", "high", "second", "high", 64000},
		{"", "", "first", "low", 32000},
		{"second", "", "second", "low", 64000},
		{"", "high", "first", "high", 32000},
		{"", "auto", "first", "auto", 32000},
		{"first", "off", "first", "none", 32000},
	} {
		profile, err := controller.Models().ResolvedSessionProfile("agent-test", test.model, test.effort)
		if err != nil {
			t.Fatal(err)
		}
		if profile.ModelID != test.wantModel || profile.ReasoningEffort != test.wantEffort || profile.ModelMetadata.ContextWindow != test.window || profile.APIKey != "test-key" {
			t.Fatalf("selection %q/%q: model=%q effort=%q window=%d", test.model, test.effort, profile.ModelID, profile.ReasoningEffort, profile.ModelMetadata.ContextWindow)
		}
	}
	for _, test := range []struct{ model, effort string }{{"unknown", ""}, {"other-model", ""}, {"image", ""}, {"first", "invalid"}} {
		if _, err := controller.Models().ResolvedSessionProfile("agent-test", test.model, test.effort); err == nil {
			t.Fatalf("accepted invalid selection %q/%q", test.model, test.effort)
		}
	}
	after, _ := controller.Agent("agent-test")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("session options modified the Agent")
	}
	profiles, err := controller.Models().SessionModelProfiles("agent-test")
	if err != nil || len(profiles) != 2 || profiles[0].ModelID != "first" || profiles[1].ModelID != "second" {
		t.Fatalf("session catalog size=%d error=%v", len(profiles), err)
	}
}
