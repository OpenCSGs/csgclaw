package api

import (
	"encoding/json"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	hub "csgclaw/internal/template"
)

func TestPresentHubTemplateModelConfig(t *testing.T) {
	item := hub.Template{ModelConfig: &apitypes.TemplateModelConfig{ModelID: "model-a", ReasoningEffort: "auto"}}
	presented := presentHubTemplate(item)
	if presented.ModelConfig == nil || *presented.ModelConfig != *item.ModelConfig {
		t.Fatalf("presented ModelConfig = %+v, want %+v", presented.ModelConfig, item.ModelConfig)
	}
	presented.ModelConfig.ModelID = "model-b"
	if item.ModelConfig.ModelID != "model-a" {
		t.Fatal("presented ModelConfig mutation changed template model defaults")
	}
	for _, test := range []struct {
		modelConfig *apitypes.TemplateModelConfig
		want        string
	}{
		{modelConfig: item.ModelConfig, want: `"model_config":{"model_id":"model-a","reasoning_effort":"auto"}`},
		{modelConfig: &apitypes.TemplateModelConfig{ReasoningEffort: "high"}, want: `"model_config":{"reasoning_effort":"high"}`},
		{},
	} {
		data, err := json.Marshal(presentHubTemplate(hub.Template{ModelConfig: test.modelConfig}))
		if err != nil {
			t.Fatal(err)
		}
		if test.want == "" {
			if strings.Contains(string(data), "model_config") {
				t.Fatalf("legacy template contains model_config: %s", data)
			}
		} else if !strings.Contains(string(data), test.want) {
			t.Fatalf("response missing %s: %s", test.want, data)
		}
	}
}
