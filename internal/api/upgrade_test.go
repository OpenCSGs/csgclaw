package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/upgrade"
)

type stubUpgradeChecker struct {
	result upgrade.CheckResult
	err    error
}

type channelStubUpgradeChecker struct {
	channel upgrade.Channel
}

func (s *channelStubUpgradeChecker) SetChannel(channel upgrade.Channel) error {
	s.channel = channel
	return nil
}

func (s *channelStubUpgradeChecker) Check(_ context.Context, currentVersion string) (upgrade.CheckResult, error) {
	latest := "v0.4.6"
	if s.channel == upgrade.ChannelBeta {
		latest = "v0.4.7-beta.1"
	}
	return upgrade.CheckResult{CurrentVersion: currentVersion, LatestVersion: latest, UpdateAvailable: true}, nil
}

func (s stubUpgradeChecker) Check(context.Context, string) (upgrade.CheckResult, error) {
	if s.err != nil {
		return upgrade.CheckResult{}, s.err
	}
	return s.result, nil
}

func TestHandleUpgradeStatus(t *testing.T) {
	checkedAt := time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC)
	manager := upgrade.NewManager(stubUpgradeChecker{
		result: upgrade.CheckResult{
			CurrentVersion:  "v0.2.5",
			LatestVersion:   "v0.2.7",
			UpdateAvailable: true,
		},
	}, "v0.2.5", upgrade.ManagerOptions{
		Now: func() time.Time { return checkedAt },
	})
	manager.Refresh(context.Background())

	srv := &Handler{}
	srv.SetUpgradeManager(manager)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/upgrade/status", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got apitypes.UpgradeStatus
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.CurrentVersion != "v0.2.5" || got.LatestVersion != "v0.2.7" || !got.UpdateAvailable {
		t.Fatalf("upgrade status = %+v, want current/latest/update_available populated", got)
	}
	if got.LastCheckedAt == nil || !got.LastCheckedAt.Equal(checkedAt) {
		t.Fatalf("LastCheckedAt = %v, want %v", got.LastCheckedAt, checkedAt)
	}
}

func TestHandleUpgradeStatusConsumesManualRestartRequired(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.toml"
	artifacts, err := upgrade.ResolveApplyArtifacts(configPath)
	if err != nil {
		t.Fatalf("ResolveApplyArtifacts() error = %v", err)
	}
	if err := artifacts.RecordManualRestartRequired("manual restart required"); err != nil {
		t.Fatalf("RecordManualRestartRequired() error = %v", err)
	}

	manager := upgrade.NewManager(stubUpgradeChecker{err: errors.New("unused")}, "v0.2.5", upgrade.ManagerOptions{})
	srv := &Handler{}
	srv.SetUpgradeManager(manager)
	srv.SetUpgradeConfigPath(configPath)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/upgrade/status", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got apitypes.UpgradeStatus
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.ManualRestartRequired {
		t.Fatalf("ManualRestartRequired = false, want true")
	}
}

func TestHandleUpgradeStatusConsumesFailureMetadata(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.toml"
	artifacts, err := upgrade.ResolveApplyArtifacts(configPath)
	if err != nil {
		t.Fatalf("ResolveApplyArtifacts() error = %v", err)
	}
	if err := artifacts.RecordFailure(errors.New("write /tmp/csgclaw-upgrade/archive.tar.gz: stream error: stream ID 3")); err != nil {
		t.Fatalf("RecordFailure() error = %v", err)
	}

	manager := upgrade.NewManager(stubUpgradeChecker{err: errors.New("unused")}, "v0.2.5", upgrade.ManagerOptions{})
	srv := &Handler{}
	srv.SetUpgradeManager(manager)
	srv.SetUpgradeConfigPath(configPath)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/upgrade/status", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got apitypes.UpgradeStatus
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.LastErrorKind != upgrade.UpgradeErrorNetworkDownload {
		t.Fatalf("LastErrorKind = %q, want %q", got.LastErrorKind, upgrade.UpgradeErrorNetworkDownload)
	}
	if got.LastErrorLogPath != artifacts.LogPath {
		t.Fatalf("LastErrorLogPath = %q, want %q", got.LastErrorLogPath, artifacts.LogPath)
	}
}

func TestHandleUpgradeStatusServiceUnavailable(t *testing.T) {
	srv := &Handler{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/upgrade/status", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestHandleUpgradeStatusMethodNotAllowed(t *testing.T) {
	manager := upgrade.NewManager(stubUpgradeChecker{err: errors.New("unused")}, "v0.2.5", upgrade.ManagerOptions{})
	srv := &Handler{}
	srv.SetUpgradeManager(manager)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upgrade/status", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
}

func TestHandleUpgradeChannelPersistsAndRefreshes(t *testing.T) {
	configPath := t.TempDir() + "/config.toml"
	if err := os.WriteFile(configPath, []byte(`[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"
show_upgrade = true
upgrade_channel = "release"
`), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	checker := &channelStubUpgradeChecker{}
	manager := upgrade.NewManager(checker, "v0.4.6", upgrade.ManagerOptions{})
	srv := &Handler{}
	srv.SetUpgradeManager(manager)
	srv.SetConfigPath(configPath)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/upgrade/channel",
		bytes.NewBufferString(`{"channel":"beta"}`),
	)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got apitypes.UpgradeStatus
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Channel != "beta" || got.LatestVersion != "v0.4.7-beta.1" {
		t.Fatalf("status = %+v, want refreshed beta channel", got)
	}
	saved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	if !strings.Contains(string(saved), `upgrade_channel = "beta"`) {
		t.Fatalf("config missing beta channel:\n%s", saved)
	}
}

func TestHandleUpgradeApply(t *testing.T) {
	manager := upgrade.NewManager(stubUpgradeChecker{err: errors.New("unused")}, "v0.2.5", upgrade.ManagerOptions{
		AutoUpgradeSupported: true,
	})
	srv := &Handler{}
	srv.SetUpgradeManager(manager)
	srv.SetUpgradeConfigPath("/tmp/csgclaw.toml")

	var got upgrade.ApplyHelperOptions
	srv.SetUpgradeApplyFunc(func(opts upgrade.ApplyHelperOptions) error {
		got = opts
		return nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upgrade/apply", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if got.ConfigPath != "/tmp/csgclaw.toml" {
		t.Fatalf("ApplyHelperOptions.ConfigPath = %q, want %q", got.ConfigPath, "/tmp/csgclaw.toml")
	}
	if got.Channel != "release" {
		t.Fatalf("ApplyHelperOptions.Channel = %q, want release", got.Channel)
	}
	if status := manager.Status(); !status.Upgrading {
		t.Fatalf("manager.Status().Upgrading = false, want true")
	}

	var body apitypes.UpgradeActionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "accepted" {
		t.Fatalf("Status = %q, want %q", body.Status, "accepted")
	}
}

func TestHandleUpgradeApplyRejectsNonOfficialInstall(t *testing.T) {
	manager := upgrade.NewManager(stubUpgradeChecker{err: errors.New("unused")}, "v0.2.5", upgrade.ManagerOptions{
		AutoUpgradeUnsupportedReason: "not_official_bundle",
	})
	srv := &Handler{}
	srv.SetUpgradeManager(manager)
	srv.SetUpgradeApplyFunc(func(upgrade.ApplyHelperOptions) error {
		t.Fatal("upgrade helper should not be started for non-official installs")
		return nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upgrade/apply", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if status := manager.Status(); status.Upgrading || status.LastError != "" {
		t.Fatalf("manager.Status() = %+v, want upgrading false and no last_error", status)
	}
}

func TestHandleUpgradeApplyServiceUnavailable(t *testing.T) {
	srv := &Handler{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upgrade/apply", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestHandleUpgradeApplyMethodNotAllowed(t *testing.T) {
	manager := upgrade.NewManager(stubUpgradeChecker{err: errors.New("unused")}, "v0.2.5", upgrade.ManagerOptions{})
	srv := &Handler{}
	srv.SetUpgradeManager(manager)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/upgrade/apply", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
}

func TestHandleUpgradeApplyHelperFailure(t *testing.T) {
	manager := upgrade.NewManager(stubUpgradeChecker{err: errors.New("unused")}, "v0.2.5", upgrade.ManagerOptions{
		AutoUpgradeSupported: true,
	})
	srv := &Handler{}
	srv.SetUpgradeManager(manager)
	srv.SetUpgradeApplyFunc(func(upgrade.ApplyHelperOptions) error {
		return errors.New("boom")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upgrade/apply", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if status := manager.Status(); status.Upgrading || status.LastError == "" {
		t.Fatalf("manager.Status() = %+v, want upgrading false and last_error populated", status)
	}
}

func TestIMEventsIncludesUpgradeStatus(t *testing.T) {
	bus := im.NewBus()
	srv := &Handler{imBus: bus}
	server := httptest.NewServer(srv.Routes())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("ReadString() handshake error = %v", err)
	}
	if got, want := line, ": connected\n"; got != want {
		t.Fatalf("handshake line = %q, want %q", got, want)
	}
	if line, err = reader.ReadString('\n'); err != nil {
		t.Fatalf("ReadString() handshake separator error = %v", err)
	} else if got, want := line, "\n"; got != want {
		t.Fatalf("handshake separator = %q, want %q", got, want)
	}

	checkedAt := time.Date(2026, 5, 6, 14, 30, 0, 0, time.UTC)
	bus.Publish(im.Event{
		Type: im.EventTypeUpgradeStatusChanged,
		Upgrade: &apitypes.UpgradeStatus{
			CurrentVersion:  "v0.2.5",
			LatestVersion:   "v0.2.7",
			UpdateAvailable: true,
			LastCheckedAt:   &checkedAt,
		},
	})

	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("ReadString() event line error = %v", err)
	}
	const prefix = "data: "
	if len(line) < len(prefix) || line[:len(prefix)] != prefix {
		t.Fatalf("event line = %q, want %q prefix", line, prefix)
	}

	var got struct {
		Type    string                  `json:"type"`
		Upgrade *apitypes.UpgradeStatus `json:"upgrade"`
	}
	if err := json.Unmarshal([]byte(line[len(prefix):]), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v; line=%q", err, line)
	}
	if got.Type != im.EventTypeUpgradeStatusChanged {
		t.Fatalf("Type = %q, want %q", got.Type, im.EventTypeUpgradeStatusChanged)
	}
	if got.Upgrade == nil {
		t.Fatal("Upgrade = nil, want payload")
	}
	if got.Upgrade.LatestVersion != "v0.2.7" || !got.Upgrade.UpdateAvailable {
		t.Fatalf("Upgrade = %+v, want latest_version and update_available populated", got.Upgrade)
	}
}
