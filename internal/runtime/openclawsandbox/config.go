package openclawsandbox

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	"csgclaw/internal/modelcap"
)

//go:embed defaults/openclaw-gateway.json
var defaultOpenClawGatewayConfig []byte

const (
	HostDir                = ".openclaw"
	HostConfig             = "openclaw.json"
	HostExecApproval       = "exec-approvals.json"
	HostGatewayLog         = "gateway.log"
	HostWorkspaceDir       = "workspace"
	BoxUserHome            = "/home/node"
	BoxDir                 = "/home/node/.openclaw"
	BoxConfigPath          = BoxDir + "/" + HostConfig
	BoxWorkspaceDir        = BoxDir + "/workspace"
	BoxProjectsDir         = BoxDir + "/workspace/projects"
	BoxGatewayLogPath      = BoxDir + "/" + HostGatewayLog
	BoxWindowsWorkspaceDir = "/workspace"

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
	return filepath.Join(Root(agentHome), HostGatewayLog)
}

func EnsureConfig(agentHome, participantID, agentID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL BaseURLResolver, feishuProvider feishu.AgentCredentialProvider) (string, error) {
	return EnsureConfigWithMCPServers(agentHome, participantID, agentID, server, model, nil, resolveBaseURL, feishuProvider)
}

func EnsureConfigWithMCPServers(agentHome, participantID, agentID string, server config.ServerConfig, model config.ModelConfig, mcpServers map[string]any, resolveBaseURL BaseURLResolver, feishuProvider feishu.AgentCredentialProvider) (string, error) {
	hostRoot := Root(agentHome)
	if err := os.MkdirAll(hostRoot, 0o755); err != nil {
		return "", fmt.Errorf("create openclaw config dir: %w", err)
	}
	data, err := renderConfigWithMCPServers(participantID, agentID, server, model, mcpServers, resolveBaseURL, feishuProvider)
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
	if err := ensureGatewayLogFile(hostRoot); err != nil {
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

func ensureGatewayLogFile(hostRoot string) error {
	target := filepath.Join(hostRoot, HostGatewayLog)
	file, err := os.OpenFile(target, os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("create openclaw gateway log: %w", err)
	}
	return file.Close()
}

func updateOpenClawWorkspaceDefault(cfg map[string]any, workspace string) {
	if goruntime.GOOS != "windows" {
		return
	}
	agents, _ := cfg["agents"].(map[string]any)
	defaults, _ := agents["defaults"].(map[string]any)
	if defaults == nil {
		return
	}
	defaults["workspace"] = workspace
}
func renderConfig(participantID, agentID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL BaseURLResolver, feishuProvider feishu.AgentCredentialProvider) ([]byte, error) {
	return renderConfigWithMCPServers(participantID, agentID, server, model, nil, resolveBaseURL, feishuProvider)
}

func renderConfigWithMCPServers(participantID, agentID string, server config.ServerConfig, model config.ModelConfig, mcpServers map[string]any, resolveBaseURL BaseURLResolver, feishuProvider feishu.AgentCredentialProvider) ([]byte, error) {
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
	updateOpenClawWorkspaceDefault(cfg, workspaceGuestPathForGOOS(goruntime.GOOS))
	if err := updateOpenClawMCP(cfg, mcpServers); err != nil {
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
	if managerBaseURL == "" {
		return fmt.Errorf("openclaw config: csgclaw manager base url is required")
	}
	llm["baseUrl"] = llmBridgeBaseURL(managerBaseURL, botID)
	llm["apiKey"] = strings.TrimSpace(server.AccessToken)
	// OpenClaw sends a bearer token to the CSGClaw LLM bridge; the bridge owns
	// the current upstream provider URL, API key, and model rewrite.
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
	applyOpenClawModelCapabilities(llm, entry, modelCfg)
	return updateOpenClawAgentDefaults(cfg, openClawBridgeProviderID, modelCfg)
}

func applyOpenClawModelCapabilities(providerCfg, entry map[string]any, modelCfg config.ModelConfig) {
	caps := modelcap.ForProviderModel(modelCfg.Provider, modelCfg.ModelID)
	providerCfg["api"] = caps.OpenClawAPI
	if caps.OpenClawAPI == modelcap.OpenClawAPICodexResponses {
		entry["api"] = caps.OpenClawAPI
	} else {
		delete(entry, "api")
	}
	entry["reasoning"] = caps.SupportsReasoningEffort
	entry["input"] = stringSliceToAny(caps.InputModalities)
	compat, _ := entry["compat"].(map[string]any)
	if compat == nil {
		compat = map[string]any{}
	}
	compat["supportsReasoningEffort"] = caps.SupportsReasoningEffort
	compat["supportedReasoningEfforts"] = stringSliceToAny(caps.SupportedReasoningEfforts)
	compat["reasoningEffortMap"] = stringMapToAny(caps.ReasoningEffortMap)
	compat["supportsUsageInStreaming"] = caps.SupportsStreamingUsage
	entry["compat"] = compat
}

func stringSliceToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func stringMapToAny(values map[string]string) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func updateOpenClawAgentDefaults(cfg map[string]any, providerID string, modelCfg config.ModelConfig) error {
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
	modelBlock["primary"] = providerID + "/" + strings.TrimSpace(modelCfg.ModelID)
	caps := modelcap.ForProviderModel(modelCfg.Provider, modelCfg.ModelID)
	if !caps.SupportsReasoningEffort {
		delete(defaults, "thinkingDefault")
		return nil
	}
	if thinkingDefault := strings.TrimSpace(modelCfg.ReasoningEffort); thinkingDefault != "" {
		defaults["thinkingDefault"] = thinkingDefault
	}
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

func updateOpenClawFeishuChannel(cfg map[string]any, agentID string, provider feishu.AgentCredentialProvider) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || provider == nil {
		return nil
	}
	participantID, app, ok := provider.BotConfigForAgent(agentID)
	if !ok {
		return nil
	}
	participantID = strings.TrimSpace(participantID)
	if participantID == "" {
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
		"defaultAccount": participantID,
		"dmPolicy":       "open",
		"allowFrom":      []any{"*"},
		"groupPolicy":    "open",
		"requireMention": true,
		"accounts": map[string]any{
			participantID: map[string]any{
				"enabled":   true,
				"appId":     appID,
				"appSecret": appSecret,
				"name":      participantID,
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
