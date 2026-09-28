package agents

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"csgclaw/internal/modelprovider"
)

func (s *ModelConfiguration) GenerateVideo(ctx context.Context, ref *modelprovider.VideoGenerationConfig, prompt string, options modelprovider.VideoGenerationOptions) (modelprovider.GeneratedVideo, error) {
	if ref == nil || ref.ProviderID == "" || ref.ModelID == "" {
		return modelprovider.GeneratedVideo{}, fmt.Errorf("video_model_not_configured")
	}
	providerID := NormalizeModelProviderID(ref.ProviderID)
	s.mu.RLock()
	provider, ok := s.llm.Providers[providerID]
	s.mu.RUnlock()
	if !ok || !containsString(provider.VideoModels, ref.ModelID) {
		return modelprovider.GeneratedVideo{}, fmt.Errorf("video_model_unavailable")
	}
	profile := s.hydrateProfileFromCatalog(AgentProfile{ModelProviderID: providerID, ModelID: ref.ModelID})
	baseURL, key := profileBaseURL(profile), profileAPIKey(profile)
	if profile.Provider == ProviderCSGHub {
		var err error
		var available bool
		baseURL, key, available, err = defaultCSGHubCredentials(ctx, &http.Client{Timeout: 10 * time.Second})
		if err != nil || !available {
			return modelprovider.GeneratedVideo{}, fmt.Errorf("video_model_unavailable")
		}
	}
	if strings.TrimSpace(baseURL) == "" {
		return modelprovider.GeneratedVideo{}, fmt.Errorf("video_model_unavailable")
	}
	capabilities := provider.VideoMetadata[ref.ModelID]
	client := &http.Client{Timeout: 30 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return http.ErrUseLastResponse
		}
		if len(via) > 0 && !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
			req.Header.Del("Authorization")
			for name := range profile.Headers {
				req.Header.Del(name)
			}
		}
		return nil
	}}
	// Older persisted provider catalogs do not contain video metadata. Read it
	// lazily once here so upgrading CSGClaw does not require recreating the Agent.
	if len(capabilities.Sizes) == 0 && len(capabilities.Seconds) == 0 {
		var directory modelprovider.ModelDiscoveryResult
		var err error
		if providerID == ModelProviderIDOpenCSG {
			directory, err = modelprovider.ListOpenCSGModelDirectoryWithClient(ctx, client, baseURL, key, profile.Headers)
		} else {
			directory, err = modelprovider.ListOpenAIModelDirectoryWithClient(ctx, client, baseURL, key, profile.Headers)
		}
		if err == nil {
			capabilities = directory.VideoMetadata[ref.ModelID]
		}
	}
	options, err := modelprovider.ResolveVideoGenerationOptions(options, capabilities)
	if err != nil {
		return modelprovider.GeneratedVideo{}, err
	}
	return modelprovider.GenerateVideo(ctx, client, baseURL, key, profile.Headers, ref.ModelID, prompt, options)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
