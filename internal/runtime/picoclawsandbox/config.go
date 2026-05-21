package picoclawsandbox

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/config"
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

func WorkspaceRoot(agentHome string) string {
	return filepath.Join(Root(agentHome), HostWorkspaceDir)
}

func HostGatewayLogPath(agentHome string) string {
	return filepath.Join(Root(agentHome), "gateway.log")
}

func WorkspaceConfigRoot(agentHome string) string {
	return Root(agentHome)
}

func EnsureConfig(agentHome, botID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL BaseURLResolver) (string, error) {
	hostRoot := Root(agentHome)
	if err := os.MkdirAll(hostRoot, 0o755); err != nil {
		return "", fmt.Errorf("create picoclaw config dir: %w", err)
	}

	data, err := RenderConfig(botID, server, model, resolveBaseURL)
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

func RenderConfig(botID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL BaseURLResolver) ([]byte, error) {
	var cfg map[string]any
	if err := json.Unmarshal(defaultGatewayConfig, &cfg); err != nil {
		return nil, fmt.Errorf("decode embedded manager picoclaw config: %w", err)
	}

	if err := updateModelList(cfg, botID, server, model, resolveBaseURL); err != nil {
		return nil, err
	}
	if err := updateCSGClawChannel(cfg, botID, server, resolveBaseURL); err != nil {
		return nil, err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manager picoclaw config: %w", err)
	}
	return data, nil
}

func updateModelList(cfg map[string]any, botID string, server config.ServerConfig, modelCfg config.ModelConfig, resolveBaseURL BaseURLResolver) error {
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
		model["api_base"] = llmBridgeBaseURL(managerBaseURL, botID)
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

func updateCSGClawChannel(cfg map[string]any, botID string, server config.ServerConfig, resolveBaseURL BaseURLResolver) error {
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
	channel["bot_id"] = botID
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

func llmBridgeBaseURL(managerBaseURL, botID string) string {
	managerBaseURL = strings.TrimRight(strings.TrimSpace(managerBaseURL), "/")
	return managerBaseURL + "/api/bots/" + strings.TrimSpace(botID) + "/llm"
}
