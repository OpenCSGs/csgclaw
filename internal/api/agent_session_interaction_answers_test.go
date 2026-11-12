package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"csgclaw/internal/agentengine"
)

func TestAgentSessionInteractionRequiresPerQuestionAnswerArray(t *testing.T) {
	engine := &fakeSessionEngine{run: func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		if err := sink.Emit(ctx, sessionDetachedQuestionEvent()); err != nil {
			t.Errorf("emit detached question: %v", err)
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
	}}
	server, _ := sessionOutputTestServer(t, engine, "")
	body := sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+"/responses", `{"input":"Plan","user_input":true}`, http.StatusOK)
	var response agentSessionResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil || len(response.Interactions) != 1 {
		t.Fatalf("expected one question form: %s, %v", body, err)
	}
	path := sessionQuestionPath + "/interactions/" + response.Interactions[0].InteractionID + "/respond"
	for _, invalid := range []string{
		`{"answers":{"source":{},"password":{"answers":[]}}}`,
		`{"answers":{"source":{"answers":null},"password":{"answers":[]}}}`,
		`{"answers":{"source":null,"password":{"answers":[]}}}`,
	} {
		sessionInteractionHTTP(t, server, http.MethodPost, path, invalid, http.StatusBadRequest)
	}
	skip := `{"answers":{"source":{"answers":[]},"password":{"answers":[]}}}`
	sessionInteractionHTTP(t, server, http.MethodPost, path, skip, http.StatusNoContent)
	sessionInteractionHTTP(t, server, http.MethodPost, path, skip, http.StatusConflict)
	if calls := engine.client.Calls(); len(calls) != 1 {
		t.Fatalf("skipping the form dispatched %d turns, want only the original turn", len(calls))
	}
}
