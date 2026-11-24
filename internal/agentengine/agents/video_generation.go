package agents

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/modelprovider"
)

func (s *ModelConfiguration) GenerateVideo(ctx context.Context, ref *modelprovider.VideoGenerationConfig, prompt string, options modelprovider.VideoGenerationOptions) (modelprovider.GeneratedVideo, error) {
	provider, profile, baseURL, key, err := s.videoProvider(ctx, ref)
	if err != nil {
		return modelprovider.GeneratedVideo{}, err
	}
	client := videoHTTPClient(30*time.Minute, profile.Headers, options.Reference != nil)
	if !options.ParametersConfirmed {
		capabilities := provider.VideoMetadata[ref.ModelID]
		if len(capabilities.Sizes) == 0 && len(capabilities.Seconds) == 0 {
			capabilities = s.refreshVideoCapabilities(ctx, ref, capabilities, profile, baseURL, key)
		}
		options, err = modelprovider.ResolveVideoGenerationOptions(options, capabilities)
		if err != nil {
			return modelprovider.GeneratedVideo{}, err
		}
	}
	return modelprovider.GenerateVideo(ctx, client, baseURL, key, profile.Headers, ref.ModelID, prompt, options)
}

// VideoGenerationCapabilities returns the supported options for the selected model.
func (s *ModelConfiguration) VideoGenerationCapabilities(ctx context.Context, ref *modelprovider.VideoGenerationConfig, refresh bool) (modelcap.VideoGeneration, error) {
	if !refresh {
		provider, err := s.videoCatalogEntry(ref)
		if err != nil {
			return modelcap.VideoGeneration{}, err
		}
		return provider.VideoMetadata[ref.ModelID], nil
	}
	provider, profile, baseURL, key, err := s.videoProvider(ctx, ref)
	if err != nil {
		return modelcap.VideoGeneration{}, err
	}
	return s.refreshVideoCapabilities(ctx, ref, provider.VideoMetadata[ref.ModelID], profile, baseURL, key), nil
}

func (s *ModelConfiguration) videoCatalogEntry(ref *modelprovider.VideoGenerationConfig) (config.ProviderConfig, error) {
	if ref == nil || ref.ProviderID == "" || ref.ModelID == "" {
		return config.ProviderConfig{}, fmt.Errorf("video_model_not_configured")
	}
	s.mu.RLock()
	provider, ok := s.llm.Providers[NormalizeModelProviderID(ref.ProviderID)]
	s.mu.RUnlock()
	if !ok || !containsString(provider.VideoModels, ref.ModelID) {
		return config.ProviderConfig{}, fmt.Errorf("video_model_unavailable")
	}
	return provider, nil
}

func (s *ModelConfiguration) videoProvider(ctx context.Context, ref *modelprovider.VideoGenerationConfig) (config.ProviderConfig, AgentProfile, string, string, error) {
	provider, err := s.videoCatalogEntry(ref)
	if err != nil {
		return provider, AgentProfile{}, "", "", err
	}
	providerID := NormalizeModelProviderID(ref.ProviderID)
	profile := s.hydrateProfileFromCatalog(AgentProfile{ModelProviderID: providerID, ModelID: ref.ModelID})
	baseURL, key := profileBaseURL(profile), profileAPIKey(profile)
	if profile.Provider == ProviderCSGHub {
		var err error
		var available bool
		baseURL, key, available, err = defaultCSGHubCredentials(ctx, &http.Client{Timeout: 10 * time.Second})
		if err != nil || !available {
			return provider, profile, "", "", fmt.Errorf("video_model_unavailable")
		}
	}
	if strings.TrimSpace(baseURL) == "" {
		return provider, profile, "", "", fmt.Errorf("video_model_unavailable")
	}
	return provider, profile, baseURL, key, nil
}

func (s *ModelConfiguration) refreshVideoCapabilities(ctx context.Context, ref *modelprovider.VideoGenerationConfig, cached modelcap.VideoGeneration, profile AgentProfile, baseURL, key string) modelcap.VideoGeneration {
	client := videoHTTPClient(5*time.Second, profile.Headers, false)
	var directory modelprovider.ModelDiscoveryResult
	var err error
	if NormalizeModelProviderID(ref.ProviderID) == ModelProviderIDOpenCSG {
		directory, err = modelprovider.ListOpenCSGModelDirectoryWithClient(ctx, client, baseURL, key, profile.Headers)
	} else {
		directory, err = modelprovider.ListOpenAIModelDirectoryWithClient(ctx, client, baseURL, key, profile.Headers)
	}
	if current, ok := directory.VideoMetadata[ref.ModelID]; err == nil && ok {
		return current
	}
	return cached
}

func videoHTTPClient(timeout time.Duration, headers map[string]string, reference bool) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return http.ErrUseLastResponse
		}
		if len(via) > 0 && (!strings.EqualFold(req.URL.Host, via[0].URL.Host) || req.URL.Scheme != via[0].URL.Scheme) {
			if reference && req.Method == http.MethodPost {
				return http.ErrUseLastResponse
			}
			req.Header.Del("Authorization")
			for name := range headers {
				req.Header.Del(name)
			}
		}
		return nil
	}}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
