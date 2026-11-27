package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
)

type sessionModelsFixture struct {
	AgentModels
	profile agent.AgentProfile
}

func (f sessionModelsFixture) ResolvedSessionProfile(_ string, model, effort string) (agent.AgentProfile, error) {
	if model == "auto" {
		profile := f.profile
		profile.ModelID, profile.ReasoningEffort = model, effort
		return profile, nil
	}
	if effort == "invalid" {
		return agent.AgentProfile{}, &agent.SessionModelError{Field: "reasoning_effort", Message: "invalid reasoning effort"}
	}
	return agent.AgentProfile{}, &agent.SessionModelError{Field: "model", Message: "selected model is unavailable"}
}

func TestAgentSessionModelValidationPrecedesExecution(t *testing.T) {
	executed := false
	engine := &fakeSessionEngine{run: func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		executed = true
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
	}}
	h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	h.agentModels = sessionModelsFixture{AgentModels: h.agentModels, profile: agent.AgentProfile{Provider: agent.ProviderCSGHub}}
	for _, test := range []struct{ body, field string }{
		{`{"input":"hello","model":"missing","stream":true}`, "model"},
		{`{"input":"hello","model":"model","reasoning_effort":"invalid"}`, "reasoning_effort"},
		{`{"input":"hello","model":"physical","auto_profile":"quality"}`, "auto_profile"},
		{`{"input":"hello","model":"auto","auto_profile":"invalid id"}`, "auto_profile"},
		{`{"input":"hello","auto_profile":"quality"}`, "auto_profile"},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-alpha/sessions/session-one/responses", strings.NewReader(test.body))
		recorder := httptest.NewRecorder()
		h.Routes().ServeHTTP(recorder, request)
		var response agentSessionErrorEnvelope
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusBadRequest || response.Error.Param == nil || *response.Error.Param != test.field || executed {
			t.Fatalf("validation response: HTTP %d body=%s executed=%t", recorder.Code, recorder.Body.String(), executed)
		}
	}
}

func TestAgentSessionAutoProfileReachesNativeTurn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CSGHUB_AIGATEWAY_BASE_URL", "")
	t.Setenv("CSGHUB_AIGATEWAY_URL", "https://qa.opencsg.com/aigateway")
	for _, test := range []struct {
		name, provider, baseURL string
		wantStatus              int
	}{
		{"built-in", agent.ProviderCSGHub, "", http.StatusOK},
		{"custom staging", agent.ProviderAPI, "https://opencsg-stg.com/aigateway/v1", http.StatusOK},
		{"custom production", agent.ProviderAPI, "https://aigateway.opencsg.com/v1", http.StatusOK},
		{"custom production portal", agent.ProviderAPI, "https://opencsg.com/aigateway/v1", http.StatusOK},
		{"custom configured QA", agent.ProviderAPI, "https://qa.opencsg.com/aigateway/v1", http.StatusOK},
		{"another OpenCSG route", agent.ProviderAPI, "https://qa.opencsg.com/other/v1", http.StatusBadRequest},
		{"third-party", agent.ProviderAPI, "https://api.example/v1", http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			var model *agentengine.TurnModel
			engine := &fakeSessionEngine{run: func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
				model = request.Model
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "ok"}
			}}
			h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
			h.agentModels = sessionModelsFixture{
				AgentModels: h.agentModels,
				profile:     agent.AgentProfile{Provider: test.provider, BaseURL: test.baseURL},
			}
			response := performAgentSessionRequest(t, h, "agent-alpha", "s", map[string]any{
				"input": "hello", "model": "auto", "auto_profile": "custom-quality", "reasoning_effort": "low",
			})
			if response.Code != test.wantStatus {
				t.Fatalf("HTTP %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if test.wantStatus == http.StatusBadRequest {
				var failure agentSessionErrorEnvelope
				if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
					t.Fatal(err)
				}
				if failure.Error.Param == nil || *failure.Error.Param != "auto_profile" || model != nil {
					t.Fatalf("invalid profile reached execution or returned the wrong error: %s selection=%+v", response.Body.String(), model)
				}
				return
			}
			if model == nil || model.ModelID != "auto" || model.AutoProfile != "custom-quality" || model.ReasoningEffort != "low" {
				t.Fatalf("native turn model selection = %+v", model)
			}
		})
	}
}
