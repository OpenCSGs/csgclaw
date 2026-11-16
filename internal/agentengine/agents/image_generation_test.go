package agents

import (
	"context"
	"errors"
	"testing"

	"csgclaw/internal/config"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/modelprovider"
)

func TestImageGenerationRejectsUnsupportedModeBeforeRequest(t *testing.T) {
	models := ModelConfiguration{
		Repository: &Repository{},
		llm: config.LLMConfig{Providers: map[string]config.ProviderConfig{
			"fixture": {
				ImageModels: []string{"generator", "editor", "legacy"},
				ImageMetadata: map[string]modelcap.ImageGeneration{
					"generator": {TextToImage: true},
					"editor":    {ImageToImage: true},
				},
			},
		}},
	}
	for _, tc := range []struct {
		model string
		edit  bool
		code  string
	}{
		{"generator", true, "image_edit_unsupported"},
		{"legacy", true, "image_edit_unsupported"},
		{"editor", false, "image_generation_unsupported"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			var references []modelprovider.ReferenceImage
			if tc.edit {
				references = []modelprovider.ReferenceImage{{}}
			}
			ref := &modelprovider.ImageGenerationConfig{ProviderID: "fixture", ModelID: tc.model}
			_, err := models.GenerateImage(context.Background(), ref, "blue sky", modelprovider.ImageGenerationOptions{}, references...)
			var result *modelprovider.ImageGenerationError
			if !errors.As(err, &result) || result.Code != tc.code {
				t.Fatalf("expected %s, got %v", tc.code, err)
			}
		})
	}
}
