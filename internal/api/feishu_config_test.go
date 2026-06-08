package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
)

func TestFeishuChannelConfigPutWritesStandaloneConfigAndReloads(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, config.ConfigFileName)
	writeMinimalAPIConfig(t, configPath)

	feishuSvc := feishu.NewService()
	feishuSvc.SetConfigPath(configPath)
	h := NewHandlerWithAuth(nil, nil, nil, nil, feishuSvc, nil, "secret", false)

	body := []byte(`{"bot_id":"u-dev","app_id":"cli_dev","app_secret":"dev-secret","admin_open_id":"ou_admin"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/channels/feishu/config", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "dev-secret") {
		t.Fatalf("response leaked app_secret: %s", rec.Body.String())
	}
	var resp apitypes.FeishuConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !resp.Configured || !resp.Reloaded || resp.AppSecret != "present" || resp.AppID != "cli_dev" {
		t.Fatalf("response = %+v, want configured reloaded masked secret", resp)
	}

	feishuPath := filepath.Join(dir, config.ChannelsDirName, feishu.FeishuChannelConfigFileName)
	data, err := os.ReadFile(feishuPath)
	if err != nil {
		t.Fatalf("ReadFile(feishu) error = %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "[bots.u-dev]") || !strings.Contains(content, `app_secret = "dev-secret"`) {
		t.Fatalf("standalone feishu config missing saved bot:\n%s", content)
	}

	apps := feishuSvc.AppConfigs()
	if got, want := apps["u-dev"].AppID, "cli_dev"; got != want {
		t.Fatalf("reloaded app_id = %q, want %q", got, want)
	}
	if got, want := apps["u-dev"].AppSecret, "dev-secret"; got != want {
		t.Fatalf("reloaded app_secret = %q, want %q", got, want)
	}
	if got, want := apps["u-dev"].AdminOpenID, "ou_admin"; got != want {
		t.Fatalf("reloaded admin_open_id = %q, want %q", got, want)
	}
}

func TestFeishuChannelConfigGetMasksSecret(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, config.ConfigFileName)
	writeMinimalAPIConfig(t, configPath)
	if err := feishu.NewFileStore(configPath).Save(feishu.Snapshot{
		AdminOpenID: "ou_admin",
		Bots: map[string]feishu.AppConfig{
			"u-dev": {AppID: "cli_dev", AppSecret: "dev-secret"},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	feishuSvc := feishu.NewService()
	feishuSvc.SetConfigPath(configPath)
	h := NewHandlerWithAuth(nil, nil, nil, nil, feishuSvc, nil, "secret", false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/config?bot_id=u-dev", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "dev-secret") {
		t.Fatalf("response leaked app_secret: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"app_secret":"present"`) {
		t.Fatalf("response missing masked secret: %s", rec.Body.String())
	}
}

func TestChannelsReloadDoesNotDuplicateAuthorization(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, config.ConfigFileName)
	writeMinimalAPIConfig(t, configPath)

	feishuSvc := feishu.NewService()
	feishuSvc.SetConfigPath(configPath)
	h := NewHandlerWithAuth(nil, nil, nil, nil, feishuSvc, nil, "secret", false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/config", nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestLegacyChannelsReloadRouteIsNotRegistered(t *testing.T) {
	h := NewHandlerWithAuth(nil, nil, nil, nil, feishu.NewService(), nil, "secret", false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/reload", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestLegacyFeishuConfigBotIDPathIsNotRegistered(t *testing.T) {
	h := NewHandlerWithAuth(nil, nil, nil, nil, feishu.NewService(), nil, "secret", false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/config/u-dev", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func writeMinimalAPIConfig(t *testing.T, path string) {
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
