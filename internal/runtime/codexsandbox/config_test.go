package codexsandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
)

func TestRenderConfigUsesFeishuAppIDAndEnvSecret(t *testing.T) {
	data, err := RenderConfig("dev", "u-dev", config.ServerConfig{
		AccessToken: "shared-token",
	}, config.ModelConfig{
		ModelID: "gpt-5.5",
	}, fixedBaseURL("http://127.0.0.1:18080"), codexSandboxFeishuProvider{
		participantID: "dev",
		app: feishu.AppConfig{
			AppID:     "cli_dev",
			AppSecret: "dev-secret",
		},
	})
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if strings.Contains(string(data), "dev-secret") {
		t.Fatalf("RenderConfig() wrote plaintext app secret:\n%s", data)
	}

	var rendered codexSandboxRootConfig
	if err := json.Unmarshal(data, &rendered); err != nil {
		t.Fatalf("RenderConfig() produced invalid JSON: %v", err)
	}
	profile := rendered.Profiles[ProfileName]
	if got, want := profile.AgentKind, "codex"; got != want {
		t.Fatalf("agentKind = %q, want %q", got, want)
	}
	if got, want := profile.Accounts.App.ID, "cli_dev"; got != want {
		t.Fatalf("accounts.app.id = %q, want %q", got, want)
	}
	if got, want := profile.Accounts.App.Secret, "${APP_SECRET}"; got != want {
		t.Fatalf("accounts.app.secret = %q, want %q", got, want)
	}
	if got, want := profile.Workspaces.Default, BoxProjectsDir; got != want {
		t.Fatalf("workspaces.default = %q, want %q", got, want)
	}
	if got, want := profile.Codex.BinaryPath, defaultCodexBinary; got != want {
		t.Fatalf("codex.binaryPath = %q, want %q", got, want)
	}
	if got, want := profile.Codex.CodexHome, BoxCodexHomeDir; got != want {
		t.Fatalf("codex.codexHome = %q, want %q", got, want)
	}
}

func TestRenderConfigAllowsMissingFeishuProvider(t *testing.T) {
	data, err := RenderConfig("dev", "u-dev", config.ServerConfig{}, config.ModelConfig{}, nil, nil)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	var rendered codexSandboxRootConfig
	if err := json.Unmarshal(data, &rendered); err != nil {
		t.Fatalf("RenderConfig() produced invalid JSON: %v", err)
	}
	profile := rendered.Profiles[ProfileName]
	if got := profile.Accounts.App.ID; got != "" {
		t.Fatalf("accounts.app.id = %q, want empty before Feishu binding", got)
	}
	if got, want := profile.Accounts.App.Secret, "${APP_SECRET}"; got != want {
		t.Fatalf("accounts.app.secret = %q, want %q", got, want)
	}
}

func TestEnsureConfigWritesCodexHomeConfig(t *testing.T) {
	agentHome := t.TempDir()
	_, err := EnsureConfig(agentHome, "dev", "u-dev", config.ServerConfig{
		AccessToken:      "shared-token",
		AdvertiseBaseURL: "http://manager.example",
	}, config.ModelConfig{
		ModelID:         "gpt-5.5",
		ReasoningEffort: "medium",
	}, fixedBaseURL("http://127.0.0.1:18080"), nil)
	if err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}

	configPath := filepath.Join(Root(agentHome), HostCodexHomeDir, codexConfigFile)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		`model = "gpt-5.5"`,
		`model_provider = "csgclaw-llm"`,
		`model_catalog_json = "model_catalog.json"`,
		`base_url = "http://127.0.0.1:18080/api/v1/agents/u-dev/llm"`,
		`wire_api = "responses"`,
		`env_key = "OPENAI_API_KEY"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("config.toml missing %q:\n%s", want, content)
		}
	}

	catalogPath := filepath.Join(Root(agentHome), HostCodexHomeDir, modelCatalogFile)
	catalog, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("ReadFile(model_catalog.json) error = %v", err)
	}
	if !strings.Contains(string(catalog), `"slug": "gpt-5.5"`) {
		t.Fatalf("model catalog missing gpt-5.5:\n%s", catalog)
	}
}

type codexSandboxFeishuProvider struct {
	participantID string
	app           feishu.AppConfig
}

func (p codexSandboxFeishuProvider) BotConfigForAgent(agentID string) (string, feishu.AppConfig, bool) {
	if agentID != "u-dev" {
		return "", feishu.AppConfig{}, false
	}
	return p.participantID, p.app, true
}
