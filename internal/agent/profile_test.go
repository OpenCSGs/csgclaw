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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	profile, results := svc.DetectDefaultProfile(context.Background())
	if !profile.ProfileComplete || profile.Provider != ProviderCSGHubLite || profile.ModelID != "qwen-test" {
		t.Fatalf("DetectDefaultProfile() profile = %+v, want complete csghub_lite qwen-test; results=%+v", profile, results)
	}
}

func TestLoadMigratesInlineAPIProfileToCatalogProviderReference(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agents.json")
	agentState := []Agent{{
		ID:          "u-alice",
		Name:        "alice",
		Role:        RoleWorker,
		RuntimeKind: RuntimeKindCodex,
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderAPI,
			BaseURL:         "https://api.openai.example/v1",
			APIKey:          "sk-team",
			Headers:         map[string]string{"X-CSG-Trace": "dev"},
			ModelID:         "gpt-test",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}}
	data, err := json.Marshal(map[string]any{"agents": agentState})
	if err != nil {
		t.Fatalf("Marshal(state) error = %v", err)
	}
	if err := os.WriteFile(statePath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile(state) error = %v", err)
	}

	svc, err := NewServiceWithLLM(config.LLMConfig{
		Default: "openai.gpt-test",
		Providers: map[string]config.ProviderConfig{
			"openai": {
				BaseURL: "https://api.openai.example/v1",
				APIKey:  "sk-team",
				Headers: map[string]string{"X-CSG-Trace": "dev"},
				Models:  []string{"gpt-test"},
			},
		},
	}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewServiceWithLLM() error = %v", err)
	}

	got, ok := svc.Agent("u-alice")
	if !ok {
		t.Fatal("Agent(u-alice) missing")
	}
	if got.Profile != "openai.gpt-test" {
		t.Fatalf("Agent().Profile = %q, want openai.gpt-test", got.Profile)
	}
	if got.AgentProfile.ModelProviderID != "openai" {
		t.Fatalf("AgentProfile.ModelProviderID = %q, want openai", got.AgentProfile.ModelProviderID)
	}
	if got.AgentProfile.BaseURL != "" || got.AgentProfile.APIKey != "" || len(got.AgentProfile.Headers) != 0 {
		t.Fatalf("stored AgentProfile kept inline credentials: %+v", got.AgentProfile)
	}
	resolved, err := svc.ResolvedAgentProfile("u-alice")
	if err != nil {
		t.Fatalf("ResolvedAgentProfile() error = %v", err)
	}
	if resolved.BaseURL != "https://api.openai.example/v1" || resolved.APIKey != "sk-team" || resolved.Headers["X-CSG-Trace"] != "dev" {
		t.Fatalf("ResolvedAgentProfile() = %+v, want catalog credentials", resolved)
	}
}

func TestDetectDefaultProfileFallsBackToCodex(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-codex"}]}`))
	}))
	defer proxy.Close()

	restore := setProfileDetectionURLs(t, "http://127.0.0.1:1/v1", proxy.URL+"/v1")
	defer restore()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
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

func TestManagerStartupProfileSkipsDetectionWhenDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"qwen-test"}]}`))
	}))
	defer server.Close()

	restore := setProfileDetectionURLs(t, server.URL+"/v1", server.URL+"/v1")
	defer restore()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "", WithStartupProfileDetectionDisabled())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	profile, results := svc.managerStartupProfile(context.Background())
	if profile.ProfileComplete {
		t.Fatalf("managerStartupProfile() profile = %+v, want incomplete when detection is disabled", profile)
	}
	if profile.Provider != ProviderCSGHubLite {
		t.Fatalf("managerStartupProfile() provider = %q, want %q", profile.Provider, ProviderCSGHubLite)
	}
	if len(results) != 0 {
		t.Fatalf("managerStartupProfile() results = %+v, want none", results)
	}
}

func TestProfileDefaultsPersistAfterProfileUpdate(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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

func TestListModelsForRequestUsesCSGHubCredentials(t *testing.T) {
	var authHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"id":"deepseek-v4-flash"}]}`))
	}))
	defer upstream.Close()

	oldCredentials := defaultCSGHubCredentials
	defer func() {
		defaultCSGHubCredentials = oldCredentials
	}()
	defaultCSGHubCredentials = func(context.Context, *http.Client) (string, string, bool, error) {
		return upstream.URL + "/v1", "gk_aigateway-key", true, nil
	}

	svc := &Service{}
	models, err := svc.ListModelsForRequest(context.Background(), ProfileModelRequest{Provider: ProviderCSGHub})
	if err != nil {
		t.Fatalf("ListModelsForRequest() error = %v", err)
	}
	if authHeader != "Bearer gk_aigateway-key" {
		t.Fatalf("Authorization = %q, want aigateway key", authHeader)
	}
	if got, want := strings.Join(models, ","), "deepseek-v4-flash"; got != want {
		t.Fatalf("models = %v, want %s", models, want)
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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
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

func TestListModelsForRequestUsesDefaultAPIKeyForMatchingProfile(t *testing.T) {
	var authHeader string
	var gotHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		gotHeader = r.Header.Get("X-Test")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-default"}]}`))
	}))
	defer upstream.Close()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.profileDefaults = normalizeProfile(AgentProfile{
		Name:     ManagerName,
		Provider: ProviderAPI,
		BaseURL:  upstream.URL + "/v1",
		APIKey:   "default-key",
		Headers:  map[string]string{"X-Test": "stored"},
		ModelID:  "gpt-default",
	}, ManagerName, "")

	models, err := svc.ListModelsForRequest(context.Background(), ProfileModelRequest{
		Provider: ProviderAPI,
		BaseURL:  upstream.URL + "/v1",
		Headers:  map[string]string{"X-Test": "draft"},
	})
	if err != nil {
		t.Fatalf("ListModelsForRequest() error = %v", err)
	}
	if authHeader != "Bearer default-key" {
		t.Fatalf("Authorization = %q, want default key", authHeader)
	}
	if gotHeader != "draft" {
		t.Fatalf("X-Test header = %q, want draft header", gotHeader)
	}
	if got, want := strings.Join(models, ","), "gpt-default"; got != want {
		t.Fatalf("models = %v, want %s", models, want)
	}
}

func TestProfileForCreateRequestUsesDefaultAPIKeyWithoutReplacingSelectedModel(t *testing.T) {
	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.profileDefaults = normalizeProfile(AgentProfile{
		Name:     ManagerName,
		Provider: ProviderAPI,
		BaseURL:  "https://api.example/v1",
		APIKey:   "default-key",
		ModelID:  "gpt-default",
	}, ManagerName, "")

	profile, err := svc.profileForCreateRequest(context.Background(), &CreateAgentSpec{
		Name:        "alice",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		AgentProfile: AgentProfile{
			Provider: ProviderAPI,
			BaseURL:  "https://api.example/v1",
			ModelID:  "gpt-selected",
		},
	})
	if err != nil {
		t.Fatalf("profileForCreateRequest() error = %v", err)
	}
	if got, want := profile.APIKey, "default-key"; got != want {
		t.Fatalf("APIKey = %q, want %q", got, want)
	}
	if got, want := profile.ModelID, "gpt-selected"; got != want {
		t.Fatalf("ModelID = %q, want %q", got, want)
	}
}

func TestListModelsForRequestDoesNotReuseStoredAPIKeyForChangedBaseURL(t *testing.T) {
	var authHeader string
	otherUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer otherUpstream.Close()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
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
