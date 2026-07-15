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
	"csgclaw/internal/channelbridge/runtimebridge"
	runtimecodex "csgclaw/internal/runtime/codex"
)

type streamResult struct {
	events <-chan BotEvent
	errs   <-chan error
}

type fakeBotClient struct {
	mu                 sync.Mutex
	streams            map[string][]streamResult
	streamCtxs         []context.Context
	sendRecords        []SendMessageRequest
	updateRecords      []updateRecord
	addReactions       []reactionAddRecord
	delReactions       []reactionDeleteRecord
	ops                []string
	updateErr          error
	addReactionErr     error
	deleteReactionErr  error
	addReactionStarted chan struct{}
	addReactionBlock   <-chan struct{}
}

type updateRecord struct {
	botID string
	req   UpdateMessageRequest
}

type reactionAddRecord struct {
	botID string
	req   AddMessageReactionRequest
}

type reactionDeleteRecord struct {
	botID string
	req   DeleteMessageReactionRequest
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
	c.ops = append(c.ops, "send:"+req.Text)
	return SendMessageResponse{MessageID: "sent-" + strconv.Itoa(len(c.sendRecords))}, nil
}

func (c *fakeBotClient) UpdateMessage(_ context.Context, botID string, req UpdateMessageRequest) (UpdateMessageResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updateRecords = append(c.updateRecords, updateRecord{botID: botID, req: req})
	c.ops = append(c.ops, "update:"+req.MessageID+":"+req.Text)
	if c.updateErr != nil {
		return UpdateMessageResponse{}, c.updateErr
	}
	return UpdateMessageResponse{MessageID: req.MessageID}, nil
}

func (c *fakeBotClient) AddMessageReaction(ctx context.Context, botID string, req AddMessageReactionRequest) (AddMessageReactionResponse, error) {
	signal(c.addReactionStarted)
	if c.addReactionBlock != nil {
		select {
		case <-ctx.Done():
			return AddMessageReactionResponse{}, ctx.Err()
		case <-c.addReactionBlock:
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.addReactions = append(c.addReactions, reactionAddRecord{botID: botID, req: req})
	reactionID := "reaction-" + strconv.Itoa(len(c.addReactions))
	c.ops = append(c.ops, "reaction-add:"+req.MessageID+":"+req.EmojiType)
	if c.addReactionErr != nil {
		return AddMessageReactionResponse{}, c.addReactionErr
	}
	return AddMessageReactionResponse{ReactionID: reactionID}, nil
}

func (c *fakeBotClient) DeleteMessageReaction(_ context.Context, botID string, req DeleteMessageReactionRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delReactions = append(c.delReactions, reactionDeleteRecord{botID: botID, req: req})
	c.ops = append(c.ops, "reaction-delete:"+req.MessageID+":"+req.ReactionID)
	return c.deleteReactionErr
}

func signal(ch chan struct{}) {
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
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

func assertCodexFinalMetadata(t *testing.T, req SendMessageRequest, sourceMessageID string) {
	t.Helper()
	for _, key := range []string{"codex", "openclaw"} {
		entry, ok := req.Metadata[key].(map[string]any)
		if !ok {
			t.Fatalf("%s metadata = %#v, want final delivery metadata", key, req.Metadata[key])
		}
		if entry["delivery_kind"] != "final" || entry["request_id"] != sourceMessageID || entry["source_message_id"] != sourceMessageID {
			t.Fatalf("%s metadata = %#v, want final delivery for %s", key, entry, sourceMessageID)
		}
	}
}

func assertCodexToolMetadata(t *testing.T, req SendMessageRequest, sourceMessageID, toolCallID string) {
	t.Helper()
	for _, key := range []string{"codex", "openclaw"} {
		entry, ok := req.Metadata[key].(map[string]any)
		if !ok {
			t.Fatalf("%s metadata = %#v, want tool delivery metadata", key, req.Metadata[key])
		}
		if entry["delivery_kind"] != "tool" || entry["request_id"] != sourceMessageID || entry["source_message_id"] != sourceMessageID {
			t.Fatalf("%s metadata = %#v, want tool delivery for %s", key, entry, sourceMessageID)
		}
		if entry["tool_call_id"] != toolCallID {
			t.Fatalf("%s metadata tool_call_id = %#v, want %s", key, entry["tool_call_id"], toolCallID)
		}
	}
}

func (c *fakeBotClient) updates() []updateRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]updateRecord, len(c.updateRecords))
	copy(out, c.updateRecords)
	return out
}

func (c *fakeBotClient) reactionAdds() []reactionAddRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]reactionAddRecord, len(c.addReactions))
	copy(out, c.addReactions)
	return out
}

func (c *fakeBotClient) reactionDeletes() []reactionDeleteRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]reactionDeleteRecord, len(c.delReactions))
	copy(out, c.delReactions)
	return out
}

func (c *fakeBotClient) operationLog() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.ops))
	copy(out, c.ops)
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

type resetCall struct {
	runtimeID       string
	conversationKey string
}

type fakePrompter struct {
	mu      sync.Mutex
	calls   []promptCall
	ensures []ensureCall
	resets  []resetCall
	prompt  func(context.Context, runtimecodex.SessionHandle, runtimecodex.PromptRequest) error
	ensure  func(context.Context, runtimecodex.SessionHandle, string) (string, error)
	reset   func(context.Context, runtimecodex.SessionHandle, string) error
}

func (p *fakePrompter) Prompt(ctx context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) (runtimecodex.PromptResponse, error) {
	call := promptCall{runtimeID: handle.RuntimeID, sessionID: req.SessionID}
	if len(req.Prompt) > 0 && req.Prompt[0].Text != nil {
		call.text = req.Prompt[0].Text.Text
	}
	p.mu.Lock()
	p.calls = append(p.calls, call)
	p.mu.Unlock()

	if p.prompt != nil {
		if err := p.prompt(ctx, handle, req); err != nil {
			return runtimecodex.PromptResponse{}, err
		}
	}
	return runtimecodex.PromptResponse{}, nil
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

func (p *fakePrompter) ResetConversationHistory(ctx context.Context, handle runtimecodex.SessionHandle, conversationKey string) error {
	p.mu.Lock()
	p.resets = append(p.resets, resetCall{runtimeID: handle.RuntimeID, conversationKey: conversationKey})
	p.mu.Unlock()
	if p.reset != nil {
		return p.reset(ctx, handle, conversationKey)
	}
	return nil
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

func (p *fakePrompter) resetCalls() []resetCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]resetCall, len(p.resets))
	copy(out, p.resets)
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
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "Hello back",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
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

func TestServiceInjectsChannelContextForFeishuEvents(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{
		Channel:       "feishu",
		ParticipantID: "manager",
		MessageID:     "om_1",
		RoomID:        "oc_alpha",
		ChatType:      "group",
		Text:          "安排 dev 做一下",
	}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"manager": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "manager", RuntimeID: "rt-manager", SessionID: "sess-manager"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		texts := prompter.texts()
		return len(texts) == 1 &&
			strings.Contains(texts[0], "channel: feishu") &&
			strings.Contains(texts[0], "room_id: oc_alpha") &&
			strings.Contains(texts[0], "participant_id: manager") &&
			strings.Contains(texts[0], "Current message:\n安排 dev 做一下")
	})
}

func TestServiceInjectsChannelContextForLocalCSGClawEvents(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{
		Channel:   "csgclaw",
		MessageID: "msg-local",
		RoomID:    "room-local",
		ChatType:  "direct",
		Text:      "find the file I sent earlier",
	}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"manager": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "manager", RuntimeID: "rt-manager", SessionID: "sess-manager"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		texts := prompter.texts()
		return len(texts) == 1 &&
			strings.Contains(texts[0], "channel: csgclaw") &&
			strings.Contains(texts[0], "room_id: room-local") &&
			strings.Contains(texts[0], "participant_id: manager") &&
			strings.Contains(texts[0], "Current message:\nfind the file I sent earlier")
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
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "hello from thread",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
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

func TestServiceAddsAttachmentManifestToPrompt(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{
		MessageID: "m-attach",
		RoomID:    "room-1",
		Text:      "please inspect",
		Attachments: []MessageAttachment{{
			ID:            "att-1",
			Name:          "diagram.png",
			Kind:          "image",
			MediaType:     "image/png",
			SizeBytes:     42,
			SHA256:        "abc123",
			DownloadURL:   "/api/v1/attachments/att-1",
			WorkspacePath: ".csgclaw/attachments/room-1/m-attach/att-1-diagram.png",
		}},
	}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{}
	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		texts := prompter.texts()
		return len(texts) == 1 &&
			strings.Contains(texts[0], "please inspect") &&
			strings.Contains(texts[0], "Attached files:") &&
			strings.Contains(texts[0], "diagram.png") &&
			strings.Contains(texts[0], ".csgclaw/attachments/room-1/m-attach/att-1-diagram.png")
	})
}

func TestServiceUsesConversationScopedSessionAndTopLevelFinalReply(t *testing.T) {
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
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "thread reply",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
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
			len(client.sentRecords()) == 1
	})
	records := client.sentRecords()
	if records[0].ThreadRootID != "" || records[0].Text != "thread reply" {
		t.Fatalf("final record = %+v, want top-level final reply", records[0])
	}
	assertCodexFinalMetadata(t, records[0], "m-1")
}

func TestServiceDeliversToolActivityMetadataBesideFinalResponse(t *testing.T) {
	for _, chatType := range []string{"direct", "group"} {
		chatType := chatType
		t.Run(chatType, func(t *testing.T) {
			t.Parallel()
			assertServiceDeliversToolActivityMetadataBesideFinalResponse(t, chatType)
		})
	}
}

func assertServiceDeliversToolActivityMetadataBesideFinalResponse(t *testing.T, chatType string) {
	t.Helper()
	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", ChatType: chatType, Text: "run it"}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:  handle.RuntimeID,
				SessionID:  req.SessionID,
				Kind:       runtimecodex.SessionEventToolCallStart,
				ToolCallID: "tool-1",
				ToolTitle:  "Run shell command",
				ToolStatus: "pending",
				Payload:    map[string]any{"tool_call_id": "tool-1", "title": "Run shell command", "status": "pending"},
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:         handle.RuntimeID,
				SessionID:         req.SessionID,
				Kind:              runtimecodex.SessionEventToolCallUpdate,
				ToolCallID:        "tool-1",
				ToolStatus:        "completed",
				ToolOutputSummary: "command output",
				Payload:           map[string]any{"tool_call_id": "tool-1", "status": "completed", "raw_output": "command output"},
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "done",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
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
		return len(client.sentRecords()) == 3
	})
	records := client.sentRecords()
	if records[0].RoomID != "room-1" || records[0].ThreadRootID != "" {
		t.Fatalf("tool start record = %+v, want top-level activity message", records[0])
	}
	if !strings.Contains(records[0].Text, runtimebridge.AgentToolMsgType) || !strings.Contains(records[0].Text, `"status":"pending"`) {
		t.Fatalf("tool start text = %s, want tool activity payload", records[0].Text)
	}
	assertCodexToolMetadata(t, records[0], "m-1", "tool-1")
	if records[1].RoomID != "room-1" || records[1].ThreadRootID != "" {
		t.Fatalf("tool update record = %+v, want top-level activity message", records[1])
	}
	if !strings.Contains(records[1].Text, runtimebridge.AgentToolMsgType) || !strings.Contains(records[1].Text, `"status":"completed"`) {
		t.Fatalf("tool update text = %s, want completed tool activity payload", records[1].Text)
	}
	assertCodexToolMetadata(t, records[1], "m-1", "tool-1")
	if records[2].RoomID != "room-1" || records[2].ThreadRootID != "" || records[2].Text != "done" {
		t.Fatalf("final record = %+v, want top-level final reply", records[2])
	}
	assertCodexFinalMetadata(t, records[2], "m-1")
}

func TestServiceAddsAndRemovesFeishuProcessingPinAroundFinalReply(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{
		Channel:       "feishu",
		ParticipantID: "manager",
		MessageID:     "om-user",
		RoomID:        "oc-alpha",
		ChatType:      "group",
		Text:          "你好",
	}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"manager": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "收到",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "manager", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return len(client.sentRecords()) == 1 && len(client.reactionAdds()) == 1 && len(client.reactionDeletes()) == 1
	})

	adds := client.reactionAdds()
	if adds[0].botID != "manager" || adds[0].req.MessageID != "om-user" || adds[0].req.EmojiType != processingPinEmoji {
		t.Fatalf("add reaction = %+v, want Pin on inbound message", adds[0])
	}
	deletes := client.reactionDeletes()
	if deletes[0].botID != "manager" || deletes[0].req.MessageID != "om-user" || deletes[0].req.ReactionID != "reaction-1" {
		t.Fatalf("delete reaction = %+v, want same inbound reaction", deletes[0])
	}
	if got := client.sentTexts(); !slices.Equal(got, []string{"收到"}) {
		t.Fatalf("sent texts = %+v, want final reply", got)
	}
}

func TestServiceDoesNotBlockPromptOnFeishuProcessingPin(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{
		Channel:       "feishu",
		ParticipantID: "manager",
		MessageID:     "om-user",
		RoomID:        "oc-alpha",
		ChatType:      "group",
		Text:          "你好",
	}

	sink := runtimecodex.NewEventSink()
	unblockReaction := make(chan struct{})
	reactionStarted := make(chan struct{}, 1)
	promptStarted := make(chan struct{}, 1)
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"manager": {{events: stream, errs: errs}},
		},
		addReactionStarted: reactionStarted,
		addReactionBlock:   unblockReaction,
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			signal(promptStarted)
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "收到",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "manager", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	select {
	case <-reactionStarted:
	case <-time.After(time.Second):
		t.Fatal("processing reaction did not start")
	}
	select {
	case <-promptStarted:
	case <-time.After(time.Second):
		t.Fatal("prompt did not start while processing reaction was blocked")
	}
	close(unblockReaction)
	waitFor(t, func() bool {
		return slices.Equal(client.sentTexts(), []string{"收到"})
	})
}

func TestServiceIgnoresFeishuProcessingPinAddFailure(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{
		Channel:       "feishu",
		ParticipantID: "manager",
		MessageID:     "om-user",
		RoomID:        "oc-alpha",
		ChatType:      "group",
		Text:          "你好",
	}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"manager": {{events: stream, errs: errs}},
		},
		addReactionErr: errors.New("reaction denied"),
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "收到",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "manager", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return slices.Equal(client.sentTexts(), []string{"收到"}) && len(client.reactionAdds()) == 1
	})
	if got := client.reactionDeletes(); len(got) != 0 {
		t.Fatalf("reaction deletes = %+v, want none after add failure", got)
	}
}

func TestServiceIgnoresFeishuProcessingPinDeleteFailure(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{
		Channel:       "feishu",
		ParticipantID: "manager",
		MessageID:     "om-user",
		RoomID:        "oc-alpha",
		ChatType:      "group",
		Text:          "你好",
	}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"manager": {{events: stream, errs: errs}},
		},
		deleteReactionErr: errors.New("delete denied"),
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "收到",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "manager", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return slices.Equal(client.sentTexts(), []string{"收到"}) && len(client.reactionAdds()) == 1 && len(client.reactionDeletes()) == 1
	})
}

func TestServiceDeliversFeishuToolActivityMetadataAndSendsFinalResponse(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{
		Channel:       "feishu",
		ParticipantID: "manager",
		MessageID:     "om-user",
		RoomID:        "oc-alpha",
		ChatType:      "group",
		Text:          "run it",
	}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"manager": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:        handle.RuntimeID,
				SessionID:        req.SessionID,
				Kind:             runtimecodex.SessionEventToolCallStart,
				ToolCallID:       "tool-1",
				ToolTitle:        "Run shell command",
				ToolStatus:       "pending",
				ToolInputSummary: `{"cmd":"pwd"}`,
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "done",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "manager", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return len(client.sentRecords()) >= 2
	})
	records := client.sentRecords()
	if records[0].RoomID != "oc-alpha" || records[0].ThreadRootID != "" {
		t.Fatalf("tool record = %+v, want top-level activity message", records[0])
	}
	assertCodexToolMetadata(t, records[0], "om-user", "tool-1")
	if records[1].RoomID != "oc-alpha" || records[1].ThreadRootID != "" || records[1].Text != "done" {
		t.Fatalf("final record = %+v, want top-level final response", records[1])
	}
	assertCodexFinalMetadata(t, records[1], "om-user")
	if updates := client.updates(); len(updates) != 0 {
		t.Fatalf("updates = %+v, want no generated-root update for tool activity", updates)
	}
}

func TestServiceDoesNotCreateGeneratedRootForFeishuToolActivity(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{
		Channel:       "feishu",
		ParticipantID: "manager",
		MessageID:     "om-user",
		RoomID:        "oc-alpha",
		ChatType:      "group",
		Text:          "run it",
	}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"manager": {{events: stream, errs: errs}},
		},
		updateErr: errors.New("update denied"),
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:        handle.RuntimeID,
				SessionID:        req.SessionID,
				Kind:             runtimecodex.SessionEventToolCallStart,
				ToolCallID:       "tool-1",
				ToolTitle:        "Run shell command",
				ToolStatus:       "pending",
				ToolInputSummary: `{"cmd":"pwd"}`,
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "done",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventPromptCompleted,
			})
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "manager", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return len(client.sentRecords()) >= 2
	})
	records := client.sentRecords()
	if records[0].ThreadRootID != "" {
		t.Fatalf("tool record = %+v, want top-level activity message", records[0])
	}
	assertCodexToolMetadata(t, records[0], "om-user", "tool-1")
	if records[1].Text != "done" || records[1].ThreadRootID != "" {
		t.Fatalf("final record = %+v, want top-level final response", records[1])
	}
	assertCodexFinalMetadata(t, records[1], "om-user")
	if updates := client.updates(); len(updates) != 0 {
		t.Fatalf("updates = %+v, want no generated-root update for tool activity", updates)
	}
}

func TestServiceDeliversToolActivityMetadataOutsideExistingThread(t *testing.T) {
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
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:  handle.RuntimeID,
				SessionID:  req.SessionID,
				Kind:       runtimecodex.SessionEventToolCallStart,
				ToolCallID: "tool-1",
				ToolTitle:  "Run shell command",
				ToolStatus: "pending",
				Payload:    map[string]any{"tool_call_id": "tool-1", "title": "Run shell command", "status": "pending"},
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "thread done",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
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
			records[0].ThreadRootID == "" &&
			records[1].RoomID == "room-1" &&
			records[1].ThreadRootID == "" &&
			records[1].Text == "thread done"
	})
	records := client.sentRecords()
	assertCodexToolMetadata(t, records[0], "m-thread-reply", "tool-1")
	assertCodexFinalMetadata(t, records[1], "m-thread-reply")
}

func TestServiceConversationResetClearsSingleThreadKey(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{
		MessageID:    "m-reset",
		RoomID:       "room-1",
		ThreadRootID: "msg-thread",
		Text:         `<slash-command name="new" arg="conversation"></slash-command> start fresh`,
	}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{}
	svc := NewService(client, prompter, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		calls := prompter.resetCalls()
		return len(calls) == 1 &&
			calls[0].runtimeID == "rt-1" &&
			calls[0].conversationKey == "room-1:msg-thread"
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
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "reply",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
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
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "once",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
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
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "still alive",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
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

func TestServiceSuppressesSupersededTurnWhileBusy(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	stream <- BotEvent{MessageID: "m-1", RoomID: "room-1", Text: "first"}

	sink := runtimecodex.NewEventSink()
	firstRelease := make(chan struct{})
	firstStarted := make(chan struct{})
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(ctx context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			text := req.Prompt[0].Text.Text
			if text == "first" {
				close(firstStarted)
				select {
				case <-firstRelease:
				case <-ctx.Done():
					return ctx.Err()
				}
				sink.Publish(runtimecodex.SessionEvent{
					RuntimeID:  handle.RuntimeID,
					SessionID:  req.SessionID,
					Kind:       runtimecodex.SessionEventToolCallStart,
					ToolCallID: "tool-stale",
					ToolTitle:  "Run shell command",
					ToolStatus: "started",
				})
			}
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "reply:" + text,
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
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

	stream <- BotEvent{MessageID: "m-2", RoomID: "room-1", Text: "second"}
	waitFor(t, func() bool {
		svc.mu.Lock()
		w := svc.workers["u-codex"]
		svc.mu.Unlock()
		if w == nil {
			return false
		}
		w.mu.Lock()
		latest := w.latest["room-1"]
		w.mu.Unlock()
		return latest == "room-1:m-2"
	})
	if got := prompter.texts(); !slices.Equal(got, []string{"first"}) {
		t.Fatalf("prompt order before release = %v, want [first]", got)
	}

	close(firstRelease)
	waitFor(t, func() bool {
		return slices.Equal(prompter.texts(), []string{"first", "second"}) &&
			slices.Equal(client.sentTexts(), []string{"reply:second"})
	})
	records := client.sentRecords()
	if len(records) != 1 {
		t.Fatalf("sent records = %+v, want only latest final", records)
	}
	assertCodexFinalMetadata(t, records[0], "m-2")
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
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
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

func TestServiceDeliversToolActivityMetadataWithoutFinalMessage(t *testing.T) {
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
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:        handle.RuntimeID,
				SessionID:        req.SessionID,
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
				SessionID:  req.SessionID,
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
		return len(client.sentRecords()) == 1
	})
	records := client.sentRecords()
	if records[0].ThreadRootID != "" {
		t.Fatalf("tool record = %+v, want top-level activity message", records[0])
	}
	assertCodexToolMetadata(t, records[0], "m-1", "tool-1")
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
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:    handle.RuntimeID,
				SessionID:    req.SessionID,
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
				SessionID:  req.SessionID,
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
		return len(client.sentRecords()) >= 2
	})
	records := client.sentRecords()
	if records[0].Text != turnPlaceholderText || records[0].ThreadRootID != "" {
		t.Fatalf("placeholder record = %+v, want top-level blank root", records[0])
	}
	if records[1].ThreadRootID != "sent-1" {
		t.Fatalf("permission activity ThreadRootID = %q, want sent-1", records[1].ThreadRootID)
	}
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
	if err := json.Unmarshal([]byte(records[1].Text), &payload); err != nil {
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
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
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
				SessionID:    req.SessionID,
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
				SessionID:        req.SessionID,
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
				SessionID:  req.SessionID,
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
		return len(client.sentRecords()) >= 3
	})
	sent := client.sentRecords()
	if sent[0].Text != turnPlaceholderText || sent[0].ThreadRootID != "" {
		t.Fatalf("placeholder record = %+v, want top-level blank root", sent[0])
	}
	var foundDecision bool
	for _, record := range sent[1:] {
		if record.ThreadRootID != "sent-1" {
			t.Fatalf("permission record = %+v, want thread root sent-1", record)
		}
		if strings.Contains(record.Text, `"status":"allowed"`) {
			foundDecision = true
		}
	}
	if !foundDecision {
		t.Fatalf("records = %+v, want allowed decision activity", sent)
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
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: "rt-other",
				SessionID: req.SessionID,
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
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "matched",
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
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

func TestServiceSuppressesCommentaryAndToolTextBeforeFinalMessage(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "msg-user", RoomID: "room-1", Text: "read it"}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "shell\ncommand: cat README.md",
				Payload:   map[string]any{"phase": "commentary"},
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID:        handle.RuntimeID,
				SessionID:        req.SessionID,
				Kind:             runtimecodex.SessionEventToolCallStart,
				ToolCallID:       "tool-1",
				ToolKind:         "exec_command",
				ToolTitle:        "Run shell command",
				ToolStatus:       "running",
				ToolInputSummary: `{"command":"Read: from README.md"}`,
				Payload:          map[string]any{"command": "Read: from README.md"},
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
				Kind:      runtimecodex.SessionEventTextDelta,
				Text:      "done",
				Payload:   map[string]any{"phase": "final_answer"},
			})
			sink.Publish(runtimecodex.SessionEvent{
				RuntimeID: handle.RuntimeID,
				SessionID: req.SessionID,
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
		return len(client.sentRecords()) == 2
	})
	records := client.sentRecords()
	if records[0].ThreadRootID != "" {
		t.Fatalf("tool ThreadRootID = %q, want top-level activity message", records[0].ThreadRootID)
	}
	assertCodexToolMetadata(t, records[0], "msg-user", "tool-1")
	if got := records[1].Text; got != "done" || strings.Contains(got, "command: cat README.md") {
		t.Fatalf("final reply leaked commentary text: %s", got)
	}
	if records[1].ThreadRootID != "" {
		t.Fatalf("final ThreadRootID = %q, want top-level final reply", records[1].ThreadRootID)
	}
	assertCodexFinalMetadata(t, records[1], "msg-user")
}

func TestServiceWaitsForDelayedEventsAfterCommentarySettle(t *testing.T) {
	t.Parallel()

	stream := make(chan BotEvent, 1)
	errs := make(chan error)
	close(errs)
	stream <- BotEvent{MessageID: "msg-user", RoomID: "room-1", Text: "write it"}

	sink := runtimecodex.NewEventSink()
	client := &fakeBotClient{
		streams: map[string][]streamResult{
			"u-codex": {{events: stream, errs: errs}},
		},
	}
	prompter := &fakePrompter{
		prompt: func(_ context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) error {
			go func() {
				time.Sleep(30 * time.Millisecond)
				sink.Publish(runtimecodex.SessionEvent{
					RuntimeID: handle.RuntimeID,
					SessionID: req.SessionID,
					Kind:      runtimecodex.SessionEventTextDelta,
					Text:      "需要写权限",
					Payload:   map[string]any{"phase": "commentary"},
				})
				sink.Publish(runtimecodex.SessionEvent{
					RuntimeID:        handle.RuntimeID,
					SessionID:        req.SessionID,
					Kind:             runtimecodex.SessionEventToolCallStart,
					ToolCallID:       "tool-1",
					ToolKind:         "exec_command",
					ToolTitle:        "Run shell command",
					ToolStatus:       "running",
					ToolInputSummary: `{"command":"cat > hello.py"}`,
				})
				sink.Publish(runtimecodex.SessionEvent{
					RuntimeID:         handle.RuntimeID,
					SessionID:         req.SessionID,
					Kind:              runtimecodex.SessionEventToolCallUpdate,
					ToolCallID:        "tool-1",
					ToolKind:          "exec_command",
					ToolTitle:         "Run shell command",
					ToolStatus:        "completed",
					ToolOutputSummary: `{"output":""}`,
				})
				sink.Publish(runtimecodex.SessionEvent{
					RuntimeID: handle.RuntimeID,
					SessionID: req.SessionID,
					Kind:      runtimecodex.SessionEventTextDelta,
					Text:      "已写入文件：`hello.py`",
					Payload:   map[string]any{"phase": "final_answer"},
				})
				sink.Publish(runtimecodex.SessionEvent{
					RuntimeID: handle.RuntimeID,
					SessionID: req.SessionID,
					Kind:      runtimecodex.SessionEventPromptCompleted,
				})
			}()
			return nil
		},
	}

	svc := NewService(client, prompter, sink)
	svc.promptSettle = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StartBot(ctx, Binding{BotID: "u-codex", RuntimeID: "rt-1", SessionID: "sess-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	defer svc.Close()

	waitFor(t, func() bool {
		return len(client.sentRecords()) >= 3
	})
	records := client.sentRecords()
	assertCodexToolMetadata(t, records[0], "msg-user", "tool-1")
	assertCodexToolMetadata(t, records[1], "msg-user", "tool-1")
	if records[2].Text != "已写入文件：`hello.py`" || records[2].ThreadRootID != "" {
		t.Fatalf("final record = %+v, want delayed top-level final reply", records[2])
	}
	assertCodexFinalMetadata(t, records[2], "msg-user")
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
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if got, want := req.URL.Path, "/api/v1/channels/csgclaw/participants/u-codex/events"; got != want {
					t.Fatalf("event stream path = %q, want %q", got, want)
				}
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

func TestHTTPClientSendMessageUsesParticipantRoute(t *testing.T) {
	t.Parallel()

	client := &HTTPClient{
		BaseURL: "http://example.test",
		Token:   "secret",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if got, want := req.Method, http.MethodPost; got != want {
					t.Fatalf("method = %q, want %q", got, want)
				}
				if got, want := req.URL.Path, "/api/v1/channels/csgclaw/participants/u-codex/messages"; got != want {
					t.Fatalf("send message path = %q, want %q", got, want)
				}
				if got, want := req.Header.Get("Authorization"), "Bearer secret"; got != want {
					t.Fatalf("authorization = %q, want %q", got, want)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"message_id":"m-1"}`)),
				}, nil
			}),
		},
	}

	got, err := client.SendMessage(context.Background(), "u-codex", SendMessageRequest{
		RoomID: "room-1",
		Text:   "hello",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if got.MessageID != "m-1" {
		t.Fatalf("MessageID = %q, want %q", got.MessageID, "m-1")
	}
}

func TestHTTPClientSendMessageUsesMultipartForAttachments(t *testing.T) {
	t.Parallel()

	client := &HTTPClient{
		BaseURL: "http://example.test",
		Token:   "secret",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if err := req.ParseMultipartForm(1024); err != nil {
					t.Fatalf("ParseMultipartForm() error = %v", err)
				}
				var payload SendMessageRequest
				if err := json.Unmarshal([]byte(req.FormValue("payload")), &payload); err != nil {
					t.Fatalf("decode multipart payload: %v", err)
				}
				if payload.RoomID != "room-1" || payload.Text != "generated report" || payload.ThreadRootID != "msg-root" {
					t.Fatalf("multipart payload = %+v", payload)
				}
				files := req.MultipartForm.File["files"]
				if len(files) != 1 || files[0].Filename != "report.txt" {
					t.Fatalf("multipart files = %+v, want report.txt", files)
				}
				file, err := files[0].Open()
				if err != nil {
					t.Fatalf("open multipart file: %v", err)
				}
				defer file.Close()
				data, err := io.ReadAll(file)
				if err != nil || string(data) != "report contents" {
					t.Fatalf("multipart file = %q, err=%v", data, err)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"message_id":"m-attach"}`)),
				}, nil
			}),
		},
	}

	response, err := client.SendMessage(context.Background(), "u-codex", SendMessageRequest{
		RoomID:       "room-1",
		Text:         "generated report",
		ThreadRootID: "msg-root",
		Attachments: []MessageAttachmentUpload{{
			Name:      "report.txt",
			MediaType: "text/plain",
			Data:      []byte("report contents"),
		}},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if response.MessageID != "m-attach" {
		t.Fatalf("MessageID = %q, want m-attach", response.MessageID)
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
var _ MessageUpdater = (*fakeBotClient)(nil)
var _ MessageReactor = (*fakeBotClient)(nil)
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
		prompt: func(context.Context, runtimecodex.SessionHandle, runtimecodex.PromptRequest) error {
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
