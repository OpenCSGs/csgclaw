package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	hub "csgclaw/internal/template"
	"csgclaw/internal/upgrade"
)

func TestHandleServerRestartStartsHelper(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.toml"
	writeMinimalAPIConfig(t, configPath)

	var started upgrade.RestartHelperOptions
	srv := &Handler{}
	srv.SetConfigPath(configPath)
	srv.SetServerRestartApplyFunc(func(opts upgrade.RestartHelperOptions) error {
		started = opts
		return nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/server/restart", nil)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST restart status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if started.ConfigPath != configPath {
		t.Fatalf("restart helper config path = %q, want %q", started.ConfigPath, configPath)
	}
}

func TestHandleServerRestartStatusConsumesManualRestartRequired(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.toml"
	writeMinimalAPIConfig(t, configPath)

	artifacts, err := upgrade.ResolveRestartArtifacts(configPath)
	if err != nil {
		t.Fatalf("ResolveRestartArtifacts() error = %v", err)
	}
	if err := artifacts.RecordManualRestartRequired("manual restart required"); err != nil {
		t.Fatalf("RecordManualRestartRequired() error = %v", err)
	}

	srv := &Handler{}
	srv.SetConfigPath(configPath)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/restart/status", nil)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got apitypes.ServerRestartStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.ManualRestartRequired {
		t.Fatalf("ManualRestartRequired = false, want true")
	}
}

func TestHandleServerConfigGetPut(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.toml"
	writeMinimalAPIConfig(t, configPath)

	srv := &Handler{}
	srv.SetConfigPath(configPath)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/config", nil)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET config status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got apitypes.ConfigSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode GET config response: %v", err)
	}
	if got.Path != configPath || got.ListenAddr == "" {
		t.Fatalf("GET config = %+v, want populated fields", got)
	}
	if !got.AccessTokenSet || got.AccessToken != "" {
		t.Fatalf("AccessTokenSet = %v AccessToken = %q, want masked response", got.AccessTokenSet, got.AccessToken)
	}
	if len(got.SupportedSandboxProviders) == 0 {
		t.Fatalf("SupportedSandboxProviders = %#v, want non-empty", got.SupportedSandboxProviders)
	}
	if got.AdvertiseBaseURLEffective == "" {
		t.Fatalf("AdvertiseBaseURLEffective = empty, want resolved manager base URL")
	}
	if got.HubLocalPath == "" {
		t.Fatalf("GET hub local path = empty, want populated default")
	}
	if got.HubOfficialURLEffective != config.DefaultOfficialHubRegistryURL {
		t.Fatalf("HubOfficialURLEffective = %q, want %q", got.HubOfficialURLEffective, config.DefaultOfficialHubRegistryURL)
	}

	body, err := json.Marshal(apitypes.UpdateConfigSettingsRequest{
		ListenAddr:             "127.0.0.1:19080",
		AdvertiseBaseURL:       "http://192.168.1.10:19080/",
		ShowUpgrade:            false,
		SandboxProvider:        "docker",
		HubLocalPath:           "/tmp/team-hub",
		DefaultManagerTemplate: "builtin.manager-codex",
		DefaultWorkerTemplate:  "builtin.codex-worker",
	})
	if err != nil {
		t.Fatalf("marshal PUT body: %v", err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/server/config", bytes.NewReader(body))
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT config status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var saved apitypes.ConfigSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&saved); err != nil {
		t.Fatalf("decode PUT config response: %v", err)
	}
	if saved.ListenAddr != "127.0.0.1:19080" || saved.ShowUpgrade {
		t.Fatalf("PUT config = %+v, want updated listen_addr and show_upgrade=false", saved)
	}
	if saved.SandboxProvider != "docker" {
		t.Fatalf("SandboxProvider = %q, want docker", saved.SandboxProvider)
	}
	if saved.AdvertiseBaseURL != "http://192.168.1.10:19080" {
		t.Fatalf("AdvertiseBaseURL = %q, want updated value without trailing slash", saved.AdvertiseBaseURL)
	}
	if saved.AdvertiseBaseURLEffective != "http://192.168.1.10:19080" {
		t.Fatalf("AdvertiseBaseURLEffective = %q, want configured manager base URL", saved.AdvertiseBaseURLEffective)
	}
	if saved.HubLocalPath != "/tmp/team-hub" {
		t.Fatalf("saved hub local path=%q, want updated value", saved.HubLocalPath)
	}
	if saved.HubOfficialURLEffective != config.DefaultOfficialHubRegistryURL {
		t.Fatalf("saved HubOfficialURLEffective = %q, want %q", saved.HubOfficialURLEffective, config.DefaultOfficialHubRegistryURL)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "127.0.0.1:19080") || !strings.Contains(content, "show_upgrade = false") {
		t.Fatalf("config content = %q, want updated server fields preserved with models section", content)
	}
	if !strings.Contains(content, `advertise_base_url = "http://192.168.1.10:19080"`) {
		t.Fatalf("config content = %q, want updated advertise_base_url", content)
	}
	if !strings.Contains(content, `path = "/tmp/team-hub"`) {
		t.Fatalf("config content = %q, want updated hub local path", content)
	}
	if strings.Contains(content, `url = "https://hub.example.com"`) {
		t.Fatalf("config content = %q, did not expect user settings to write official hub URL", content)
	}
}

func TestHandleServerConfigReturnsDockerDesktopCallbackURL(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("host.docker.internal callback URL is only the Docker Desktop default")
	}

	dir := t.TempDir()
	configPath := dir + "/config.toml"
	content := `[server]
listen_addr = "0.0.0.0:19080"
access_token = "secret"

[sandbox]
provider = "docker"

[models]
default = "default.model"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["model"]
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	srv := &Handler{}
	srv.SetConfigPath(configPath)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/config", nil)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET config status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got apitypes.ConfigSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode GET config response: %v", err)
	}
	if got.AdvertiseBaseURLEffective != "http://host.docker.internal:19080" {
		t.Fatalf("AdvertiseBaseURLEffective = %q, want Docker Desktop host alias", got.AdvertiseBaseURLEffective)
	}
}

func TestHandleServerConfigRejectsInvalidBootstrapBeforeSave(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.toml"
	writeMinimalAPIConfig(t, configPath)

	original, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	hubSvc, err := hub.NewService(config.HubConfig{}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}

	srv := &Handler{}
	srv.SetConfigPath(configPath)
	srv.SetHubService(hubSvc)

	body, err := json.Marshal(apitypes.UpdateConfigSettingsRequest{
		ListenAddr:             "127.0.0.1:19080",
		AdvertiseBaseURL:       "http://192.168.1.10:19080",
		ShowUpgrade:            false,
		SandboxProvider:        "docker",
		DefaultManagerTemplate: "builtin.manager-codex",
		DefaultWorkerTemplate:  "missing.worker-template",
	})
	if err != nil {
		t.Fatalf("marshal PUT body: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/server/config", bytes.NewReader(body))
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT config status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "resolve bootstrap worker template") {
		t.Fatalf("body = %q, want bootstrap worker template validation error", rec.Body.String())
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != string(original) {
		t.Fatalf("config content changed after rejected PUT:\n%s", string(data))
	}
}

func TestHandleServerConfigValidatesBootstrapWithHubBeforeSave(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.toml"
	writeMinimalAPIConfig(t, configPath)

	hubSvc, err := hub.NewService(config.HubConfig{}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}

	srv := &Handler{}
	srv.SetConfigPath(configPath)
	srv.SetHubService(hubSvc)

	body, err := json.Marshal(apitypes.UpdateConfigSettingsRequest{
		ListenAddr:             "127.0.0.1:19080",
		AdvertiseBaseURL:       "http://192.168.1.10:19080",
		ShowUpgrade:            false,
		SandboxProvider:        "docker",
		DefaultManagerTemplate: "builtin.manager-codex",
		DefaultWorkerTemplate:  "builtin.codex-worker",
	})
	if err != nil {
		t.Fatalf("marshal PUT body: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/server/config", bytes.NewReader(body))
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT config status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "127.0.0.1:19080") || !strings.Contains(content, "show_upgrade = false") {
		t.Fatalf("config content = %q, want updated server fields", content)
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
