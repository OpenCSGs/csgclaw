package openclawsandbox

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
)

//go:embed defaults/openclaw-gateway.json
var defaultOpenClawGatewayConfig []byte

const (
	HostDir           = ".openclaw"
	HostConfig        = "openclaw.json"
	HostExecApproval  = "exec-approvals.json"
	HostWorkspaceDir  = "workspace"
	BoxUserHome       = "/home/node"
	BoxDir            = "/home/node/.openclaw"
	BoxWorkspaceDir   = BoxDir + "/workspace"
	BoxProjectsDir    = BoxDir + "/workspace/projects"
	BoxGatewayLogPath = BoxDir + "/gateway.log"

	openClawBridgeProviderID = "csgclaw-llm"
)

type BaseURLResolver func(config.ServerConfig) string

func Root(agentHome string) string {
	return filepath.Join(agentHome, HostDir)
}

func workspaceRoot(agentHome string) string {
	return filepath.Join(Root(agentHome), HostWorkspaceDir)
}

func HostGatewayLogPath(agentHome string) string {
	return filepath.Join(Root(agentHome), "gateway.log")
}

func EnsureConfig(agentHome, participantID, agentID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL BaseURLResolver, feishuProvider feishu.BotCredentialProvider) (string, error) {
	hostRoot := Root(agentHome)
	if err := os.MkdirAll(hostRoot, 0o755); err != nil {
		return "", fmt.Errorf("create openclaw config dir: %w", err)
	}
	data, err := renderConfig(participantID, agentID, server, model, resolveBaseURL, feishuProvider)
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(hostRoot, HostConfig)
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
	if err := writeExecApprovalsAllowAll(hostRoot); err != nil {
		return "", err
	}
	return hostRoot, nil
}

// writeExecApprovalsAllowAll seeds ~/.openclaw/exec-approvals.json so the
// gateway-side approval daemon never prompts the agent for /approve. OpenClaw
// takes the stricter of tools.exec.* and the file's defaults; without this file
// the file-side defaults (deny + on-miss) still gate every command. The
// wildcard allowlist keeps commands working even when a model-generated exec
// call explicitly narrows itself to allowlist mode.
func writeExecApprovalsAllowAll(hostRoot string) error {
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
				"security":        "full",
				"ask":             "off",
				"askFallback":     "full",
				"autoAllowSkills": true,
				"allowlist": []map[string]any{
					{"pattern": "*"},
				},
			},
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode openclaw exec-approvals: %w", err)
	}
	target := filepath.Join(hostRoot, HostExecApproval)
	newContent := append(data, '\n')
	if existing, readErr := os.ReadFile(target); readErr == nil && string(existing) == string(newContent) {
		return nil // already up-to-date; avoid spurious VirtioFS write events
	}
	if err := os.WriteFile(target, newContent, 0o600); err != nil {
		return fmt.Errorf("write openclaw exec-approvals: %w", err)
	}
	return nil
}

func renderConfig(participantID, agentID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL BaseURLResolver, feishuProvider feishu.BotCredentialProvider) ([]byte, error) {
	participantID = strings.TrimSpace(participantID)
	agentID = strings.TrimSpace(agentID)
	if participantID == "" {
		participantID = agentID
	}
	if agentID == "" {
		agentID = participantID
	}
	var cfg map[string]any
	if err := json.Unmarshal(defaultOpenClawGatewayConfig, &cfg); err != nil {
		return nil, fmt.Errorf("decode embedded openclaw config: %w", err)
	}
	if err := updateOpenClawModelProvider(cfg, agentID, server, model, resolveBaseURL); err != nil {
		return nil, err
	}
	if err := updateOpenClawCsgclawChannel(cfg, participantID, server, resolveBaseURL); err != nil {
		return nil, err
	}
	if err := updateOpenClawFeishuChannel(cfg, agentID, feishuProvider); err != nil {
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

func updateOpenClawModelProvider(cfg map[string]any, botID string, server config.ServerConfig, modelCfg config.ModelConfig, resolveBaseURL BaseURLResolver) error {
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
	managerBaseURL := managerBaseURL(server, resolveBaseURL)
	modelID := strings.TrimSpace(modelCfg.ModelID)
	if modelID == "" {
		return fmt.Errorf("openclaw config: model id is required")
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

func updateOpenClawCsgclawChannel(cfg map[string]any, participantID string, server config.ServerConfig, resolveBaseURL BaseURLResolver) error {
	channels, ok := cfg["channels"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing channels")
	}
	ch, ok := channels["csgclaw"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing channels.csgclaw")
	}
	if baseURL := managerBaseURL(server, resolveBaseURL); baseURL != "" {
		ch["baseUrl"] = baseURL
	}
	if server.AccessToken != "" {
		ch["accessToken"] = server.AccessToken
	}
	ch["botId"] = participantID
	ch["enabled"] = true
	return nil
}

func updateOpenClawFeishuChannel(cfg map[string]any, botID string, provider feishu.BotCredentialProvider) error {
	botID = strings.TrimSpace(botID)
	if botID == "" || provider == nil {
		return nil
	}
	app, ok := provider.BotConfig(botID)
	if !ok {
		return nil
	}
	appID := strings.TrimSpace(app.AppID)
	appSecret := strings.TrimSpace(app.AppSecret)
	if appID == "" || appSecret == "" {
		return nil
	}

	channels, ok := cfg["channels"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing channels")
	}
	channels["feishu"] = map[string]any{
		"enabled":        true,
		"connectionMode": "websocket",
		"defaultAccount": botID,
		"dmPolicy":       "open",
		"allowFrom":      []any{"*"},
		"groupPolicy":    "open",
		"requireMention": true,
		"accounts": map[string]any{
			botID: map[string]any{
				"enabled":   true,
				"appId":     appID,
				"appSecret": appSecret,
				"name":      botID,
			},
		},
	}
	if err := enableOpenClawPlugin(cfg, "feishu"); err != nil {
		return err
	}
	return nil
}

func enableOpenClawPlugin(cfg map[string]any, pluginID string) error {
	plugins, ok := cfg["plugins"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded openclaw config is missing plugins")
	}
	entries, ok := plugins["entries"].(map[string]any)
	if !ok {
		entries = map[string]any{}
		plugins["entries"] = entries
	}
	entry, _ := entries[pluginID].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
	}
	entry["enabled"] = true
	entries[pluginID] = entry
	return nil
}

func managerBaseURL(server config.ServerConfig, resolveBaseURL BaseURLResolver) string {
	if resolveBaseURL == nil {
		return strings.TrimRight(strings.TrimSpace(server.AdvertiseBaseURL), "/")
	}
	return strings.TrimRight(strings.TrimSpace(resolveBaseURL(server)), "/")
}

func llmBridgeBaseURL(managerBaseURL, agentID string) string {
	managerBaseURL = strings.TrimRight(strings.TrimSpace(managerBaseURL), "/")
	return managerBaseURL + "/api/v1/agents/" + strings.TrimSpace(agentID) + "/llm"
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
