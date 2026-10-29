package template

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
)

func TestTemplateModelConfigRoundTrip(t *testing.T) {
	for _, test := range []struct {
		name        string
		modelConfig *apitypes.TemplateModelConfig
		want        *apitypes.TemplateModelConfig
	}{
		{name: "omitted"},
		{name: "empty", modelConfig: &apitypes.TemplateModelConfig{}},
		{name: "model", modelConfig: &apitypes.TemplateModelConfig{ModelID: " model-a "}, want: &apitypes.TemplateModelConfig{ModelID: "model-a"}},
		{name: "effort", modelConfig: &apitypes.TemplateModelConfig{ReasoningEffort: " HIGH "}, want: &apitypes.TemplateModelConfig{ReasoningEffort: "high"}},
		{name: "auto", modelConfig: &apitypes.TemplateModelConfig{ModelID: "auto", ReasoningEffort: "auto"}, want: &apitypes.TemplateModelConfig{ModelID: "auto", ReasoningEffort: "auto"}},
		{name: "off", modelConfig: &apitypes.TemplateModelConfig{ReasoningEffort: "off"}, want: &apitypes.TemplateModelConfig{ReasoningEffort: "none"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewLocalStore(t.TempDir())
			published, err := store.Publish(t.Context(), PublishSpec{Name: "assistant", RuntimeKind: "codex", ModelConfig: test.modelConfig})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(published.ModelConfig, test.want) {
				t.Fatalf("Publish().ModelConfig = %+v, want %+v", published.ModelConfig, test.want)
			}
			reloaded, err := store.Get(t.Context(), published.ID)
			if err != nil || !reflect.DeepEqual(reloaded.ModelConfig, test.want) {
				t.Fatalf("Get().ModelConfig = %+v, error = %v", reloaded.ModelConfig, err)
			}
			data, err := os.ReadFile(filepath.Join(store.templateRoot(published.ID), localManifestFileName))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "[model_config]") != (test.want != nil) {
				t.Fatalf("agent.toml model_config presence does not match configuration: %s", data)
			}
			if test.want != nil {
				if strings.Contains(string(data), "model_id =") != (test.want.ModelID != "") ||
					strings.Contains(string(data), "reasoning_effort =") != (test.want.ReasoningEffort != "") {
					t.Fatalf("agent.toml optional model fields do not match configuration: %s", data)
				}
			}
		})
	}
}

func TestTemplateModelConfigReasoningValidation(t *testing.T) {
	for _, effort := range []string{"", "auto", "none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		t.Run(effort, func(t *testing.T) {
			manifest := templateManifest{Name: "assistant", RuntimeKind: "codex", ModelConfig: &apitypes.TemplateModelConfig{ReasoningEffort: effort}}
			if err := validateManifest(manifest); err != nil {
				t.Fatalf("validateManifest() reasoning_effort %q error = %v", effort, err)
			}
		})
	}
	store := NewLocalStore(t.TempDir())
	_, err := store.Publish(t.Context(), PublishSpec{Name: "assistant", RuntimeKind: "codex", ModelConfig: &apitypes.TemplateModelConfig{ReasoningEffort: "unknown"}})
	if err == nil || !strings.Contains(err.Error(), "model_config.reasoning_effort") {
		t.Fatalf("Publish() with invalid reasoning_effort error = %v, want validation error", err)
	}
	root := store.templateRoot("assistant")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`"unknown"`, "true", "42"} {
		manifest := "name = 'assistant'\nruntime_kind = 'codex'\n[model_config]\nreasoning_effort = " + value + "\n"
		if err := os.WriteFile(filepath.Join(root, localManifestFileName), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Get(t.Context(), "assistant"); err == nil {
			t.Fatalf("Get() accepted invalid reasoning_effort=%s", value)
		}
	}
}

func TestLocalStoreReplaceModelConfig(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	spec := PublishSpec{Name: "assistant", RuntimeKind: "codex", ModelConfig: &apitypes.TemplateModelConfig{ModelID: "model-a", ReasoningEffort: "high"}}
	if _, err := store.Publish(t.Context(), spec); err != nil {
		t.Fatal(err)
	}
	for _, modelConfig := range []*apitypes.TemplateModelConfig{{ModelID: "model-b", ReasoningEffort: "auto"}, nil} {
		spec.ModelConfig = modelConfig
		replaced, err := store.Replace(t.Context(), "assistant", spec)
		if err != nil || !reflect.DeepEqual(replaced.ModelConfig, modelConfig) {
			t.Fatalf("Replace().ModelConfig = %+v, error = %v", replaced.ModelConfig, err)
		}
	}
}
