package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWithChannelFilesUsesStandaloneFeishuConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[models]
default = "default.minimax-m2.7"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["minimax-m2.7"]

[channels.feishu]
admin_open_id = "ou_legacy"

[channels.feishu.u-manager]
app_id = "cli_legacy_manager"
app_secret = "legacy-manager-secret"

[channels.feishu.u-dev]
app_id = "cli_legacy_dev"
app_secret = "legacy-dev-secret"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	feishuPath := filepath.Join(dir, ChannelsDirName, FeishuChannelConfigFileName)
	feishuContent := `[global]
admin_open_id = "ou_standalone"

[bots.u-dev]
app_id = "cli_standalone_dev"
app_secret = "standalone-dev-secret"

[bots.u-worker]
app_id = "cli_worker"
app_secret = "worker-secret"
`
	if err := os.MkdirAll(filepath.Dir(feishuPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(feishuPath, []byte(feishuContent), 0o600); err != nil {
		t.Fatalf("WriteFile(feishu) error = %v", err)
	}

	cfg, err := LoadWithChannelFiles(path)
	if err != nil {
		t.Fatalf("LoadWithChannelFiles() error = %v", err)
	}

	if got, want := cfg.Channels.FeishuAdminOpenID, "ou_standalone"; got != want {
		t.Fatalf("FeishuAdminOpenID = %q, want %q", got, want)
	}
	if _, ok := cfg.Channels.Feishu["u-manager"]; ok {
		t.Fatalf("legacy u-manager app config was loaded: %+v", cfg.Channels.Feishu["u-manager"])
	}
	if got, want := cfg.Channels.Feishu["u-dev"].AppID, "cli_standalone_dev"; got != want {
		t.Fatalf("u-dev app_id = %q, want %q", got, want)
	}
	if got, want := cfg.Channels.Feishu["u-dev"].AppSecret, "standalone-dev-secret"; got != want {
		t.Fatalf("u-dev app_secret = %q, want %q", got, want)
	}
	if got, want := cfg.Channels.Feishu["u-worker"].AppID, "cli_worker"; got != want {
		t.Fatalf("u-worker app_id = %q, want %q", got, want)
	}
	if got, want := len(cfg.Channels.Feishu), 2; got != want {
		t.Fatalf("feishu app config count = %d, want %d", got, want)
	}
}

func TestLoadWithChannelFilesIgnoresLegacyFeishuWhenStandaloneConfigIsMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[models]
default = "default.minimax-m2.7"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["minimax-m2.7"]

[channels.feishu.u-manager]
app_id = "cli_legacy_manager"
app_secret = "legacy-manager-secret"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	cfg, err := LoadWithChannelFiles(path)
	if err != nil {
		t.Fatalf("LoadWithChannelFiles() error = %v", err)
	}
	if got := cfg.Channels.FeishuAdminOpenID; got != "" {
		t.Fatalf("FeishuAdminOpenID = %q, want empty", got)
	}
	if len(cfg.Channels.Feishu) != 0 {
		t.Fatalf("legacy feishu channel config was loaded: %+v", cfg.Channels.Feishu)
	}
}

func TestSaveFeishuChannelConfigWritesStandaloneFileOnly(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config.toml")
	mainContent := "# keep me\n[server]\nlisten_addr = \"127.0.0.1:18080\"\n"
	if err := os.WriteFile(mainPath, []byte(mainContent), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	feishuPath := filepath.Join(dir, ChannelsDirName, FeishuChannelConfigFileName)
	channels := ChannelsConfig{
		FeishuAdminOpenID: "ou_admin",
		Feishu: map[string]FeishuConfig{
			"u-dev": {
				AppID:     "cli_dev",
				AppSecret: "dev-secret",
			},
			"u-manager": {
				AppID:     "cli_manager",
				AppSecret: "manager-secret",
			},
		},
	}
	if err := SaveFeishuChannelConfig(feishuPath, channels); err != nil {
		t.Fatalf("SaveFeishuChannelConfig() error = %v", err)
	}

	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	if got := string(mainData); got != mainContent {
		t.Fatalf("main config changed:\n%s", got)
	}

	data, err := os.ReadFile(feishuPath)
	if err != nil {
		t.Fatalf("ReadFile(feishu) error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"[global]",
		`admin_open_id = "ou_admin"`,
		"[bots.u-dev]",
		`app_id = "cli_dev"`,
		`app_secret = "dev-secret"`,
		"[bots.u-manager]",
		`app_id = "cli_manager"`,
		`app_secret = "manager-secret"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("standalone feishu config missing %q:\n%s", want, content)
		}
	}

	info, err := os.Stat(feishuPath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("feishu config mode = %v, want %v", got, want)
	}
}

func TestSaveFeishuChannelConfigRoundTripsEscapedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ChannelsDirName, FeishuChannelConfigFileName)
	secret := `sec-$DOLLAR-quote-"-slash-\\`
	channels := ChannelsConfig{
		FeishuAdminOpenID: "ou_$admin",
		Feishu: map[string]FeishuConfig{
			"u-dev": {AppID: `cli_"dev"`, AppSecret: secret},
		},
	}
	if err := SaveFeishuChannelConfig(path, channels); err != nil {
		t.Fatalf("SaveFeishuChannelConfig() error = %v", err)
	}
	loaded, err := LoadFeishuChannelConfig(path)
	if err != nil {
		t.Fatalf("LoadFeishuChannelConfig() error = %v", err)
	}
	if got := loaded.FeishuAdminOpenID; got != channels.FeishuAdminOpenID {
		t.Fatalf("admin_open_id = %q, want %q", got, channels.FeishuAdminOpenID)
	}
	if got := loaded.Feishu["u-dev"].AppID; got != channels.Feishu["u-dev"].AppID {
		t.Fatalf("app_id = %q, want %q", got, channels.Feishu["u-dev"].AppID)
	}
	if got := loaded.Feishu["u-dev"].AppSecret; got != secret {
		t.Fatalf("app_secret = %q, want %q", got, secret)
	}
}

func TestSaveFeishuChannelConfigRejectsInvalidBotID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ChannelsDirName, FeishuChannelConfigFileName)
	err := SaveFeishuChannelConfig(path, ChannelsConfig{Feishu: map[string]FeishuConfig{"u.dev": {AppID: "cli", AppSecret: "secret"}}})
	if err == nil {
		t.Fatalf("SaveFeishuChannelConfig() error = nil, want invalid bot id")
	}
}

func TestFeishuChannelConfigPathUsesChannelsSubdirectory(t *testing.T) {
	dir := t.TempDir()
	path, err := FeishuChannelConfigPath(filepath.Join(dir, ConfigFileName))
	if err != nil {
		t.Fatalf("FeishuChannelConfigPath() error = %v", err)
	}
	if got, want := path, filepath.Join(dir, ChannelsDirName, FeishuChannelConfigFileName); got != want {
		t.Fatalf("FeishuChannelConfigPath() = %q, want %q", got, want)
	}
}
