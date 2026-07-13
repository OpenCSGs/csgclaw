package picoclawsandbox

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

//go:embed defaults/picoclaw-config.json
var defaultGatewayConfig []byte

//go:embed defaults/manager-security.yml
var defaultSecurityConfig string

const (
	HostDir           = ".picoclaw"
	HostConfig        = "config.json"
	HostSecurity      = ".security.yml"
	HostWorkspaceDir  = "workspace"
	BoxUserHome       = "/home/picoclaw"
	BoxDir            = BoxUserHome + "/.picoclaw"
	BoxWorkspaceDir   = BoxDir + "/workspace"
	BoxProjectsDir    = BoxDir + "/workspace/projects"
	BoxGatewayLogPath = BoxDir + "/gateway.log"
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

func WorkspaceConfigRoot(agentHome string) string {
	return Root(agentHome)
}

func EnsureConfig(agentHome, participantID, agentID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL BaseURLResolver, feishuProviders ...feishu.AgentCredentialProvider) (string, error) {
	return EnsureConfigWithMCPServers(agentHome, participantID, agentID, server, model, nil, resolveBaseURL, feishuProviders...)
}

func EnsureConfigWithMCPServers(agentHome, participantID, agentID string, server config.ServerConfig, model config.ModelConfig, mcpServers map[string]any, resolveBaseURL BaseURLResolver, feishuProviders ...feishu.AgentCredentialProvider) (string, error) {
	hostRoot := Root(agentHome)
	if err := os.MkdirAll(hostRoot, 0o755); err != nil {
		return "", fmt.Errorf("create picoclaw config dir: %w", err)
	}

	data, err := RenderConfigWithMCPServers(participantID, agentID, server, model, mcpServers, resolveBaseURL, feishuProviders...)
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(hostRoot, HostConfig)
	if err := os.WriteFile(configPath, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write manager picoclaw config: %w", err)
	}
	securityData := RenderSecurityConfig(server, model)
	securityPath := filepath.Join(hostRoot, HostSecurity)
	if err := os.WriteFile(securityPath, []byte(securityData), 0o600); err != nil {
		return "", fmt.Errorf("write manager security config: %w", err)
	}
	return hostRoot, nil
}

func RenderConfig(participantID, agentID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL BaseURLResolver, feishuProviders ...feishu.AgentCredentialProvider) ([]byte, error) {
	return RenderConfigWithMCPServers(participantID, agentID, server, model, nil, resolveBaseURL, feishuProviders...)
}

func RenderConfigWithMCPServers(participantID, agentID string, server config.ServerConfig, model config.ModelConfig, mcpServers map[string]any, resolveBaseURL BaseURLResolver, feishuProviders ...feishu.AgentCredentialProvider) ([]byte, error) {
	participantID = strings.TrimSpace(participantID)
	agentID = strings.TrimSpace(agentID)
	if participantID == "" {
		participantID = agentID
	}
	if agentID == "" {
		agentID = participantID
	}
	var cfg map[string]any
	if err := json.Unmarshal(defaultGatewayConfig, &cfg); err != nil {
		return nil, fmt.Errorf("decode embedded manager picoclaw config: %w", err)
	}

	if err := updateModelList(cfg, agentID, server, model, resolveBaseURL); err != nil {
		return nil, err
	}
	if err := updateCSGClawChannel(cfg, participantID, server, resolveBaseURL); err != nil {
		return nil, err
	}
	if err := updateFeishuChannel(cfg, agentID, firstFeishuProvider(feishuProviders)); err != nil {
		return nil, err
	}
	if err := updatePicoClawMCP(cfg, mcpServers); err != nil {
		return nil, err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manager picoclaw config: %w", err)
	}
	return data, nil
}

func updatePicoClawMCP(cfg map[string]any, mcpServers map[string]any) error {
	servers, err := agentruntime.NormalizeMCPServers(mcpServers)
	if err != nil {
		return err
	}
	tools, ok := cfg["tools"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded manager picoclaw config is missing tools")
	}
	mcpRoot, _ := tools["mcp"].(map[string]any)
	if mcpRoot == nil {
		mcpRoot = map[string]any{}
		tools["mcp"] = mcpRoot
	}
	if servers == nil {
		mcpRoot["enabled"] = false
		delete(mcpRoot, "servers")
		return nil
	}
	mcpRoot["enabled"] = true
	mcpRoot["servers"] = resolvePicoClawMCPWorkspaceServers(servers, BoxWorkspaceDir)
	return nil
}

func resolvePicoClawMCPWorkspaceServers(servers map[string]any, workspaceGuestPath string) map[string]any {
	workspaceGuestPath = strings.TrimSpace(workspaceGuestPath)
	if workspaceGuestPath == "" {
		return servers
	}
	out := make(map[string]any, len(servers))
	for name, rawEntry := range servers {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			out[name] = rawEntry
			continue
		}
		next := make(map[string]any, len(entry))
		for key, value := range entry {
			next[key] = value
		}
		if args, ok := next["args"].([]any); ok {
			next["args"] = resolvePicoClawMCPWorkspaceArgs(args, workspaceGuestPath)
		}
		out[name] = next
	}
	return out
}

func resolvePicoClawMCPWorkspaceArgs(args []any, workspaceGuestPath string) []any {
	out := make([]any, len(args))
	for idx, arg := range args {
		text, ok := arg.(string)
		if !ok {
			out[idx] = arg
			continue
		}
		out[idx] = resolvePicoClawMCPWorkspaceArg(text, workspaceGuestPath)
	}
	return out
}

func resolvePicoClawMCPWorkspaceArg(arg, workspaceGuestPath string) string {
	for _, placeholder := range []string{"${workspace}", "${workspaceDir}", "{workspace}", "{workspaceDir}"} {
		if arg == placeholder {
			return workspaceGuestPath
		}
		if strings.HasPrefix(arg, placeholder+"/") {
			return path.Join(workspaceGuestPath, strings.TrimPrefix(arg, placeholder+"/"))
		}
	}
	return arg
}

func updateFeishuChannel(cfg map[string]any, agentID string, provider feishu.AgentCredentialProvider) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || provider == nil {
		return nil
	}
	_, app, ok := provider.BotConfigForAgent(agentID)
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
		return fmt.Errorf("embedded manager picoclaw config is missing channels")
	}
	channel, _ := channels["feishu"].(map[string]any)
	if channel == nil {
		channel = map[string]any{}
		channels["feishu"] = channel
	}
	channel["enabled"] = true
	channel["app_id"] = appID
	channel["app_secret"] = appSecret
	return nil
}

func firstFeishuProvider(providers []feishu.AgentCredentialProvider) feishu.AgentCredentialProvider {
	for _, provider := range providers {
		if provider != nil {
			return provider
		}
	}
	return nil
}

func updateModelList(cfg map[string]any, agentID string, server config.ServerConfig, modelCfg config.ModelConfig, resolveBaseURL BaseURLResolver) error {
	modelList, ok := cfg["model_list"].([]any)
	if !ok || len(modelList) == 0 {
		return fmt.Errorf("embedded manager picoclaw config is missing model_list[0]")
	}
	model, ok := modelList[0].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded manager picoclaw config has invalid model_list[0]")
	}
	if modelID := strings.TrimSpace(modelCfg.ModelID); modelID != "" {
		model["model_name"] = modelID
		model["model"] = BridgeModelID(modelID)
	}
	if agents, ok := cfg["agents"].(map[string]any); ok {
		if defaults, ok := agents["defaults"].(map[string]any); ok {
			if modelID := strings.TrimSpace(modelCfg.ModelID); modelID != "" {
				defaults["model_name"] = modelID
			}
		}
	}

	if managerBaseURL := managerBaseURL(server, resolveBaseURL); managerBaseURL != "" {
		model["api_base"] = llmBridgeBaseURL(managerBaseURL, agentID)
	}
	if server.AccessToken != "" {
		model["api_key"] = server.AccessToken
	}
	return nil
}

func BridgeModelID(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(modelID), "openai/") {
		return modelID
	}
	if prefix, rest, ok := strings.Cut(modelID, ":"); ok && strings.EqualFold(strings.TrimSpace(prefix), "openai") && strings.TrimSpace(rest) != "" {
		return "openai/" + strings.TrimSpace(rest)
	}
	return "openai/" + modelID
}

func updateCSGClawChannel(cfg map[string]any, participantID string, server config.ServerConfig, resolveBaseURL BaseURLResolver) error {
	channels, ok := cfg["channels"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded manager picoclaw config is missing channels")
	}
	channel, ok := channels["csgclaw"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded manager picoclaw config is missing channels.csgclaw")
	}
	if baseURL := managerBaseURL(server, resolveBaseURL); baseURL != "" {
		channel["base_url"] = baseURL
	}
	if server.AccessToken != "" {
		channel["access_token"] = server.AccessToken
	}
	delete(channel, "bot_id")
	channel["participant_id"] = participantID
	channel["enabled"] = true
	return nil
}

func RenderSecurityConfig(server config.ServerConfig, model config.ModelConfig) string {
	modelID := model.ModelID
	apiKey := strings.TrimSpace(server.AccessToken)
	if apiKey == "" {
		apiKey = model.APIKey
	}

	content := strings.ReplaceAll(defaultSecurityConfig, "__MODEL_ID__", modelID)
	content = strings.ReplaceAll(content, "__API_KEY__", apiKey)
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content
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
