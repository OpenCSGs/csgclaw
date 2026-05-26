package codexbridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	csgclawchannel "csgclaw/internal/channel/csgclaw"
	"csgclaw/internal/channel/runtimebridge"
	runtimecodex "csgclaw/internal/runtime/codex"

	acp "github.com/coder/acp-go-sdk"
)

type streamResult struct {
	events <-chan BotEvent
	errs   <-chan error
}

type fakeBotClient struct {
	mu          sync.Mutex
	streams     map[string][]streamResult
	streamCtxs  []context.Context
	sendRecords []SendMessageRequest
}

func (c *fakeBotClient) StreamEvents(ctx context.Context, botID, _ string) (<-chan BotEvent, <-chan error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.streamCtxs = append(c.streamCtxs, ctx)
	results := c.streams[botID]
	if len(results) == 0 {
		events := make(chan BotEvent)
		close(events)
		errs := make(chan error)
		close(errs)
		return events, errs
	}
	next := results[0]
	c.streams[botID] = results[1:]
	return next.events, next.errs
}

func (c *fakeBotClient) streamContexts() []context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]context.Context, len(c.streamCtxs))
	copy(out, c.streamCtxs)
	return out
}

func (c *fakeBotClient) SendMessage(_ context.Context, _ string, req SendMessageRequest) (SendMessageResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sendRecords = append(c.sendRecords, req)
	return SendMessageResponse{MessageID: "sent-" + strconv.Itoa(len(c.sendRecords))}, nil
}

func (c *fakeBotClient) sentTexts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.sendRecords))
	for _, req := range c.sendRecords {
		out = append(out, req.Text)
	}
	return out
}

func (c *fakeBotClient) sentRecords() []SendMessageRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]SendMessageRequest, len(c.sendRecords))
	copy(out, c.sendRecords)
	return out
}

type promptCall struct {
	runtimeID string
	sessionID string
	text      string
}

type ensureCall struct {
	runtimeID       string
	conversationKey string
}

type fakePrompter struct {
	mu      sync.Mutex
	calls   []promptCall
	ensures []ensureCall
	prompt  func(context.Context, runtimecodex.SessionHandle, acp.PromptRequest) error
	ensure  func(context.Context, runtimecodex.SessionHandle, string) (string, error)
}

func (p *fakePrompter) Prompt(ctx context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) (acp.PromptResponse, error) {
	call := promptCall{runtimeID: handle.RuntimeID, sessionID: string(req.SessionId)}
	if len(req.Prompt) > 0 && req.Prompt[0].Text != nil {
		call.text = req.Prompt[0].Text.Text
	}
	p.mu.Lock()
	p.calls = append(p.calls, call)
	p.mu.Unlock()

	if p.prompt != nil {
		if err := p.prompt(ctx, handle, req); err != nil {
			return acp.PromptResponse{}, err
		}
	}
	return acp.PromptResponse{}, nil
}

func (p *fakePrompter) EnsureSession(ctx context.Context, handle runtimecodex.SessionHandle, conversationKey string) (string, error) {
	p.mu.Lock()
	p.ensures = append(p.ensures, ensureCall{runtimeID: handle.RuntimeID, conversationKey: conversationKey})
	p.mu.Unlock()
	if p.ensure != nil {
		return p.ensure(ctx, handle, conversationKey)
	}
	return "", nil
}

func (p *fakePrompter) texts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.calls))
	for _, call := range p.calls {
		out = append(out, call.text)
	}
	return out
}

func (p *fakePrompter) sessionIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.calls))
	for _, call := range p.calls {
		out = append(out, call.sessionID)
	}
	return out
}

func (p *fakePrompter) ensureCalls() []ensureCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ensureCall, len(p.ensures))
	copy(out, p.ensures)
	return out
}

func TestServiceRoundTrip(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "hello"}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "Hello back",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return slices.Equal(prompter.texts(), []string{"hello"}) && slices.Equal(client.sentTexts(), []string{"Hello back"})
	})
}

func TestServiceEnsuresConversationSessionAndInjectsHiddenThreadContext(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{
		MessageID:    "m-1",
		RoomID:       "room-1",
		ThreadRootID: "msg-root",
		Text:         "hi",
		ThreadContext: &BotThreadContext{
			RootMessageID: "msg-root",
			Context: []BotThreadContextMessage{
				{ID: "msg-before", SenderID: "u-admin", Content: "Need help with deployment", CreatedAt: "2026-05-20T08:00:00Z"},
				{ID: "msg-root", SenderID: "u-admin", Content: "Can you coordinate release?", CreatedAt: "2026-05-20T08:01:00Z"},
			},
		},
	}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		ensure: func(_ context.Context, _ runtimecodex.SessionHandle, key string) (string, error) {
			if key != "room-1:msg-root" {
				t.Fatalf("conversation key = %q, want room-1:msg-root", key)
			}
			return "acp-thread-session", nil
		},
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "hello from thread",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		texts := prompter.texts()
		return slices.Equal(prompter.sessionIDs(), []string{"acp-thread-session"}) &&
			len(texts) == 1 &&
			strings.Contains(texts[0], "Hidden thread context") &&
			strings.Contains(texts[0], "Need help with deployment") &&
			strings.Contains(texts[0], "Current thread message:\nhi") &&
			slices.Equal(client.sentTexts(), []string{"hello from thread"})
	})
	if got := prompter.ensureCalls(); len(got) != 1 || got[0].conversationKey != "room-1:msg-root" {
		t.Fatalf("EnsureSession calls = %+v, want one thread conversation key", got)
	}
}

func TestServiceUsesConversationScopedSessionsAndThreadReplies(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", ThreadRootID: "msg-root", Text: "hello in thread"}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "thread reply",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return slices.Equal(prompter.sessionIDs(), []string{"sess-1:room-1:msg-root"}) &&
			len(client.sentRecords()) == 1 &&
			client.sentRecords()[0].ThreadRootID == "msg-root"
	})
}

func TestServiceProjectsToolCallsAsActivityCardsAlongsideResponse(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "run it"}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:  handle.RuntimeID,
				SessionID:  string(req.SessionId),
				Kind:       runtimecodex.SessionEventToolCallStart,
				ToolCallID: "tool-1",
				ToolTitle:  "Run shell command",
				ToolStatus: string(acp.ToolCallStatusPending),
				Payload:    acp.SessionUpdateToolCall{ToolCallId: "tool-1", Title: "Run shell command", Status: acp.ToolCallStatusPending},
			})
			completed := acp.ToolCallStatusCompleted
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:         handle.RuntimeID,
				SessionID:         string(req.SessionId),
				Kind:              runtimecodex.SessionEventToolCallUpdate,
				ToolCallID:        "tool-1",
				ToolStatus:        string(acp.ToolCallStatusCompleted),
				ToolOutputSummary: "command output",
				Payload:           acp.SessionToolCallUpdate{ToolCallId: "tool-1", Status: &completed, RawOutput: "command output"},
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "done",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		records := client.sentRecords()
		return len(records) == 3 &&
			records[0].RoomID == "room-1" &&
			records[0].ThreadRootID == "" &&
			strings.Contains(records[0].Text, runtimebridge.AgentToolMsgType) &&
			records[1].RoomID == "room-1" &&
			records[1].ThreadRootID == "" &&
			strings.Contains(records[1].Text, runtimebridge.AgentToolMsgType) &&
			strings.Contains(records[1].Text, "command output") &&
			records[2].RoomID == "room-1" &&
			records[2].ThreadRootID == "" &&
			records[2].Text == "done"
	})
}

func TestServiceKeepsToolActivityInsideExistingThread(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-thread-reply", RoomID: "room-1", ThreadRootID: "msg-root", Text: "run it"}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:  handle.RuntimeID,
				SessionID:  string(req.SessionId),
				Kind:       runtimecodex.SessionEventToolCallStart,
				ToolCallID: "tool-1",
				ToolTitle:  "Run shell command",
				ToolStatus: string(acp.ToolCallStatusPending),
				Payload:    acp.SessionUpdateToolCall{ToolCallId: "tool-1", Title: "Run shell command", Status: acp.ToolCallStatusPending},
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "thread done",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		records := client.sentRecords()
		return len(records) == 2 &&
			records[0].RoomID == "room-1" &&
			records[0].ThreadRootID == "msg-root" &&
			strings.Contains(records[0].Text, runtimebridge.AgentToolMsgType) &&
			records[1].RoomID == "room-1" &&
			records[1].ThreadRootID == "msg-root" &&
			records[1].Text == "thread done"
	})
}

func TestServiceDedupesMessagesWithinConversationScope(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 2)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", ThreadRootID: "msg-a", Text: "first thread"}
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", ThreadRootID: "msg-b", Text: "second thread"}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "reply",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return slices.Equal(prompter.texts(), []string{"first thread", "second thread"})
	})
}

func TestServiceDedupesReplayAcrossReconnect(t *testing.T) {
	t.Parallel()

	first := make(chan BotEvent, 1)
	firstErrs := make(chan error)
	second := make(chan BotEvent, 1)
	secondErrs := make(chan error)
	first <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "hello"}
	second <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "hello"}
	close(first)
	close(second)
	close(firstErrs)
	close(secondErrs)

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {
				{events: first, errs: firstErrs},
				{events: second, errs: secondErrs},
			},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "once",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return slices.Equal(prompter.texts(), []string{"hello"}) && slices.Equal(client.sentTexts(), []string{"once"})
	})
}

func TestServiceWorkerOutlivesStartContext(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "still alive",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()
	cancel()

	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "hello after request"}

	waitFor(t, func() bool {
		return slices.Equal(prompter.texts(), []string{"hello after request"}) &&
			slices.Equal(client.sentTexts(), []string{"still alive"})
	})
	for _, streamCtx := range client.streamContexts() {
		select {
		case <-streamCtx.Done():
			t.Fatal("stream context was canceled with StartBot caller context")
		default:
		}
	}
}

func TestServiceQueuesWhileBusy(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 2)
	errs := make(chan error)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "first"}
	stream <- BotEvent{MessageID: "m-2", RoomID: "room-1", Text: "second"}
	close(errs)

	sink := runtimecodex.NewEventSink()
	firstRelease := make(chan struct{})
	firstStarted := make(chan struct{})
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(ctx context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			text := req.Prompt[0].Text.Text
			if text == "first" {
				close(firstStarted)
				select {
				case <-firstRelease:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "reply:" + text,
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	select {
	case <-firstStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("first prompt did not start")
	}

	time.Sleep(150 * time.Millisecond)
	if got := prompter.texts(); !slices.Equal(got, []string{"first"}) {
		t.Fatalf("prompt order before release = %v, want [first]", got)
	}

	close(firstRelease)
	waitFor(t, func() bool {
		return slices.Equal(prompter.texts(), []string{"first", "second"}) &&
			slices.Equal(client.sentTexts(), []string{"reply:first", "reply:second"})
	})
}

func TestServiceFlushesAfterPromptSettlesWithoutTerminalEvent(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "hello"}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "settled reply",
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	svc.promptSettle = 25 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return slices.Equal(client.sentTexts(), []string{"settled reply"})
	})
}

func TestServiceProjectsToolEventsAsAgentActivity(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "hello"}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:        handle.RuntimeID,
				SessionID:        string(req.SessionId),
				Kind:             runtimecodex.SessionEventToolCallStart,
				ReceivedAt:       time.Now().UTC(),
				ToolCallID:       "tool-1",
				ToolKind:         "execute",
				ToolTitle:        "Run shell command",
				ToolStatus:       "in_progress",
				ToolInputSummary: `{"cmd":"go test ./internal/runtime/codex"}`,
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:  handle.RuntimeID,
				SessionID:  string(req.SessionId),
				Kind:       runtimecodex.SessionEventPromptCompleted,
				ReceivedAt: time.Now().UTC(),
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return len(client.sentTexts()) == 1
	})
	text := client.sentTexts()[0]
	if strings.Contains(text, "Running tool:") {
		t.Fatalf("tool event rendered as plain text: %s", text)
	}
	var payload struct {
		Type    string `json:"type"`
		RoomID  string `json:"room_id"`
		Content struct {
			MsgType string `json:"msgtype"`
			Tool    struct {
				ID           string `json:"id"`
				Kind         string `json:"kind"`
				Status       string `json:"status"`
				InputSummary string `json:"input_summary"`
			} `json:"tool"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("tool activity json decode: %v; text=%s", err, text)
	}
	if payload.Type != runtimebridge.AgentActivityType || payload.RoomID != "room-1" || payload.Content.MsgType != runtimebridge.AgentToolMsgType {
		t.Fatalf("payload = %+v, want tool activity", payload)
	}
	if payload.Content.Tool.ID == "" || payload.Content.Tool.ID == "tool-1" || payload.Content.Tool.Kind != "execute" || payload.Content.Tool.Status != "running" {
		t.Fatalf("tool payload = %+v", payload.Content.Tool)
	}
	if strings.Contains(text, "rt-1") || strings.Contains(text, "sess-1") || strings.Contains(text, "tool-1") {
		t.Fatalf("tool activity leaked execution identity: %s", text)
	}
}

func TestServiceProjectsPermissionEventsAsAgentActivity(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "hello"}

	now := time.Now().UTC()
	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:    handle.RuntimeID,
				SessionID:    string(req.SessionId),
				Kind:         runtimecodex.SessionEventPermissionRequest,
				ReceivedAt:   now,
				ToolCallID:   "tool-1",
				ToolTitle:    "Run shell command",
				ActionID:     "perm-1",
				ActionStatus: string(runtimecodex.PermissionStatusPending),
				Payload: runtimecodex.PermissionSnapshot{
					ID:          "perm-1",
					Title:       "Run shell command",
					Status:      runtimecodex.PermissionStatusPending,
					RequestedAt: now,
					ExpiresAt:   now.Add(time.Minute),
					Options: []runtimecodex.PermissionOptionSnapshot{
						{ID: "once", Kind: "allow_once", Label: "Allow once"},
						{ID: "reject", Kind: "reject_once", Label: "Reject"},
					},
				},
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:  handle.RuntimeID,
				SessionID:  string(req.SessionId),
				Kind:       runtimecodex.SessionEventPromptCompleted,
				ReceivedAt: time.Now().UTC(),
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return len(client.sentTexts()) == 1
	})
	var payload struct {
		Type    string `json:"type"`
		Channel string `json:"channel"`
		Content struct {
			MsgType string `json:"msgtype"`
			Action  struct {
				ID      string `json:"id"`
				Kind    string `json:"kind"`
				Status  string `json:"status"`
				Options []struct {
					ID string `json:"id"`
				} `json:"options"`
			} `json:"action"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(client.sentTexts()[0]), &payload); err != nil {
		t.Fatalf("permission activity json decode: %v", err)
	}
	if payload.Type != runtimebridge.AgentActivityType || payload.Content.MsgType != runtimebridge.AgentActionMsgType {
		t.Fatalf("payload = %+v, want permission activity", payload)
	}
	if payload.Channel != csgclawchannel.ChannelID {
		t.Fatalf("channel = %q, want %s", payload.Channel, csgclawchannel.ChannelID)
	}
	if payload.Content.Action.ID != "perm-1" || payload.Content.Action.Kind != "permission" || payload.Content.Action.Status != "pending" || len(payload.Content.Action.Options) != 2 {
		t.Fatalf("permission payload = %+v", payload.Content.Action)
	}
}

func TestServiceUsesStableMessageIDForPermissionDecisionActivity(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "hello"}

	now := time.Now().UTC()
	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			pending := runtimecodex.PermissionSnapshot{
				ID:          "perm-1",
				Title:       "Run shell command",
				Status:      runtimecodex.PermissionStatusPending,
				RequestedAt: now,
				ExpiresAt:   now.Add(time.Minute),
				Options: []runtimecodex.PermissionOptionSnapshot{
					{ID: "once", Kind: "allow_once", Label: "Allow once"},
				},
			}
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:    handle.RuntimeID,
				SessionID:    string(req.SessionId),
				Kind:         runtimecodex.SessionEventPermissionRequest,
				ReceivedAt:   now,
				ToolCallID:   "tool-1",
				ToolTitle:    "Run shell command",
				ActionID:     "perm-1",
				ActionStatus: string(runtimecodex.PermissionStatusPending),
				Payload:      pending,
			})
			decided := pending
			decided.Status = runtimecodex.PermissionStatusAllowed
			decided.Decision = &runtimecodex.PermissionDecisionSnapshot{
				OptionID:  "once",
				Kind:      "allow_once",
				DecidedAt: now.Add(time.Second),
			}
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:        handle.RuntimeID,
				SessionID:        string(req.SessionId),
				Kind:             runtimecodex.SessionEventPermissionDecision,
				ReceivedAt:       now.Add(time.Second),
				ToolCallID:       "tool-1",
				ToolTitle:        "Run shell command",
				ActionID:         "perm-1",
				ActionStatus:     string(runtimecodex.PermissionStatusAllowed),
				ActionOptionID:   "once",
				ActionOptionKind: "allow_once",
				Payload:          decided,
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:  handle.RuntimeID,
				SessionID:  string(req.SessionId),
				Kind:       runtimecodex.SessionEventPromptCompleted,
				ReceivedAt: time.Now().UTC(),
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return len(client.sentRecords()) == 2
	})
	sent := client.sentRecords()
	if sent[0].MessageID == "" || sent[0].MessageID != sent[1].MessageID {
		t.Fatalf("permission message ids = %q / %q, want stable non-empty id", sent[0].MessageID, sent[1].MessageID)
	}
	if !strings.Contains(sent[1].Text, `"status":"allowed"`) {
		t.Fatalf("decision activity = %s, want allowed status", sent[1].Text)
	}
}

func TestServiceIgnoresEventsFromOtherBindings(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "hello"}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: "rt-other",
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "wrong runtime",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: "sess-other",
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "wrong session",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "matched",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: string(req.SessionId),
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return slices.Equal(client.sentTexts(), []string{"matched"})
	})
}

func TestHTTPClientDecodeSSE(t *testing.T) {
	t.Parallel()

	payload := ": connected\n\n" +
		"event: message\n" +
		"data: {\"message_id\":\"m-1\",\"room_id\":\"room-1\",\"chat_type\":\"direct\",\"text\":\"hello\"}\n\n"

	events := make(chan BotEvent, 1)
	if err := decodeSSE(context.Background(), strings.NewReader(payload), events, nil); err != nil {
		t.Fatalf("decodeSSE() error = %v", err)
	}
	close(events)

	got, ok := <-events
	if !ok {
		t.Fatal("decodeSSE() produced no events")
	}
	if got.MessageID != "m-1" || got.RoomID != "room-1" || got.ChatType != "direct" || got.Text != "hello" {
		t.Fatalf("decoded event = %+v", got)
	}
}

func TestHTTPClientStreamEventsMentionOnly(t *testing.T) {
	t.Parallel()

	client := &HTTPClient{
		BaseURL:     "http://example.test",
		MentionOnly: true,
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						"event: message\n" +
							"data: {\"message_id\":\"m-1\",\"room_id\":\"room-1\",\"chat_type\":\"group\",\"text\":\"plain hello\"}\n\n" +
							"event: message\n" +
							"data: {\"message_id\":\"m-2\",\"room_id\":\"room-1\",\"chat_type\":\"group\",\"text\":\"<at user_id=\\\"u-codex\\\"></at> hello\"}\n\n" +
							"event: message\n" +
							"data: {\"message_id\":\"m-3\",\"room_id\":\"room-1\",\"chat_type\":\"group\",\"text\":\"@codex hello\",\"mentions\":[\"u-codex\"],\"thread_root_id\":\"msg-root\"}\n\n" +
							"event: message\n" +
							"data: {\"message_id\":\"m-4\",\"room_id\":\"room-2\",\"chat_type\":\"direct\",\"text\":\"direct hello\"}\n\n",
					)),
				}, nil
			}),
		},
	}

	events, errs := client.StreamEvents(context.Background(), "u-codex", "")
	var got []BotEvent
	for event := range events {
		got = append(got, event)
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("StreamEvents() error = %v", err)
		}
	}

	if len(got) != 3 {
		t.Fatalf("received %d events, want 3: %+v", len(got), got)
	}
	if got[0].MessageID != "m-2" {
		t.Fatalf("received first event = %+v, want m-2", got[0])
	}
	if got[1].MessageID != "m-3" {
		t.Fatalf("received second event = %+v, want m-3", got[1])
	}
	if got[1].ThreadRootID != "msg-root" {
		t.Fatalf("received second event ThreadRootID = %q, want msg-root", got[1].ThreadRootID)
	}
	if got[2].MessageID != "m-4" {
		t.Fatalf("received third event = %+v, want m-4", got[2])
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestHasInboundBotAtMention(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		botID   string
		want    bool
	}{
		{name: "empty content", content: "", botID: "u-codex", want: false},
		{name: "empty bot id", content: `<at user_id="u-codex"></at>`, botID: "", want: false},
		{name: "no mention", content: "hello", botID: "u-codex", want: false},
		{name: "wrong mention", content: `<at user_id="u-other"></at> hello`, botID: "u-codex", want: false},
		{name: "match", content: `<at user_id="u-codex"></at> hello`, botID: "u-codex", want: true},
		{name: "trimmed id", content: `<at user_id=" u-codex "></at> hello`, botID: "u-codex", want: true},
		{name: "later mention matches", content: `<at user_id="u-other"></at> hi <at user_id="u-codex"></at> hello`, botID: "u-codex", want: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := hasInboundBotAtMention(tc.content, tc.botID); got != tc.want {
				t.Fatalf("hasInboundBotAtMention(%q, %q) = %v, want %v", tc.content, tc.botID, got, tc.want)
			}
		})
	}
}

func waitFor(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("condition not satisfied before timeout")
}

var _ BotClient = (*fakeBotClient)(nil)
var _ SessionPrompter = (*fakePrompter)(nil)

func TestWorkerReturnsPromptError(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "hello"}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(context.Context, runtimecodex.SessionHandle, acp.PromptRequest) error {
			return errors.New("boom")
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return slices.Equal(client.sentTexts(), []string{"Runtime error: boom"})
	})
}
