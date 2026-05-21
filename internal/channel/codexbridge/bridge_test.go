package codexbridge

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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

	sink := NewEventSink()
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

	sink := NewEventSink()
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

	sink := NewEventSink()
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

func TestServiceAttachesToolCallsToResponseMessageThread(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "run it"}

	sink := NewEventSink()
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
				RuntimeID:  handle.RuntimeID,
				SessionID:  string(req.SessionId),
				Kind:       runtimecodex.SessionEventToolCallUpdate,
				ToolCallID: "tool-1",
				ToolStatus: string(acp.ToolCallStatusCompleted),
				Payload:    acp.SessionToolCallUpdate{ToolCallId: "tool-1", Status: &completed, RawOutput: "command output"},
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
			records[0].Text == "done" &&
			records[1].RoomID == "room-1" &&
			records[1].ThreadRootID == "sent-1" &&
			strings.HasPrefix(records[1].Text, "🔧 Running tool: Run shell command") &&
			records[2].RoomID == "room-1" &&
			records[2].ThreadRootID == "sent-1" &&
			strings.HasPrefix(records[2].Text, "🔧 Tool completed: Run shell command") &&
			strings.Contains(records[2].Text, "command output")
	})
}

func TestServiceKeepsToolCallAttachmentsInsideExistingThread(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-thread-reply", RoomID: "room-1", ThreadRootID: "msg-root", Text: "run it"}

	sink := NewEventSink()
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
			records[0].Text == "thread done" &&
			records[1].RoomID == "room-1" &&
			records[1].ThreadRootID == "msg-root" &&
			strings.HasPrefix(records[1].Text, "🔧 Running tool: Run shell command")
	})
}

func TestServiceDedupesMessagesWithinConversationScope(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 2)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", ThreadRootID: "msg-a", Text: "first thread"}
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", ThreadRootID: "msg-b", Text: "second thread"}

	sink := NewEventSink()
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

	sink := NewEventSink()
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

	sink := NewEventSink()
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

	sink := NewEventSink()
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

	sink := NewEventSink()
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

func TestServiceIgnoresEventsFromOtherBindings(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "hello"}

	sink := NewEventSink()
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
							"data: {\"message_id\":\"m-3\",\"room_id\":\"room-2\",\"chat_type\":\"direct\",\"text\":\"direct hello\"}\n\n",
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

	if len(got) != 2 {
		t.Fatalf("received %d events, want 2: %+v", len(got), got)
	}
	if got[0].MessageID != "m-2" {
		t.Fatalf("received first event = %+v, want m-2", got[0])
	}
	if got[1].MessageID != "m-3" {
		t.Fatalf("received second event = %+v, want m-3", got[1])
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

	sink := NewEventSink()
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
		return slices.Equal(client.sentTexts(), []string{"Codex runtime error: boom"})
	})
}
