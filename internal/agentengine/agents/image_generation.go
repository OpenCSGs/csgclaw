package agents

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"csgclaw/internal/cliproxy"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/modelprovider"
)

func (s *ModelConfiguration) GenerateImage(ctx context.Context, ref *modelprovider.ImageGenerationConfig, prompt string, options modelprovider.ImageGenerationOptions, references ...modelprovider.ReferenceImage) (modelprovider.GeneratedImage, error) {
	empty := modelprovider.GeneratedImage{}
	if ref == nil || ref.ProviderID == "" || ref.ModelID == "" {
		return empty, fmt.Errorf("image_model_not_configured")
	}
	modes := s.ImageGenerationModes(ref)
	if modes == (modelcap.ImageGeneration{}) {
		return empty, fmt.Errorf("image_model_unavailable")
	}
	edit := len(references) > 0
	if err := modelprovider.ValidateImageGenerationMode(modes, edit); err != nil {
		return empty, err
	}
	options = modelprovider.ResolveImageGenerationOptions(NormalizeModelProviderID(ref.ProviderID), ref.ModelID, edit, options)
	profile := s.hydrateProfileFromCatalog(AgentProfile{ModelProviderID: ref.ProviderID, ModelID: ref.ModelID})
	baseURL, key := profileBaseURL(profile), profileAPIKey(profile)
	switch profile.Provider {
	case ProviderCodex:
		status, err := cliproxy.Default().AuthStatus(ctx, ProviderCodex)
		if err != nil || !status.Authenticated {
			return empty, fmt.Errorf("image_model_unavailable")
		}
		baseURL, err = cliproxy.Default().ProviderBaseURL(ctx, ProviderCodex)
		if err != nil {
			return empty, fmt.Errorf("image_model_unavailable")
		}
		key = cliproxy.LocalAPIKey
	case ProviderCSGHub:
		var ok bool
		var err error
		baseURL, key, ok, err = defaultCSGHubCredentials(ctx, &http.Client{Timeout: 10 * time.Second})
		if err != nil || !ok {
			return empty, fmt.Errorf("image_model_unavailable")
		}
	case ProviderClaudeCode:
		return empty, fmt.Errorf("image_model_unavailable")
	}
	if strings.TrimSpace(baseURL) == "" {
		return empty, fmt.Errorf("image_model_unavailable")
	}
	return modelprovider.GenerateImageWithOptions(ctx, &http.Client{Timeout: 8 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, baseURL, key, profile.Headers, ref.ModelID, prompt, options, references...)
}

// ImageGenerationModes returns the operations available to the runtime.
func (s *ModelConfiguration) ImageGenerationModes(ref *modelprovider.ImageGenerationConfig) modelcap.ImageGeneration {
	if ref == nil || ref.ProviderID == "" || ref.ModelID == "" {
		return modelcap.ImageGeneration{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	provider := s.llm.Providers[NormalizeModelProviderID(ref.ProviderID)]
	if modes, ok := provider.ImageMetadata[ref.ModelID]; ok && modes != (modelcap.ImageGeneration{}) {
		return modes
	}
	if modelprovider.IsGPTImageModel(ref.ModelID) {
		return modelcap.ImageGeneration{TextToImage: true, ImageToImage: true}
	}
	if slices.Contains(provider.ImageModels, ref.ModelID) {
		return modelcap.ImageGeneration{TextToImage: true}
	}
	return modelcap.ImageGeneration{}
}
