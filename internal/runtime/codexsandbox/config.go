package codexsandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/codexmodel"
	"csgclaw/internal/config"
)

const (
	HostDir          = ".codex-sandbox"
	HostConfig       = "config.json"
	HostGatewayLog   = "gateway.log"
	HostWorkspaceDir = "workspace"
	HostCodexHomeDir = "codex-home"

	BoxUserHome       = "/home/codex"
	BoxDir            = BoxUserHome + "/.codex-sandbox"
	BoxConfigPath     = BoxDir + "/" + HostConfig
	BoxWorkspaceDir   = BoxDir + "/workspace"
	BoxProjectsDir    = BoxWorkspaceDir + "/projects"
	BoxCodexHomeDir   = BoxDir + "/" + HostCodexHomeDir
	BoxGatewayLogPath = BoxDir + "/" + HostGatewayLog

	ProfileName        = "csgclaw"
	appSecretEnvKey    = "APP_SECRET"
	defaultCodexBinary = "/usr/local/bin/codex"
	codexConfigFile    = "config.toml"
	modelCatalogFile   = "model_catalog.json"
	csgclawProviderID  = "csgclaw-llm"
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

func EnsureConfig(agentHome, participantID, agentID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL BaseURLResolver, provider feishu.AgentCredentialProvider) (string, error) {
	hostRoot := Root(agentHome)
	if err := os.MkdirAll(hostRoot, 0o755); err != nil {
		return "", fmt.Errorf("create codex sandbox config dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(hostRoot, HostWorkspaceDir, "projects"), 0o755); err != nil {
		return "", fmt.Errorf("create codex sandbox workspace projects dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(hostRoot, HostCodexHomeDir), 0o755); err != nil {
		return "", fmt.Errorf("create codex sandbox codex home dir: %w", err)
	}
	if err := ensureGatewayLogFile(hostRoot); err != nil {
		return "", err
	}
	if err := ensureCodexHomeConfig(hostRoot, agentID, server, model, resolveBaseURL); err != nil {
		return "", err
	}

	data, err := RenderConfig(participantID, agentID, server, model, resolveBaseURL, provider)
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(hostRoot, HostConfig)
	if err := os.WriteFile(configPath, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write codex sandbox config: %w", err)
	}
	return hostRoot, nil
}

func ensureGatewayLogFile(hostRoot string) error {
	target := filepath.Join(hostRoot, HostGatewayLog)
	file, err := os.OpenFile(target, os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("create codex sandbox gateway log: %w", err)
	}
	return file.Close()
}

func ensureCodexHomeConfig(hostRoot, agentID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL BaseURLResolver) error {
	codexHome := filepath.Join(hostRoot, HostCodexHomeDir)
	model = model.Resolved()
	if strings.TrimSpace(model.ModelID) == "" {
		return nil
	}
	baseURL := llmBridgeBaseURL(managerBaseURL(server, resolveBaseURL), agentID)
	if baseURL == "" {
		return nil
	}

	configPath := filepath.Join(codexHome, codexConfigFile)
	if err := os.WriteFile(configPath, []byte(renderCodexConfig(model.ModelID, baseURL)), 0o600); err != nil {
		return fmt.Errorf("write codex sandbox codex config: %w", err)
	}
	catalog, err := json.MarshalIndent(codexmodel.Catalog(codexmodel.Profile{
		ModelID:         strings.TrimSpace(model.ModelID),
		ReasoningEffort: strings.TrimSpace(model.ReasoningEffort),
	}), "", "  ")
	if err != nil {
		return fmt.Errorf("encode codex sandbox model catalog: %w", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, modelCatalogFile), append(catalog, '\n'), 0o600); err != nil {
		return fmt.Errorf("write codex sandbox model catalog: %w", err)
	}
	return nil
}

func renderCodexConfig(modelID, baseURL string) string {
	modelID = strings.TrimSpace(modelID)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if modelID == "" || baseURL == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "model = %s\n", strconv.Quote(modelID))
	fmt.Fprintf(&b, "model_provider = %s\n", strconv.Quote(csgclawProviderID))
	fmt.Fprintf(&b, "model_catalog_json = %s\n\n", strconv.Quote(modelCatalogFile))
	fmt.Fprintf(&b, "[model_providers.%s]\n", csgclawProviderID)
	fmt.Fprintf(&b, "name = %s\n", strconv.Quote("CSGClaw LLM bridge"))
	fmt.Fprintf(&b, "base_url = %s\n", strconv.Quote(baseURL))
	fmt.Fprintf(&b, "wire_api = %s\n", strconv.Quote("responses"))
	b.WriteString("supports_websockets = false\n")
	fmt.Fprintf(&b, "env_key = %s\n", strconv.Quote("OPENAI_API_KEY"))
	return b.String()
}

func RenderConfig(participantID, agentID string, server config.ServerConfig, model config.ModelConfig, resolveBaseURL BaseURLResolver, provider feishu.AgentCredentialProvider) ([]byte, error) {
	participantID = strings.TrimSpace(participantID)
	agentID = strings.TrimSpace(agentID)
	if participantID == "" {
		participantID = agentID
	}
	if agentID == "" {
		agentID = participantID
	}

	appID := ""
	if provider != nil && agentID != "" {
		if _, app, ok := provider.BotConfigForAgent(agentID); ok {
			appID = strings.TrimSpace(app.AppID)
		}
	}

	root := codexSandboxRootConfig{
		SchemaVersion: 2,
		ActiveProfile: ProfileName,
		Preferences:   map[string]any{},
		Migrations: codexSandboxMigrations{
			PermissionDefaultsV1: []string{ProfileName},
		},
		Profiles: map[string]codexSandboxProfileConfig{
			ProfileName: {
				SchemaVersion: 2,
				AgentKind:     "codex",
				Accounts: codexSandboxAccounts{
					App: codexSandboxAppCredentials{
						ID:     appID,
						Secret: "${" + appSecretEnvKey + "}",
						Tenant: "feishu",
					},
				},
				Preferences: map[string]any{
					"messageReply":  "markdown",
					"showToolCalls": true,
				},
				Access: codexSandboxAccess{
					AllowedUsers:          []string{},
					AllowedChats:          []string{},
					Admins:                []string{},
					RequireMentionInGroup: true,
				},
				Workspaces: codexSandboxWorkspaces{Default: BoxProjectsDir},
				Permissions: codexSandboxPermissions{
					DefaultAccess: "full",
					MaxAccess:     "full",
				},
				Codex: codexSandboxCodexConfig{
					BinaryPath:       defaultCodexBinary,
					CodexHome:        BoxCodexHomeDir,
					IgnoreUserConfig: false,
					IgnoreRules:      false,
					InheritCodexHome: false,
				},
				Attachments: codexSandboxAttachments{
					MaxCount:      10,
					MaxBytes:      100 * 1024 * 1024,
					MaxFileBytes:  25 * 1024 * 1024,
					ImageMaxBytes: 25 * 1024 * 1024,
					CacheTTLMS:    24 * 60 * 60 * 1000,
					CacheMaxBytes: 512 * 1024 * 1024,
				},
				Comments: map[string]any{},
				LarkCLI: codexSandboxLarkCLI{
					IdentityPreset: "bot-only",
				},
			},
		},
	}

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode codex sandbox config: %w", err)
	}
	return data, nil
}

func managerBaseURL(server config.ServerConfig, resolveBaseURL BaseURLResolver) string {
	if resolveBaseURL == nil {
		return strings.TrimRight(strings.TrimSpace(server.AdvertiseBaseURL), "/")
	}
	return strings.TrimRight(strings.TrimSpace(resolveBaseURL(server)), "/")
}

func llmBridgeBaseURL(managerBaseURL, agentID string) string {
	managerBaseURL = strings.TrimRight(strings.TrimSpace(managerBaseURL), "/")
	if managerBaseURL == "" || strings.TrimSpace(agentID) == "" {
		return ""
	}
	return managerBaseURL + "/api/v1/agents/" + strings.TrimSpace(agentID) + "/llm"
}

type codexSandboxRootConfig struct {
	SchemaVersion int                                  `json:"schemaVersion"`
	ActiveProfile string                               `json:"activeProfile"`
	Preferences   map[string]any                       `json:"preferences"`
	Migrations    codexSandboxMigrations               `json:"migrations"`
	Profiles      map[string]codexSandboxProfileConfig `json:"profiles"`
}

type codexSandboxMigrations struct {
	PermissionDefaultsV1 []string `json:"permissionDefaultsV1"`
}

type codexSandboxProfileConfig struct {
	SchemaVersion int                     `json:"schemaVersion"`
	AgentKind     string                  `json:"agentKind"`
	Accounts      codexSandboxAccounts    `json:"accounts"`
	Preferences   map[string]any          `json:"preferences"`
	Access        codexSandboxAccess      `json:"access"`
	Workspaces    codexSandboxWorkspaces  `json:"workspaces"`
	Permissions   codexSandboxPermissions `json:"permissions"`
	Codex         codexSandboxCodexConfig `json:"codex"`
	Attachments   codexSandboxAttachments `json:"attachments"`
	Comments      map[string]any          `json:"comments"`
	LarkCLI       codexSandboxLarkCLI     `json:"larkCli"`
}

type codexSandboxAccounts struct {
	App codexSandboxAppCredentials `json:"app"`
}

type codexSandboxAppCredentials struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
	Tenant string `json:"tenant"`
}

type codexSandboxAccess struct {
	AllowedUsers          []string `json:"allowedUsers"`
	AllowedChats          []string `json:"allowedChats"`
	Admins                []string `json:"admins"`
	RequireMentionInGroup bool     `json:"requireMentionInGroup"`
}

type codexSandboxWorkspaces struct {
	Default string `json:"default"`
}

type codexSandboxPermissions struct {
	DefaultAccess string `json:"defaultAccess"`
	MaxAccess     string `json:"maxAccess"`
}

type codexSandboxCodexConfig struct {
	BinaryPath       string `json:"binaryPath"`
	CodexHome        string `json:"codexHome"`
	IgnoreUserConfig bool   `json:"ignoreUserConfig"`
	IgnoreRules      bool   `json:"ignoreRules"`
	InheritCodexHome bool   `json:"inheritCodexHome"`
}

type codexSandboxAttachments struct {
	MaxCount      int `json:"maxCount"`
	MaxBytes      int `json:"maxBytes"`
	MaxFileBytes  int `json:"maxFileBytes"`
	ImageMaxBytes int `json:"imageMaxBytes"`
	CacheTTLMS    int `json:"cacheTtlMs"`
	CacheMaxBytes int `json:"cacheMaxBytes"`
}

type codexSandboxLarkCLI struct {
	IdentityPreset string `json:"identityPreset"`
}
