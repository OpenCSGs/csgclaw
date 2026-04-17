package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultDirUsesSharedAppDirName(t *testing.T) {
	dir, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir() error = %v", err)
	}

	if got, want := filepath.Base(dir), AppDirName; got != want {
		t.Fatalf("filepath.Base(DefaultDir()) = %q, want %q", got, want)
	}
}

func TestDefaultAgentsPathUsesDomainSubdirectory(t *testing.T) {
	path, err := DefaultAgentsPath()
	if err != nil {
		t.Fatalf("DefaultAgentsPath() error = %v", err)
	}

	if got, want := filepath.Base(path), StateFileName; got != want {
		t.Fatalf("filepath.Base(DefaultAgentsPath()) = %q, want %q", got, want)
	}
	if got, want := filepath.Base(filepath.Dir(path)), AgentsDirName; got != want {
		t.Fatalf("filepath.Base(filepath.Dir(DefaultAgentsPath())) = %q, want %q", got, want)
	}
}

func TestDefaultIMStatePathUsesDomainSubdirectory(t *testing.T) {
	path, err := DefaultIMStatePath()
	if err != nil {
		t.Fatalf("DefaultIMStatePath() error = %v", err)
	}

	if got, want := filepath.Base(path), StateFileName; got != want {
		t.Fatalf("filepath.Base(DefaultIMStatePath()) = %q, want %q", got, want)
	}
	if got, want := filepath.Base(filepath.Dir(path)), IMDirName; got != want {
		t.Fatalf("filepath.Base(filepath.Dir(DefaultIMStatePath())) = %q, want %q", got, want)
	}
}

func TestLoadAppliesDefaultManagerImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
advertise_base_url = "http://127.0.0.1:18080"

[models]
default = "default.minimax-m2.7"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["minimax-m2.7"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.Bootstrap.ManagerImage, DefaultManagerImage; got != want {
		t.Fatalf("cfg.Bootstrap.ManagerImage = %q, want %q", got, want)
	}
	if got, want := cfg.Server.AccessToken, DefaultAccessToken; got != want {
		t.Fatalf("cfg.Server.AccessToken = %q, want %q", got, want)
	}
	if got, want := cfg.Model.Provider, ProviderLLMAPI; got != want {
		t.Fatalf("cfg.Model.Provider = %q, want %q", got, want)
	}
	if got, want := cfg.Models.Default, "default.minimax-m2.7"; got != want {
		t.Fatalf("cfg.Models.Default = %q, want %q", got, want)
	}
}

func TestLoadReadsModelsProviderPool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[models]
default = "remote.gpt-5.4"

[models.providers.remote]
base_url = "https://example.test/v1"
api_key = "sk-test"
models = ["gpt-5.4", "gpt-5.4-mini"]
reasoning_effort = "medium"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.Models.Default, "remote.gpt-5.4"; got != want {
		t.Fatalf("cfg.Models.Default = %q, want %q", got, want)
	}
	if got, want := cfg.Model.ModelID, "gpt-5.4"; got != want {
		t.Fatalf("cfg.Model.ModelID = %q, want %q", got, want)
	}
	if got, want := cfg.Models.Providers["remote"].BaseURL, "https://example.test/v1"; got != want {
		t.Fatalf("cfg.Models.Providers[remote].BaseURL = %q, want %q", got, want)
	}
	if got, want := strings.Join(cfg.Models.Providers["remote"].Models, ","), "gpt-5.4,gpt-5.4-mini"; got != want {
		t.Fatalf("cfg.Models.Providers[remote].Models = %q, want %q", got, want)
	}
	if got, want := cfg.Models.Providers["remote"].ReasoningEffort, "medium"; got != want {
		t.Fatalf("cfg.Models.Providers[remote].ReasoningEffort = %q, want %q", got, want)
	}
}

func TestLoadReadsCSGHubLiteProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[models]
default = "csghub-lite.Qwen/Qwen3-0.6B-GGUF"

[models.providers.csghub-lite]
base_url = "http://127.0.0.1:11435/v1"
api_key = "local"
models = ["Qwen/Qwen3-0.6B-GGUF", "Qwen/Qwen3-1.7B-GGUF"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.Models.Default, "csghub-lite.Qwen/Qwen3-0.6B-GGUF"; got != want {
		t.Fatalf("cfg.Models.Default = %q, want %q", got, want)
	}
	if got, want := cfg.Model.BaseURL, "http://127.0.0.1:11435/v1"; got != want {
		t.Fatalf("cfg.Model.BaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.Model.APIKey, "local"; got != want {
		t.Fatalf("cfg.Model.APIKey = %q, want %q", got, want)
	}
	if got, want := cfg.Model.ModelID, "Qwen/Qwen3-0.6B-GGUF"; got != want {
		t.Fatalf("cfg.Model.ModelID = %q, want %q", got, want)
	}
	if got, want := strings.Join(cfg.Models.Providers["csghub-lite"].Models, ","), "Qwen/Qwen3-0.6B-GGUF,Qwen/Qwen3-1.7B-GGUF"; got != want {
		t.Fatalf("csghub-lite models = %q, want %q", got, want)
	}
}

func TestLoadRejectsLegacyLLMSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[llm]
default_profile = "remote-main"

[llm.profiles.remote-main]
provider = "llm-api"
base_url = "https://example.test/v1"
api_key = "sk-test"
model_id = "gpt-5.4"
reasoning_effort = "medium"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want legacy [llm] rejection")
	}
	if !strings.Contains(err.Error(), "legacy config section [llm] is no longer supported") {
		t.Fatalf("Load() error = %q, want legacy [llm] rejection", err)
	}
}

func TestLoadSupportsNamedFeishuChannelConfigs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
advertise_base_url = "http://127.0.0.1:18080"

[models]
default = "default.minimax-m2.7"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["minimax-m2.7"]

[channels.feishu]
admin_open_id = "ou_admin"

[channels.feishu.manager]
app_id = "cli_manager"
app_secret = "manager-secret"

[channels.feishu.dev]
app_id = "cli_dev"
app_secret = "dev-secret"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.Channels.Feishu["manager"].AppID, "cli_manager"; got != want {
		t.Fatalf("manager app_id = %q, want %q", got, want)
	}
	if got, want := cfg.Channels.FeishuAdminOpenID, "ou_admin"; got != want {
		t.Fatalf("feishu admin_open_id = %q, want %q", got, want)
	}
	if got, want := cfg.Channels.Feishu["manager"].AppSecret, "manager-secret"; got != want {
		t.Fatalf("manager app_secret = %q, want %q", got, want)
	}
	if got, want := cfg.Channels.Feishu["dev"].AppID, "cli_dev"; got != want {
		t.Fatalf("dev app_id = %q, want %q", got, want)
	}
	if got, want := cfg.Channels.Feishu["dev"].AppSecret, "dev-secret"; got != want {
		t.Fatalf("dev app_secret = %q, want %q", got, want)
	}
}

func TestSaveWritesModelsSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	models := SingleProfileLLM(ModelConfig{
		BaseURL: "http://127.0.0.1:4000",
		APIKey:  "sk",
		ModelID: "minimax-m2.7",
	})
	cfg := Config{
		Server: ServerConfig{
			ListenAddr:       "127.0.0.1:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		},
		Models: models,
		LLM:    models,
		Bootstrap: BootstrapConfig{
			ManagerImage: "img",
		},
		Channels: ChannelsConfig{
			FeishuAdminOpenID: "ou_admin",
			Feishu: map[string]FeishuConfig{
				"manager": {
					AppID:     "cli_manager",
					AppSecret: "manager-secret",
				},
				"dev": {
					AppID:     "cli_dev",
					AppSecret: "dev-secret",
				},
			},
		},
	}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "access_token = \"shared-token\"") {
		t.Fatalf("saved config missing server access token:\n%s", content)
	}
	if !strings.Contains(content, "[models]") || !strings.Contains(content, "[models.providers.default]") {
		t.Fatalf("saved config missing models sections:\n%s", content)
	}
	if !strings.Contains(content, `default = "default.minimax-m2.7"`) {
		t.Fatalf("saved config missing canonical models.default:\n%s", content)
	}
	if !strings.Contains(content, `models = ["minimax-m2.7"]`) {
		t.Fatalf("saved config missing models array:\n%s", content)
	}
	if strings.Contains(content, "[llm]") || strings.Contains(content, "model_id = ") {
		t.Fatalf("saved config should not contain legacy llm/profile keys:\n%s", content)
	}
	for _, want := range []string{
		"[channels.feishu.dev]",
		`admin_open_id = "ou_admin"`,
		`app_id = "cli_dev"`,
		`app_secret = "dev-secret"`,
		"[channels.feishu.manager]",
		`app_id = "cli_manager"`,
		`app_secret = "manager-secret"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("saved config missing %q:\n%s", want, content)
		}
	}
}

func TestSaveWritesCSGHubLiteProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	models := LLMConfig{
		Default:        "csghub-lite.Qwen/Qwen3-0.6B-GGUF",
		DefaultProfile: "csghub-lite.Qwen/Qwen3-0.6B-GGUF",
		Providers: map[string]ProviderConfig{
			"csghub-lite": {
				BaseURL: "http://127.0.0.1:11435/v1",
				APIKey:  "local",
				Models:  []string{"Qwen/Qwen3-0.6B-GGUF", "Qwen/Qwen3-1.7B-GGUF"},
			},
		},
	}
	cfg := Config{
		Server: ServerConfig{
			ListenAddr:  "127.0.0.1:18080",
			AccessToken: "shared-token",
		},
		Models: models,
		LLM:    models,
		Bootstrap: BootstrapConfig{
			ManagerImage: "img",
		},
	}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		`default = "csghub-lite.Qwen/Qwen3-0.6B-GGUF"`,
		`[models.providers.csghub-lite]`,
		`base_url = "http://127.0.0.1:11435/v1"`,
		`api_key = "local"`,
		`models = ["Qwen/Qwen3-0.6B-GGUF", "Qwen/Qwen3-1.7B-GGUF"]`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("saved config missing %q:\n%s", want, content)
		}
	}
}

func TestLLMConfigMissingFields(t *testing.T) {
	missing := (ModelConfig{}).MissingFields()
	got := strings.Join(missing, ",")
	want := "base_url,api_key,model_id"
	if got != want {
		t.Fatalf("MissingFields() = %q, want %q", got, want)
	}
}

func TestValidateRejectsUnsupportedProvider(t *testing.T) {
	err := (ModelConfig{
		Provider: "local-codex",
		ModelID:  "gpt-5.4",
	}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want unsupported provider rejection")
	}
	if !strings.Contains(err.Error(), "only \"llm-api\" is supported now") {
		t.Fatalf("Validate() error = %q, want unsupported provider rejection", err)
	}
}
