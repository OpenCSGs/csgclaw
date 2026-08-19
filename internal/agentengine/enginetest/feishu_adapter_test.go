package enginetest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel/feishu"
)

// feishuAdapterHarness is deliberately test-only. It accepts only the Engine
// contract and the existing channel credential owner.
type feishuAdapterHarness struct {
	engine      agentengine.Interface
	credentials feishu.AgentCredentialProvider
}

func (h feishuAdapterHarness) Ingress(ctx context.Context, agentID string, key agentengine.ConversationKey, eventID, text string) agentengine.TurnResult {
	_, app, ok := h.credentials.BotConfigForAgent(agentID)
	if !ok || strings.TrimSpace(app.AppID) == "" || strings.TrimSpace(app.AppSecret) == "" {
		return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorAgentUnavailable, Message: "Feishu binding is unavailable"}}
	}
	return h.engine.Conversations(agentID).Run(ctx, agentengine.TurnRequest{
		ID: agentengine.TurnID(eventID), ConversationKey: key,
		Admission: agentengine.AdmissionSupersede, Interaction: agentengine.InteractionSkipUserInput,
		Input: []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: text}},
	}, nil)
}

type feishuCredentialStub struct {
	app feishu.AppConfig
}

func (p feishuCredentialStub) BotConfigForAgent(string) (string, feishu.AppConfig, bool) {
	return "participant-feishu", p.app, true
}

func TestFeishuAdapterHarnessUsesEngineWithoutLeakingChannelSecrets(t *testing.T) {
	seed := runningAgent("agent-feishu")
	client := NewMemoryClient(seed)
	client.SetTurnBehavior(func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: request.Input[0].Text, Dispatched: true}
	})
	const appID = "cli_feishu_only"
	const appSecret = "feishu-secret-only"
	harness := feishuAdapterHarness{
		engine:      client,
		credentials: feishuCredentialStub{app: feishu.AppConfig{AppID: appID, AppSecret: appSecret}},
	}
	result := harness.Ingress(context.Background(), seed.ID, "chat-1", "event-1", "hello from Feishu")
	if result.Status != agentengine.TurnSucceeded {
		t.Fatalf("result = %+v", result)
	}
	retried := harness.Ingress(context.Background(), seed.ID, "chat-1", "event-1", "hello from Feishu")
	if retried.Status != agentengine.TurnSucceeded || retried.Output != result.Output {
		t.Fatalf("retried result = %+v", retried)
	}
	calls := client.Calls()
	if len(calls) != 1 || calls[0].AgentID != seed.ID || calls[0].Request.ConversationKey != "chat-1" || calls[0].Request.ID != "event-1" || calls[0].Request.Admission != agentengine.AdmissionSupersede {
		t.Fatalf("Engine calls = %+v", calls)
	}
	agentItem, err := client.Agents().Get(context.Background(), seed.ID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(struct {
		Agent  agentengine.Agent
		Calls  []TurnCall
		Result agentengine.TurnResult
	}{Agent: agentItem, Calls: calls, Result: result})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), appID) || strings.Contains(string(encoded), appSecret) {
		t.Fatalf("Feishu channel credentials leaked into Engine data: %s", encoded)
	}
}
