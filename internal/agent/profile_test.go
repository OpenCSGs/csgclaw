package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/config"
	"csgclaw/internal/modelprovider"
)

func TestDetectDefaultProfileUsesCSGHubLiteWhenAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"qwen-test"}]}`))
	}))
	defer server.Close()

	restore := setProfileDetectionURLs(t, server.URL+"/v1", "http://127.0.0.1:1/v1")
	defer restore()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	profile, results := svc.DetectDefaultProfile(context.Background())
	if !profile.ProfileComplete || profile.Provider != ProviderCSGHubLite || profile.ModelID != "qwen-test" {
		t.Fatalf("DetectDefaultProfile() profile = %+v, want complete csghub_lite qwen-test; results=%+v", profile, results)
	}
}

func TestDetectDefaultProfileFallsBackToCodex(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-codex"}]}`))
	}))
	defer proxy.Close()

	restore := setProfileDetectionURLs(t, "http://127.0.0.1:1/v1", proxy.URL+"/v1")
	defer restore()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	profile, results := svc.DetectDefaultProfile(context.Background())
	if !profile.ProfileComplete || profile.Provider != ProviderCodex || profile.ModelID != "gpt-codex" {
		t.Fatalf("DetectDefaultProfile() profile = %+v, want complete codex gpt-codex; results=%+v", profile, results)
	}
}

func TestDetectDefaultProfileFallsBackToClaudeCode(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet"}]}`))
	}))
	defer proxy.Close()

	restore := setProfileDetectionURLs(t, "http://127.0.0.1:1/v1", "http://127.0.0.1:1/v1", proxy.URL+"/v1")
	defer restore()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	profile, results := svc.DetectDefaultProfile(context.Background())
	if !profile.ProfileComplete || profile.Provider != ProviderClaudeCode || profile.ModelID != "claude-sonnet" {
		t.Fatalf("DetectDefaultProfile() profile = %+v, want complete claude_code claude-sonnet; results=%+v", profile, results)
	}
}

func TestDetectDefaultProfileAllFailedReturnsIncompleteProfile(t *testing.T) {
	restore := setProfileDetectionURLs(t, "http://127.0.0.1:1/v1", "http://127.0.0.1:1/v1", "http://127.0.0.1:1/v1")
	defer restore()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	profile, results := svc.DetectDefaultProfile(context.Background())
	if profile.ProfileComplete {
		t.Fatalf("DetectDefaultProfile() profile = %+v, want incomplete", profile)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3: %+v", len(results), results)
	}
	for _, result := range results {
		if result.Status != "failed" || result.Error == "" {
			t.Fatalf("result = %+v, want failed with error", result)
		}
	}
}

func TestProfileDefaultsPersistAfterProfileUpdate(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:   ManagerUserID,
		Name: ManagerName,
		Role: RoleManager,
	}
	if _, err := svc.UpdateAgentProfile(ManagerUserID, AgentProfile{
		Name:            ManagerName,
		Provider:        ProviderCSGHubLite,
		ModelID:         "qwen-default",
		ReasoningEffort: "medium",
	}); err != nil {
		t.Fatalf("UpdateAgentProfile() error = %v", err)
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if state.ProfileDefaults.Provider != ProviderCSGHubLite || state.ProfileDefaults.ModelID != "qwen-default" {
		t.Fatalf("profile_defaults = %+v, want csghub_lite qwen-default", state.ProfileDefaults)
	}
}

func TestRedactedProfileViewIncludesSafeAPIKeyPreview(t *testing.T) {
	view := RedactedProfileView(AgentProfile{
		Name:   "alice",
		APIKey: "sk-live-secret-token",
	}, nil)
	if !view.APIKeySet {
		t.Fatalf("APIKeySet = false, want true")
	}
	if got, want := view.APIKeyPreview, "sk-l..."; got != want {
		t.Fatalf("APIKeyPreview = %q, want %q", got, want)
	}
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(data), "secret-token") {
		t.Fatalf("redacted view leaked API key: %s", string(data))
	}
	if strings.Contains(string(data), `"api_key"`) {
		t.Fatalf("redacted view includes api_key: %s", string(data))
	}
}

func TestRedactedProfileViewOmitsAPIKeyPreviewForShortKeys(t *testing.T) {
	view := RedactedProfileView(AgentProfile{
		Name:   "alice",
		APIKey: "local",
	}, nil)
	if !view.APIKeySet {
		t.Fatalf("APIKeySet = false, want true")
	}
	if view.APIKeyPreview != "" {
		t.Fatalf("APIKeyPreview = %q, want empty for short key", view.APIKeyPreview)
	}
}

func TestListModelsForRequestUsesCLIProxyChoicesForDropdown(t *testing.T) {
	oldChoices := listCLIProxyModelChoices
	defer func() {
		listCLIProxyModelChoices = oldChoices
	}()
	var gotProvider string
	listCLIProxyModelChoices = func(ctx context.Context, provider string) ([]string, error) {
		gotProvider = provider
		return []string{"gpt-5.4", "gpt-5.5", "gpt-4.1", "gpt-5.4-mini"}, nil
	}

	svc := &Service{}
	models, err := svc.ListModelsForRequest(context.Background(), ProfileModelRequest{Provider: ProviderCodex})
	if err != nil {
		t.Fatalf("ListModelsForRequest() error = %v", err)
	}
	if gotProvider != ProviderCodex {
		t.Fatalf("CLIProxy choices provider = %q, want %q", gotProvider, ProviderCodex)
	}
	want := []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-4.1"}
	if strings.Join(models, ",") != strings.Join(want, ",") {
		t.Fatalf("models = %v, want %v", models, want)
	}
}

func TestListModelsForRequestUsesStoredAgentAPIKeyForMatchingProfile(t *testing.T) {
	var authHeader string
	var gotHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		gotHeader = r.Header.Get("X-Test")
		_, _ = w.Write([]byte(`{"data":[{"id":"deepseek-chat"}]}`))
	}))
	defer upstream.Close()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:   "u-alice",
		Name: "alice",
		AgentProfile: normalizeProfile(AgentProfile{
			Name:     "alice",
			Provider: ProviderAPI,
			BaseURL:  upstream.URL + "/v1",
			APIKey:   "stored-key",
			Headers:  map[string]string{"X-Test": "stored"},
			ModelID:  "deepseek-chat",
		}, "alice", ""),
	}

	models, err := svc.ListModelsForRequest(context.Background(), ProfileModelRequest{
		AgentID:  "u-alice",
		Provider: ProviderAPI,
		BaseURL:  upstream.URL + "/v1",
		Headers:  map[string]string{"X-Test": "draft"},
	})
	if err != nil {
		t.Fatalf("ListModelsForRequest() error = %v", err)
	}
	if authHeader != "Bearer stored-key" {
		t.Fatalf("Authorization = %q, want stored key", authHeader)
	}
	if gotHeader != "draft" {
		t.Fatalf("X-Test header = %q, want draft header", gotHeader)
	}
	if got, want := strings.Join(models, ","), "deepseek-chat"; got != want {
		t.Fatalf("models = %v, want %s", models, want)
	}
}

func TestListModelsForRequestDoesNotReuseStoredAPIKeyForChangedBaseURL(t *testing.T) {
	var authHeader string
	otherUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer otherUpstream.Close()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:   "u-alice",
		Name: "alice",
		AgentProfile: normalizeProfile(AgentProfile{
			Name:     "alice",
			Provider: ProviderAPI,
			BaseURL:  "https://api.deepseek.com",
			APIKey:   "stored-key",
			ModelID:  "deepseek-chat",
		}, "alice", ""),
	}

	_, err = svc.ListModelsForRequest(context.Background(), ProfileModelRequest{
		AgentID:  "u-alice",
		Provider: ProviderAPI,
		BaseURL:  otherUpstream.URL + "/v1",
	})
	if err == nil {
		t.Fatal("ListModelsForRequest() error = nil, want upstream auth failure")
	}
	if authHeader != "" {
		t.Fatalf("Authorization sent to changed base URL = %q, want empty", authHeader)
	}
}

func TestSortModelIDsOrdersLatestKnownFamiliesFirst(t *testing.T) {
	got := sortModelIDs([]string{
		"misc-model",
		"gpt-5.4-mini",
		"gpt-4.1",
		"gpt-5.5",
		"Qwen/Qwen3-32B",
		"claude-sonnet-4.5",
		"gpt-5.4",
		"gpt-5.5",
	})
	want := []string{
		"gpt-5.5",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-4.1",
		"claude-sonnet-4.5",
		"Qwen/Qwen3-32B",
		"misc-model",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("sortModelIDs() = %v, want %v", got, want)
	}
}

func setProfileDetectionURLs(t *testing.T, csgHubLiteURL, cliProxyURL string, claudeCodeURL ...string) func() {
	t.Helper()
	oldCSGHubLite := defaultCSGHubLiteBaseURL
	oldListCLIProxyModels := listCLIProxyModels
	oldListCLIProxyModelChoices := listCLIProxyModelChoices
	claudeURL := cliProxyURL
	if len(claudeCodeURL) > 0 {
		claudeURL = claudeCodeURL[0]
	}
	defaultCSGHubLiteBaseURL = csgHubLiteURL
	listCLIProxyModels = func(ctx context.Context, provider string) ([]string, error) {
		switch provider {
		case ProviderCodex:
			return modelprovider.ListOpenAIModels(ctx, cliProxyURL, "local")
		case ProviderClaudeCode:
			return modelprovider.ListOpenAIModels(ctx, claudeURL, "local")
		default:
			return nil, nil
		}
	}
	return func() {
		defaultCSGHubLiteBaseURL = oldCSGHubLite
		listCLIProxyModels = oldListCLIProxyModels
		listCLIProxyModelChoices = oldListCLIProxyModelChoices
	}
}
