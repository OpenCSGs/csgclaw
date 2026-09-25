package execution

import (
	"context"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/enginetest"
	channel "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/interaction"
	feishustate "csgclaw/internal/channel/feishu/state"
)

func TestDetachedAnswerContinuesSameConversationOnce(t *testing.T) {
	engine := enginetest.NewMemoryClient(agentengine.Agent{ID: "agent-1", Spec: agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{Adapter: "codex"}}, Status: agentengine.AgentStatus{State: agentengine.AgentStateRunning, Ready: true}})
	continued := make(chan agentengine.TurnRequest, 2)
	engine.SetTurnBehavior(func(ctx context.Context, _ string, req agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		if req.ID != "turn" {
			continued <- req
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true, Output: "answer"}
		}
		err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventOutputItem, Output: &agentengine.OutputItem{Kind: agentengine.OutputItemRequestUserInput, Payload: activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "choice", Header: "Choice", Question: "Continue?", IsOther: true}}}}})
		if err != nil {
			t.Error(err)
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
	})
	store := feishustate.NewStore()
	runner, err := NewRunner(RunnerOptions{Engine: engine, State: store})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	msg := runnerMessage("event", "turn", "conversation", "question")
	msg.Source.SenderID = "human"
	if err = runner.Submit(ctx, msg); err != nil {
		t.Fatal(err)
	}
	var requestID string
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, intent := range store.Pending() {
			if intent.InteractionID != "" {
				requestID = intent.InteractionID
				break
			}
		}
		if requestID != "" && runner.ActiveTurn("conversation") == "" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if requestID == "" {
		t.Fatal("question card not created")
	}
	input := interaction.Input{AgentID: msg.AgentID, ConversationKey: msg.ConversationKey, TurnID: msg.TurnID, InteractionID: requestID, ResponderID: "other", FormValue: map[string]any{"answer_0": "continue"}}
	if err = runner.ResolveInteraction(ctx, input); err == nil {
		t.Fatal("accepted another user's answer")
	}
	input.ResponderID = "human"
	if err = runner.ResolveInteraction(ctx, input); err != nil {
		t.Fatal(err)
	}
	select {
	case req := <-continued:
		if string(req.ConversationKey) != msg.ConversationKey || !strings.Contains(req.Input[0].Text, "continue") {
			t.Fatalf("continuation=%+v", req)
		}
	case <-time.After(time.Second):
		t.Fatal("no continuation")
	}
	if err = runner.ResolveInteraction(ctx, input); err == nil {
		t.Fatal("duplicate answer accepted")
	}
	if err = runner.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	for _, intent := range store.Pending() {
		if intent.Kind == channel.DeliveryCard && intent.ID == "turn:reply:000000:create" {
			t.Fatal("empty completion card accompanied the question")
		}
	}
}
func TestPreflightFailureCompletesCOTAndSendsIndependentCard(t *testing.T) {
	store := feishustate.NewStore()
	runner, _ := NewRunner(RunnerOptions{Engine: fakeEngine{&fakeConversation{}}, State: store})
	msg := runnerMessage("event", "turn", "conversation", "")
	msg.Files = []channel.InboundFile{{ID: "missing", Kind: "file"}}
	if err := runner.Submit(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	complete, ok := store.Delivery("turn:cot:complete")
	if !ok || complete.Reason != "error" {
		t.Fatalf("complete=%+v", complete)
	}
	reply, ok := store.Delivery("turn:reply:000000:create")
	if !ok || reply.Kind != channel.DeliveryCard {
		t.Fatalf("reply=%+v", reply)
	}
}

func TestNativePermissionUpdatesCardWithoutRuntimeDecisionEvent(t *testing.T) {
	engine := enginetest.NewMemoryClient(agentengine.Agent{ID: "agent-1", Spec: agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{Adapter: "dsh"}}, Status: agentengine.AgentStatus{State: agentengine.AgentStateRunning, Ready: true}})
	ready := make(chan struct{})
	release := make(chan struct{})
	engine.SetTurnBehavior(func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		request := agentengine.InteractionRequest{ID: "permission", Kind: agentengine.InteractionPermission, Title: "Run command", Payload: activity.ActivitySnapshot{ID: "permission", Status: activity.ActionStatusPending, Options: []activity.ActionOptionSnapshot{{ID: "yes", Label: "Allow", Kind: "allow_once"}}}}
		if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventInteractionRequest, Interaction: &request}); err != nil {
			t.Error(err)
		}
		close(ready)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true, Output: "done"}
	})
	store := feishustate.NewStore()
	runner, _ := NewRunner(RunnerOptions{Engine: engine, State: store})
	msg := runnerMessage("event", "turn", "conversation", "run")
	msg.Source.SenderID = "human"
	if err := runner.Submit(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("permission not requested")
	}
	err := runner.ResolveInteraction(context.Background(), interaction.Input{AgentID: "agent-1", ConversationKey: "conversation", TurnID: "turn", InteractionID: "permission", ResponderID: "human", OptionID: "yes"})
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	card, ok := store.Delivery("turn:interaction:permission:final")
	if !ok || card.Kind != channel.DeliveryCardUpdate || !strings.Contains(cardText(card.Card), "已允许") {
		t.Fatalf("card=%+v", card)
	}
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}
