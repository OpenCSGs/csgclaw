package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/hub"
	"csgclaw/internal/upgrade"
)

func TestHandleConfigFileGetPut(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.toml"
	writeMinimalAPIConfig(t, configPath)

	srv := &Handler{}
	srv.SetConfigPath(configPath)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got apitypes.ConfigFileResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if got.Path != configPath || got.Content == "" {
		t.Fatalf("GET response = %+v, want path and content", got)
	}

	updated := got.Content + "\n# edited\n"
	body, err := json.Marshal(apitypes.UpdateConfigRequest{Content: updated})
	if err != nil {
		t.Fatalf("marshal PUT body: %v", err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewReader(body))
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var saved apitypes.ConfigFileResponse
	if err := json.NewDecoder(rec.Body).Decode(&saved); err != nil {
		t.Fatalf("decode PUT response: %v", err)
	}
	if saved.Content != updated {
		t.Fatalf("PUT content = %q, want %q", saved.Content, updated)
	}
}

func TestHandleConfigApplyStartsHelper(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.toml"
	writeMinimalAPIConfig(t, configPath)

	var started upgrade.RestartHelperOptions
	srv := &Handler{}
	srv.SetConfigPath(configPath)
	srv.SetConfigRestartApplyFunc(func(opts upgrade.RestartHelperOptions) error {
		started = opts
		return nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/apply", nil)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST apply status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if started.ConfigPath != configPath {
		t.Fatalf("restart helper config path = %q, want %q", started.ConfigPath, configPath)
	}
}

func TestHandleConfigRestartStatusConsumesManualRestartRequired(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/restart/status", nil)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got apitypes.ConfigRestartStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.ManualRestartRequired {
		t.Fatalf("ManualRestartRequired = false, want true")
	}
}

func TestHandleConfigSettingsGetPut(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.toml"
	writeMinimalAPIConfig(t, configPath)

	srv := &Handler{}
	srv.SetConfigPath(configPath)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/settings", nil)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET settings status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got apitypes.ConfigSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode GET settings response: %v", err)
	}
	if got.Path != configPath || got.ListenAddr == "" {
		t.Fatalf("GET settings = %+v, want populated fields", got)
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

	body, err := json.Marshal(apitypes.UpdateConfigSettingsRequest{
		ListenAddr:             "127.0.0.1:19080",
		AdvertiseBaseURL:       "http://192.168.1.10:19080/",
		ShowUpgrade:            false,
		SandboxProvider:        "docker",
		DefaultManagerTemplate: "builtin.picoclaw-manager",
		DefaultWorkerTemplate:  "builtin.picoclaw-worker",
	})
	if err != nil {
		t.Fatalf("marshal PUT body: %v", err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/config/settings", bytes.NewReader(body))
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT settings status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var saved apitypes.ConfigSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&saved); err != nil {
		t.Fatalf("decode PUT settings response: %v", err)
	}
	if saved.ListenAddr != "127.0.0.1:19080" || saved.ShowUpgrade {
		t.Fatalf("PUT settings = %+v, want updated listen_addr and show_upgrade=false", saved)
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
}

func TestHandleConfigSettingsRejectsInvalidBootstrapBeforeSave(t *testing.T) {
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
		DefaultManagerTemplate: "builtin.openclaw-manager",
		DefaultWorkerTemplate:  "builtin.picoclaw-worker",
	})
	if err != nil {
		t.Fatalf("marshal PUT body: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/config/settings", bytes.NewReader(body))
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT settings status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported runtime_kind") {
		t.Fatalf("body = %q, want bootstrap runtime validation error", rec.Body.String())
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != string(original) {
		t.Fatalf("config content changed after rejected PUT:\n%s", string(data))
	}
}

func TestHandleConfigSettingsValidatesBootstrapWithHubBeforeSave(t *testing.T) {
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
		DefaultManagerTemplate: "builtin.picoclaw-manager",
		DefaultWorkerTemplate:  "builtin.picoclaw-worker",
	})
	if err != nil {
		t.Fatalf("marshal PUT body: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/config/settings", bytes.NewReader(body))
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT settings status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
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
