package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"csgclaw/internal/modelprovider"
)

var checkResponsesAPIForProvider = modelprovider.CheckResponsesAPI

const (
	codexWireAPIResponses = "responses"
	codexWireAPIChat      = "chat"
)

type responsesProbeResult struct {
	wireAPI string
	err     error
}

type responsesProbeCache struct {
	mu      sync.Mutex
	results map[string]responsesProbeResult
}

var codexResponsesProbeCache = responsesProbeCache{results: make(map[string]responsesProbeResult)}

func TestOnlySetResponsesAPIProbe(probe func(context.Context, string, string, string, map[string]string) error) func() {
	previous := checkResponsesAPIForProvider
	checkResponsesAPIForProvider = probe
	codexResponsesProbeCache.clear()
	return func() {
		checkResponsesAPIForProvider = previous
		codexResponsesProbeCache.clear()
	}
}

func (s *Service) ensureCodexWireAPI(ctx context.Context, runtimeKind string, profile AgentProfile) error {
	if strings.TrimSpace(runtimeKind) != RuntimeKindCodex {
		return nil
	}
	target, ok, err := responsesProbeTarget(profile)
	if err != nil {
		return fmt.Errorf("validate Codex provider Responses API: %w", err)
	}
	if !ok {
		return nil
	}
	if _, err := codexResponsesProbeCache.wireAPI(ctx, target); err != nil {
		return fmt.Errorf("validate Codex provider Responses API at %s for model %s: %w", target.baseURL, target.modelID, err)
	}
	return nil
}

type responsesProbeTargetConfig struct {
	provider string
	baseURL  string
	apiKey   string
	modelID  string
	headers  map[string]string
}

func responsesProbeTarget(profile AgentProfile) (responsesProbeTargetConfig, bool, error) {
	profile = normalizeProfile(profile, profile.Name, profile.Description)
	switch profile.Provider {
	case ProviderCodex, ProviderClaudeCode:
		// CLIProxy-backed profiles do not expose their upstream as a static OpenAI-compatible
		// provider here. Their auth and routing are validated by the existing CLIProxy flow.
		return responsesProbeTargetConfig{}, false, nil
	}
	baseURL := profileBaseURL(profile)
	if strings.TrimSpace(baseURL) == "" {
		return responsesProbeTargetConfig{}, false, fmt.Errorf("provider base_url is required")
	}
	return responsesProbeTargetConfig{
		provider: profile.Provider,
		baseURL:  baseURL,
		apiKey:   profileAPIKey(profile),
		modelID:  strings.TrimSpace(profile.ModelID),
		headers:  normalizeStringMap(profile.Headers),
	}, true, nil
}

func (c *responsesProbeCache) wireAPI(ctx context.Context, target responsesProbeTargetConfig) (string, error) {
	key := responsesProbeCacheKey(target)
	c.mu.Lock()
	if c.results == nil {
		c.results = make(map[string]responsesProbeResult)
	}
	if result, ok := c.results[key]; ok {
		c.mu.Unlock()
		return result.wireAPI, result.err
	}
	c.mu.Unlock()

	err := checkResponsesAPIForProvider(ctx, target.baseURL, target.apiKey, target.modelID, target.headers)
	result := responsesProbeResult{wireAPI: codexWireAPIResponses}
	if errors.Is(err, modelprovider.ErrResponsesAPIUnsupported) {
		result.wireAPI = codexWireAPIChat
		err = nil
	}
	result.err = err

	if err == nil {
		c.mu.Lock()
		c.results[key] = result
		c.mu.Unlock()
	}
	return result.wireAPI, result.err
}

func (c *responsesProbeCache) cachedWireAPI(target responsesProbeTargetConfig) (string, bool) {
	key := responsesProbeCacheKey(target)
	c.mu.Lock()
	defer c.mu.Unlock()
	result, ok := c.results[key]
	if !ok || result.err != nil || strings.TrimSpace(result.wireAPI) == "" {
		return "", false
	}
	return result.wireAPI, true
}

func (c *responsesProbeCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results = make(map[string]responsesProbeResult)
}

func responsesProbeCacheKey(target responsesProbeTargetConfig) string {
	return strings.TrimSpace(target.provider) + "\x00" + strings.TrimRight(strings.TrimSpace(target.baseURL), "/") + "\x00" + strings.TrimSpace(target.modelID)
}

func codexWireAPIForProfile(runtimeKind string, profile AgentProfile) string {
	if strings.TrimSpace(runtimeKind) != RuntimeKindCodex {
		return ""
	}
	return codexWireAPIResponses
}
