package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/llm"
	hub "csgclaw/internal/template"
)

type interpretationTemplateStore struct{ hub.Store }

func (interpretationTemplateStore) Get(_ context.Context, id string) (hub.Template, error) {
	return hub.Template{ID: id, Name: "Reviewer", Description: "Original description"}, nil
}

func (interpretationTemplateStore) ListWorkspace(_ context.Context, id, path string) (apitypes.WorkspaceListing, error) {
	root := "skills"
	if strings.HasSuffix(id, "/legacy-reviewer") {
		root = "workspace/skills"
	}
	switch path {
	case root:
		return apitypes.WorkspaceListing{Entries: []apitypes.WorkspaceEntry{
			{Name: "review-skill", Type: "dir"},
			{Name: "assets-only", Type: "dir"},
			{Name: "directory-manifest", Type: "dir"},
			{Name: "README.md", Type: "file"},
			{Name: ".skillhub-lock.json", Type: "file"},
		}}, nil
	case root + "/review-skill":
		return apitypes.WorkspaceListing{Entries: []apitypes.WorkspaceEntry{{Name: "SKILL.md", Type: "file"}}}, nil
	case root + "/assets-only":
		return apitypes.WorkspaceListing{Entries: []apitypes.WorkspaceEntry{{Name: "examples", Type: "dir"}}}, nil
	case root + "/directory-manifest":
		return apitypes.WorkspaceListing{Entries: []apitypes.WorkspaceEntry{{Name: "SKILL.md", Type: "dir"}}}, nil
	default:
		return apitypes.WorkspaceListing{}, hub.ErrTemplateNotFound
	}
}

func (interpretationTemplateStore) ReadWorkspaceFile(_ context.Context, _ string, path string) (apitypes.WorkspaceFile, error) {
	if path == "mcps/mcp.json" {
		return apitypes.WorkspaceFile{Content: `{"mcpServers":{"review-connector":{"url":"https://private.example","headers":{"Authorization":"test-private-key"}},"disabled-connector":{"enabled":false}}}`}, nil
	}
	return apitypes.WorkspaceFile{Content: "Review documents for clarity."}, nil
}

type interpretationProfiles struct {
	t       *testing.T
	baseURL string
}

func (p interpretationProfiles) ResolvedAgentProfile(id string) (agent.AgentProfile, error) {
	p.t.Helper()
	if id != agent.ManagerUserID {
		p.t.Errorf("model resolved for %q, want Manager", id)
	}
	return agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: p.baseURL, ModelID: "manager-model"}, nil
}

func TestInterpretHubTemplateUsesManagerOnlyForOfficialTemplates(t *testing.T) {
	var calls atomic.Int32
	var unavailable atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Model    string
			Messages []map[string]string
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "manager-model" || len(request.Messages) != 2 ||
			!strings.Contains(request.Messages[1]["content"], "Review documents for clarity.") ||
			!strings.Contains(request.Messages[1]["content"], "review-skill") ||
			strings.Contains(request.Messages[1]["content"], "assets-only") ||
			strings.Contains(request.Messages[1]["content"], "directory-manifest") ||
			strings.Contains(request.Messages[1]["content"], "README.md") ||
			strings.Contains(request.Messages[1]["content"], ".skillhub-lock.json") ||
			!strings.Contains(request.Messages[1]["content"], "review-connector") ||
			strings.Contains(request.Messages[1]["content"], "disabled-connector") ||
			strings.Contains(request.Messages[1]["content"], "test-private-key") ||
			strings.Contains(request.Messages[1]["content"], "private.example") ||
			!strings.Contains(request.Messages[1]["content"], "Simplified Chinese") {
			t.Errorf("unexpected model request: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		if unavailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":"model_unavailable","message":"Model unavailable"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"- **文档审阅**：检查表达是否清晰。"}}]}`))
	}))
	defer upstream.Close()
	handler := &Handler{serverAccessToken: "test-server-token", llm: llm.NewService(config.ModelConfig{}, interpretationProfiles{t, upstream.URL})}
	service, err := hub.NewService(config.HubConfig{
		DefaultRegistry: "local",
		Registries: []config.HubRegistryConfig{
			{Name: "local", Kind: hub.RegistryKindLocal, Path: t.TempDir(), Enabled: true},
			{Name: "official", Kind: hub.RegistryKindRemote, Enabled: true},
		},
	}, func(config.HubRegistryConfig) (hub.Store, error) { return interpretationTemplateStore{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	handler.SetHubService(service)
	anonymous := httptest.NewRecorder()
	handler.Routes().ServeHTTP(anonymous, httptest.NewRequest(http.MethodPost,
		"/api/v1/hub/templates/agentichub~s~reviewer/description:interpret", strings.NewReader(`{"locale":"zh"}`)))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", anonymous.Code)
	}
	for _, tc := range []struct {
		id     string
		status int
	}{
		{"local.Reviewer", http.StatusForbidden},
		{"alice~s~reviewer", http.StatusForbidden},
		{"agentichub~s~reviewer", http.StatusOK},
		{"agentichub~s~legacy-reviewer", http.StatusOK},
		{"agentichub~s~reviewer", http.StatusServiceUnavailable},
	} {
		unavailable.Store(tc.status == http.StatusServiceUnavailable)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/hub/templates/"+tc.id+"/description:interpret", strings.NewReader(`{"locale":"zh"}`))
		req.Header.Set("Authorization", "Bearer test-server-token")
		req.Header.Set("Content-Type", "application/json")
		handler.Routes().ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%s: status = %d, want %d: %s", tc.id, rec.Code, tc.status, rec.Body.String())
		}
		if unavailable.Load() && !strings.Contains(rec.Body.String(), "template_interpretation_model_unavailable") {
			t.Fatalf("missing model unavailable error: %s", rec.Body.String())
		}
		if tc.status == http.StatusOK {
			var result descriptionResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || !strings.Contains(result.Description, "**文档审阅**") {
				t.Fatalf("unexpected interpretation: %+v, %v", result, err)
			}
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("manager calls = %d, want 3", calls.Load())
	}
}
