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

type sessionModelsFixture struct{ AgentModels }

func (sessionModelsFixture) ResolvedSessionProfile(_ string, model, effort string) (agent.AgentProfile, error) {
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
	h.agentModels = sessionModelsFixture{AgentModels: h.agentModels}
	for _, test := range []struct{ body, field string }{
		{`{"input":"hello","model":"missing","stream":true}`, "model"},
		{`{"input":"hello","model":"model","reasoning_effort":"invalid"}`, "reasoning_effort"},
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
