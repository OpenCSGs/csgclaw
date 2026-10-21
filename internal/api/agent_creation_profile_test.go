package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	hub "csgclaw/internal/template"
)

func TestCreateParticipantPreservesInstructionsAndSkillSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		skills []string
	}{
		{name: "selected", skills: []string{"skill-creator"}},
		{name: "empty", skills: []string{}},
		{name: "omitted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agentSvc, _ := mustNewSeededServiceWithPath(t, nil)
			t.Cleanup(func() { _ = agentSvc.Close() })
			agentSvc.Models().SetLLMConfig(config.LLMConfig{Default: "service.chat", Providers: map[string]config.ProviderConfig{
				"service": {BaseURL: "https://model.example/v1", APIKey: "test-key", Models: []string{"chat"}},
			}})
			requestHub := mustNewLocalTemplateHubService(t, "worker", hub.Template{
				ID: "worker", Name: "worker", Role: hub.TemplateRoleWorker, RuntimeKind: agent.RuntimeNameCodex,
			})
			if err := requestHub.WriteWorkspaceFile(context.Background(), "local.worker", "instructions/AGENTS.md", "# Template instructions\n"); err != nil {
				t.Fatal(err)
			}
			imSvc := im.NewService()
			engine := agentengine.New(agentSvc)
			handler := &Handler{
				svc: agentSvc, hub: requestHub, im: imSvc, agentEngine: engine,
				workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
				participant: participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(engine), participant.WithIMService(imSvc)),
			}
			spec := map[string]any{
				"name": "reviewer", "role": "worker", "from_template": "local.worker",
				"instructions": "Review carefully.\nUse targeted tests.",
				"model_config": map[string]any{"model_provider_id": "service", "model_id": "chat"},
			}
			if tc.skills != nil {
				spec["skills"] = tc.skills
			}
			body, err := json.Marshal(map[string]any{
				"id": "reviewer", "name": "reviewer", "type": "agent",
				"agent_binding": map[string]any{"mode": "create", "agent": spec},
			})
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants", strings.NewReader(string(body))))
			if rec.Code != http.StatusCreated {
				t.Fatalf("create = %d: %s", rec.Code, rec.Body)
			}
			var created apitypes.Participant
			if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
			got, err := engine.Agents().Get(context.Background(), created.AgentID, agentengine.AgentGetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Spec.Instructions != spec["instructions"] {
				t.Fatalf("instructions = %q, want %q", got.Spec.Instructions, spec["instructions"])
			}
			if tc.skills == nil {
				if !slices.Contains(got.Spec.Skills, "skill-creator") || !slices.Contains(got.Spec.Skills, "skill-installer") {
					t.Fatalf("omitted skills lost defaults: %v", got.Spec.Skills)
				}
			} else if !slices.Equal(got.Spec.Skills, tc.skills) {
				t.Fatalf("skills = %v, want %v", got.Spec.Skills, tc.skills)
			}
		})
	}
}
