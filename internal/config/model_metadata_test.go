package config

import (
	"csgclaw/internal/modelcap"
	"path/filepath"
	"testing"
)

func TestModelMetadataRoundTripAndClone(t *testing.T) {
	for _, name := range []string{"models.json", StateFileName} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			cfg := LLMConfig{Providers: map[string]ProviderConfig{"custom": {BaseURL: "http://local/v1", APIKey: "fixture", Models: []string{"m"}, ModelMetadata: map[string]modelcap.Metadata{"m": {ContextWindow: 64000}}, ModelOverrides: map[string]modelcap.Metadata{"m": {ContextWindow: 16000}}}}}
			if err := SaveModels(path, cfg); err != nil {
				t.Fatal(err)
			}
			got, ok, err := LoadModels(path)
			if err != nil || !ok {
				t.Fatalf("load %v %v", ok, err)
			}
			if got.Providers["custom"].ModelOverrides["m"].ContextWindow != 16000 || got.Providers["custom"].ModelMetadata["m"].ContextWindow != 64000 {
				t.Fatalf("lost metadata: %+v", got.Providers["custom"])
			}
			cloned := got.Normalized()
			cloned.Providers["custom"].ModelOverrides["m"] = modelcap.Metadata{}
			if got.Providers["custom"].ModelOverrides["m"].ContextWindow != 16000 {
				t.Fatal("normalization aliases overrides")
			}
		})
	}
}
