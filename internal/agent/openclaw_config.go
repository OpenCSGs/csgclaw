package agent

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/config"
)

//go:embed defaults/openclaw-gateway.json
var defaultOpenClawGatewayConfig []byte

const (
	hostOpenClawDir              = ".openclaw"
	hostOpenClawConfig           = "openclaw.json"
	hostOpenClawExecApproval     = "exec-approvals.json"
	hostOpenClawLogs             = "logs"
	boxOpenClawUserHome          = "/home/node"
	boxOpenClawDir               = "/home/node/.openclaw"
	boxOpenClawWorkspaceDir      = boxOpenClawDir + "/workspace"
	boxOpenClawProjectsDir       = boxOpenClawDir + "/workspace/projects"
	openClawGatewayLog           = boxOpenClawDir + "/gateway.log"
	openClawBridgeProviderID     = "csgclaw-llm"
	openClawMinimaxProviderID    = "csgclaw-minimax"
	openClawMinimaxCNBaseURL     = "https://api.minimaxi.com/anthropic"
	openClawMinimaxGlobalBaseURL = "https://api.minimax.io/anthropic"
)

func agentOpenClawRoot(agentName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve host home dir: %w", err)
	}
	return filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, agentName, hostOpenClawDir), nil
}

func ensureAgentOpenClawConfig(agentName, botID string, server config.ServerConfig, model config.ModelConfig) (string, error) {
	hostRoot, err := agentOpenClawRoot(agentName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(hostRoot, hostOpenClawLogs), 0o755); err != nil {
		return "", fmt.Errorf("create openclaw logs dir: %w", err)
	}
	data, err := renderAgentOpenClawConfig(botID, server, model)
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(hostRoot, hostOpenClawConfig)
	newContent := append(data, '\n')
	// Skip the write if the file already contains identical content. Writing
	// openclaw.json while the gateway is running triggers a hot-reload that
	// spawns a new gateway process; if the old process still holds the lock the
	// new one will fail with "lock timeout". Avoiding unnecessary writes prevents
	// that race.
	if existing, readErr := os.ReadFile(configPath); readErr == nil && string(existing) == string(newContent) {
		// File is up-to-date; no write needed.
	} else {
		if writeErr := os.WriteFile(configPath, newContent, 0o600); writeErr != nil {
			return "", fmt.Errorf("write openclaw config: %w", writeErr)
		}
	}
	if err := writeOpenClawExecApprovalsAllowAll(hostRoot); err != nil {
		return "", err
	}
	return hostRoot, nil
}

// writeOpenClawExecApprovalsAllowAll seeds ~/.openclaw/exec-approvals.json so the
// gateway-side approval daemon never prompts the agent for /approve. OpenClaw
// takes the stricter of tools.exec.* and the file's defaults; without this file
// the file-side defaults (deny + on-miss) still gate every command.
func writeOpenClawExecApprovalsAllowAll(hostRoot string) error {
	payload := map[string]any{
		"version": 1,
		"defaults": map[string]any{
			"security":        "full",
			"ask":             "off",
			"askFallback":     "full",
			"autoAllowSkills": true,
		},
		"agents": map[string]any{
			"*": map[string]any{
				"security":    "full",
				"ask":         "off",
				"askFallback": "full",
			},
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode openclaw exec-approvals: %w", err)
	}
	target := filepath.Join(hostRoot, hostOpenClawExecApproval)
	newContent := append(data, '\n')
	if existing, readErr := os.ReadFile(target); readErr == nil && string(existing) == string(newContent) {
		return nil // already up-to-date; avoid spurious VirtioFS write events
	}
	if err := os.WriteFile(target, newContent, 0o600); err != nil {
		return fmt.Errorf("write openclaw exec-approvals: %w", err)
	}
	return nil
}

func renderAgentOpenClawConfig(botID string, server config.ServerConfig, model config.ModelConfig) ([]byte, error) {
	var cfg map[string]any
	if err := json.Unmarshal(defaultOpenClawGatewayConfig, &cfg); err != nil {
		return nil, fmt.Errorf("decode embedded openclaw config: %w", err)
	}
	if err := updateOpenClawModelProvider(cfg, botID, server, model); err != nil {
		return nil, err
	}
	if err := updateOpenClawCsgclawChannel(cfg, botID, server); err != nil {
		return nil, err
	}
	if err := updateOpenClawGatewayAuth(cfg, server); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode openclaw config: %w", err)
	}
	return data, nil
}

func updateOpenClawModelProvider(cfg map[string]any, botID string, server config.ServerConfig, modelCfg config.ModelConfig) error {
	modelCfg = modelCfg.Resolved()
	modelsRoot, ok := cfg["models"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing models")
	}
	providers, ok := modelsRoot["providers"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing models.providers")
	}
	llm, ok := providers[openClawBridgeProviderID].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing models.providers.%s", openClawBridgeProviderID)
	}
	managerBaseURL := resolveManagerBaseURL(server)
	modelID := strings.TrimSpace(modelCfg.ModelID)
	if modelID == "" {
		return fmt.Errorf("openclaw config: model id is required")
	}
	if shouldUseOpenClawMinimaxProvider(modelCfg) {
		providers[openClawMinimaxProviderID] = buildOpenClawMinimaxProvider(modelCfg)
		delete(providers, openClawBridgeProviderID)
		return updateOpenClawPrimaryModel(cfg, openClawMinimaxProviderID, modelID)
	}
	if base := strings.TrimSpace(modelCfg.BaseURL); base != "" {
		llm["baseUrl"] = strings.TrimRight(base, "/")
	} else {
		llm["baseUrl"] = llmBridgeBaseURL(managerBaseURL, botID)
	}
	apiKey := strings.TrimSpace(modelCfg.APIKey)
	if apiKey == "" {
		apiKey = server.AccessToken
	}
	if apiKey != "" {
		llm["apiKey"] = apiKey
	}
	// OpenClaw: auth "token" + authHeader for upstreams that require Authorization: Bearer <sk-...> (e.g. Infini MaaS).
	llm["auth"] = "token"
	llm["authHeader"] = true
	modelList, ok := llm["models"].([]any)
	if !ok || len(modelList) == 0 {
		return fmt.Errorf("embedded openclaw config is missing models.providers.%s.models[0]", openClawBridgeProviderID)
	}
	entry, ok := modelList[0].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config has invalid %s.models[0]", openClawBridgeProviderID)
	}
	entry["id"] = modelID
	entry["name"] = modelID
	return updateOpenClawPrimaryModel(cfg, openClawBridgeProviderID, modelID)
}

func shouldUseOpenClawMinimaxProvider(modelCfg config.ModelConfig) bool {
	base := strings.TrimSpace(modelCfg.BaseURL)
	if base == "" {
		return false
	}
	// Infini MaaS is OpenAI-compatible at .../maas/v1; model IDs can contain "minimax" but
	// must not use OpenClaw's built-in api.minimaxi.com Anthropic adapter.
	if isInfiniMaaSOpenAICompatBaseURL(base) {
		return false
	}
	modelID := strings.ToLower(strings.TrimSpace(modelCfg.ModelID))
	if strings.Contains(modelID, "minimax") {
		return true
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return strings.Contains(host, "minimax")
}

func isInfiniMaaSOpenAICompatBaseURL(base string) bool {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), "cloud.infini-ai.com")
}

func buildOpenClawMinimaxProvider(modelCfg config.ModelConfig) map[string]any {
	modelID := strings.TrimSpace(modelCfg.ModelID)
	baseURL := openClawMinimaxAnthropicBaseURL(modelCfg.BaseURL)
	provider := map[string]any{
		"baseUrl":    baseURL,
		"api":        "anthropic-messages",
		"authHeader": true,
		"models": []any{
			map[string]any{
				"id":            modelID,
				"name":          modelID,
				"reasoning":     false,
				"input":         []any{"text"},
				"contextWindow": 204800,
				"maxTokens":     131072,
				"cost": map[string]any{
					"input":      0.3,
					"output":     1.2,
					"cacheRead":  0.06,
					"cacheWrite": 0.375,
				},
			},
		},
	}
	if apiKey := strings.TrimSpace(modelCfg.APIKey); apiKey != "" {
		provider["apiKey"] = apiKey
	}
	return provider
}

func openClawMinimaxAnthropicBaseURL(rawBaseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil {
		return openClawMinimaxCNBaseURL
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "api.minimax.io":
		return openClawMinimaxGlobalBaseURL
	case "api.minimaxi.com":
		return openClawMinimaxCNBaseURL
	default:
		if strings.TrimSpace(rawBaseURL) != "" {
			return strings.TrimRight(rawBaseURL, "/")
		}
		return openClawMinimaxCNBaseURL
	}
}

func updateOpenClawPrimaryModel(cfg map[string]any, providerID, modelID string) error {
	agents, ok := cfg["agents"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing agents")
	}
	defaults, ok := agents["defaults"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing agents.defaults")
	}
	modelBlock, ok := defaults["model"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing agents.defaults.model")
	}
	modelBlock["primary"] = providerID + "/" + modelID
	return nil
}

func updateOpenClawCsgclawChannel(cfg map[string]any, botID string, server config.ServerConfig) error {
	channels, ok := cfg["channels"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing channels")
	}
	ch, ok := channels["csgclaw"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing channels.csgclaw")
	}
	if baseURL := resolveManagerBaseURL(server); baseURL != "" {
		ch["baseUrl"] = baseURL
	}
	if server.AccessToken != "" {
		ch["accessToken"] = server.AccessToken
	}
	ch["botId"] = botID
	ch["enabled"] = true
	return nil
}

func updateOpenClawGatewayAuth(cfg map[string]any, server config.ServerConfig) error {
	gw, ok := cfg["gateway"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing gateway")
	}
	auth, ok := gw["auth"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing gateway.auth")
	}
	if server.AccessToken != "" {
		auth["token"] = server.AccessToken
	}
	return nil
}
