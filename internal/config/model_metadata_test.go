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
			cfg := LLMConfig{Providers: map[string]ProviderConfig{"custom": {BaseURL: "http://local/v1", APIKey: "fixture", Models: []string{"m"}, ModelMetadata: map[string]modelcap.Metadata{"m": {ContextWindow: 64000}}, ModelOverrides: map[string]modelcap.Metadata{"m": {ContextWindow: 16000}}, VideoMetadata: map[string]modelcap.VideoGeneration{"video": {Sizes: []string{"768P"}, Seconds: []int{6}}}}}}
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
			if video := got.Providers["custom"].VideoMetadata["video"]; len(video.Sizes) != 1 || video.Sizes[0] != "768P" || len(video.Seconds) != 1 || video.Seconds[0] != 6 {
				t.Fatalf("lost video metadata: %+v", video)
			}
			cloned := got.Normalized()
			cloned.Providers["custom"].ModelOverrides["m"] = modelcap.Metadata{}
			cloned.Providers["custom"].VideoMetadata["video"] = modelcap.VideoGeneration{}
			if got.Providers["custom"].ModelOverrides["m"].ContextWindow != 16000 {
				t.Fatal("normalization aliases overrides")
			}
			if got.Providers["custom"].VideoMetadata["video"].Sizes[0] != "768P" {
				t.Fatal("normalization aliases video metadata")
			}
		})
	}
}
