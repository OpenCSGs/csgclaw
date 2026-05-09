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

func TestLoadUsesDefaultManagerImageWhenOverrideIsEmpty(t *testing.T) {
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
	if got := cfg.Bootstrap.ManagerImageOverride; got != "" {
		t.Fatalf("cfg.Bootstrap.ManagerImageOverride = %q, want empty", got)
	}
	if got, want := cfg.Bootstrap.EffectiveManagerImage(), DefaultManagerImage; got != want {
		t.Fatalf("cfg.Bootstrap.EffectiveManagerImage() = %q, want %q", got, want)
	}
	if got, want := cfg.Server.AccessToken, DefaultAccessToken; got != want {
		t.Fatalf("cfg.Server.AccessToken = %q, want %q", got, want)
	}
	if cfg.Server.NoAuth {
		t.Fatal("cfg.Server.NoAuth = true, want false")
	}
	if got, want := cfg.Sandbox.Provider, DefaultSandboxProvider; got != want {
		t.Fatalf("cfg.Sandbox.Provider = %q, want %q", got, want)
	}
	if got, want := strings.Join(cfg.Sandbox.EffectiveDebianRegistries(), ","), strings.Join(DefaultDebianRegistries, ","); got != want {
		t.Fatalf("cfg.Sandbox.EffectiveDebianRegistries() = %q, want %q", got, want)
	}
	if got, want := cfg.Model.Provider, ProviderLLMAPI; got != want {
		t.Fatalf("cfg.Model.Provider = %q, want %q", got, want)
	}
	if got, want := cfg.Models.Default, "default.minimax-m2.7"; got != want {
		t.Fatalf("cfg.Models.Default = %q, want %q", got, want)
	}
}

func TestLoadReadsSandboxConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[sandbox]
provider = "boxlite"
home_dir_name = "sandbox-home"
debian_registries_override = ["registry.a", " docker.io ", "registry.a"]
storage_path = "/shared/csgclaw"

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
	if got, want := cfg.Sandbox.Provider, BoxLiteCLIProvider; got != want {
		t.Fatalf("cfg.Sandbox.Provider = %q, want %q", got, want)
	}
	if got, want := strings.Join(cfg.Sandbox.DebianRegistriesOverride, ","), "registry.a,docker.io"; got != want {
		t.Fatalf("cfg.Sandbox.DebianRegistriesOverride = %q, want %q", got, want)
	}
	if got, want := strings.Join(cfg.Sandbox.EffectiveDebianRegistries(), ","), "registry.a,docker.io"; got != want {
		t.Fatalf("cfg.Sandbox.EffectiveDebianRegistries() = %q, want %q", got, want)
	}
	if got, want := cfg.Sandbox.StoragePath, "/shared/csgclaw"; got != want {
		t.Fatalf("cfg.Sandbox.StoragePath = %q, want %q", got, want)
	}
}

func TestLoadNormalizesLegacyBoxLiteCLIProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[sandbox]
provider = "boxlite-cli"

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
	if got, want := cfg.Sandbox.Provider, BoxLiteCLIProvider; got != want {
		t.Fatalf("cfg.Sandbox.Provider = %q, want %q", got, want)
	}
}

func TestLoadReadsDockerSandboxConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[sandbox]
provider = "docker"
home_dir_name = "docker-runtime"
docker_cli_path = "/custom/docker"

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
	if got, want := cfg.Sandbox.Provider, DockerProvider; got != want {
		t.Fatalf("cfg.Sandbox.Provider = %q, want %q", got, want)
	}
	if got, want := cfg.Sandbox.DockerCLIPath, "/custom/docker"; got != want {
		t.Fatalf("cfg.Sandbox.DockerCLIPath = %q, want %q", got, want)
	}
	if got, want := cfg.Sandbox.EffectiveDockerCLIPath(), "/custom/docker"; got != want {
		t.Fatalf("EffectiveDockerCLIPath() = %q, want %q", got, want)
	}
}

func TestSandboxEffectiveDockerCLIPathDefault(t *testing.T) {
	cfg := SandboxConfig{Provider: DockerProvider}.Resolved()
	if got, want := cfg.EffectiveDockerCLIPath(), "docker"; got != want {
		t.Fatalf("EffectiveDockerCLIPath() = %q, want %q", got, want)
	}
}

func TestLoadExpandsEnvironmentVariablesInConfigValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	t.Setenv("SANDBOX_PROVIDER", CSGHubProvider)

	content := `[server]
listen_addr = "127.0.0.1:18080"

[sandbox]
provider = "${SANDBOX_PROVIDER}"

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
	if got, want := cfg.Sandbox.Provider, CSGHubProvider; got != want {
		t.Fatalf("cfg.Sandbox.Provider = %q, want %q", got, want)
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

func TestLoadIgnoresLegacyFeishuChannelConfigs(t *testing.T) {
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

	if got := cfg.Channels.FeishuAdminOpenID; got != "" {
		t.Fatalf("feishu admin_open_id = %q, want empty", got)
	}
	if len(cfg.Channels.Feishu) != 0 {
		t.Fatalf("legacy feishu channel config was loaded: %+v", cfg.Channels.Feishu)
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
		Sandbox: SandboxConfig{
			Provider:                 BoxLiteCLIProvider,
			StoragePath:              "/mnt/csgclaw",
			DebianRegistriesOverride: []string{"registry.a", "docker.io"},
		},
		Bootstrap: BootstrapConfig{
			ManagerImageOverride: "img",
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
	if !strings.Contains(content, "no_auth = false") {
		t.Fatalf("saved config missing server no_auth:\n%s", content)
	}
	if !strings.Contains(content, "[models]") || !strings.Contains(content, "[models.providers.default]") {
		t.Fatalf("saved config missing models sections:\n%s", content)
	}
	if !strings.Contains(content, "[sandbox]") || !strings.Contains(content, `provider = "boxlite"`) {
		t.Fatalf("saved config missing sandbox section:\n%s", content)
	}
	if strings.Contains(content, "boxlite_cli_path") {
		t.Fatalf("saved config should not contain boxlite_cli_path:\n%s", content)
	}
	if !strings.Contains(content, `debian_registries_override = ["registry.a", "docker.io"]`) {
		t.Fatalf("saved config missing sandbox debian_registries_override:\n%s", content)
	}
	if !strings.Contains(content, `storage_path = "/mnt/csgclaw"`) {
		t.Fatalf("saved config missing storage_path:\n%s", content)
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
	for _, notWant := range []string{
		"[channels.feishu",
		"admin_open_id",
		"cli_dev",
		"dev-secret",
		"cli_manager",
		"manager-secret",
	} {
		if strings.Contains(content, notWant) {
			t.Fatalf("saved config should not contain feishu channel config %q:\n%s", notWant, content)
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
			ManagerImageOverride: "img",
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

func TestSaveFormatsTopLevelSectionsWithoutExtraWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	models := SingleProfileLLM(ModelConfig{
		BaseURL: "http://127.0.0.1:4000",
		APIKey:  "sk",
		ModelID: "local.minimax-m2.5",
	})
	cfg := Config{
		Server: ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://192.168.2.52:18080",
			AccessToken:      "your_access_token",
			NoAuth:           true,
		},
		Models: models,
		LLM:    models,
		Bootstrap: BootstrapConfig{
			ManagerImageOverride: "ghcr.io/russellluo/picoclaw:2026.4.25",
		},
		Sandbox: SandboxConfig{
			Provider: BoxLiteCLIProvider,
		},
	}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	want := `# Generated by csgclaw.

[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://192.168.2.52:18080"
access_token = "your_access_token"
no_auth = true

[bootstrap]
manager_image_override = "ghcr.io/russellluo/picoclaw:2026.4.25"

[sandbox]
provider = "boxlite"
debian_registries_override = []

[models]
default = "default.local.minimax-m2.5"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["local.minimax-m2.5"]
`
	if got := string(data); got != want {
		t.Fatalf("saved config mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSaveWritesEmptySandboxDebianRegistriesOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg := Config{
		Server: ServerConfig{
			ListenAddr:       "127.0.0.1:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		},
		Sandbox: SandboxConfig{
			Provider: BoxLiteCLIProvider,
		},
		Models: SingleProfileLLM(ModelConfig{
			BaseURL: "http://127.0.0.1:4000",
			APIKey:  "sk",
			ModelID: "minimax-m2.7",
		}),
	}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), `debian_registries_override = []`) {
		t.Fatalf("saved config missing empty sandbox debian_registries_override:\n%s", string(data))
	}
}

func TestSaveRewritesLegacyBoxLiteCLIProviderAfterLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[sandbox]
provider = "boxlite-cli"

[models]
default = "default.gpt-test"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["gpt-test"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	saved := string(data)
	if !strings.Contains(saved, `provider = "boxlite"`) {
		t.Fatalf("saved config missing canonical sandbox provider:\n%s", saved)
	}
	if strings.Contains(saved, `provider = "boxlite-cli"`) {
		t.Fatalf("saved config kept legacy sandbox provider alias:\n%s", saved)
	}
}

func TestLoadExpandsServerEnvValues(t *testing.T) {
	t.Setenv("IP", "1.2.3.4")
	t.Setenv("PORT", "18080")
	t.Setenv("ACCESS_TOKEN", "your_access_token")
	t.Setenv("NO_AUTH", "true")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "0.0.0.0:${PORT}"
advertise_base_url = "http://${IP}:${PORT}"
access_token = "${ACCESS_TOKEN}"
no_auth = "${NO_AUTH}"

[models]
default = "default.gpt-test"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["gpt-test"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.Server.ListenAddr, "0.0.0.0:18080"; got != want {
		t.Fatalf("cfg.Server.ListenAddr = %q, want %q", got, want)
	}
	if got, want := cfg.Server.AdvertiseBaseURL, "http://1.2.3.4:18080"; got != want {
		t.Fatalf("cfg.Server.AdvertiseBaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.Server.AccessToken, "your_access_token"; got != want {
		t.Fatalf("cfg.Server.AccessToken = %q, want %q", got, want)
	}
	if !cfg.Server.NoAuth {
		t.Fatal("cfg.Server.NoAuth = false, want true")
	}
}

func TestLoadExpandsNonServerEnvValues(t *testing.T) {
	t.Setenv("MANAGER_IMAGE", "picoclaw:test")
	t.Setenv("SANDBOX_PROVIDER", BoxLiteCLIProvider)
	t.Setenv("MODEL_SELECTOR", "remote.gpt-env")
	t.Setenv("MODEL_BASE_HOST", "models.example.test")
	t.Setenv("MODEL_API_KEY", "sk-env")
	t.Setenv("MODEL_ID", "gpt-env")
	t.Setenv("REASONING_EFFORT", "HIGH")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[bootstrap]
manager_image_override = "${MANAGER_IMAGE}"

[sandbox]
provider = "${SANDBOX_PROVIDER}"

[models]
default = "${MODEL_SELECTOR}"

[models.providers.remote]
base_url = "https://${MODEL_BASE_HOST}/v1"
api_key = "${MODEL_API_KEY}"
models = ["${MODEL_ID}", "gpt-static"]
reasoning_effort = "${REASONING_EFFORT}"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.Bootstrap.ManagerImageOverride, "picoclaw:test"; got != want {
		t.Fatalf("cfg.Bootstrap.ManagerImageOverride = %q, want %q", got, want)
	}
	if got, want := cfg.Sandbox.Provider, BoxLiteCLIProvider; got != want {
		t.Fatalf("cfg.Sandbox.Provider = %q, want %q", got, want)
	}
	if got, want := cfg.Models.Default, "remote.gpt-env"; got != want {
		t.Fatalf("cfg.Models.Default = %q, want %q", got, want)
	}
	if got, want := cfg.Model.BaseURL, "https://models.example.test/v1"; got != want {
		t.Fatalf("cfg.Model.BaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.Model.APIKey, "sk-env"; got != want {
		t.Fatalf("cfg.Model.APIKey = %q, want %q", got, want)
	}
	if got, want := cfg.Model.ModelID, "gpt-env"; got != want {
		t.Fatalf("cfg.Model.ModelID = %q, want %q", got, want)
	}
	if got, want := cfg.Models.Providers["remote"].ReasoningEffort, "high"; got != want {
		t.Fatalf("cfg.Models.Providers[remote].ReasoningEffort = %q, want %q", got, want)
	}
	if got, want := strings.Join(cfg.Models.Providers["remote"].Models, ","), "gpt-env,gpt-static"; got != want {
		t.Fatalf("cfg.Models.Providers[remote].Models = %q, want %q", got, want)
	}
}

func TestSavePreservesEnvPlaceholdersAfterLoad(t *testing.T) {
	t.Setenv("IP", "1.2.3.4")
	t.Setenv("PORT", "18080")
	t.Setenv("ACCESS_TOKEN", "your_access_token")
	t.Setenv("MANAGER_IMAGE", "picoclaw:test")
	t.Setenv("SANDBOX_PROVIDER", BoxLiteCLIProvider)
	t.Setenv("MODEL_SELECTOR", "remote.gpt-env")
	t.Setenv("MODEL_BASE_HOST", "models.example.test")
	t.Setenv("MODEL_API_KEY", "sk-env")
	t.Setenv("MODEL_ID", "gpt-env")
	t.Setenv("REASONING_EFFORT", "medium")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "0.0.0.0:${PORT}"
advertise_base_url = "http://${IP}:${PORT}"
access_token = "${ACCESS_TOKEN}"

[bootstrap]
manager_image_override = "${MANAGER_IMAGE}"

[sandbox]
provider = "${SANDBOX_PROVIDER}"

[models]
default = "${MODEL_SELECTOR}"

[models.providers.remote]
base_url = "https://${MODEL_BASE_HOST}/v1"
api_key = "${MODEL_API_KEY}"
models = ["${MODEL_ID}", "gpt-static"]
reasoning_effort = "${REASONING_EFFORT}"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	t.Setenv("IP", "5.6.7.8")
	t.Setenv("PORT", "19090")
	t.Setenv("ACCESS_TOKEN", "changed_access_token")
	t.Setenv("MANAGER_IMAGE", "changed-image")
	t.Setenv("MODEL_API_KEY", "changed-model-key")

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	saved := string(data)
	for _, want := range []string{
		`listen_addr = "0.0.0.0:${PORT}"`,
		`advertise_base_url = "http://${IP}:${PORT}"`,
		`access_token = "${ACCESS_TOKEN}"`,
		`manager_image_override = "${MANAGER_IMAGE}"`,
		`provider = "${SANDBOX_PROVIDER}"`,
		`default = "${MODEL_SELECTOR}"`,
		`base_url = "https://${MODEL_BASE_HOST}/v1"`,
		`api_key = "${MODEL_API_KEY}"`,
		`models = ["${MODEL_ID}", "gpt-static"]`,
		`reasoning_effort = "${REASONING_EFFORT}"`,
	} {
		if !strings.Contains(saved, want) {
			t.Fatalf("saved config missing %q:\n%s", want, saved)
		}
	}
	if strings.Contains(saved, "[channels.feishu") {
		t.Fatalf("saved config should not contain feishu channel config:\n%s", saved)
	}
}

func TestSaveUsesResolvedValuesAfterConfigMutation(t *testing.T) {
	t.Setenv("IP", "1.2.3.4")
	t.Setenv("PORT", "18080")
	t.Setenv("ACCESS_TOKEN", "your_access_token")
	t.Setenv("MODEL_ID", "gpt-env")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "0.0.0.0:${PORT}"
advertise_base_url = "http://${IP}:${PORT}"
access_token = "${ACCESS_TOKEN}"

[models]
default = "remote.${MODEL_ID}"

[models.providers.remote]
base_url = "https://models.example.test/v1"
api_key = "sk"
models = ["${MODEL_ID}", "gpt-static"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg.Server.ListenAddr = "0.0.0.0:19090"
	cfg.Server.AdvertiseBaseURL = "http://5.6.7.8:19090"
	cfg.Server.AccessToken = "changed_access_token"
	provider := cfg.Models.Providers["remote"]
	provider.Models = []string{"gpt-changed"}
	cfg.Models.Providers["remote"] = provider
	cfg.Models.Default = "remote.gpt-changed"

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	saved := string(data)
	for _, want := range []string{
		`listen_addr = "0.0.0.0:19090"`,
		`advertise_base_url = "http://5.6.7.8:19090"`,
		`access_token = "changed_access_token"`,
		`default = "remote.gpt-changed"`,
		`models = ["gpt-changed"]`,
	} {
		if !strings.Contains(saved, want) {
			t.Fatalf("saved config missing %q:\n%s", want, saved)
		}
	}
	for _, stale := range []string{
		`${PORT}`,
		`${IP}`,
		`${ACCESS_TOKEN}`,
		`${MODEL_ID}`,
	} {
		if strings.Contains(saved, stale) {
			t.Fatalf("saved config kept stale placeholder %q:\n%s", stale, saved)
		}
	}
}

func TestLoadIgnoresLegacyBoxLiteCLIPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[sandbox]
provider = "boxlite"
home_dir_name = "sandbox-home"
boxlite_cli_path = "/custom/boxlite"

[models]
default = "default.gpt-test"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["gpt-test"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.Sandbox.Provider, BoxLiteCLIProvider; got != want {
		t.Fatalf("cfg.Sandbox.Provider = %q, want %q", got, want)
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
