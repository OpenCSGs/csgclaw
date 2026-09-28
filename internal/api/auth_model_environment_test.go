package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/auth"
	"csgclaw/internal/config"
	"csgclaw/internal/modelprovider"
	agentruntime "csgclaw/internal/runtime"
	codexruntime "csgclaw/internal/runtime/codex"
	hub "csgclaw/internal/template"
)

// Keep the real Codex configuration validator and HTTP probe while avoiding
// starting a model process in this login/template-creation integration test.
type environmentProbeRuntime struct{ fakeCompatRuntime }

func (environmentProbeRuntime) ValidateConfig(ctx context.Context, current agentruntime.RuntimeConfigSnapshot) error {
	return (&codexruntime.Runtime{}).ValidateConfig(ctx, current)
}
func (environmentProbeRuntime) RestartRequired(agentruntime.RuntimeConfigChange) (bool, error) {
	return false, nil
}
func (environmentProbeRuntime) ReconcileConfig(context.Context, agentruntime.Handle, agentruntime.RuntimeConfigChange) error {
	return nil
}

func TestAuthEnvironmentSwitchTemplateCreation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(codexruntime.TestOnlySetResponsesAPIProbe(modelprovider.CheckResponsesOrChatCompletionsAPI))
	model := "production-model"
	var probed []string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/v1/models") {
			fmt.Fprintf(w, `{"data":[{"id":%q}]}`, model)
			return
		}
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode probe: %v", err)
		}
		probed = append(probed, payload.Model)
		if payload.Model != model {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"code":"invalid_request_error","message":"model not found in this environment"}}`)
			return
		}
		fmt.Fprint(w, `{"id":"probe","object":"response","status":"completed","output":[]}`)
	}))
	defer gateway.Close()
	store, err := auth.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	login := func() {
		t.Helper()
		if err := store.SaveCSGHubProviderCredentials(auth.CSGHubProviderCredentials{
			AIGatewayBaseURL: gateway.URL + "/" + model + "/v1", AIGatewayBuiltinAPIKey: "gk_test_" + model,
		}); err != nil {
			t.Fatal(err)
		}
	}
	login()
	configPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := config.Config{Models: config.LLMConfig{
		Default:   "opencsg.production-model",
		Providers: map[string]config.ProviderConfig{"opencsg": {Models: []string{model}}},
	}}
	registryRoot := t.TempDir()
	cfg.Hub = config.HubConfig{DefaultRegistry: "local", Registries: []config.HubRegistryConfig{{Name: "local", Kind: hub.RegistryKindLocal, Path: registryRoot, Enabled: true}}}
	if _, err := hub.NewLocalStore(registryRoot).Publish(context.Background(), hub.PublishSpec{ID: "resume-scorer", Name: "resume-scorer", RuntimeKind: agent.RuntimeKindCodex}); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(configPath); err != nil {
		t.Fatal(err)
	}
	svc, err := agent.NewControllerWithLLM(cfg.Models, cfg.Server, "", filepath.Join(t.TempDir(), "state.json"),
		agent.WithRuntime(environmentProbeRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}}))
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	h := &Handler{svc: svc, agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc}
	h.SetConfigPath(configPath)
	t.Cleanup(stubAuthCallback(func(*http.Request, string) (string, error) { login(); return "/", nil }))
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	for i, current := range []string{"production-model", "staging-model"} {
		model = current
		rec := request(http.MethodGet, "/api/v1/auth/callback", "")
		if rec.Code != http.StatusFound {
			t.Fatalf("callback: %d %s", rec.Code, rec.Body.String())
		}
		defaults := request(http.MethodGet, "/api/v1/agent-profile-defaults", "")
		var profile agent.AgentProfile
		if err := json.Unmarshal(defaults.Body.Bytes(), &profile); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			profile = agent.AgentProfile{ModelProviderID: "opencsg", ModelID: model}
		}
		t.Logf("environment=%s defaults=%s", model, defaults.Body.String())
		body, _ := json.Marshal(map[string]any{"name": fmt.Sprintf("scorer-%d", i), "from_template": "local.resume-scorer", "agent_profile": profile})
		rec = request(http.MethodPost, "/api/v1/agents", string(body))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create in %s: status=%d body=%s probes=%v", model, rec.Code, rec.Body.String(), probed)
		}
		if profile.ModelID != model {
			t.Fatalf("default model=%q, want %q", profile.ModelID, model)
		}
	}
}
