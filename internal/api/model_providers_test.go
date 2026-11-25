package api

import (
	"context"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type modelProviderTestResponse struct {
	Providers []struct {
		ID          string   `json:"id"`
		Kind        string   `json:"kind"`
		DisplayName string   `json:"display_name"`
		Preset      string   `json:"preset"`
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
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw response: %v", err)
	}
	if _, ok := raw["default_selector"]; ok {
		t.Fatalf("catalog leaked default_selector: %s", rec.Body.String())
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
	if got.Providers[1].DisplayName != "CSGHub Lite" {
		t.Fatalf("CSGHub Lite display name = %q, want CSGHub Lite", got.Providers[1].DisplayName)
	}
	if got.Providers[4].Preset != "openai" {
		t.Fatalf("custom provider preset = %q, want openai", got.Providers[4].Preset)
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
		"preset":"openai",
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
	if got := cfgAfterCheck.Models.Providers["openai"].Preset; got != "openai" {
		t.Fatalf("preset after check = %q, want openai", got)
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

func TestModelProviderCheckClearsStaleModelsAndRecovers(t *testing.T) {
	var response atomic.Value
	response.Store(`{"data":[]}`)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected upstream request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(response.Load().(string)))
	}))
	defer upstream.Close()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	configText := `[models.providers.openai]
preset = "openai"
base_url = "` + upstream.URL + `/v1"
api_key = "test-key"
status = "failed"
models = ["retired-model"]
`
	if err := os.WriteFile(configPath, []byte(configText), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := newModelProviderTestHandler(t, configPath, nil)
	for _, want := range []string{"", "recovered-model"} {
		if want != "" {
			response.Store(`{"data":[{"id":"recovered-model"}]}`)
		}
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers/openai/check", strings.NewReader("{}")))
		var result agent.ModelProviderCheckResult
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil ||
			result.Status != agent.ModelProviderStatusConnected || strings.Join(result.Models, ",") != want {
			t.Fatalf("check = %d %s, want connected with models %q", rec.Code, rec.Body.String(), want)
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			t.Fatal(err)
		}
		provider := cfg.Models.Providers["openai"]
		if provider.Status != agent.ModelProviderStatusConnected || strings.Join(provider.Models, ",") != want {
			t.Fatalf("persisted status = %s, models = %v, want connected with %q", provider.Status, provider.Models, want)
		}
	}
}

func TestOpenCSGModelProviderCheckRecoversCatalogWithDisplayedGatewayURL(t *testing.T) {
	const gatewayURL = "https://platform.example/aigateway/v1"
	configPath := filepath.Join(t.TempDir(), "config.toml")
	writeCachedOpenCSGModelsConfig(t, configPath)
	handler := &Handler{}
	handler.SetConfigPath(configPath)
	routes := handler.Routes()

	t.Run("background check times out", func(t *testing.T) {
		t.Cleanup(stubModelProviderCheck(func(context.Context, agent.ModelProviderCheckInput) agent.ModelProviderCheckResult {
			return agent.ModelProviderCheckResult{
				Status:        agent.ModelProviderStatusFailed,
				Message:       "The model service request timed out. Please try again.",
				LastCheckedAt: "2026-11-19T00:42:58Z",
			}
		}))
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers/opencsg/check", strings.NewReader(`{}`)))
		if rec.Code != http.StatusOK {
			t.Fatalf("background check status = %d: %s", rec.Code, rec.Body.String())
		}
		provider := readOpenCSGProvider(t, handler)
		if provider.Status != agent.ModelProviderStatusFailed || strings.Join(provider.Models, ",") != "prod-model" {
			t.Fatalf("catalog after timeout: status = %q, models = %v", provider.Status, provider.Models)
		}
	})

	t.Cleanup(stubModelProviderCheck(func(_ context.Context, input agent.ModelProviderCheckInput) agent.ModelProviderCheckResult {
		if input.ID != agent.ModelProviderIDOpenCSG || !input.Verify || input.BaseURL != "" {
			t.Errorf("expected signed-in OpenCSG check, got ID = %q, verify = %t, request base URL = %q", input.ID, input.Verify, input.BaseURL)
		}
		return agent.ModelProviderCheckResult{
			Status:          agent.ModelProviderStatusConnected,
			ResolvedBaseURL: gatewayURL,
			Models:          []string{"recovered-model", "new-model"},
			LastCheckedAt:   "2026-11-19T09:04:12Z",
		}
	}))
	body := strings.NewReader(`{"connect":true,"base_url":"` + gatewayURL + `"}`)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers/opencsg/check", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("detail check status = %d: %s", rec.Code, rec.Body.String())
	}
	var checked modelProviderCheckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &checked); err != nil || checked.Provider == nil ||
		strings.Join(checked.Provider.Models, ",") != "recovered-model,new-model" {
		t.Fatalf("detail check did not return the saved provider: %s (error %v)", rec.Body.String(), err)
	}
	provider := readOpenCSGProvider(t, handler)
	if provider.Status != agent.ModelProviderStatusConnected || strings.Join(provider.Models, ",") != "recovered-model,new-model" ||
		provider.LastCheckedAt != "2026-11-19T09:04:12Z" {
		t.Fatalf("catalog after recovery: status = %q, models = %v, checked at = %q", provider.Status, provider.Models, provider.LastCheckedAt)
	}
}

func TestModelProviderCheckPreservesConcurrentCatalogUpdates(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	writeCachedOpenCSGModelsConfig(t, configPath)
	handler := &Handler{}
	handler.SetConfigPath(configPath)
	routes := handler.Routes()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/model-providers"+path, strings.NewReader(body)))
		return rec
	}
	if rec := request(http.MethodPost, "", `{"id":"remote","display_name":"Remote","models":["old-remote"]}`); rec.Code != http.StatusCreated {
		t.Fatalf("create remote: %d %s", rec.Code, rec.Body.String())
	}
	started := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(stubModelProviderCheck(func(_ context.Context, input agent.ModelProviderCheckInput) agent.ModelProviderCheckResult {
		if input.ID == "remote" {
			close(started)
			<-release
		}
		return agent.ModelProviderCheckResult{
			ID: input.ID, Status: agent.ModelProviderStatusConnected, Models: []string{"new-" + input.ID},
		}
	}))
	completed := make(chan *httptest.ResponseRecorder, 1)
	go func() { completed <- request(http.MethodPost, "/remote/check", `{}`) }()
	<-started
	opencsg := request(http.MethodPost, "/opencsg/check", `{}`)
	close(release)
	remote := <-completed
	if opencsg.Code != http.StatusOK || remote.Code != http.StatusOK {
		t.Fatalf("checks OpenCSG = %d %s, Remote = %d %s", opencsg.Code, opencsg.Body, remote.Code, remote.Body)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"opencsg", "remote"} {
		models := cfg.Models.Providers[id].Models
		if len(models) != 1 || models[0] != "new-"+id {
			t.Errorf("saved %s models = %v, want [new-%s]", id, models, id)
		}
	}
}

func TestModelProviderCreateKeepsWindowsHubRegistryPathStable(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

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
path = "C:\\Users\\dangw\\.csgclaw\\hub"
enabled = true

[models]
default = "openai.gpt-test"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	srv := newModelProviderTestHandler(t, configPath, nil)

	body := strings.NewReader(`{
		"id":"openai",
		"display_name":"Team OpenAI",
		"base_url":"https://api.openai.example/v1",
		"api_key":"sk-team",
		"models":["gpt-test"]
	}`)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	saved := string(data)
	if !strings.Contains(saved, `path = "C:\\Users\\dangw\\.csgclaw\\hub"`) {
		t.Fatalf("saved config missing canonical Windows path:\n%s", saved)
	}
	if strings.Contains(saved, `path = "C:\\\\Users\\\\dangw\\\\.csgclaw\\\\hub"`) {
		t.Fatalf("saved config double-escaped Windows path:\n%s", saved)
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

func TestModelProviderDeleteRejectsSelectorOnlyAgentReference(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

[models]
default = "codex.gpt-test"

[models.providers.openai]
base_url = "https://api.openai.example/v1"
api_key = "sk-team"
models = ["gpt-test"]
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	agentState := []agent.Agent{{
		ID: "agent-selector", Name: "selector", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		CreatedAt: time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC), Profile: "codex.gpt-test",
		AgentProfile: agent.AgentProfile{
			Provider: agent.ProviderCodex, ModelProviderID: "codex", ModelID: "gpt-test", ProfileComplete: true,
		},
		ProfileComplete: true,
	}}
	handler := newModelProviderTestHandler(t, configPath, agentState)
	patchRecorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(patchRecorder, httptest.NewRequest(http.MethodPatch, "/api/v1/agents/agent-selector", strings.NewReader(`{"profile":"openai.gpt-test"}`)))
	if patchRecorder.Code != http.StatusOK {
		t.Fatalf("selector PATCH status = %d, want %d; body=%s", patchRecorder.Code, http.StatusOK, patchRecorder.Body.String())
	}
	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/model-providers/openai", nil))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("DELETE selector-referenced provider status = %d, want %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
}

func TestModelProviderUpdateRejectsUnreachableTransportWhileProviderIsInUse(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

[models]
default = "openai.gpt-test"

[models.providers.openai]
base_url = "https://working.example/v1"
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
			ModelProviderID: "openai",
			ModelID:         "gpt-test",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}}
	srv := newModelProviderTestHandler(t, configPath, agentState)

	previousCheck := appCheckModelProvider
	appCheckModelProvider = func(context.Context, agent.ModelProviderCheckInput) agent.ModelProviderCheckResult {
		return agent.ModelProviderCheckResult{
			ID:            "openai",
			Status:        agent.ModelProviderStatusFailed,
			Message:       "request models: status 404 Not Found",
			LastCheckedAt: time.Now().UTC().Format(time.RFC3339),
		}
	}
	defer func() { appCheckModelProvider = previousCheck }()

	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"base_url":"https://wrong.example/v1"}`)
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/model-providers/openai", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "status 404") {
		t.Fatalf("PUT body = %q, want provider check failure", rec.Body.String())
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load(config) error = %v", err)
	}
	if got := cfg.Models.Providers["openai"].BaseURL; got != "https://working.example/v1" {
		t.Fatalf("provider base_url = %q, want previous working URL preserved", got)
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
	svc, err := agent.NewControllerWithLLM(cfg.Models, cfg.Server, "manager-image:test", statePath, agent.WithRuntime(modelProviderFakeRuntime{kind: agent.RuntimeKindCodex}))
	if err != nil {
		t.Fatalf("NewServiceWithLLM() error = %v", err)
	}
	srv := &Handler{svc: svc, agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc}
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
