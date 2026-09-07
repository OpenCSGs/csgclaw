package binding

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	channeltypes "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/channel/feishu/transport"
	"csgclaw/internal/im"
)

type pipelineTestEngine struct {
	mu       sync.Mutex
	requests []agentengine.TurnRequest
	run      chan struct{}
}

func (*pipelineTestEngine) Agents() agentengine.AgentInterface { return nil }
func (*pipelineTestEngine) RuntimeExtensions(string) agentengine.RuntimeExtensionInterface {
	return nil
}
func (e *pipelineTestEngine) Conversations(string) agentengine.ConversationInterface {
	return pipelineTestConversation{engine: e}
}

type pipelineTestConversation struct{ engine *pipelineTestEngine }

func (pipelineTestConversation) Files() agentengine.FileInterface {
	return agentengine.NewFileStore().Scope("agent-1")
}

func (c pipelineTestConversation) Run(ctx context.Context, req agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
	c.engine.mu.Lock()
	c.engine.requests = append(c.engine.requests, req)
	run := c.engine.run
	c.engine.mu.Unlock()
	if run != nil {
		run <- struct{}{}
	}
	_ = sink.Emit(ctx, agentengine.TurnEvent{Sequence: 1, Kind: agentengine.TurnEventTextDelta, Text: "answer"})
	return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "answer"}
}
func (pipelineTestConversation) Cancel(context.Context, agentengine.ConversationKey, agentengine.TurnID) error {
	return nil
}
func (pipelineTestConversation) Reset(context.Context, agentengine.ConversationKey) error { return nil }
func (pipelineTestConversation) Resolve(context.Context, agentengine.InteractionResolution) error {
	return nil
}

type pipelineTestAdapter struct {
	transport.Adapter
	sink transport.Sink

	mu          sync.Mutex
	started     bool
	closed      bool
	texts       []transport.SendTextRequest
	textUpdates []transport.UpdateTextRequest
	cards       []transport.SendCardRequest
	cardUpdates []transport.UpdateCardRequest
}

func (a *pipelineTestAdapter) Start(context.Context) error {
	a.mu.Lock()
	a.started = true
	a.mu.Unlock()
	return nil
}
func (a *pipelineTestAdapter) Close(context.Context) error {
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	return nil
}
func (*pipelineTestAdapter) Identity() transport.Identity {
	return transport.Identity{OpenID: "bot-open-id"}
}
func (*pipelineTestAdapter) PrepareIdentity(context.Context) (transport.Identity, error) {
	return transport.Identity{OpenID: "bot-open-id"}, nil
}
func (a *pipelineTestAdapter) SendText(_ context.Context, req transport.SendTextRequest) (transport.SendResult, error) {
	a.mu.Lock()
	a.texts = append(a.texts, req)
	a.mu.Unlock()
	return transport.SendResult{MessageID: "message-out"}, nil
}
func (a *pipelineTestAdapter) UpdateText(_ context.Context, req transport.UpdateTextRequest) error {
	a.mu.Lock()
	a.textUpdates = append(a.textUpdates, req)
	a.mu.Unlock()
	return nil
}
func (a *pipelineTestAdapter) SendCard(_ context.Context, req transport.SendCardRequest) (transport.SendResult, error) {
	a.mu.Lock()
	a.cards = append(a.cards, req)
	a.mu.Unlock()
	return transport.SendResult{MessageID: "message-out"}, nil
}
func (a *pipelineTestAdapter) UpdateCard(_ context.Context, req transport.UpdateCardRequest) error {
	a.mu.Lock()
	a.cardUpdates = append(a.cardUpdates, req)
	a.mu.Unlock()
	return nil
}
func (*pipelineTestAdapter) AddReaction(context.Context, transport.AddReactionRequest) (transport.AddReactionResult, error) {
	return transport.AddReactionResult{ReactionID: "reaction"}, nil
}
func (*pipelineTestAdapter) DeleteReaction(context.Context, transport.DeleteReactionRequest) error {
	return nil
}

func TestPipelineRoutesFeishuMessageThroughAgentEngine(t *testing.T) {
	run := make(chan struct{}, 1)
	engine := &pipelineTestEngine{run: run}
	var adapter *pipelineTestAdapter
	factory, err := NewPipelineFactory(PipelineFactoryOptions{
		Engine: engine,
		Transport: transport.FactoryFunc(func(_ transport.Config, sink transport.Sink) (transport.Adapter, error) {
			adapter = &pipelineTestAdapter{sink: sink}
			return adapter, nil
		}),
		MediaRoot: filepath.Join(t.TempDir(), "media"),
	})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := factory.NewWorker(Resolved{
		Binding: channelBinding("agent-1", "bot-1"),
		App:     feishu.AppConfig{AppID: "app", AppSecret: "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = worker.Close(context.Background()) })

	now := time.Now().UTC()
	if err := adapter.sink.HandleEvent(context.Background(), transport.Event{
		Kind: transport.EventMessage, EventID: "event-1", OccurredAt: now,
		Message: &transport.Message{
			ID: "message-1", ChatID: "chat-1", ChatType: transport.ChatP2P,
			Text: "hello", Sender: transport.Identity{OpenID: "human"}, CreatedAt: now,
		},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-run:
	case <-time.After(time.Second):
		t.Fatal("message did not reach Agent Engine")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		adapter.mu.Lock()
		found := false
		for _, update := range adapter.textUpdates {
			found = found || update.Markdown && strings.Contains(update.Text, "answer")
		}
		created := len(adapter.texts) > 0 && adapter.texts[0].Markdown
		usedCard := len(adapter.cards) > 0 || len(adapter.cardUpdates) > 0
		adapter.mu.Unlock()
		if usedCard {
			t.Fatal("hosted Feishu pipeline unexpectedly used Card delivery")
		}
		if created && found {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("terminal answer was not delivered")
}

func TestPipelineLocalHandoffUsesIntakeAndDeduplicatesWebSocketDelivery(t *testing.T) {
	run := make(chan struct{}, 2)
	engine := &pipelineTestEngine{run: run}
	var adapter *pipelineTestAdapter
	factory, err := NewPipelineFactory(PipelineFactoryOptions{
		Engine: engine,
		Transport: transport.FactoryFunc(func(_ transport.Config, sink transport.Sink) (transport.Adapter, error) {
			adapter = &pipelineTestAdapter{sink: sink}
			return adapter, nil
		}),
		MediaRoot: filepath.Join(t.TempDir(), "media"),
	})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := factory.NewWorker(Resolved{
		Binding: channelBinding("agent-dev", "u-dev"),
		App:     feishu.AppConfig{AppID: "app-dev", AppSecret: "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = worker.Close(context.Background()) })

	createdAt := time.Now().UTC()
	event := feishu.MessageEvent{
		Type: feishu.MessageEventTypeMessageCreated, RoomID: "oc-room",
		MentionBotID: "u-dev",
		Message: &im.Message{
			ID: "om-handoff", SenderID: "ou-manager", Content: "请自我介绍一下", CreatedAt: createdAt,
			Mentions: []im.Mention{{ID: "bot-open-id", Name: "u-dev"}},
		},
	}
	local := worker.(LocalMessageWorker)
	if err := local.HandleLocalMessage(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	select {
	case <-run:
	case <-time.After(time.Second):
		t.Fatal("local handoff did not reach Agent Engine")
	}

	if err := adapter.sink.HandleEvent(context.Background(), transport.Event{
		Kind: transport.EventMessage, EventID: "event-websocket", OccurredAt: createdAt,
		Message: &transport.Message{
			ID: "om-handoff", ChatID: "oc-room", ChatType: transport.ChatGroup,
			Text: "请自我介绍一下", Sender: transport.Identity{OpenID: "ou-manager"}, CreatedAt: createdAt,
			Mentions: []transport.Mention{{OpenID: "bot-open-id", Name: "u-dev"}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if len(engine.requests) != 1 {
		t.Fatalf("Engine runs = %d, want one logical message execution", len(engine.requests))
	}
	if len(engine.requests[0].Input) == 0 || !strings.Contains(engine.requests[0].Input[0].Text, "channel: feishu") ||
		!strings.Contains(engine.requests[0].Input[0].Text, "participant_id: u-dev") ||
		!strings.Contains(engine.requests[0].Input[0].Text, "请自我介绍一下") {
		t.Fatalf("Engine input = %#v, want rendered Feishu context", engine.requests[0].Input)
	}
}

type failingStartupAdapter struct {
	*pipelineTestAdapter
	event transport.Event
	err   error
}

func (a *failingStartupAdapter) Start(ctx context.Context) error {
	_ = a.sink.HandleEvent(ctx, a.event)
	return a.err
}

func TestPipelineStartupFailureDoesNotExecuteBufferedEvent(t *testing.T) {
	engine := &pipelineTestEngine{}
	startErr := errors.New("websocket unavailable")
	now := time.Now().UTC()
	factory, err := NewPipelineFactory(PipelineFactoryOptions{
		Engine: engine,
		Transport: transport.FactoryFunc(func(_ transport.Config, sink transport.Sink) (transport.Adapter, error) {
			return &failingStartupAdapter{
				pipelineTestAdapter: &pipelineTestAdapter{sink: sink},
				err:                 startErr,
				event: transport.Event{
					Kind: transport.EventMessage, EventID: "event-start", OccurredAt: now,
					Message: &transport.Message{
						ID: "message-start", ChatID: "chat-1", ChatType: transport.ChatP2P,
						Text: "do not run", Sender: transport.Identity{OpenID: "human"}, CreatedAt: now,
					},
				},
			}, nil
		}),
		MediaRoot: filepath.Join(t.TempDir(), "media"),
	})
	if err != nil {
		t.Fatal(err)
	}
	worker, _ := factory.NewWorker(Resolved{
		Binding: channelBinding("agent-1", "bot-1"),
		App:     feishu.AppConfig{AppID: "app", AppSecret: "secret"},
	})
	if err := worker.Start(context.Background()); !errors.Is(err, startErr) {
		t.Fatalf("Start() error = %v", err)
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if len(engine.requests) != 0 {
		t.Fatalf("Engine runs = %d before transport readiness", len(engine.requests))
	}
}

func channelBinding(agentID, participantID string) channeltypes.Binding {
	return channeltypes.Binding{
		ID: participantID, Channel: feishu.ChannelID, AgentID: agentID, ParticipantID: participantID,
	}
}

func (pipelineTestConversation) GetInteraction(context.Context, agentengine.ConversationKey, string) (agentengine.InteractionRequest, error) {
	return agentengine.InteractionRequest{}, &agentengine.TurnError{Code: agentengine.ErrorInteractionNotFound, Message: "no interaction in this test fixture"}
}
