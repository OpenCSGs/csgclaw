package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

type modelProviderTestResponse struct {
	Providers []struct {
		ID          string   `json:"id"`
		Kind        string   `json:"kind"`
		DisplayName string   `json:"display_name"`
		Builtin     bool     `json:"builtin"`
		BaseURL     string   `json:"base_url"`
		APIKey      string   `json:"api_key"`
		APIKeySet   bool     `json:"api_key_set"`
		Models      []string `json:"models"`
		Status      string   `json:"status"`
		Message     string   `json:"message"`
	} `json:"providers"`
}

func TestModelProviderCatalogListsBuiltinsBeforeCustomProviders(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

[models]
default = "openai.gpt-test"

[models.providers.openai]
display_name = "Team OpenAI"
base_url = "https://api.openai.example/v1"
api_key = "sk-team"
models = ["gpt-test"]
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	srv := newModelProviderTestHandler(t, configPath, nil)

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/model-providers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got modelProviderTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Providers) < 5 {
		t.Fatalf("providers len = %d, want at least 5: %+v", len(got.Providers), got.Providers)
	}
	wantOrder := []string{"opencsg", "csghub-lite", "codex", "claude_code", "openai"}
	for i, want := range wantOrder {
		if got.Providers[i].ID != want {
			t.Fatalf("providers[%d].id = %q, want %q; providers=%+v", i, got.Providers[i].ID, want, got.Providers)
		}
	}
	if got.Providers[0].Kind != "opencsg" {
		t.Fatalf("opencsg kind = %q, want opencsg", got.Providers[0].Kind)
	}
	if got.Providers[4].APIKey != "" {
		t.Fatalf("custom provider leaked api_key = %q", got.Providers[4].APIKey)
	}
	if !got.Providers[4].APIKeySet {
		t.Fatalf("custom provider api_key_set = false, want true")
	}
}

func TestModelProviderCatalogCreateCheckAndSaveCustomProvider(t *testing.T) {
	var sawAuth, sawHeader bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("upstream path = %q, want /v1/models", r.URL.Path)
		}
		sawAuth = r.Header.Get("Authorization") == "Bearer sk-live"
		sawHeader = r.Header.Get("X-CSG-Trace") == "dev"
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-live"},{"id":"gpt-live-mini"}]}`))
	}))
	defer upstream.Close()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	writeMinimalAPIConfig(t, configPath)
	srv := newModelProviderTestHandler(t, configPath, nil)

	createBody := strings.NewReader(`{
		"id":"openai",
		"display_name":"Team OpenAI",
		"base_url":"` + upstream.URL + `/v1",
		"api_key":"sk-live",
		"headers":{"X-CSG-Trace":"dev"},
		"models":["gpt-seed"]
	}`)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers", createBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-live") {
		t.Fatalf("create response leaked api key: %s", rec.Body.String())
	}

	checkBody := strings.NewReader(`{
		"base_url":"` + upstream.URL + `/v1",
		"api_key":"sk-live",
		"headers":{"X-CSG-Trace":"dev"}
	}`)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers/openai/check", checkBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("check status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !sawAuth || !sawHeader {
		t.Fatalf("upstream saw auth=%t header=%t, want both true", sawAuth, sawHeader)
	}
	if !strings.Contains(rec.Body.String(), `"status":"connected"`) ||
		!strings.Contains(rec.Body.String(), `"gpt-live-mini"`) {
		t.Fatalf("check response = %s, want connected status and discovered models", rec.Body.String())
	}
	cfgAfterCheck, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load(config after check) error = %v", err)
	}
	if got := strings.Join(cfgAfterCheck.Models.Providers["openai"].Models, ","); got != "gpt-live,gpt-live-mini" {
		t.Fatalf("models after check = %q, want discovered models persisted", got)
	}
	if got := cfgAfterCheck.Models.Providers["openai"].Status; got != agent.ModelProviderStatusConnected {
		t.Fatalf("status after check = %q, want connected", got)
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/model-providers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET after check status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var catalogAfterCheck modelProviderTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &catalogAfterCheck); err != nil {
		t.Fatalf("decode catalog after check: %v", err)
	}
	if got := catalogAfterCheck.Providers[len(catalogAfterCheck.Providers)-1].Status; got != agent.ModelProviderStatusConnected {
		t.Fatalf("catalog status after check = %q, want connected", got)
	}

	saveBody := strings.NewReader(`{
		"display_name":"Team OpenAI",
		"base_url":"` + upstream.URL + `/v1",
		"api_key":"sk-live",
		"headers":{"X-CSG-Trace":"dev"},
		"models":["gpt-live","gpt-live-mini"]
	}`)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/model-providers/openai", saveBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load(config) error = %v", err)
	}
	provider := cfg.Models.Providers["openai"]
	if got, want := provider.DisplayName, "Team OpenAI"; got != want {
		t.Fatalf("DisplayName = %q, want %q", got, want)
	}
	if got, want := provider.APIKey, "sk-live"; got != want {
		t.Fatalf("APIKey = %q, want %q", got, want)
	}
	if got, want := provider.Headers["X-CSG-Trace"], "dev"; got != want {
		t.Fatalf("Headers[X-CSG-Trace] = %q, want %q", got, want)
	}
}

func TestModelProviderCreateRejectsDuplicateDisplayName(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

[models]
default = "openai.gpt-test"

[models.providers.openai]
display_name = "Team OpenAI"
base_url = "https://api.openai.example/v1"
api_key = "sk-team"
models = ["gpt-test"]
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	srv := newModelProviderTestHandler(t, configPath, nil)

	body := strings.NewReader(`{"id":"deepseek","display_name":"Team OpenAI"}`)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers", body))
	if rec.Code != http.StatusConflict {
		t.Fatalf("POST status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestModelProviderCatalogKeepsBuiltinDisplayNamesCanonical(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

[models]
default = "codex.gpt-test"

[models.providers.codex]
display_name = "Custom Codex"
models = ["gpt-test"]
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	srv := newModelProviderTestHandler(t, configPath, nil)

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/model-providers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got modelProviderTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, provider := range got.Providers {
		if provider.ID == "codex" && provider.DisplayName != "Codex" {
			t.Fatalf("codex display_name = %q, want Codex", provider.DisplayName)
		}
	}
}

func TestModelProviderDeleteRejectsProviderReferencedByAgent(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

[models]
default = "openai.gpt-test"

[models.providers.openai]
base_url = "https://api.openai.example/v1"
api_key = "sk-team"
models = ["gpt-test"]
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	agentState := []agent.Agent{{
		ID:          "u-alice",
		Name:        "alice",
		Role:        agent.RoleWorker,
		RuntimeKind: agent.RuntimeKindCodex,
		CreatedAt:   time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		Profile:     "openai.gpt-test",
		AgentProfile: agent.AgentProfile{
			Name:            "alice",
			Provider:        agent.ProviderAPI,
			ModelID:         "gpt-test",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}}
	srv := newModelProviderTestHandler(t, configPath, agentState)

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/model-providers/openai", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("DELETE status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestAgentUpdateStoresCatalogModelSelector(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

[models]
default = "openai.gpt-test"

[models.providers.openai]
base_url = "https://api.openai.example/v1"
api_key = "sk-team"
models = ["gpt-test"]
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	agentState := []agent.Agent{{
		ID:          "u-alice",
		Name:        "alice",
		Role:        agent.RoleWorker,
		RuntimeKind: agent.RuntimeKindCodex,
		CreatedAt:   time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		AgentProfile: agent.AgentProfile{
			Name:            "alice",
			Provider:        agent.ProviderCSGHubLite,
			ModelID:         "qwen-old",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}}
	srv := newModelProviderTestHandler(t, configPath, agentState)

	body := strings.NewReader(`{
		"profile":"openai.gpt-test",
		"agent_profile":{
			"name":"alice",
			"provider":"api",
			"model_provider_id":"openai",
			"model_id":"gpt-test",
			"reasoning_effort":"high",
			"enable_fast_mode":true
		}
	}`)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/agents/u-alice", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"profile":"openai.gpt-test"`) {
		t.Fatalf("PATCH response = %s, want canonical profile selector", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"model_provider_id":"openai"`) {
		t.Fatalf("PATCH response = %s, want model_provider_id", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-team") {
		t.Fatalf("PATCH response leaked provider secret: %s", rec.Body.String())
	}
}

func newModelProviderTestHandler(t *testing.T, configPath string, agents []agent.Agent) *Handler {
	t.Helper()
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load(config) error = %v", err)
	}
	statePath := ""
	if len(agents) > 0 {
		statePath = filepath.Join(t.TempDir(), "agents.json")
		data, err := json.Marshal(map[string]any{"agents": agents})
		if err != nil {
			t.Fatalf("Marshal(agent state) error = %v", err)
		}
		if err := os.WriteFile(statePath, append(data, '\n'), 0o600); err != nil {
			t.Fatalf("WriteFile(agent state) error = %v", err)
		}
	}
	svc, err := agent.NewServiceWithLLM(cfg.Models, cfg.Server, "manager-image:test", statePath, agent.WithRuntime(modelProviderFakeRuntime{kind: agent.RuntimeKindCodex}))
	if err != nil {
		t.Fatalf("NewServiceWithLLM() error = %v", err)
	}
	srv := &Handler{svc: svc}
	srv.SetConfigPath(configPath)
	return srv
}

type modelProviderFakeRuntime struct {
	kind string
}

func (f modelProviderFakeRuntime) Kind() string {
	return f.kind
}

func (f modelProviderFakeRuntime) Layout(string) agentruntime.Layout {
	return agentruntime.Layout{}
}

func (f modelProviderFakeRuntime) New(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
	return agentruntime.Handle{RuntimeID: "runtime"}, nil
}

func (f modelProviderFakeRuntime) Start(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateRunning, nil
}

func (f modelProviderFakeRuntime) Stop(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateStopped, nil
}

func (f modelProviderFakeRuntime) Delete(context.Context, agentruntime.Handle) error {
	return nil
}

func (f modelProviderFakeRuntime) State(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateStopped, nil
}

func (f modelProviderFakeRuntime) Info(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
	return agentruntime.Info{State: agentruntime.StateStopped, CreatedAt: time.Now().UTC()}, nil
}
