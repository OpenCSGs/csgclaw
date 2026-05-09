package feishu

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/config"
)

func TestConfigUpdateGetAndLoad(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, config.ConfigFileName)
	writeMinimalConfig(t, configPath)

	cfgStore := NewConfig(configPath)

	view, err := cfgStore.Update(Update{
		BotID:       "u-dev",
		AppID:       "cli_dev",
		AppSecret:   "dev-secret",
		AdminOpenID: "ou_admin",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !view.Configured || !view.HasSecret || view.AppID != "cli_dev" {
		t.Fatalf("Update() view = %+v, want configured masked secret without reload", view)
	}

	feishuPath := filepath.Join(dir, config.ChannelsDirName, config.FeishuChannelConfigFileName)
	data, err := os.ReadFile(feishuPath)
	if err != nil {
		t.Fatalf("ReadFile(feishu) error = %v", err)
	}
	if content := string(data); !strings.Contains(content, "[bots.u-dev]") || !strings.Contains(content, `app_secret = "dev-secret"`) {
		t.Fatalf("standalone feishu config missing saved bot:\n%s", content)
	}

	cfg, err := cfgStore.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	apps := AppsFromChannels(cfg.Channels)
	if got, want := apps["u-dev"].AppID, "cli_dev"; got != want {
		t.Fatalf("app_id = %q, want %q", got, want)
	}
	if got, want := apps["u-dev"].AppSecret, "dev-secret"; got != want {
		t.Fatalf("app_secret = %q, want %q", got, want)
	}
	if got, want := apps["u-dev"].AdminOpenID, "ou_admin"; got != want {
		t.Fatalf("admin_open_id = %q, want %q", got, want)
	}

	got, err := cfgStore.Get("u-dev")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !got.Configured || !got.HasSecret || got.AppID != "cli_dev" {
		t.Fatalf("Get() view = %+v, want masked saved config", got)
	}

	if got := SortedBotIDs(cfg.Channels); len(got) != 1 || got[0] != "u-dev" {
		t.Fatalf("SortedBotIDs() = %#v, want u-dev", got)
	}
}

func TestConfigUpdateValidatesRequest(t *testing.T) {
	cfgStore := NewConfig("")
	_, err := cfgStore.Update(Update{BotID: "u-dev", AppSecret: "secret"})
	if err == nil || !IsValidationError(err) || err.Error() != "app_id is required" {
		t.Fatalf("Update() error = %v, want app_id validation error", err)
	}
}

func writeMinimalConfig(t *testing.T, path string) {
	t.Helper()
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

[models]
default = "default.model"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["model"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
}
