package onboard

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

func TestEnsureStateCreatesConfigAndBootstrapsManagerState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(t.TempDir(), "config.toml")
	wantAgentsPath, err := config.DefaultAgentsPath()
	if err != nil {
		t.Fatalf("DefaultAgentsPath() error = %v", err)
	}
	wantIMStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		t.Fatalf("DefaultIMStatePath() error = %v", err)
	}

	var gotIMStatePath string
	var gotAgentsPath string
	var gotManagerIMStatePath string
	var gotCfg config.Config
	restore := stubEnsureStateDeps(t)
	defer restore()
	EnsureIMBootstrapState = func(path string) error {
		gotIMStatePath = path
		return nil
	}
	CreateManagerParticipant = func(_ context.Context, agentsPath, imStatePath string, cfg config.Config) (participant.Participant, error) {
		gotAgentsPath = agentsPath
		gotManagerIMStatePath = imStatePath
		gotCfg = cfg
		return participant.Participant{ID: agent.ManagerParticipantID}, nil
	}

	result, err := EnsureState(context.Background(), EnsureStateOptions{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("EnsureState() error = %v", err)
	}

	if gotIMStatePath != wantIMStatePath {
		t.Fatalf("EnsureIMBootstrapState path = %q, want %q", gotIMStatePath, wantIMStatePath)
	}
	if gotAgentsPath != wantAgentsPath {
		t.Fatalf("CreateManagerParticipant agentsPath = %q, want %q", gotAgentsPath, wantAgentsPath)
	}
	if gotManagerIMStatePath != wantIMStatePath {
		t.Fatalf("CreateManagerParticipant imStatePath = %q, want %q", gotManagerIMStatePath, wantIMStatePath)
	}
	if result.ConfigPath != configPath {
		t.Fatalf("result.ConfigPath = %q, want %q", result.ConfigPath, configPath)
	}
	if got, want := gotCfg.Server.ListenAddr, config.DefaultListenAddr(); got != want {
		t.Fatalf("cfg.Server.ListenAddr = %q, want %q", got, want)
	}
	if got, want := gotCfg.Server.AccessToken, config.DefaultAccessToken; got != want {
		t.Fatalf("cfg.Server.AccessToken = %q, want %q", got, want)
	}
	if got := gotCfg.Sandbox.Provider; got != "" {
		t.Fatalf("cfg.Sandbox.Provider = %q, want empty dynamic default", got)
	}
	if got, want := gotCfg.Sandbox.Resolved().Provider, config.DockerProvider; got != want {
		t.Fatalf("cfg.Sandbox.Provider = %q, want %q", got, want)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		`[server]`,
		`show_upgrade = true`,
		`[bootstrap]`,
		`[sandbox]`,
		`provider = ""`,
		`debian_registries_override = []`,
		`[hub]`,
		`default_registry = "builtin"`,
		`default_publish_registry = "local"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("saved config missing %q:\n%s", want, string(data))
		}
	}
}

func TestCreateManagerParticipantBootstrapsAdminParticipant(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	agentsPath := filepath.Join(dir, "agents.json")
	imStatePath := filepath.Join(dir, "im", "state.json")
	if err := im.EnsureBootstrapState(imStatePath); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}

	ctx := context.WithValue(context.Background(), noAuthDetectContextKey{}, true)
	if _, err := createManagerParticipant(ctx, agentsPath, imStatePath, defaultConfig()); err != nil {
		t.Fatalf("createManagerParticipant() error = %v", err)
	}

	participantsPath, err := defaultParticipantsPath()
	if err != nil {
		t.Fatalf("defaultParticipantsPath() error = %v", err)
	}
	store, err := participant.NewStore(participantsPath)
	if err != nil {
		t.Fatalf("participant.NewStore() error = %v", err)
	}
	admin, ok := store.Get(participant.ChannelCSGClaw, "pt-admin")
	if !ok {
		t.Fatal("admin participant was not created")
	}
	if admin.Type != participant.TypeHuman {
		t.Fatalf("admin participant type = %q, want %q", admin.Type, participant.TypeHuman)
	}
	if admin.AgentID != "" {
		t.Fatalf("admin participant agent_id = %q, want empty", admin.AgentID)
	}
	if admin.ChannelUserRef != im.AdminUserID {
		t.Fatalf("admin participant channel_user_ref = %q, want %q", admin.ChannelUserRef, im.AdminUserID)
	}
}

func TestEnsureStateNoAuthDetectCreatesManagerWithoutDetectionResults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if _, err := EnsureState(context.Background(), EnsureStateOptions{
		ConfigPath:   configPath,
		NoAuthDetect: true,
	}); err != nil {
		t.Fatalf("EnsureState() error = %v", err)
	}

	agentsPath, err := config.DefaultAgentsPath()
	if err != nil {
		t.Fatalf("DefaultAgentsPath() error = %v", err)
	}
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var state struct {
		Agents struct {
			Items []struct {
				ID               string                         `json:"id"`
				ModelConfig      agent.AgentProfile             `json:"model_config"`
				DetectionResults []agent.ProfileDetectionResult `json:"detection_results,omitempty"`
			} `json:"items"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	var manager *struct {
		ID               string                         `json:"id"`
		ModelConfig      agent.AgentProfile             `json:"model_config"`
		DetectionResults []agent.ProfileDetectionResult `json:"detection_results,omitempty"`
	}
	for i := range state.Agents.Items {
		if state.Agents.Items[i].ID == agent.ManagerUserID {
			manager = &state.Agents.Items[i]
			break
		}
	}
	if manager == nil {
		t.Fatalf("manager agent %q not found in state: %s", agent.ManagerUserID, string(data))
	}
	if manager.ModelConfig.ProfileComplete {
		t.Fatalf("manager model_config = %+v; want incomplete", manager.ModelConfig)
	}
	if manager.ModelConfig.ModelProviderID != agent.ModelProviderIDCSGHubLite {
		t.Fatalf("manager model_provider_id = %q, want %q", manager.ModelConfig.ModelProviderID, agent.ModelProviderIDCSGHubLite)
	}
	if len(manager.DetectionResults) != 0 {
		t.Fatalf("manager detection_results = %+v, want empty", manager.DetectionResults)
	}
}

func TestEnsureStatePreservesExistingStaticLLMConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte(`# Generated by csgclaw.

[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = ""
access_token = "your_access_token"

[bootstrap]
default_manager_template = "builtin.manager-codex"
default_worker_template = "builtin.picoclaw-worker"

[sandbox]
provider = "boxlite"
home_dir_name = "boxlite"
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
path = "/tmp/hub"
enabled = true

[models]
default = "default.gpt-test"

[models.providers.default]
base_url = "http://llm.test/v1"
api_key = "secret"
models = ["gpt-test"]
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var gotCfg config.Config
	restore := stubEnsureStateDeps(t)
	defer restore()
	EnsureIMBootstrapState = func(string) error { return nil }
	CreateManagerParticipant = func(_ context.Context, _, _ string, cfg config.Config) (participant.Participant, error) {
		gotCfg = cfg
		return participant.Participant{}, nil
	}

	if _, err := EnsureState(context.Background(), EnsureStateOptions{ConfigPath: configPath}); err != nil {
		t.Fatalf("EnsureState() error = %v", err)
	}
	if got, want := gotCfg.Models.Default, "default.gpt-test"; got != want {
		t.Fatalf("cfg.Models.Default = %q, want %q", got, want)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), `[models.providers.default]`) || strings.Contains(string(data), `api_key = "secret"`) {
		t.Fatalf("saved config should migrate static llm config out of config.toml:\n%s", string(data))
	}
	modelsPath, err := config.ModelsPathForConfigPath(configPath)
	if err != nil {
		t.Fatalf("ModelsPathForConfigPath() error = %v", err)
	}
	models, ok, err := config.LoadModels(modelsPath)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadModels() ok = false, want true")
	}
	if got, want := models.Providers["default"].APIKey, "secret"; got != want {
		t.Fatalf("models API key = %q, want %q", got, want)
	}
}

func TestEnsureStateCreatesBackupBeforeMigratingExistingConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	root := filepath.Join(home, ".csgclaw")
	configPath := filepath.Join(root, "config.toml")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := `# Generated by csgclaw.

[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = ""
access_token = "your_access_token"

[bootstrap]
default_manager_template = "builtin.manager-codex"
default_worker_template = "builtin.picoclaw-worker"

[sandbox]
provider = "boxlite"
home_dir_name = "boxlite"
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
path = "/tmp/hub"
enabled = true

[models]
default = "default.gpt-test"

[models.providers.default]
base_url = "http://llm.test/v1"
api_key = "secret"
models = ["gpt-test"]
`
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	restore := stubEnsureStateDeps(t)
	defer restore()
	EnsureIMBootstrapState = func(string) error { return nil }
	CreateManagerParticipant = func(context.Context, string, string, config.Config) (participant.Participant, error) {
		return participant.Participant{}, nil
	}

	if _, err := EnsureState(context.Background(), EnsureStateOptions{ConfigPath: configPath}); err != nil {
		t.Fatalf("EnsureState() error = %v", err)
	}

	backups := findStoreBackups(t, home)
	if len(backups) != 1 {
		t.Fatalf("backup count = %d, want 1; entries = %#v", len(backups), backups)
	}
	backupConfig, err := os.ReadFile(filepath.Join(home, backups[0], "config.toml"))
	if err != nil {
		t.Fatalf("read backup config: %v", err)
	}
	if string(backupConfig) != original {
		t.Fatalf("backup config was not the original config:\n%s", string(backupConfig))
	}
}

func TestEnsureStateDoesNotBackupNewStoreFormatForConfigCompletion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	root := filepath.Join(home, ".csgclaw")
	configPath := filepath.Join(root, "config.toml")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`# Generated by csgclaw.

[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = ""
access_token = "your_access_token"

[bootstrap]
default_manager_template = "builtin.manager-codex"
default_worker_template = "builtin.picoclaw-worker"

[sandbox]
provider = "boxlite"
debian_registries_override = []

[hub]
default_registry = "builtin"
default_publish_registry = "local"
`), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	writeJSONFile(t, filepath.Join(root, "state.json"), map[string]any{
		"version": 1,
		"model_providers": map[string]any{
			"items": map[string]any{
				"default": map[string]any{
					"base_url": "http://llm.test/v1",
					"api_key":  "secret",
					"models":   []string{"gpt-test"},
				},
			},
		},
		"agents":       map[string]any{"items": []any{}},
		"participants": map[string]any{"items": []any{}},
	})

	restore := stubEnsureStateDeps(t)
	defer restore()
	EnsureIMBootstrapState = func(string) error { return nil }
	CreateManagerParticipant = func(context.Context, string, string, config.Config) (participant.Participant, error) {
		return participant.Participant{}, nil
	}

	if _, err := EnsureState(context.Background(), EnsureStateOptions{ConfigPath: configPath}); err != nil {
		t.Fatalf("EnsureState() error = %v", err)
	}
	if backups := findStoreBackups(t, home); len(backups) != 0 {
		t.Fatalf("backup count = %d, want 0 for new store format; entries = %#v", len(backups), backups)
	}
}

func TestEnsureStateDoesNotRewriteExistingCompleteConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(t.TempDir(), "config.toml")
	original := `# custom config header

[server]
listen_addr = "127.0.0.1:19090"
advertise_base_url = "http://example.test"
access_token = "custom-token"
no_auth = false

[bootstrap]
manager_image_override = ""

[sandbox]
provider = "boxlite"
home_dir_name = "boxlite"
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
path = "/tmp/hub"
enabled = true

[models]
default = "default.gpt-test"

[models.providers.default]
base_url = "http://llm.test/v1"
api_key = "secret"
models = ["gpt-test"]
`
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	restore := stubEnsureStateDeps(t)
	defer restore()
	EnsureIMBootstrapState = func(string) error { return nil }
	CreateManagerParticipant = func(_ context.Context, _, _ string, cfg config.Config) (participant.Participant, error) {
		if got, want := cfg.Server.ListenAddr, "127.0.0.1:19090"; got != want {
			t.Fatalf("cfg.Server.ListenAddr = %q, want %q", got, want)
		}
		return participant.Participant{}, nil
	}

	if _, err := EnsureState(context.Background(), EnsureStateOptions{ConfigPath: configPath}); err != nil {
		t.Fatalf("EnsureState() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), "[models]") || strings.Contains(string(data), "[models.providers.default]") {
		t.Fatalf("EnsureState() should migrate models out of config.toml:\n%s", string(data))
	}
	modelsPath, err := config.ModelsPathForConfigPath(configPath)
	if err != nil {
		t.Fatalf("ModelsPathForConfigPath() error = %v", err)
	}
	models, ok, err := config.LoadModels(modelsPath)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadModels() ok = false, want true")
	}
	if got := models.Default; got != "" {
		t.Fatalf("models default = %q, want empty because agents.model_defaults owns defaults", got)
	}
}

func TestEnsureStateKeepsExistingCompleteConfigWithCanonicalBoxLiteProvider(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(t.TempDir(), "config.toml")
	original := `# custom config header

[server]
listen_addr = "127.0.0.1:19090"
advertise_base_url = "http://example.test"
access_token = "custom-token"
no_auth = false

[bootstrap]
manager_image_override = ""

[sandbox]
provider = "boxlite"
home_dir_name = "boxlite"
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
path = "/tmp/hub"
enabled = true

[models]
default = "default.gpt-test"

[models.providers.default]
base_url = "http://llm.test/v1"
api_key = "secret"
models = ["gpt-test"]
`
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	restore := stubEnsureStateDeps(t)
	defer restore()
	EnsureIMBootstrapState = func(string) error { return nil }
	CreateManagerParticipant = func(_ context.Context, _, _ string, cfg config.Config) (participant.Participant, error) {
		if got, want := cfg.Sandbox.Provider, config.BoxLiteProvider; got != want {
			t.Fatalf("cfg.Sandbox.Provider = %q, want %q", got, want)
		}
		return participant.Participant{}, nil
	}

	if _, err := EnsureState(context.Background(), EnsureStateOptions{ConfigPath: configPath}); err != nil {
		t.Fatalf("EnsureState() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), "[models]") || strings.Contains(string(data), "[models.providers.default]") {
		t.Fatalf("EnsureState() should migrate models out of config.toml:\n%s", string(data))
	}
	modelsPath, err := config.ModelsPathForConfigPath(configPath)
	if err != nil {
		t.Fatalf("ModelsPathForConfigPath() error = %v", err)
	}
	models, ok, err := config.LoadModels(modelsPath)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadModels() ok = false, want true")
	}
	if got, want := models.Providers["default"].BaseURL, "http://llm.test/v1"; got != want {
		t.Fatalf("models BaseURL = %q, want %q", got, want)
	}
}

func TestEnsureStateRewritesLegacyBootstrapTemplateRefs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(t.TempDir(), "config.toml")
	original := `[server]
listen_addr = "127.0.0.1:19090"
advertise_base_url = "http://example.test"
access_token = "custom-token"
no_auth = false

[bootstrap]
default_manager_template = "builtin/picoclaw-manager"
default_worker_template = "local/review-worker"

[sandbox]
provider = "boxlite"
debian_registries_override = []

[models]
default = "default.gpt-test"

[models.providers.default]
base_url = "http://llm.test/v1"
api_key = "secret"
models = ["gpt-test"]
`
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	restore := stubEnsureStateDeps(t)
	defer restore()
	EnsureIMBootstrapState = func(string) error { return nil }
	CreateManagerParticipant = func(_ context.Context, _, _ string, cfg config.Config) (participant.Participant, error) {
		if got, want := cfg.Bootstrap.DefaultManagerTemplate, "builtin.manager-codex"; got != want {
			t.Fatalf("cfg.Bootstrap.DefaultManagerTemplate = %q, want %q", got, want)
		}
		if got, want := cfg.Bootstrap.DefaultWorkerTemplate, "local.review-worker"; got != want {
			t.Fatalf("cfg.Bootstrap.DefaultWorkerTemplate = %q, want %q", got, want)
		}
		return participant.Participant{}, nil
	}

	if _, err := EnsureState(context.Background(), EnsureStateOptions{ConfigPath: configPath}); err != nil {
		t.Fatalf("EnsureState() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		`default_manager_template = "builtin.manager-codex"`,
		`default_worker_template = "local.review-worker"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("EnsureState() rewrite missing %q:\n%s", want, content)
		}
	}
}

func TestEnsureStateCompletesExistingConfigMissingBootstrapDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(t.TempDir(), "config.toml")
	original := `[server]
listen_addr = "127.0.0.1:18080"
advertise_base_url = ""
access_token = "your_access_token"
`
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	restore := stubEnsureStateDeps(t)
	defer restore()
	EnsureIMBootstrapState = func(string) error { return nil }
	CreateManagerParticipant = func(_ context.Context, _, _ string, cfg config.Config) (participant.Participant, error) {
		if got, want := cfg.Sandbox.Resolved().Provider, config.DockerProvider; got != want {
			t.Fatalf("cfg.Sandbox.Provider = %q, want %q", got, want)
		}
		return participant.Participant{}, nil
	}

	if _, err := EnsureState(context.Background(), EnsureStateOptions{ConfigPath: configPath}); err != nil {
		t.Fatalf("EnsureState() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		`no_auth = false`,
		`show_upgrade = true`,
		`[bootstrap]`,
		`default_manager_template = "builtin.manager-codex"`,
		`default_worker_template = "builtin.picoclaw-worker"`,
		`[sandbox]`,
		`provider = ""`,
		`debian_registries_override = []`,
		`[hub]`,
		`default_registry = "builtin"`,
		`default_publish_registry = "local"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("completed config missing %q:\n%s", want, content)
		}
	}
}

func TestEnsureStateCompletesExistingConfigMissingHubDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(t.TempDir(), "config.toml")
	original := `# custom config header

[server]
listen_addr = "127.0.0.1:19090"
advertise_base_url = "http://example.test"
access_token = "custom-token"
no_auth = false

[bootstrap]
default_manager_template = "builtin.manager-codex"
default_worker_template = "builtin.picoclaw-worker"

[sandbox]
provider = "boxlite"
home_dir_name = "boxlite"
debian_registries_override = []

[models]
default = "default.gpt-test"

[models.providers.default]
base_url = "http://llm.test/v1"
api_key = "secret"
models = ["gpt-test"]
`
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	restore := stubEnsureStateDeps(t)
	defer restore()
	EnsureIMBootstrapState = func(string) error { return nil }
	CreateManagerParticipant = func(_ context.Context, _, _ string, cfg config.Config) (participant.Participant, error) {
		if got, want := cfg.Hub.DefaultRegistry, "builtin"; got != want {
			t.Fatalf("cfg.Hub.DefaultRegistry = %q, want %q", got, want)
		}
		if got, want := cfg.Hub.DefaultPublishRegistry, "local"; got != want {
			t.Fatalf("cfg.Hub.DefaultPublishRegistry = %q, want %q", got, want)
		}
		return participant.Participant{}, nil
	}

	if _, err := EnsureState(context.Background(), EnsureStateOptions{ConfigPath: configPath}); err != nil {
		t.Fatalf("EnsureState() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		`# Generated by csgclaw.`,
		`[hub]`,
		`default_registry = "builtin"`,
		`default_publish_registry = "local"`,
		`[[hub.registries]]`,
		`name = "builtin"`,
		`kind = "builtin"`,
		`enabled = true`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("completed config missing %q:\n%s", want, content)
		}
	}
}

func TestEnsureStateCompletesExistingConfigMissingHubRegistries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(t.TempDir(), "config.toml")
	original := `# custom config header

[server]
listen_addr = "127.0.0.1:19090"
advertise_base_url = "http://example.test"
access_token = "custom-token"
no_auth = false

[bootstrap]
default_manager_template = "builtin.manager-codex"
default_worker_template = "builtin.picoclaw-worker"

[sandbox]
provider = "boxlite"
debian_registries_override = []

[hub]
default_registry = "builtin"
default_publish_registry = "local"

[models]
default = "default.gpt-test"

[models.providers.default]
base_url = "http://llm.test/v1"
api_key = "secret"
models = ["gpt-test"]
`
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	restore := stubEnsureStateDeps(t)
	defer restore()
	EnsureIMBootstrapState = func(string) error { return nil }
	CreateManagerParticipant = func(_ context.Context, _, _ string, cfg config.Config) (participant.Participant, error) {
		if got, want := len(cfg.Hub.Registries), 3; got != want {
			t.Fatalf("len(cfg.Hub.Registries) = %d, want %d", got, want)
		}
		return participant.Participant{}, nil
	}

	if _, err := EnsureState(context.Background(), EnsureStateOptions{ConfigPath: configPath}); err != nil {
		t.Fatalf("EnsureState() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		`[[hub.registries]]`,
		`name = "builtin"`,
		`name = "local"`,
		`kind = "local"`,
		`enabled = true`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("completed config missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, `name = "official"`) {
		t.Fatalf("completed config should not persist default official registry:\n%s", content)
	}
}

func findStoreBackups(t *testing.T, parent string) []string {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	var backups []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".csgclaw_backup_") {
			backups = append(backups, entry.Name())
		}
	}
	return backups
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func stubEnsureStateDeps(t *testing.T) func() {
	t.Helper()
	origCreateManager := CreateManagerParticipant
	origEnsureIMBootstrapState := EnsureIMBootstrapState
	return func() {
		CreateManagerParticipant = origCreateManager
		EnsureIMBootstrapState = origEnsureIMBootstrapState
	}
}
