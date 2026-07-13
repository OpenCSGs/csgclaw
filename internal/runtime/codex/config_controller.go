package codex

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"csgclaw/internal/auth"
	"csgclaw/internal/modelprovider"
	agentruntime "csgclaw/internal/runtime"
)

var checkResponsesAPIForProvider = modelprovider.CheckResponsesAPI
var openCSGCredentialsForResponsesProbe = func(ctx context.Context) (string, string, bool, error) {
	store, err := auth.DefaultStore()
	if err != nil {
		return "", "", false, err
	}
	return store.EnsureAIGatewayCredentials(ctx, &http.Client{})
}

type responsesProbeCache struct {
	mu      sync.Mutex
	results map[string]struct{}
}

var codexResponsesProbeCache = responsesProbeCache{results: make(map[string]struct{})}

func TestOnlySetResponsesAPIProbe(probe func(context.Context, string, string, string, map[string]string) error) func() {
	previous := checkResponsesAPIForProvider
	checkResponsesAPIForProvider = probe
	codexResponsesProbeCache.clear()
	return func() {
		checkResponsesAPIForProvider = previous
		codexResponsesProbeCache.clear()
	}
}

func (r *Runtime) ValidateConfig(ctx context.Context, current agentruntime.RuntimeConfigSnapshot) error {
	target, ok, err := responsesProbeTarget(ctx, current.Profile)
	if err != nil {
		return fmt.Errorf("validate Codex provider Responses API: %w", err)
	}
	if !ok {
		return nil
	}
	if err := codexResponsesProbeCache.validate(ctx, target); err != nil {
		return fmt.Errorf("validate Codex provider Responses API at %s for model %s: %w", target.baseURL, target.modelID, err)
	}
	return nil
}

func (r *Runtime) RestartRequired(change agentruntime.RuntimeConfigChange) (bool, error) {
	return codexWorkspaceOptionChanged(change.Previous.Options, change.Current.Options), nil
}

func (r *Runtime) ReconcileConfig(ctx context.Context, h agentruntime.Handle, change agentruntime.RuntimeConfigChange) error {
	_ = change
	return r.RefreshCodexHomeAgentsFile(ctx, h)
}

func (r *Runtime) ValidateMCPServers(_ context.Context, current agentruntime.MCPServersSnapshot) error {
	return agentruntime.ValidateMCPServers(current.Servers)
}

func (r *Runtime) MCPServersRestartRequired(change agentruntime.MCPServersChange) (bool, error) {
	return agentruntime.MCPServersNeedsRestart(change.Previous.Servers, change.Current.Servers)
}

func (r *Runtime) ReconcileMCPServers(_ context.Context, h agentruntime.Handle, _ agentruntime.MCPServersChange) error {
	agentRef, err := r.resolveAgent(h)
	if err != nil {
		return err
	}
	codexHomeDir, err := r.resolveCodexHomeDir(agentRef.ID)
	if err != nil {
		return err
	}
	workspaceDir, err := r.resolveWorkspaceDir(agentRef.ID, agentRef.RuntimeOptions)
	if err != nil {
		return err
	}
	return r.seedCodexHomeConfig(codexHomeDir, workspaceDir, agentRef.Profile.Normalized(), agentRef.MCPServers)
}

func (r *Runtime) ListMCPServers(_ context.Context, h agentruntime.Handle, _ agentruntime.MCPServersSnapshot) (agentruntime.MCPServersSnapshot, error) {
	agentRef, err := r.resolveAgent(h)
	if err != nil {
		return agentruntime.MCPServersSnapshot{}, err
	}
	codexHomeDir, err := r.resolveCodexHomeDir(agentRef.ID)
	if err != nil {
		return agentruntime.MCPServersSnapshot{}, err
	}
	configPath := filepath.Join(codexHomeDir, configFileName)
	raw, err := r.readFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return agentruntime.MCPServersSnapshot{}, nil
		}
		return agentruntime.MCPServersSnapshot{}, fmt.Errorf("read runtime codex mcp config %s: %w", configPath, err)
	}
	config, err := parseCodexMCPServers(string(raw))
	if err != nil {
		return agentruntime.MCPServersSnapshot{}, err
	}
	return agentruntime.MCPServersSnapshot{Servers: config}, nil
}

type responsesProbeTargetConfig struct {
	provider string
	baseURL  string
	apiKey   string
	modelID  string
	headers  map[string]string
}

func responsesProbeTarget(ctx context.Context, profile agentruntime.RuntimeProfileConfig) (responsesProbeTargetConfig, bool, error) {
	provider := strings.ToLower(strings.TrimSpace(profile.Provider))
	switch provider {
	case "codex", "claude_code", "claude-code":
		return responsesProbeTargetConfig{}, false, nil
	}
	if provider == "csghub" || provider == "opencsg" {
		baseURL, apiKey, ok, err := openCSGCredentialsForResponsesProbe(ctx)
		if err != nil {
			return responsesProbeTargetConfig{}, false, fmt.Errorf("resolve OpenCSG credentials: %w", err)
		}
		if !ok {
			return responsesProbeTargetConfig{}, false, fmt.Errorf("OpenCSG sign-in is required")
		}
		return responsesProbeTargetConfig{
			provider: provider,
			baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
			apiKey:   strings.TrimSpace(apiKey),
			modelID:  strings.TrimSpace(profile.ModelID),
			headers:  normalizeHeaders(profile.Headers),
		}, true, nil
	}
	baseURL := strings.TrimRight(strings.TrimSpace(profile.BaseURL), "/")
	if baseURL == "" {
		return responsesProbeTargetConfig{}, false, fmt.Errorf("provider base_url is required")
	}
	return responsesProbeTargetConfig{
		provider: provider,
		baseURL:  baseURL,
		apiKey:   strings.TrimSpace(profile.APIKey),
		modelID:  strings.TrimSpace(profile.ModelID),
		headers:  normalizeHeaders(profile.Headers),
	}, true, nil
}

func (c *responsesProbeCache) validate(ctx context.Context, target responsesProbeTargetConfig) error {
	key := responsesProbeCacheKey(target)
	c.mu.Lock()
	if c.results == nil {
		c.results = make(map[string]struct{})
	}
	if _, ok := c.results[key]; ok {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	err := checkResponsesAPIForProvider(ctx, target.baseURL, target.apiKey, target.modelID, target.headers)
	if errors.Is(err, modelprovider.ErrResponsesAPIUnsupported) {
		err = nil
	}

	if err == nil {
		c.mu.Lock()
		c.results[key] = struct{}{}
		c.mu.Unlock()
	}
	return err
}

func (c *responsesProbeCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results = make(map[string]struct{})
}

func responsesProbeCacheKey(target responsesProbeTargetConfig) string {
	return strings.TrimSpace(target.provider) + "\x00" + strings.TrimRight(strings.TrimSpace(target.baseURL), "/") + "\x00" + strings.TrimSpace(target.modelID)
}

func normalizeHeaders(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func codexWorkspaceOptionChanged(previous, current map[string]any) bool {
	previousOpts, err := DecodeRuntimeOptions(previous)
	if err != nil {
		return true
	}
	currentOpts, err := DecodeRuntimeOptions(current)
	if err != nil {
		return true
	}
	return previousOpts.LocalWorkspaceDir != currentOpts.LocalWorkspaceDir
}
