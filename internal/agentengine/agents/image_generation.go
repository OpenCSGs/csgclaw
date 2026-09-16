package agents

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"csgclaw/internal/cliproxy"
	"csgclaw/internal/modelprovider"
)

func (s *ModelConfiguration) GenerateImage(ctx context.Context, ref *modelprovider.ImageGenerationConfig, prompt string) (modelprovider.GeneratedImage, error) {
	empty := modelprovider.GeneratedImage{}
	if ref == nil || ref.ProviderID == "" || ref.ModelID == "" {
		return empty, fmt.Errorf("image_model_not_configured")
	}
	if !s.supportsImageModel(ref) {
		return empty, fmt.Errorf("image_model_unavailable")
	}
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
	return modelprovider.GenerateImage(ctx, &http.Client{Timeout: 8 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, baseURL, key, profile.Headers, ref.ModelID, prompt)
}

func (s *ModelConfiguration) supportsImageModel(ref *modelprovider.ImageGenerationConfig) bool {
	if ref == nil {
		return false
	}
	if modelprovider.IsGPTImageModel(ref.ModelID) {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, model := range s.llm.Providers[NormalizeModelProviderID(ref.ProviderID)].ImageModels {
		if model == ref.ModelID {
			return true
		}
	}
	return false
}
