package server

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/enginetest"
	"csgclaw/internal/agentsession"
	"csgclaw/internal/api"
)

func TestInvalidSecretAnswerIsAbsentFromResponseAndAccessLog(t *testing.T) {
	const agentID = "agent-example"
	engine := enginetest.NewMemoryClient(agentengine.Agent{
		ID:     agentID,
		Spec:   agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{Adapter: "codex"}},
		Status: agentengine.AgentStatus{State: agentengine.AgentStateRunning, RuntimeID: "runtime-example", Ready: true},
	})
	bindings, err := agentsession.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	binding, err := bindings.GetOrCreate(agentID, "example")
	if err != nil {
		t.Fatal(err)
	}
	engine.SetTurnBehavior(func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventOutputItem, Output: &agentengine.OutputItem{
			Kind: agentengine.OutputItemRequestUserInput,
			Payload: activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{
				{ID: "key", Header: "Key", Question: "Enter the key", IsSecret: true},
			}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
	})
	result := engine.Conversations(agentID).Run(t.Context(), agentengine.TurnRequest{
		ID: "first", ConversationKey: agentengine.ConversationKey(binding.ConversationKey),
		Interaction: agentengine.InteractionUserInputOnly, Input: []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: "Start"}},
	}, nil)
	if len(result.Interactions) != 1 {
		t.Fatalf("expected one form, got %+v", result)
	}
	apiHandler := api.NewHandlerWithAccessToken(api.AgentServices{}, engine, nil, nil, nil, nil, nil, "test-token")
	apiHandler.SetAgentEngine(engine, bindings)
	var logs bytes.Buffer
	handler := accessLog(slog.New(slog.NewJSONHandler(&logs, nil)), apiHandler.Routes())
	path := "/api/v1/agents/" + agentID + "/sessions/example/interactions/" + result.Interactions[0].ID + "/respond"
	const marker = "private-answer-fixture"
	for _, tc := range []struct{ name, body, message string }{
		{"invalid option", `{"answers":{"key":{"answers":["` + marker + `"]}}}`, "unknown option label"},
		{"invalid question", `{"answers":{"` + marker + `":{"answers":[]}}}`, "unknown question id"},
		{"invalid JSON field", `{"answers":{"key":{"` + marker + `":[]}}}`, "invalid answer submission JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs.Reset()
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			for _, output := range []string{rec.Body.String(), logs.String()} {
				if !strings.Contains(output, tc.message) || strings.Contains(output, marker) {
					t.Fatalf("expected only the validation reason in error output, got %s", output)
				}
			}
		})
	}
}
