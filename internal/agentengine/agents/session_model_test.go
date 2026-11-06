package agents

import (
	"encoding/json"
	"fmt"
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

func TestSessionModelOptionsFollowProviderUpdates(t *testing.T) {
	service := sessionModelTestConfiguration(3)
	before, err := service.ResolvedSessionProfile("agent-test", "model-0001", "")
	if err != nil {
		t.Fatal(err)
	}
	updated := service.llm.Normalized()
	provider := updated.Providers["example"]
	provider.Models = []string{"model-0000", "replacement", "gpt-image-1", "video"}
	provider.VideoModels = []string{"video"}
	provider.APIKey = "updated-key"
	provider.BaseURL = "https://updated.example/v1"
	provider.ModelMetadata["replacement"] = modelcap.Metadata{ContextWindow: 64000}
	provider.ModelOverrides = map[string]modelcap.Metadata{"replacement": {ContextWindow: 96000}}
	provider.Headers = map[string]string{"X-Provider": "updated"}
	updated.Providers["example"] = provider
	service.SetLLMConfig(updated)
	selected, err := service.ResolvedSessionProfile("agent-test", "replacement", "")
	if err != nil {
		t.Fatal(err)
	}
	if selected.APIKey != "updated-key" || selected.BaseURL != provider.BaseURL || selected.ModelMetadata.ContextWindow != 96000 || selected.ModelMetadata.ContextSource != "user" || selected.ReasoningEffort != before.ReasoningEffort {
		t.Fatalf("updated selection = %+v", selected)
	}
	for _, model := range []string{"model-0001", "gpt-image-1", "video"} {
		if _, err := service.ResolvedSessionProfile("agent-test", model, ""); err == nil {
			t.Fatalf("accepted unavailable chat model %q", model)
		}
	}
	selected.Headers["X-Provider"] = "changed"
	catalog, err := service.SessionModelProfiles("agent-test")
	if err != nil || len(catalog) != 2 || catalog[0].ModelID != "model-0000" || catalog[1].ModelID != "replacement" || catalog[1].Headers["X-Provider"] != "updated" || catalog[1].ModelMetadata.ContextWindow != 96000 {
		t.Fatalf("updated catalog = %+v, error = %v", catalog, err)
	}
}

func TestSessionModelOptionsResolveInlineProvider(t *testing.T) {
	service := sessionModelTestConfiguration(2)
	item := service.agents["agent-test"]
	item.AgentProfile.ModelProviderID = ""
	item.AgentProfile.BaseURL = service.llm.Providers["example"].BaseURL
	item.AgentProfile.APIKey = service.llm.Providers["example"].APIKey
	service.agents["agent-test"] = item
	selected, err := service.ResolvedSessionProfile("agent-test", "model-0001", "high")
	if err != nil || selected.ModelProviderID != "example" || selected.ModelID != "model-0001" || selected.ReasoningEffort != "high" || selected.ModelMetadata.ContextWindow != 32000 {
		t.Fatalf("inline selection = %+v, error = %v", selected, err)
	}
}

func TestSessionModelResolutionAllocationBudget(t *testing.T) {
	const models = 512
	service := sessionModelTestConfiguration(models)
	for _, test := range []struct {
		name   string
		budget float64
		run    func() error
	}{
		{"selected", 4096, func() error {
			_, err := service.ResolvedSessionProfile("agent-test", "model-0001", "high")
			return err
		}},
		{"catalog", models * 1100, func() error {
			_, err := service.SessionModelProfiles("agent-test")
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Include profile normalization and race instrumentation while rejecting
			// repeated provider resolution for unrelated models in a large catalog.
			allocations := testing.AllocsPerRun(3, func() {
				if err := test.run(); err != nil {
					t.Fatal(err)
				}
			})
			if allocations > test.budget {
				t.Fatalf("model resolution allocated %.0f objects, budget %.0f", allocations, test.budget)
			}
		})
	}
}

func BenchmarkSessionModelResolution512(b *testing.B) {
	service := sessionModelTestConfiguration(512)
	b.Run("selected", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			if _, err := service.ResolvedSessionProfile("agent-test", "model-0001", "high"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("catalog", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			if _, err := service.SessionModelProfiles("agent-test"); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func sessionModelTestConfiguration(count int) *ModelConfiguration {
	models := make([]string, count)
	metadata := make(map[string]modelcap.Metadata, count)
	for index := range models {
		models[index] = fmt.Sprintf("model-%04d", index)
		metadata[models[index]] = modelcap.Metadata{ContextWindow: 32000}
	}
	return &ModelConfiguration{
		Repository: &Repository{agents: map[string]Agent{"agent-test": {
			ID: "agent-test", Name: "test", RuntimeKind: RuntimeKindCodex,
			AgentProfile: AgentProfile{Provider: ProviderAPI, ModelProviderID: "example", ModelID: models[0], ReasoningEffort: "low"},
		}}},
		llm: config.LLMConfig{Default: "example." + models[0], Providers: map[string]config.ProviderConfig{
			"example": {BaseURL: "https://models.example/v1", APIKey: "test-key", Models: models, ModelMetadata: metadata},
		}}.Normalized(),
	}
}
