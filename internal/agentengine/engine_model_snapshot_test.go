package agentengine

import (
	"context"
	"sync/atomic"
	"testing"

	"csgclaw/internal/activity"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/runtime"
)

func TestConversationReplayRetainsOriginalTurnModel(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*contract.TurnModel)
	}{
		{name: "model", mutate: func(model *contract.TurnModel) { model.ModelID = "model-b" }},
		{name: "reasoning_effort", mutate: func(model *contract.TurnModel) { model.ReasoningEffort = "high" }},
		{name: "metadata", mutate: func(model *contract.TurnModel) { model.Metadata.ContextWindow = 64_000 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtimeImpl := &fakeConversationRuntime{workspace: t.TempDir()}
			var promptCalls atomic.Int32
			runtimeImpl.prompt = func(_ context.Context, runtimeID, sessionID, _ string) error {
				promptCalls.Add(1)
				runtimeImpl.publish(activity.RuntimeEvent{RuntimeID: runtimeID, SessionID: sessionID, Kind: activity.RuntimeEventPromptCompleted})
				return nil
			}
			engine := New(newTestAgentService(t, []agent.Agent{{
				ID: "agent-a", Name: "A", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
				RuntimeID: "runtime-a", Status: string(runtime.StateRunning),
			}}, runtimeImpl))
			originalModel := contract.TurnModel{
				ModelID: "model-a", ReasoningEffort: "low",
				Metadata: modelcap.Resolved{ContextWindow: 32_000, ContextSource: "configured"},
			}
			callerModel := originalModel
			request := TurnRequest{
				ID: "turn-model", ConversationKey: "conversation-model", Input: textInput("hello"),
				Model: &callerModel,
			}
			conversation := engine.Conversations("agent-a")
			if result := conversation.Run(context.Background(), request, nil); result.Status != TurnSucceeded || !result.Dispatched {
				t.Fatalf("initial result = %+v", result)
			}

			test.mutate(request.Model)
			if result := conversation.Run(context.Background(), request, nil); result.Status != TurnFailed || result.Error == nil || result.Error.Code != ErrorInvalidRequest || result.Dispatched {
				t.Fatalf("changed model request = %+v, want invalid_request without replay", result)
			}
			request.Model = &originalModel
			if result := conversation.Run(context.Background(), request, nil); result.Status != TurnSucceeded || !result.Dispatched {
				t.Fatalf("original model replay = %+v", result)
			}
			if calls := promptCalls.Load(); calls != 1 {
				t.Fatalf("prompt calls = %d, want 1", calls)
			}
		})
	}
}
