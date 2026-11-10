package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
)

func TestSessionReasoningCapabilitiesRejectBeforeExecution(t *testing.T) {
	var turns []agentengine.TurnRequest
	engine := &fakeSessionEngine{run: func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		turns = append(turns, request)
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "ok"}
	}}
	item := sessionCodexAgent("agent-alpha", "Alpha")
	item.AgentProfile = agent.AgentProfile{Provider: agent.ProviderCodex, ModelProviderID: agent.ModelProviderIDCodex, ModelID: "gpt-5.5", ReasoningEffort: "max"}
	handler, _, bindings, _ := newAgentSessionTestHandler(t, []agent.Agent{item}, engine, "")
	models := handler.agentModels.(*agent.ModelConfiguration)
	models.SetLLMConfig(config.LLMConfig{Default: "codex.gpt-5.5", Providers: map[string]config.ProviderConfig{
		agent.ModelProviderIDCodex: {Models: []string{"gpt-5.5", "gpt-6.1-sol"}},
	}})
	before, err := models.ResolvedAgentProfile(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	checkRejected := func(recorder *httptest.ResponseRecorder) {
		t.Helper()
		var response agentSessionErrorEnvelope
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusBadRequest || response.Error.Code != "invalid_request" || response.Error.Param == nil || *response.Error.Param != "reasoning_effort" || len(turns) != 0 || len(bindings.Bindings()) != 0 {
			t.Fatalf("validation: HTTP %d body=%s turns=%d bindings=%d", recorder.Code, recorder.Body.String(), len(turns), len(bindings.Bindings()))
		}
	}
	for _, stream := range []bool{false, true} {
		checkRejected(performAgentSessionRequest(t, handler, item.ID, "invalid", map[string]any{"input": "hello", "model": "gpt-5.5", "reasoning_effort": "max", "stream": stream}))
	}
	multipart := sessionMultipartForm(t, map[string][]string{"input": {`"hello"`}, "model": {"gpt-6.1-sol"}, "reasoning_effort": {"minimal"}}, map[string][]sessionUploadFixture{"file": {{"note.txt", "text/plain", "notes"}}})
	defer cleanupMessagePayload(multipart)
	multipart.Header.Set("Authorization", "Bearer test-server-token")
	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, multipart)
	checkRejected(recorder)
	for _, test := range []struct {
		body map[string]any
		want string
	}{
		{map[string]any{"input": "hello", "model": "gpt-6.1-sol", "reasoning_effort": "max"}, "max"},
		{map[string]any{"input": "hello", "reasoning_effort": "auto"}, "auto"},
		{map[string]any{"input": "hello", "model": "gpt-5.5"}, "max"},
		{map[string]any{"input": "hello"}, ""},
	} {
		recorder := performAgentSessionRequest(t, handler, item.ID, "valid", test.body)
		if recorder.Code != http.StatusOK {
			t.Fatalf("accepted selection: HTTP %d body=%s", recorder.Code, recorder.Body.String())
		}
		selected := turns[len(turns)-1].Model
		if test.want == "" {
			if selected != nil {
				t.Fatalf("omitted options supplied selection %+v", selected)
			}
		} else if selected == nil || selected.ReasoningEffort != test.want {
			t.Fatalf("selection = %+v, want effort %q", selected, test.want)
		}
	}
	after, err := models.ResolvedAgentProfile(item.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("request validation changed Profile defaults", err)
	}
}
