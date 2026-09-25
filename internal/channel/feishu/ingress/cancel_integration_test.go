package ingress

import (
	"context"
	"errors"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/enginetest"
	channel "csgclaw/internal/channel"
	feishuctx "csgclaw/internal/channel/feishu/context"
	"csgclaw/internal/channel/feishu/execution"
	"csgclaw/internal/channel/feishu/interaction"
	"csgclaw/internal/channel/feishu/state"
	"csgclaw/internal/channel/feishu/transport"
)

func TestStopCallbackCancelsEngineAndKeepsCOTFailureIndependent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	engine := enginetest.NewMemoryClient(agentengine.Agent{ID: "agent", Spec: agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{Adapter: "codex"}}, Status: agentengine.AgentStatus{State: agentengine.AgentStateRunning, Ready: true}})
	started := make(chan string, 2)
	engine.SetTurnBehavior(func(ctx context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		started <- string(request.ID)
		<-ctx.Done()
		return agentengine.TurnResult{Status: agentengine.TurnCanceled}
	})
	store := state.NewStore()
	runner, err := execution.NewRunner(execution.RunnerOptions{Engine: engine, State: store})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = runner.Wait(context.Background()) }()
	binding := channel.Binding{ID: "binding", AgentID: "agent", Channel: "feishu"}
	message := channel.InboundMessage{AgentID: "agent", ConversationKey: "conversation", TurnID: "old", Text: "work", Source: channel.Source{Channel: "feishu", BindingID: "binding", ChatID: "chat", ChatType: "p2p", MessageID: "user-message", EventID: "event", SenderID: "owner"}}
	if err := runner.Submit(ctx, message); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	control, ok := store.Delivery("old:cot:create")
	if !ok {
		t.Fatal("no COT message")
	}
	control.MessageID = "control-message"
	if err := store.MarkDelivered(control); err != nil {
		t.Fatal(err)
	}
	cot, _ := store.Delivery("old:cot:create")
	cot.COTID, cot.MessageID = "cot", control.MessageID
	if err := store.MarkDelivered(cot); err != nil {
		t.Fatal(err)
	}
	callback := transport.Event{Kind: transport.EventCardAction, EventID: "click", CardAction: &transport.CardAction{MessageID: control.MessageID, ChatID: "chat", Operator: transport.Identity{OpenID: "owner"}, ActionValue: map[string]any{"operation": "cancel", "turn_id": "forged"}}}
	route, err := normalizeCardAction(binding, callback, runner, store)
	if err != nil || !route.trusted {
		t.Fatalf("route=%+v error=%v", route, err)
	}
	forged := interaction.CancelRequest{BindingID: "binding", AgentID: "agent", ConversationKey: "conversation", TurnID: "old", MessageID: control.MessageID, ChatID: "chat", RequesterID: "other"}
	if err := runner.CancelRequest(ctx, forged); err == nil {
		t.Fatal("accepted another user's cancellation")
	}
	intake := &Intake{binding: binding, state: store, runner: runner}
	if err := intake.handleCard(ctx, route); err != nil {
		t.Fatal(err)
	}
	record, _ := store.Get("old")
	if record.Status != channel.TurnCanceled {
		t.Fatalf("task=%s", record.Status)
	}
	end, ok := store.Delivery("old:cot:complete")
	if !ok || len(end.Events) != 0 {
		t.Fatalf("completion=%+v", end)
	}
	events, ok := store.Delivery("old:cot:final-events")
	if !ok || events.Kind != channel.DeliveryCOTUpdate || len(events.Events) == 0 {
		t.Fatal("missing final COT events")
	}
	if err := store.MarkFailed(end.ID, errors.New("completion failed")); err != nil {
		t.Fatal(err)
	}
	message.TurnID, message.Source.EventID = "new", "event-new"
	if err := runner.Submit(ctx, message); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for i := 0; i < 2; i++ {
		if err := intake.handleCard(ctx, route); err != nil {
			t.Fatal(err)
		}
	}
	if runner.ActiveTurn("conversation") != "new" {
		t.Fatal("old callback canceled new turn")
	}
	end, _ = store.Delivery(end.ID)
	if end.Status != channel.DeliveryPending {
		t.Fatalf("completion retry=%s", end.Status)
	}
	record, _ = store.Get("old")
	if record.Status != channel.TurnCanceled {
		t.Fatal("presentation failure changed task state")
	}
}

func TestTextStopCancelsCapturedTaskWithoutStartingAnotherRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	engine := enginetest.NewMemoryClient(agentengine.Agent{ID: "agent", Spec: agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{Adapter: "codex"}}, Status: agentengine.AgentStatus{State: agentengine.AgentStateRunning, Ready: true}})
	started := make(chan string, 2)
	engine.SetTurnBehavior(func(ctx context.Context, _ string, req agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		started <- string(req.ID)
		<-ctx.Done()
		return agentengine.TurnResult{Status: agentengine.TurnCanceled}
	})
	store := state.NewStore()
	runner, err := execution.NewRunner(execution.RunnerOptions{Engine: engine, State: store})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = runner.Wait(context.Background()) }()
	binding := channel.Binding{ID: "binding", AgentID: "agent", Channel: "feishu"}
	intake, err := NewIntake(IntakeOptions{Binding: binding, State: store, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	message := channel.InboundMessage{AgentID: "agent", ConversationKey: feishuctx.ChatConversationKey("binding", "chat", ""), TurnID: "old", Text: "work", Source: channel.Source{Channel: "feishu", BindingID: "binding", ChatID: "chat", ChatType: "p2p", SenderID: "owner", EventID: "work-event", MessageID: "user-message"}}
	if err := runner.Submit(ctx, message); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	event := transport.Event{Kind: transport.EventMessage, EventID: "stop-event", Message: &transport.Message{ID: "stop-message", ChatID: "chat", ChatType: transport.ChatP2P, ContentType: "text", Text: "/stop", Sender: transport.Identity{OpenID: "owner"}}}
	if err := intake.HandleEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	item := <-intake.queue
	if item.stop == nil || item.message != nil || item.stop.TurnID != "old" {
		t.Fatal("stop was not bound to the active task")
	}
	unauthorized := *item.stop
	unauthorized.Source.SenderID = "other"
	if err := runner.Stop(ctx, unauthorized); err == nil {
		t.Fatal("another user canceled the task")
	}
	intake.process(ctx, item)
	record, _ := store.Get("old")
	if record.Status != channel.TurnCanceled {
		t.Fatalf("task status=%s", record.Status)
	}
	if _, ok := store.Delivery("old:cot:complete"); !ok {
		t.Fatal("missing COT completion")
	}
	select {
	case id := <-started:
		t.Fatalf("stop started task %s", id)
	default:
	}
	message.TurnID = "new"
	if err := runner.Submit(ctx, message); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	intake.process(ctx, item)
	if runner.ActiveTurn(message.ConversationKey) != "new" {
		t.Fatal("delayed stop canceled a newer task")
	}
}
