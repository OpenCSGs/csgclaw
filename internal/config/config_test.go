package config

import (
	"encoding/json"
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

func TestDefaultAgentsPathUsesRootStateFile(t *testing.T) {
	path, err := DefaultAgentsPath()
	if err != nil {
		t.Fatalf("DefaultAgentsPath() error = %v", err)
	}

	if got, want := filepath.Base(path), StateFileName; got != want {
		t.Fatalf("filepath.Base(DefaultAgentsPath()) = %q, want %q", got, want)
	}
	if got, want := filepath.Base(filepath.Dir(path)), AppDirName; got != want {
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

func TestDefaultTeamsDirUsesDomainSubdirectory(t *testing.T) {
	dir, err := DefaultTeamsDir()
	if err != nil {
		t.Fatalf("DefaultTeamsDir() error = %v", err)
	}

	if got, want := filepath.Base(dir), "teams"; got != want {
		t.Fatalf("filepath.Base(DefaultTeamsDir()) = %q, want %q", got, want)
	}
}

func TestLoadUsesDefaultBootstrapTemplatesWhenSectionIsEmpty(t *testing.T) {
	restore := stubSandboxProviderExecutablePath(t, filepath.Join(t.TempDir(), "bin", "csgclaw"))
	defer restore()

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
	if got, want := cfg.Bootstrap.ResolvedDefaultManagerTemplate(), DefaultBootstrapManagerTemplate; got != want {
		t.Fatalf("cfg.Bootstrap.ResolvedDefaultManagerTemplate() = %q, want %q", got, want)
	}
	if got, want := cfg.Bootstrap.ResolvedDefaultWorkerTemplate(), DefaultBootstrapWorkerTemplate; got != want {
		t.Fatalf("cfg.Bootstrap.ResolvedDefaultWorkerTemplate() = %q, want %q", got, want)
	}
	if got, want := cfg.Server.AccessToken, DefaultAccessToken; got != want {
		t.Fatalf("cfg.Server.AccessToken = %q, want %q", got, want)
	}
	if cfg.Server.NoAuth {
		t.Fatal("cfg.Server.NoAuth = true, want false")
	}
	if !cfg.Server.ShowUpgrade {
		t.Fatal("cfg.Server.ShowUpgrade = false, want true")
	}
	if got, want := cfg.Sandbox.Provider, DockerProvider; got != want {
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

func TestLoadReadsServerShowUpgrade(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
show_upgrade = false

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
	if cfg.Server.ShowUpgrade {
		t.Fatal("cfg.Server.ShowUpgrade = true, want false")
	}
}

func TestLoadSaveModelsProviderDisplayNameAndHeadersJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[models]
default = "openai.gpt-4.1"

[models.providers.openai]
display_name = "Team OpenAI"
base_url = "https://api.openai.example/v1"
api_key = "sk-team"
headers_json = "{\"X-CSG-Trace\":\"dev\",\"X-Org\":\"eng\"}"
models = ["gpt-4.1", "gpt-4o-mini"]
reasoning_effort = "high"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	provider := cfg.Models.Providers["openai"]
	if got, want := provider.DisplayName, "Team OpenAI"; got != want {
		t.Fatalf("DisplayName = %q, want %q", got, want)
	}
	if got, want := provider.ReasoningEffort, "high"; got != want {
		t.Fatalf("ReasoningEffort = %q, want %q while loading legacy config", got, want)
	}
	if got, want := cfg.Model.ReasoningEffort, "high"; got != want {
		t.Fatalf("cfg.Model.ReasoningEffort = %q, want %q while loading legacy config", got, want)
	}
	if got, want := provider.Headers["X-CSG-Trace"], "dev"; got != want {
		t.Fatalf("Headers[X-CSG-Trace] = %q, want %q", got, want)
	}
	if got, want := provider.Headers["X-Org"], "eng"; got != want {
		t.Fatalf("Headers[X-Org] = %q, want %q", got, want)
	}

	savedPath := filepath.Join(dir, "saved.toml")
	if err := cfg.Save(savedPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	saved, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("ReadFile(saved) error = %v", err)
	}
	savedText := string(saved)
	if strings.Contains(savedText, "[models]") || strings.Contains(savedText, "[models.providers.openai]") {
		t.Fatalf("saved config should not contain model provider sections:\n%s", savedText)
	}

	modelsPath, err := ModelsPathForConfigPath(savedPath)
	if err != nil {
		t.Fatalf("ModelsPathForConfigPath() error = %v", err)
	}
	savedModels, ok, err := LoadModels(modelsPath)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadModels() ok = false, want true")
	}
	savedProvider := savedModels.Providers["openai"]
	if got, want := savedProvider.DisplayName, "Team OpenAI"; got != want {
		t.Fatalf("saved DisplayName = %q, want %q", got, want)
	}
	if got, want := savedProvider.Headers["X-CSG-Trace"], "dev"; got != want {
		t.Fatalf("saved Headers[X-CSG-Trace] = %q, want %q", got, want)
	}
	if got, want := savedProvider.Headers["X-Org"], "eng"; got != want {
		t.Fatalf("saved Headers[X-Org] = %q, want %q", got, want)
	}
	if got := savedProvider.ReasoningEffort; got != "" {
		t.Fatalf("saved ReasoningEffort = %q, want empty because reasoning is stored on agent profiles", got)
	}

	reloaded, err := Load(savedPath)
	if err != nil {
		t.Fatalf("Load(saved) error = %v", err)
	}
	if got, want := reloaded.Models.Providers["openai"].Headers["X-Org"], "eng"; got != want {
		t.Fatalf("reloaded Headers[X-Org] = %q, want %q", got, want)
	}
	if got := reloaded.Models.Providers["openai"].ReasoningEffort; got != "" {
		t.Fatalf("reloaded ReasoningEffort = %q, want empty from models.json", got)
	}
}

func TestSaveModelsRootStateOmitsDefaultModel(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, StateFileName)
	llm := LLMConfig{
		Default:        "openai.gpt-4.1",
		DefaultProfile: "openai.gpt-4.1",
		Providers: map[string]ProviderConfig{
			"openai": {
				DisplayName: "Team OpenAI",
				BaseURL:     "https://api.openai.example/v1",
				APIKey:      "sk-team",
				Models:      []string{"gpt-4.1"},
			},
		},
	}

	if err := SaveModels(statePath, llm); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}

	var state map[string]any
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(state) error = %v", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	modelProviders := state["model_providers"].(map[string]any)
	if _, ok := modelProviders["default_model"]; ok {
		t.Fatalf("model_providers.default_model persisted: %#v", modelProviders)
	}
	if _, ok := modelProviders["items"].(map[string]any)["openai"]; !ok {
		t.Fatalf("model_providers.items.openai missing: %#v", modelProviders)
	}
}

func TestBootstrapValidateUsesDefaultTemplatesWhenUnset(t *testing.T) {
	cfg := BootstrapConfig{
		DefaultManagerTemplate: "",
		DefaultWorkerTemplate:  "",
	}

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestBootstrapResolvedDefaults(t *testing.T) {
	cfg := BootstrapConfig{
		DefaultManagerTemplate: "local.review-manager",
		DefaultWorkerTemplate:  "local.review-worker",
	}
	if got, want := cfg.ResolvedDefaultManagerTemplate(), "local.review-manager"; got != want {
		t.Fatalf("ResolvedDefaultManagerTemplate() = %q, want %q", got, want)
	}
	if got, want := cfg.ResolvedDefaultWorkerTemplate(), "local.review-worker"; got != want {
		t.Fatalf("ResolvedDefaultWorkerTemplate() = %q, want %q", got, want)
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
	if got, want := cfg.Sandbox.Provider, BoxLiteProvider; got != want {
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

func TestLoadUsesBuiltinHubRegistryWhenSectionMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

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
	if got, want := cfg.Hub.DefaultRegistry, DefaultHubRegistry; got != want {
		t.Fatalf("cfg.Hub.DefaultRegistry = %q, want %q", got, want)
	}
	if got, want := cfg.Hub.DefaultPublishRegistry, DefaultHubPublishRegistry; got != want {
		t.Fatalf("cfg.Hub.DefaultPublishRegistry = %q, want %q", got, want)
	}
	if got, want := cfg.Bootstrap.ResolvedDefaultManagerTemplate(), DefaultBootstrapManagerTemplate; got != want {
		t.Fatalf("cfg.Bootstrap.ResolvedDefaultManagerTemplate() = %q, want %q", got, want)
	}
	if got, want := cfg.Bootstrap.ResolvedDefaultWorkerTemplate(), DefaultBootstrapWorkerTemplate; got != want {
		t.Fatalf("cfg.Bootstrap.ResolvedDefaultWorkerTemplate() = %q, want %q", got, want)
	}
	if got, want := len(cfg.Hub.Registries), 3; got != want {
		t.Fatalf("len(cfg.Hub.Registries) = %d, want %d", got, want)
	}
	registry := cfg.Hub.Registries[0]
	if got, want := registry.Name, DefaultHubRegistry; got != want {
		t.Fatalf("cfg.Hub.Registries[0].Name = %q, want %q", got, want)
	}
	if got, want := registry.Kind, HubRegistryKindBuiltin; got != want {
		t.Fatalf("cfg.Hub.Registries[0].Kind = %q, want %q", got, want)
	}
	if !registry.Enabled {
		t.Fatal("cfg.Hub.Registries[0].Enabled = false, want true")
	}
	localRegistry := cfg.Hub.Registries[1]
	if got, want := localRegistry.Name, DefaultHubPublishRegistry; got != want {
		t.Fatalf("cfg.Hub.Registries[1].Name = %q, want %q", got, want)
	}
	if got, want := localRegistry.Kind, HubRegistryKindLocal; got != want {
		t.Fatalf("cfg.Hub.Registries[1].Kind = %q, want %q", got, want)
	}
	if got, want := localRegistry.Path, filepath.Join(dir, AppDirName, HubDirName); got != want {
		t.Fatalf("cfg.Hub.Registries[1].Path = %q, want %q", got, want)
	}
	if !localRegistry.Enabled {
		t.Fatal("cfg.Hub.Registries[1].Enabled = false, want true")
	}
	officialRegistry := cfg.Hub.Registries[2]
	if got, want := officialRegistry.Name, DefaultOfficialHubRegistryName; got != want {
		t.Fatalf("cfg.Hub.Registries[2].Name = %q, want %q", got, want)
	}
	if got, want := officialRegistry.Kind, HubRegistryKindRemote; got != want {
		t.Fatalf("cfg.Hub.Registries[2].Kind = %q, want %q", got, want)
	}
	if got, want := officialRegistry.URL, DefaultOfficialHubRegistryURL; got != want {
		t.Fatalf("cfg.Hub.Registries[2].URL = %q, want %q", got, want)
	}
	if !officialRegistry.Enabled {
		t.Fatal("cfg.Hub.Registries[2].Enabled = false, want true")
	}
}

func TestLoadReadsHubConfig(t *testing.T) {
	t.Setenv("CSGCLAW_HUB_TOKEN", "hub-secret")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[hub]
default_registry = "team"
default_publish_registry = "local"

[bootstrap]
default_manager_template = "builtin.picoclaw-manager"
default_worker_template = "builtin.openclaw-worker"

[[hub.registries]]
name = "builtin"
kind = "builtin"

[[hub.registries]]
name = "local"
kind = "local"
path = "/tmp/hub"
enabled = true

[[hub.registries]]
name = "team"
kind = "remote"
url = "https://hub.example.com/"
token = "${CSGCLAW_HUB_TOKEN}"
enabled = false

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
	if got, want := cfg.Hub.DefaultRegistry, "team"; got != want {
		t.Fatalf("cfg.Hub.DefaultRegistry = %q, want %q", got, want)
	}
	if got, want := cfg.Hub.DefaultPublishRegistry, "local"; got != want {
		t.Fatalf("cfg.Hub.DefaultPublishRegistry = %q, want %q", got, want)
	}
	if got, want := cfg.Bootstrap.ResolvedDefaultManagerTemplate(), "builtin.picoclaw-manager"; got != want {
		t.Fatalf("cfg.Bootstrap.ResolvedDefaultManagerTemplate() = %q, want %q", got, want)
	}
	if got, want := cfg.Bootstrap.ResolvedDefaultWorkerTemplate(), "builtin.openclaw-worker"; got != want {
		t.Fatalf("cfg.Bootstrap.ResolvedDefaultWorkerTemplate() = %q, want %q", got, want)
	}
	if got, want := len(cfg.Hub.Registries), 4; got != want {
		t.Fatalf("len(cfg.Hub.Registries) = %d, want %d", got, want)
	}
	if got, want := cfg.Hub.Registries[0].Enabled, true; got != want {
		t.Fatalf("cfg.Hub.Registries[0].Enabled = %t, want %t", got, want)
	}
	if got, want := cfg.Hub.Registries[1].Path, "/tmp/hub"; got != want {
		t.Fatalf("cfg.Hub.Registries[1].Path = %q, want %q", got, want)
	}
	if got, want := cfg.Hub.Registries[2].URL, DefaultOfficialHubRegistryURL; got != want {
		t.Fatalf("cfg.Hub.Registries[2].URL = %q, want %q", got, want)
	}
	if got, want := cfg.Hub.Registries[2].Enabled, true; got != want {
		t.Fatalf("cfg.Hub.Registries[2].Enabled = %t, want %t", got, want)
	}
	if got, want := cfg.Hub.Registries[3].URL, "https://hub.example.com"; got != want {
		t.Fatalf("cfg.Hub.Registries[3].URL = %q, want %q", got, want)
	}
	if got, want := cfg.Hub.Registries[3].Token, "hub-secret"; got != want {
		t.Fatalf("cfg.Hub.Registries[3].Token = %q, want %q", got, want)
	}
	if got, want := cfg.Hub.Registries[3].Enabled, false; got != want {
		t.Fatalf("cfg.Hub.Registries[3].Enabled = %t, want %t", got, want)
	}
}

func TestLoadLegacyOfficialHubRegistryURLAndSaveMigratesIt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[bootstrap]
default_manager_template = "builtin.picoclaw-manager"
default_worker_template = "builtin.picoclaw-worker"

[[hub.registries]]
name = "official"
kind = "remote"
url = "https://csgclaw.opencsg.com"
enabled = true
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	officialRegistry := cfg.Hub.Registries[2]
	if got, want := officialRegistry.URL, DefaultOfficialHubRegistryURL; got != want {
		t.Fatalf("official registry URL = %q, want %q", got, want)
	}
	if !cfg.NeedsMigrationRewrite() {
		t.Fatal("NeedsMigrationRewrite() = false, want true")
	}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	saved := string(data)
	if strings.Contains(saved, LegacyOfficialHubRegistryURL) {
		t.Fatalf("saved config still contains legacy official URL:\n%s", saved)
	}
	if !strings.Contains(saved, `url = "https://hub.opencsg.com"`) {
		t.Fatalf("saved config missing migrated official URL:\n%s", saved)
	}
}

func TestLoadCustomRemoteHubRegistryURLAndSavePreservesIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[bootstrap]
default_manager_template = "builtin.picoclaw-manager"
default_worker_template = "builtin.picoclaw-worker"

[[hub.registries]]
name = "team"
kind = "remote"
url = "https://hub.example.com/"
enabled = true
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	registry := cfg.Hub.Registries[3]
	if got, want := registry.URL, "https://hub.example.com"; got != want {
		t.Fatalf("custom remote registry URL = %q, want %q", got, want)
	}
	if cfg.NeedsMigrationRewrite() {
		t.Fatal("NeedsMigrationRewrite() = true, want false")
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	saved := string(data)
	if !strings.Contains(saved, `name = "team"`+"\n"+`kind = "remote"`+"\n"+`url = "https://hub.example.com/"`) {
		t.Fatalf("saved config missing custom remote URL in team registry:\n%s", saved)
	}
}

func TestLoadLegacyBootstrapTemplateRefsAndSaveMigratesThem(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[bootstrap]
default_manager_template = "builtin/picoclaw-manager"
default_worker_template = "local/review-worker"

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
	if got, want := cfg.Bootstrap.DefaultManagerTemplate, "builtin.picoclaw-manager"; got != want {
		t.Fatalf("cfg.Bootstrap.DefaultManagerTemplate = %q, want %q", got, want)
	}
	if got, want := cfg.Bootstrap.DefaultWorkerTemplate, "local.review-worker"; got != want {
		t.Fatalf("cfg.Bootstrap.DefaultWorkerTemplate = %q, want %q", got, want)
	}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	saved := string(data)
	if strings.Contains(saved, "builtin/picoclaw-manager") || strings.Contains(saved, "local/review-worker") {
		t.Fatalf("saved config should not contain legacy slash template refs:\n%s", saved)
	}
	for _, want := range []string{
		`default_manager_template = "builtin.picoclaw-manager"`,
		`default_worker_template = "local.review-worker"`,
	} {
		if !strings.Contains(saved, want) {
			t.Fatalf("saved config missing %q:\n%s", want, saved)
		}
	}
}

func TestLoadAddsDefaultLocalPublishRegistryWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[hub]
default_registry = "builtin"
default_publish_registry = "local"

[[hub.registries]]
name = "builtin"
kind = "builtin"
enabled = true

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
	if got, want := len(cfg.Hub.Registries), 3; got != want {
		t.Fatalf("len(cfg.Hub.Registries) = %d, want %d", got, want)
	}
	localRegistry := cfg.Hub.Registries[1]
	if got, want := localRegistry.Name, "local"; got != want {
		t.Fatalf("cfg.Hub.Registries[1].Name = %q, want %q", got, want)
	}
	if got, want := localRegistry.Kind, HubRegistryKindLocal; got != want {
		t.Fatalf("cfg.Hub.Registries[1].Kind = %q, want %q", got, want)
	}
	if got, want := localRegistry.Path, filepath.Join(dir, AppDirName, HubDirName); got != want {
		t.Fatalf("cfg.Hub.Registries[1].Path = %q, want %q", got, want)
	}
	if !localRegistry.Enabled {
		t.Fatal("cfg.Hub.Registries[1].Enabled = false, want true")
	}
	officialRegistry := cfg.Hub.Registries[2]
	if got, want := officialRegistry.Name, DefaultOfficialHubRegistryName; got != want {
		t.Fatalf("cfg.Hub.Registries[2].Name = %q, want %q", got, want)
	}
	if got, want := officialRegistry.URL, DefaultOfficialHubRegistryURL; got != want {
		t.Fatalf("cfg.Hub.Registries[2].URL = %q, want %q", got, want)
	}
}

func TestLoadRejectsRemovedLegacyBoxLiteProvider(t *testing.T) {
	restore := stubSandboxProviderExecutablePath(t, filepath.Join(t.TempDir(), "bin", "csgclaw"))
	defer restore()

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

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want unsupported sandbox provider error")
	}
	if !strings.Contains(err.Error(), `unsupported sandbox provider "boxlite-cli"`) {
		t.Fatalf("Load() error = %q, want unsupported legacy provider error", err)
	}
}

func TestLoadReadsDockerSandboxConfig(t *testing.T) {
	restore := stubSandboxProviderExecutablePath(t, filepath.Join(t.TempDir(), "bin", "csgclaw"))
	defer restore()

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

func TestLoadUsesBundledBoxLiteWhenSandboxProviderUnset(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bundle", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "boxlite"), []byte(""), 0o755); err != nil {
		t.Fatalf("WriteFile(boxlite) error = %v", err)
	}
	restore := stubSandboxProviderExecutablePath(t, filepath.Join(binDir, "csgclaw"))
	defer restore()

	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

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
	if got, want := cfg.Sandbox.Provider, BoxLiteProvider; got != want {
		t.Fatalf("cfg.Sandbox.Provider = %q, want %q", got, want)
	}
}

func TestLoadKeepsExplicitDockerProviderWhenBundledBoxLiteIsPresent(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bundle", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "boxlite"), []byte(""), 0o755); err != nil {
		t.Fatalf("WriteFile(boxlite) error = %v", err)
	}
	restore := stubSandboxProviderExecutablePath(t, filepath.Join(binDir, "csgclaw"))
	defer restore()

	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[sandbox]
provider = "docker"

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
	if got := cfg.Server.ListenAddr; got != "127.0.0.1:18080" {
		t.Fatalf("cfg.Server.ListenAddr = %q, want %q", got, "127.0.0.1:18080")
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
			ShowUpgrade:      true,
		},
		Models: models,
		LLM:    models,
		Sandbox: SandboxConfig{
			Provider:                 BoxLiteProvider,
			StoragePath:              "/mnt/csgclaw",
			DebianRegistriesOverride: []string{"registry.a", "docker.io"},
		},
		Bootstrap: BootstrapConfig{
			DefaultManagerTemplate: "local.review-manager",
			DefaultWorkerTemplate:  "local.review-worker",
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
	if !strings.Contains(content, "show_upgrade = true") {
		t.Fatalf("saved config missing server show_upgrade:\n%s", content)
	}
	if strings.Contains(content, "[models]") || strings.Contains(content, "[models.providers.default]") {
		t.Fatalf("saved config should not contain models sections:\n%s", content)
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
	modelsPath, err := ModelsPathForConfigPath(path)
	if err != nil {
		t.Fatalf("ModelsPathForConfigPath() error = %v", err)
	}
	savedModels, ok, err := LoadModels(modelsPath)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadModels() ok = false, want true")
	}
	if got := savedModels.Default; got != "" {
		t.Fatalf("saved models default = %q, want empty because agents.profile_defaults owns defaults", got)
	}
	if got, want := strings.Join(savedModels.Providers["default"].Models, ","), "minimax-m2.7"; got != want {
		t.Fatalf("saved models provider models = %q, want %q", got, want)
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
			ShowUpgrade: true,
		},
		Models: models,
		LLM:    models,
		Bootstrap: BootstrapConfig{
			DefaultManagerTemplate: "local.review-manager",
			DefaultWorkerTemplate:  "local.review-worker",
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
	if strings.Contains(content, "[models]") || strings.Contains(content, "[models.providers.csghub-lite]") {
		t.Fatalf("saved config should not contain models sections:\n%s", content)
	}
	modelsPath, err := ModelsPathForConfigPath(path)
	if err != nil {
		t.Fatalf("ModelsPathForConfigPath() error = %v", err)
	}
	savedModels, ok, err := LoadModels(modelsPath)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadModels() ok = false, want true")
	}
	provider := savedModels.Providers["csghub-lite"]
	if got := savedModels.Default; got != "" {
		t.Fatalf("saved models default = %q, want empty because agents.profile_defaults owns defaults", got)
	}
	if got, want := provider.BaseURL, "http://127.0.0.1:11435/v1"; got != want {
		t.Fatalf("saved BaseURL = %q, want %q", got, want)
	}
	if got, want := provider.APIKey, "local"; got != want {
		t.Fatalf("saved APIKey = %q, want %q", got, want)
	}
	if got, want := strings.Join(provider.Models, ","), "Qwen/Qwen3-0.6B-GGUF,Qwen/Qwen3-1.7B-GGUF"; got != want {
		t.Fatalf("saved Models = %q, want %q", got, want)
	}
}

func TestSaveFormatsTopLevelSectionsWithoutExtraWhitespace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
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
			ShowUpgrade:      true,
		},
		Models: models,
		LLM:    models,
		Bootstrap: BootstrapConfig{
			DefaultManagerTemplate: "builtin.picoclaw-manager",
			DefaultWorkerTemplate:  "builtin.picoclaw-worker",
		},
		Sandbox: SandboxConfig{
			Provider: BoxLiteProvider,
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
show_upgrade = true

[bootstrap]
default_manager_template = "builtin.picoclaw-manager"
default_worker_template = "builtin.picoclaw-worker"

[sandbox]
provider = "boxlite"
debian_registries_override = []

[hub]
default_registry = "builtin"
default_publish_registry = "local"

[[hub.registries]]
name = "builtin"
kind = "builtin"
enabled = true

[[hub.registries]]
name = "local"
kind = "local"
path = "` + filepath.Join(dir, AppDirName, HubDirName) + `"
enabled = true

[[hub.registries]]
name = "official"
kind = "remote"
url = "https://hub.opencsg.com"
enabled = true
`
	if got := string(data); got != want {
		t.Fatalf("saved config mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	modelsPath, err := ModelsPathForConfigPath(path)
	if err != nil {
		t.Fatalf("ModelsPathForConfigPath() error = %v", err)
	}
	savedModels, ok, err := LoadModels(modelsPath)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadModels() ok = false, want true")
	}
	if got := savedModels.Default; got != "" {
		t.Fatalf("saved models default = %q, want empty because agents.profile_defaults owns defaults", got)
	}
	if got, want := savedModels.Providers["default"].BaseURL, "http://127.0.0.1:4000"; got != want {
		t.Fatalf("saved model BaseURL = %q, want %q", got, want)
	}
}

func TestSaveWritesHubConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg := Config{
		Server: ServerConfig{
			ListenAddr:       "127.0.0.1:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
			ShowUpgrade:      true,
		},
		Hub: HubConfig{
			DefaultRegistry:        "builtin",
			DefaultPublishRegistry: "team",
			Registries: []HubRegistryConfig{
				{Name: "builtin", Kind: "builtin", Enabled: true},
				{Name: "team", Kind: "remote", URL: "https://hub.example.com", Token: "secret", Enabled: true},
			},
		},
		Bootstrap: BootstrapConfig{
			DefaultManagerTemplate: "builtin.picoclaw-manager",
			DefaultWorkerTemplate:  "builtin.openclaw-worker",
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
	content := string(data)
	for _, want := range []string{
		"[bootstrap]",
		`default_manager_template = "builtin.picoclaw-manager"`,
		`default_worker_template = "builtin.openclaw-worker"`,
		"[hub]",
		`default_registry = "builtin"`,
		`default_publish_registry = "team"`,
		"[[hub.registries]]",
		`name = "builtin"`,
		`kind = "builtin"`,
		`url = "https://hub.example.com"`,
		`token = "secret"`,
		`enabled = true`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("saved config missing %q:\n%s", want, content)
		}
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
			ShowUpgrade:      true,
		},
		Sandbox: SandboxConfig{
			Provider: BoxLiteProvider,
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

func TestSavePreservesCanonicalBoxLiteProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[sandbox]
provider = "boxlite"

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
}

func TestSavePreservesWindowsHubRegistryPathEscaping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

[hub]
default_registry = "builtin"
default_publish_registry = "local"

[[hub.registries]]
name = "builtin"
kind = "builtin"
enabled = true

[[hub.registries]]
name = "local"
kind = "local"
path = "C:\\Users\\dangw\\.csgclaw\\hub"
enabled = true

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
	if got, want := cfg.Hub.Resolved().Registries[1].Path, `C:\Users\dangw\.csgclaw\hub`; got != want {
		t.Fatalf("loaded hub path = %q, want %q", got, want)
	}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	saved := string(data)
	if !strings.Contains(saved, `path = "C:\\Users\\dangw\\.csgclaw\\hub"`) {
		t.Fatalf("saved config missing canonical Windows path:\n%s", saved)
	}
	if strings.Contains(saved, `path = "C:\\\\Users\\\\dangw\\\\.csgclaw\\\\hub"`) {
		t.Fatalf("saved config double-escaped Windows path:\n%s", saved)
	}
}

func TestSaveKeepsSandboxProviderUnsetWhenItUsesDynamicDefault(t *testing.T) {
	restore := stubSandboxProviderExecutablePath(t, filepath.Join(t.TempDir(), "bin", "csgclaw"))
	defer restore()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[sandbox]
provider = ""

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
	if got, want := cfg.Sandbox.Provider, DockerProvider; got != want {
		t.Fatalf("cfg.Sandbox.Provider = %q, want %q", got, want)
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(data); !strings.Contains(got, `provider = ""`) {
		t.Fatalf("saved config should keep sandbox provider unset:\n%s", got)
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
	t.Setenv("MANAGER_TEMPLATE", "local.review-manager")
	t.Setenv("WORKER_TEMPLATE", "local.review-worker")
	t.Setenv("SANDBOX_PROVIDER", BoxLiteProvider)
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
default_manager_template = "${MANAGER_TEMPLATE}"
default_worker_template = "${WORKER_TEMPLATE}"

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

	if got, want := cfg.Bootstrap.DefaultManagerTemplate, "local.review-manager"; got != want {
		t.Fatalf("cfg.Bootstrap.DefaultManagerTemplate = %q, want %q", got, want)
	}
	if got, want := cfg.Bootstrap.DefaultWorkerTemplate, "local.review-worker"; got != want {
		t.Fatalf("cfg.Bootstrap.DefaultWorkerTemplate = %q, want %q", got, want)
	}
	if got, want := cfg.Sandbox.Provider, BoxLiteProvider; got != want {
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
	t.Setenv("MANAGER_TEMPLATE", "builtin.picoclaw-manager")
	t.Setenv("WORKER_TEMPLATE", "builtin.openclaw-worker")
	t.Setenv("SANDBOX_PROVIDER", BoxLiteProvider)
	t.Setenv("HUB_DEFAULT_REGISTRY", "team")
	t.Setenv("HUB_PUBLISH_REGISTRY", "local")
	t.Setenv("HUB_URL", "hub.example.test")
	t.Setenv("HUB_TOKEN", "hub-secret")
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
default_manager_template = "${MANAGER_TEMPLATE}"
default_worker_template = "${WORKER_TEMPLATE}"

[sandbox]
provider = "${SANDBOX_PROVIDER}"

[hub]
default_registry = "${HUB_DEFAULT_REGISTRY}"
default_publish_registry = "${HUB_PUBLISH_REGISTRY}"

[[hub.registries]]
name = "team"
kind = "remote"
url = "https://${HUB_URL}"
token = "${HUB_TOKEN}"
enabled = true

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
	t.Setenv("MANAGER_TEMPLATE", "changed-manager-template")
	t.Setenv("WORKER_TEMPLATE", "changed-worker-template")
	t.Setenv("HUB_URL", "changed-hub.example.test")
	t.Setenv("HUB_TOKEN", "changed-hub-token")
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
		`default_manager_template = "${MANAGER_TEMPLATE}"`,
		`default_worker_template = "${WORKER_TEMPLATE}"`,
		`provider = "${SANDBOX_PROVIDER}"`,
		`default_registry = "${HUB_DEFAULT_REGISTRY}"`,
		`default_publish_registry = "${HUB_PUBLISH_REGISTRY}"`,
		`url = "https://${HUB_URL}"`,
		`token = "${HUB_TOKEN}"`,
	} {
		if !strings.Contains(saved, want) {
			t.Fatalf("saved config missing %q:\n%s", want, saved)
		}
	}
	if strings.Contains(saved, "[models]") || strings.Contains(saved, "MODEL_API_KEY") {
		t.Fatalf("saved config should not contain model settings:\n%s", saved)
	}
	modelsPath, err := ModelsPathForConfigPath(path)
	if err != nil {
		t.Fatalf("ModelsPathForConfigPath() error = %v", err)
	}
	savedModels, ok, err := LoadModels(modelsPath)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadModels() ok = false, want true")
	}
	if got := savedModels.Default; got != "" {
		t.Fatalf("saved models default = %q, want empty because agents.profile_defaults owns defaults", got)
	}
	savedProvider := savedModels.Providers["remote"]
	if got, want := savedProvider.BaseURL, "https://models.example.test/v1"; got != want {
		t.Fatalf("saved model BaseURL = %q, want %q", got, want)
	}
	if got, want := savedProvider.APIKey, "sk-env"; got != want {
		t.Fatalf("saved model APIKey = %q, want %q", got, want)
	}
	if got, want := strings.Join(savedProvider.Models, ","), "gpt-env,gpt-static"; got != want {
		t.Fatalf("saved model list = %q, want %q", got, want)
	}
	if got := savedProvider.ReasoningEffort; got != "" {
		t.Fatalf("saved reasoning effort = %q, want empty because reasoning is stored on agent profiles", got)
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
	if strings.Contains(saved, "[models]") {
		t.Fatalf("saved config should not contain model settings:\n%s", saved)
	}
	modelsPath, err := ModelsPathForConfigPath(path)
	if err != nil {
		t.Fatalf("ModelsPathForConfigPath() error = %v", err)
	}
	savedModels, ok, err := LoadModels(modelsPath)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadModels() ok = false, want true")
	}
	if got := savedModels.Default; got != "" {
		t.Fatalf("saved models default = %q, want empty because agents.profile_defaults owns defaults", got)
	}
	if got, want := strings.Join(savedModels.Providers["remote"].Models, ","), "gpt-changed"; got != want {
		t.Fatalf("saved model list = %q, want %q", got, want)
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
	if got, want := cfg.Sandbox.Provider, BoxLiteProvider; got != want {
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

func TestResolveAdvertiseBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		server ServerConfig
		want   string
	}{
		{
			name:   "configured",
			server: ServerConfig{AdvertiseBaseURL: "https://csgclaw.example.com/"},
			want:   "https://csgclaw.example.com",
		},
		{
			name:   "empty uses loopback and listen port",
			server: ServerConfig{ListenAddr: "127.0.0.1:19090"},
			want:   "http://127.0.0.1:19090",
		},
		{
			name:   "empty keeps explicit listen host",
			server: ServerConfig{ListenAddr: "192.168.2.52:19090"},
			want:   "http://192.168.2.52:19090",
		},
		{
			name:   "empty keeps localhost listen host",
			server: ServerConfig{ListenAddr: "localhost:19090"},
			want:   "http://localhost:19090",
		},
		{
			name:   "wildcard listen host falls back to loopback",
			server: ServerConfig{ListenAddr: "0.0.0.0:19090"},
			want:   "http://127.0.0.1:19090",
		},
		{
			name:   "empty uses default http port",
			server: ServerConfig{},
			want:   "http://127.0.0.1:" + DefaultHTTPPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveAdvertiseBaseURL(tt.server); got != tt.want {
				t.Fatalf("ResolveAdvertiseBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
