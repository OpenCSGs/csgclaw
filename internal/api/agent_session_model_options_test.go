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
	"csgclaw/internal/config"
)

func TestParseAgentSessionModelOptionsJSON(t *testing.T) {
	for _, test := range []struct {
		name, body, model, effort, profile, wantError string
	}{
		{name: "defaults", body: `{"input":"hello"}`},
		{name: "empty defaults", body: `{"input":"hello","model":" ","reasoning_effort":" "}`},
		{name: "model", body: `{"input":"hello","model":" model-a "}`, model: "model-a"},
		{name: "effort", body: `{"input":"hello","reasoning_effort":"high"}`, effort: "high"},
		{name: "both", body: `{"input":"hello","model":"model-b","reasoning_effort":" low "}`, model: "model-b", effort: "low"},
		{name: "auto profile", body: `{"input":"hello","model":"auto","reasoning_effort":"high","auto_profile":" quality "}`, model: "auto", effort: "high", profile: "quality"},
		{name: "case insensitive", body: `{"input":"hello","MODEL":"model-b","Reasoning_Effort":"high"}`, model: "model-b", effort: "high"},
		{name: "invalid model type", body: `{"input":"hello","model":42}`, wantError: "cannot unmarshal number"},
		{name: "invalid effort type", body: `{"input":"hello","reasoning_effort":true}`, wantError: "cannot unmarshal bool"},
		{name: "invalid profile type", body: `{"input":"hello","model":"auto","auto_profile":42}`, wantError: "cannot unmarshal number"},
		{name: "unknown field", body: `{"input":"hello","model":"model-a","unexpected":true}`, wantError: `unknown field "unexpected"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			input, _, err := parseAgentSessionResponseRequest(httptest.NewRecorder(), request, false)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil || input.model != test.model || input.reasoningEffort != test.effort || input.autoProfile != test.profile {
				t.Fatalf("model = %q, effort = %q, profile = %q, error = %v", input.model, input.reasoningEffort, input.autoProfile, err)
			}
		})
	}
}

func TestParseAgentSessionModelOptionsMultipart(t *testing.T) {
	for _, test := range []struct {
		name, model, effort, profile, wantError string
		fields                                  map[string][]string
	}{
		{name: "defaults"},
		{name: "empty defaults", fields: map[string][]string{"model": {" "}, "reasoning_effort": {" "}}},
		{name: "both", fields: map[string][]string{"model": {" model-a "}, "reasoning_effort": {" high "}}, model: "model-a", effort: "high"},
		{name: "auto profile", fields: map[string][]string{"model": {"auto"}, "auto_profile": {" quality "}}, model: "auto", profile: "quality"},
		{name: "model repeated", fields: map[string][]string{"model": {"model-a", "model-b"}}, wantError: "expected at most one model field"},
		{name: "effort repeated", fields: map[string][]string{"reasoning_effort": {"low", "high"}}, wantError: "expected at most one reasoning_effort field"},
		{name: "profile repeated", fields: map[string][]string{"model": {"auto"}, "auto_profile": {"quality", "economy"}}, wantError: "expected at most one auto_profile field"},
		{name: "unknown field", fields: map[string][]string{"model": {"model-a"}, "unexpected": {"true"}}, wantError: `unknown multipart field "unexpected"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			fields := map[string][]string{"input": {`"hello"`}}
			for key, values := range test.fields {
				fields[key] = values
			}
			request := sessionMultipartForm(t, fields, map[string][]sessionUploadFixture{"file": {{"note.txt", "text/plain", "notes"}}})
			defer cleanupMessagePayload(request)
			input, _, err := parseAgentSessionResponseRequest(httptest.NewRecorder(), request, true)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil || input.model != test.model || input.reasoningEffort != test.effort || input.autoProfile != test.profile {
				t.Fatalf("model = %q, effort = %q, profile = %q, error = %v", input.model, input.reasoningEffort, input.autoProfile, err)
			}
		})
	}
}

func TestAgentSessionTurnModelOptionsRemainRequestScoped(t *testing.T) {
	var requests []agentengine.TurnRequest
	engine := &fakeSessionEngine{run: func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		requests = append(requests, request)
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "ok"}
	}}
	handler, _, bindings, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	selected, err := handler.resolveSessionAgent("agent-alpha")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := bindings.GetOrCreate(selected.ID, "same-session")
	if err != nil {
		t.Fatal(err)
	}
	model := &agentengine.TurnModel{ModelID: "model-a", ReasoningEffort: "high"}
	for _, option := range []*agentengine.TurnModel{model, nil} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		_, writeResult := handler.runSessionResponse(request.Context(), recorder, request, selected, binding, agentSessionInput{Text: "hello", turnModel: option}, false, nil, agentengine.AdmissionRejectIfBusy)
		writeResult()
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	}
	if len(requests) != 2 || requests[0].Model == nil || requests[0].Model.ModelID != model.ModelID || requests[0].Model.ReasoningEffort != model.ReasoningEffort {
		t.Fatalf("requests = %+v", requests)
	}
	if requests[1].Model != nil || requests[0].ConversationKey != requests[1].ConversationKey {
		t.Fatalf("subsequent turn = %+v, initial conversation = %q", requests[1], requests[0].ConversationKey)
	}
}

func TestAgentSessionRejectsMediaGenerationModelsBeforeExecution(t *testing.T) {
	item := sessionCodexAgent("agent-alpha", "Alpha")
	item.AgentProfile = agent.AgentProfile{Provider: agent.ProviderAPI, ModelProviderID: "fixture", ModelID: "chat"}
	calls := 0
	engine := &fakeSessionEngine{run: func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		calls++
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "ok"}
	}}
	handler, _, bindings, _ := newAgentSessionTestHandler(t, []agent.Agent{item}, engine, "")
	handler.agentModels.(*agent.ModelConfiguration).SetLLMConfig(config.LLMConfig{Default: "fixture.chat", Providers: map[string]config.ProviderConfig{
		"fixture": {BaseURL: "https://example.com/v1", APIKey: "test-key", Models: []string{"chat", "image", "video"}, ImageModels: []string{"image"}, VideoModels: []string{"video"}},
	}})
	for _, model := range []string{"gpt-image-1", "image", "video"} {
		recorder := performAgentSessionRequest(t, handler, item.ID, "media", map[string]any{"input": "hello", "model": model})
		var result agentSessionErrorEnvelope
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil || recorder.Code != http.StatusBadRequest || result.Error.Param == nil || *result.Error.Param != "model" || calls != 0 || len(bindings.Bindings()) != 0 {
			t.Fatalf("%s: HTTP %d body=%s calls=%d bindings=%d error=%v", model, recorder.Code, recorder.Body.String(), calls, len(bindings.Bindings()), err)
		}
	}
}
