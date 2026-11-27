package api

import (
	"context"
	"csgclaw/internal/config"
	"csgclaw/internal/llm"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	runtimeinstructions "csgclaw/internal/runtime/instructions"
)

func TestAgentDescriptionSourceUsesEnabledCapabilities(t *testing.T) {
	item := agentengine.Agent{
		Spec: agentengine.AgentSpec{
			Name:         "Research",
			Instructions: "Summarize articles",
			MCPServers: map[string]agentengine.MCPServerConfig{
				"search":          {"token": "private-token"},
				"disabled-server": {"enabled": false},
			},
		},
		Status: agentengine.AgentStatus{SkillSummaries: []agentengine.SkillSummary{
			{Enabled: true, Name: "citations", Description: "Format citations"},
			{Name: "disabled-skill"},
		}},
	}
	source, err := agentDescriptionSource(item, "English")
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err := json.Unmarshal(source, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["instructions"] != "Summarize articles" ||
		fields["enabled_skills"] != "citations Format citations" ||
		fields["enabled_mcp_servers"] != "search" {
		t.Fatalf("unexpected source: %s", source)
	}
	if strings.Contains(string(source), "private-token") {
		t.Fatal("source included MCP credentials")
	}
}

func TestAgentDescriptionUsesEffectiveInstructionsWithoutManagedBoilerplate(t *testing.T) {
	item := agentengine.Agent{
		Status: agentengine.AgentStatus{Instructions: &agentengine.InstructionsStatus{
			Effective: "Review customer visit plans.\n\n" + runtimeinstructions.RenderRuntimeAgentsInstructionsBlock("agent-worker", "Summarize public research."),
		}},
	}
	source, err := agentDescriptionSource(item, "English")
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err := json.Unmarshal(source, &fields); err != nil {
		t.Fatal(err)
	}
	got := fields["instructions"]
	if !strings.Contains(got, "Review customer visit plans.") || !strings.Contains(got, "Summarize public research.") ||
		strings.Contains(got, "Managed Runtime Instructions") || strings.Contains(got, "BEGIN CSGCLAW-INSTRUCTIONS") {
		t.Fatalf("unexpected effective instructions: %q", got)
	}
}

type agentDescriptionTestProfile struct {
	t       *testing.T
	baseURL string
}

func (p agentDescriptionTestProfile) ResolvedAgentProfile(id string) (agent.AgentProfile, error) {
	if id != "agent-research" {
		p.t.Errorf("model resolved for %q", id)
	}
	return agent.AgentProfile{Provider: agent.ProviderAPI, BaseURL: p.baseURL, ModelID: "research-model",
		RequestOptions: map[string]any{"tools": []string{"must-not-be-sent"}}}, nil
}

func TestGenerateAgentDescriptionUsesAgentModelWithoutTools(t *testing.T) {
	engine := &fakeSessionEngine{run: func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		t.Error("generation started a runtime session")
		return agentengine.TurnResult{Status: agentengine.TurnFailed}
	}}
	h, imSvc, _, _ := newAgentSessionTestHandler(t, []agent.Agent{{
		ID: "agent-research", Name: "research", Description: "Existing description",
		Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindDSH,
	}}, engine, "")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model    string
			Messages []map[string]string
			Tools    json.RawMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "research-model" || len(request.Tools) != 0 || len(request.Messages) != 2 ||
			!strings.Contains(request.Messages[1]["content"], "Simplified Chinese") {
			t.Errorf("unexpected model request: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"我可以整理研究资料。\n\n- **资料整理**：汇总要点。"}}]}`))
	}))
	defer upstream.Close()
	h.llm = llm.NewService(config.ModelConfig{}, agentDescriptionTestProfile{t, upstream.URL})

	router := h.Routes()
	path := "/api/v1/agents/agent-research/description:generate"
	for _, tc := range []struct {
		name, origin, host, peer, token, contentType string
		status                                       int
	}{
		{"no browser origin", "", "localhost:18080", "127.0.0.1:1234", "", "application/json", 401},
		{"cross origin", "http://other.example", "localhost:18080", "127.0.0.1:1234", "", "application/json", 401},
		{"remote caller", "http://localhost:18080", "localhost:18080", "192.0.2.1:1234", "", "application/json", 401},
		{"invalid credential", "http://localhost:18080", "localhost:18080", "127.0.0.1:1234", "Bearer wrong", "application/json", 401},
		{"non JSON", "http://localhost:18080", "localhost:18080", "127.0.0.1:1234", "", "text/plain", 415},
		{"local browser", "http://localhost:18080", "localhost:18080", "127.0.0.1:1234", "", "application/json", 200},
		{"management bearer", "", "localhost:18080", "192.0.2.1:1234", "Bearer test-server-token", "application/json", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"locale":"zh"}`))
			req.Host, req.RemoteAddr = tc.host, tc.peer
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Authorization", tc.token)
			req.Header.Set("Content-Type", tc.contentType)
			probe := httptest.NewRequest(http.MethodPost, "/api/v1/description-access", strings.NewReader(`{}`))
			probe.Host, probe.RemoteAddr, probe.Header = req.Host, req.RemoteAddr, req.Header.Clone()
			accessRec := httptest.NewRecorder()
			router.ServeHTTP(accessRec, probe)
			var access struct{ Allowed bool }
			if err := json.Unmarshal(accessRec.Body.Bytes(), &access); err != nil || accessRec.Code != http.StatusOK ||
				access.Allowed != (tc.status != http.StatusUnauthorized) {
				t.Fatalf("probe status = %d, body = %s, err = %v", accessRec.Code, accessRec.Body.String(), err)
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
			if tc.status == http.StatusOK {
				var response descriptionResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil ||
					response.Description != "我可以整理研究资料。\n\n- **资料整理**：汇总要点。" {
					t.Fatalf("generated description = %q, err = %v", response.Description, err)
				}
			}
		})
	}
	got, err := engine.Agents().Get(context.Background(), "agent-research", agentengine.AgentGetOptions{})
	if err != nil || got.Spec.Description != "Existing description" || len(imSvc.ListRooms()) != 0 {
		t.Fatalf("generation changed saved state: description=%q, err=%v", got.Spec.Description, err)
	}
}
