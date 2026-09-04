package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	channeltypes "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/presentation"
	feishustate "csgclaw/internal/channel/feishu/state"
	"csgclaw/internal/channel/feishu/transport"
)

type fakeEngine struct{ conversation *fakeConversation }

func (e fakeEngine) Agents() agentengine.AgentInterface                             { return nil }
func (e fakeEngine) RuntimeExtensions(string) agentengine.RuntimeExtensionInterface { return nil }
func (e fakeEngine) Conversations(string) agentengine.ConversationInterface {
	return e.conversation
}

type fakeConversation struct {
	mu       sync.Mutex
	run      func(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult
	requests []agentengine.TurnRequest
	resets   int
}

func (*fakeConversation) Files() agentengine.FileInterface {
	return agentengine.NewFileStore().Scope("agent-1")
}

func (c *fakeConversation) Run(ctx context.Context, req agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
	c.mu.Lock()
	c.requests = append(c.requests, req)
	run := c.run
	c.mu.Unlock()
	if run == nil {
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
	}
	return run(ctx, req, sink)
}
func (*fakeConversation) Cancel(context.Context, agentengine.ConversationKey, agentengine.TurnID) error {
	return nil
}
func (c *fakeConversation) Reset(context.Context, agentengine.ConversationKey) error {
	c.mu.Lock()
	c.resets++
	c.mu.Unlock()
	return nil
}
func (*fakeConversation) Resolve(context.Context, agentengine.InteractionResolution) error {
	return nil
}

type filePreparerFunc func(context.Context, channeltypes.InboundMessage) ([]agentengine.InputPart, func(), error)

func (f filePreparerFunc) Prepare(ctx context.Context, message channeltypes.InboundMessage) ([]agentengine.InputPart, func(), error) {
	return f(ctx, message)
}

func TestRunnerLeavesEngineVisibleSupersedeToEngineWithoutChannelWaitQueue(t *testing.T) {
	firstStarted := make(chan context.Context, 1)
	firstRelease := make(chan struct{})
	secondStarted := make(chan struct{})
	conversation := &fakeConversation{}
	conversation.run = func(ctx context.Context, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		if request.ID == "turn-1" {
			firstStarted <- ctx
			select {
			case <-ctx.Done():
				return agentengine.TurnResult{Status: agentengine.TurnCanceled}
			case <-firstRelease:
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
			}
		}
		close(secondStarted)
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "latest"}
	}
	store := feishustate.NewStore()
	runner, err := NewRunner(RunnerOptions{Engine: fakeEngine{conversation}, State: store})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Submit(context.Background(), runnerMessage("event-1", "turn-1", "conversation", "first")); err != nil {
		t.Fatal(err)
	}
	firstCtx := <-firstStarted
	start := time.Now()
	if err := runner.Submit(context.Background(), runnerMessage("event-2", "turn-2", "conversation", "second")); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("second Submit waited for a channel queue: %s", elapsed)
	}
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("latest turn did not reach Engine")
	}
	if err := firstCtx.Err(); err != nil {
		t.Fatalf("channel canceled a Turn that had already entered Agent Engine: %v", err)
	}
	close(firstRelease)
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	conversation.mu.Lock()
	defer conversation.mu.Unlock()
	if len(conversation.requests) != 2 {
		t.Fatalf("Engine requests = %d, want 2", len(conversation.requests))
	}
	for _, request := range conversation.requests {
		if request.Admission != agentengine.AdmissionSupersede {
			t.Fatalf("admission = %q, want supersede", request.Admission)
		}
	}
}

func TestRunnerFailedReplacementPreflightDoesNotCancelEngineVisibleTurn(t *testing.T) {
	firstStarted := make(chan context.Context, 1)
	firstRelease := make(chan struct{})
	preparationFailed := make(chan struct{})
	conversation := &fakeConversation{run: func(ctx context.Context, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		if request.ID != "turn-1" {
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
		}
		firstStarted <- ctx
		select {
		case <-ctx.Done():
			return agentengine.TurnResult{Status: agentengine.TurnCanceled}
		case <-firstRelease:
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
		}
	}}
	preparer := filePreparerFunc(func(_ context.Context, message channeltypes.InboundMessage) ([]agentengine.InputPart, func(), error) {
		if message.TurnID != "turn-2" {
			return nil, nil, errors.New("unexpected attachment Turn")
		}
		close(preparationFailed)
		return nil, nil, errors.New("attachment unavailable")
	})
	store := feishustate.NewStore()
	runner, err := NewRunner(RunnerOptions{Engine: fakeEngine{conversation}, State: store, Files: preparer})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Submit(context.Background(), runnerMessage("event-1", "turn-1", "conversation", "first")); err != nil {
		t.Fatal(err)
	}
	firstCtx := <-firstStarted
	second := runnerMessage("event-2", "turn-2", "conversation", "second")
	second.Files = []channeltypes.InboundFile{{Kind: "file", ID: "file-1"}}
	if err := runner.Submit(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	<-preparationFailed
	if err := firstCtx.Err(); err != nil {
		t.Fatalf("failed replacement preparation canceled the Engine-visible Turn: %v", err)
	}
	close(firstRelease)
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	conversation.mu.Lock()
	requests := len(conversation.requests)
	conversation.mu.Unlock()
	if requests != 1 {
		t.Fatalf("Engine requests = %d, want only the original Turn", requests)
	}
	if record, ok := store.Get(second.TurnID); !ok || record.Status != channeltypes.TurnFailed {
		t.Fatalf("failed replacement record = %#v, found=%t", record, ok)
	}
}

func TestRunnerCancelsSupersededPreflightBeforeEngine(t *testing.T) {
	preparationStarted := make(chan struct{})
	preparationCanceled := make(chan struct{})
	secondStarted := make(chan struct{})
	conversation := &fakeConversation{run: func(_ context.Context, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		if request.ID == "turn-2" {
			close(secondStarted)
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
	}}
	preparer := filePreparerFunc(func(ctx context.Context, message channeltypes.InboundMessage) ([]agentengine.InputPart, func(), error) {
		if message.TurnID != "turn-1" {
			return nil, nil, errors.New("unexpected attachment Turn")
		}
		close(preparationStarted)
		<-ctx.Done()
		close(preparationCanceled)
		return nil, nil, ctx.Err()
	})
	runner, err := NewRunner(RunnerOptions{Engine: fakeEngine{conversation}, State: feishustate.NewStore(), Files: preparer})
	if err != nil {
		t.Fatal(err)
	}
	first := runnerMessage("event-1", "turn-1", "conversation", "first")
	first.Files = []channeltypes.InboundFile{{Kind: "file", ID: "file-1"}}
	if err := runner.Submit(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	<-preparationStarted
	if err := runner.Submit(context.Background(), runnerMessage("event-2", "turn-2", "conversation", "second")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-preparationCanceled:
	case <-time.After(time.Second):
		t.Fatal("superseded preflight was not canceled")
	}
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("replacement did not reach Engine")
	}
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerWaitTracksSupersededRunUntilItExits(t *testing.T) {
	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	defer func() {
		select {
		case <-firstRelease:
		default:
			close(firstRelease)
		}
	}()
	secondStarted := make(chan struct{})
	conversation := &fakeConversation{run: func(_ context.Context, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		if request.ID == "turn-1" {
			close(firstStarted)
			<-firstRelease
			return agentengine.TurnResult{Status: agentengine.TurnCanceled}
		}
		close(secondStarted)
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
	}}
	runner, err := NewRunner(RunnerOptions{Engine: fakeEngine{conversation}, State: feishustate.NewStore()})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Submit(context.Background(), runnerMessage("event-1", "turn-1", "conversation", "first")); err != nil {
		t.Fatal(err)
	}
	<-firstStarted
	if err := runner.Submit(context.Background(), runnerMessage("event-2", "turn-2", "conversation", "second")); err != nil {
		t.Fatal(err)
	}
	<-secondStarted

	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := runner.Wait(waitCtx); err == nil {
		t.Fatal("Wait returned before the superseded run exited")
	}
	close(firstRelease)
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerResetUsesEngineControl(t *testing.T) {
	conversation := &fakeConversation{}
	store := feishustate.NewStore()
	runner, err := NewRunner(RunnerOptions{Engine: fakeEngine{conversation}, State: store})
	if err != nil {
		t.Fatal(err)
	}
	message := runnerMessage("event-reset", "turn-reset", "conversation", "/new")
	if err := runner.Reset(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	conversation.mu.Lock()
	resets := conversation.resets
	conversation.mu.Unlock()
	if resets != 1 {
		t.Fatalf("Engine Reset calls = %d, want 1", resets)
	}
	if record, ok := store.Get(message.TurnID); !ok || record.Status != channeltypes.TurnSucceeded {
		t.Fatalf("reset record = %#v, found=%t", record, ok)
	}
}

func TestRunnerRendersEngineEventsIntoMemoryDelivery(t *testing.T) {
	runFinished := make(chan struct{})
	conversation := &fakeConversation{run: func(ctx context.Context, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		if err := sink.Emit(ctx, agentengine.TurnEvent{Sequence: 1, Kind: agentengine.TurnEventTextDelta, Text: "answer"}); err != nil {
			t.Fatal(err)
		}
		close(runFinished)
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "answer"}
	}}
	store := feishustate.NewStore()
	runner, err := NewRunner(RunnerOptions{
		Engine: fakeEngine{conversation}, State: store, Presentation: presentation.ModeMarkdown,
	})
	if err != nil {
		t.Fatal(err)
	}
	message := runnerMessage("event", "turn", "conversation", "hello")
	if err := runner.Submit(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	<-runFinished
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	finalID := presentationUpdateID(presentation.ModeMarkdown, message.TurnID, 2, true)
	final, ok := store.Delivery(finalID)
	if !ok || final.Kind != channeltypes.DeliveryMarkdownUpdate || !strings.Contains(final.Text, "answer") {
		t.Fatalf("final delivery = %#v, found=%t", final, ok)
	}
}

func TestRunnerQueuesOutputFilesForChatDelivery(t *testing.T) {
	runFinished := make(chan struct{})
	conversation := &fakeConversation{run: func(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		defer close(runFinished)
		return agentengine.TurnResult{
			Status: agentengine.TurnSucceeded,
			Output: "done",
			Files: []agentengine.OutputFile{
				{OutputFileMetadata: agentengine.OutputFileMetadata{
					ID: "file-image", Name: "result.png", MediaType: "image/png", SizeBytes: 3, SHA256: strings.Repeat("a", 64),
				}},
				{OutputFileMetadata: agentengine.OutputFileMetadata{
					ID: "file-report", Name: "report.pdf", MediaType: "application/pdf", SizeBytes: 3, SHA256: strings.Repeat("b", 64),
				}},
				{OutputFileMetadata: agentengine.OutputFileMetadata{
					ID: "file-svg", Name: "diagram.svg", MediaType: "image/svg+xml", SizeBytes: 3, SHA256: strings.Repeat("c", 64),
				}},
			},
		}
	}}
	store := feishustate.NewStore()
	runner, err := NewRunner(RunnerOptions{
		Engine: fakeEngine{conversation}, State: store, Presentation: presentation.ModeMarkdown,
	})
	if err != nil {
		t.Fatal(err)
	}
	message := runnerMessage("event-files", "turn-files", "conversation", "create a chart")
	if err := runner.Submit(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runFinished:
	case <-time.After(time.Second):
		t.Fatal("Engine run did not finish")
	}
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}

	image, imageOK := store.Delivery(fileDeliveryID(message.TurnID, 0))
	file, fileOK := store.Delivery(fileDeliveryID(message.TurnID, 1))
	svg, svgOK := store.Delivery(fileDeliveryID(message.TurnID, 2))
	if !imageOK || image.Kind != channeltypes.DeliveryFile || image.FileID != "file-image" {
		t.Fatalf("image intent = %#v, found=%t", image, imageOK)
	}
	if !fileOK || file.Kind != channeltypes.DeliveryFile || file.FileID != "file-report" {
		t.Fatalf("file intent = %#v, found=%t", file, fileOK)
	}
	if !svgOK || svg.Kind != channeltypes.DeliveryFile || svg.FileID != "file-svg" {
		t.Fatalf("svg intent = %#v, found=%t", svg, svgOK)
	}
}

func TestFileDeliveryIntentsEnforceOutboundPolicyAndWarnUser(t *testing.T) {
	message := runnerMessage("event-file-policy", "turn-file-policy", "conversation", "create files")
	files := make([]agentengine.OutputFile, 0, maxFeishuOutputFileCount+2)
	for index := 0; index < maxFeishuOutputFileCount+1; index++ {
		files = append(files, agentengine.OutputFile{OutputFileMetadata: agentengine.OutputFileMetadata{
			ID: fmt.Sprintf("file-%d", index), Name: fmt.Sprintf("file-%d.txt", index), MediaType: "text/plain", SizeBytes: 1,
		}})
	}
	files = append(files, agentengine.OutputFile{OutputFileMetadata: agentengine.OutputFileMetadata{
		ID: "file-large", Name: "large.zip", MediaType: "application/zip", SizeBytes: transport.FileUploadLimitBytes + 1,
	}})

	intents := (&Runner{}).fileDeliveryIntents(message, files, 10)
	if len(intents) != maxFeishuOutputFileCount+1 {
		t.Fatalf("intent count = %d, want %d", len(intents), maxFeishuOutputFileCount+1)
	}
	for index, intent := range intents[:maxFeishuOutputFileCount] {
		if intent.Kind != channeltypes.DeliveryFile || intent.FileID != fmt.Sprintf("file-%d", index) || intent.Sequence != uint64(10+index) {
			t.Fatalf("file intent %d = %#v", index, intent)
		}
	}
	warning := intents[len(intents)-1]
	if warning.Kind != channeltypes.DeliveryText || warning.ID != message.TurnID+":files:warning" ||
		warning.Sequence != 10+maxFeishuOutputFileCount || !strings.Contains(warning.Text, "could not send 2 generated file(s)") {
		t.Fatalf("warning intent = %#v", warning)
	}
}

func TestFileDeliveryIntentsEnforceAggregateSizeLimit(t *testing.T) {
	message := runnerMessage("event-file-total", "turn-file-total", "conversation", "create files")
	files := []agentengine.OutputFile{
		{OutputFileMetadata: agentengine.OutputFileMetadata{
			ID: "file-30", Name: "first.zip", MediaType: "application/zip", SizeBytes: transport.FileUploadLimitBytes,
		}},
		{OutputFileMetadata: agentengine.OutputFileMetadata{
			ID: "file-21", Name: "second.zip", MediaType: "application/zip", SizeBytes: 21 << 20,
		}},
	}

	intents := (&Runner{}).fileDeliveryIntents(message, files, 1)
	if len(intents) != 2 || intents[0].Kind != channeltypes.DeliveryFile || intents[0].FileID != "file-30" ||
		intents[1].Kind != channeltypes.DeliveryText || !strings.Contains(intents[1].Text, "could not send 1 generated file(s)") {
		t.Fatalf("intents = %#v", intents)
	}
}

func TestFileDeliveryIntentsRejectEmptyFileAndWarnUser(t *testing.T) {
	message := runnerMessage("event-empty-file", "turn-empty-file", "conversation", "create an empty file")
	files := []agentengine.OutputFile{{OutputFileMetadata: agentengine.OutputFileMetadata{
		ID: "file-empty", Name: "empty.txt", MediaType: "text/plain", SizeBytes: 0,
	}}}

	intents := (&Runner{}).fileDeliveryIntents(message, files, 4)
	if len(intents) != 1 || intents[0].Kind != channeltypes.DeliveryText ||
		intents[0].ID != message.TurnID+":files:warning" || intents[0].Sequence != 4 ||
		!strings.Contains(intents[0].Text, "could not send 1 generated file(s)") {
		t.Fatalf("intents = %#v", intents)
	}
}

func TestRunnerDoesNotQueueOutputFilesForFailedTurn(t *testing.T) {
	runFinished := make(chan struct{})
	conversation := &fakeConversation{run: func(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		defer close(runFinished)
		return agentengine.TurnResult{
			Status: agentengine.TurnFailed,
			Error:  &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: "failed"},
			Files: []agentengine.OutputFile{{OutputFileMetadata: agentengine.OutputFileMetadata{
				ID: "file-failed", Name: "failed.pdf", MediaType: "application/pdf", SizeBytes: 3,
			}}},
		}
	}}
	store := feishustate.NewStore()
	runner, err := NewRunner(RunnerOptions{Engine: fakeEngine{conversation}, State: store})
	if err != nil {
		t.Fatal(err)
	}
	message := runnerMessage("event-failed-files", "turn-failed-files", "conversation", "fail")
	if err := runner.Submit(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runFinished:
	case <-time.After(time.Second):
		t.Fatal("Engine run did not finish")
	}
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if intent, ok := store.Delivery(fileDeliveryID(message.TurnID, 0)); ok {
		t.Fatalf("failed turn queued file delivery: %#v", intent)
	}
}

func TestBaseIntentDoesNotTurnOrdinaryReplyIntoTopic(t *testing.T) {
	message := runnerMessage("event", "turn", "conversation", "hello")
	message.Source.RootID = "root-1"
	message.Source.ParentID = "parent-1"
	intent := baseIntent(message, "intent", 1)
	if intent.ThreadID != "" || intent.ReplyTo != message.Source.MessageID {
		t.Fatalf("ordinary reply intent = %#v, want reply to current message without thread", intent)
	}

	message.Source.ThreadID = "thread-1"
	intent = baseIntent(message, "thread-intent", 1)
	if intent.ThreadID != "thread-1" || intent.ReplyTo != "root-1" {
		t.Fatalf("topic reply intent = %#v, want real thread route", intent)
	}
}

func runnerMessage(eventID, turnID, conversationKey, text string) channeltypes.InboundMessage {
	return channeltypes.InboundMessage{
		Source: channeltypes.Source{
			Channel:   "feishu",
			BindingID: "binding-1",
			EventID:   eventID,
			MessageID: "message-" + eventID,
			ChatID:    "chat-1",
		},
		AgentID:         "agent-1",
		ConversationKey: conversationKey,
		TurnID:          turnID,
		Text:            text,
	}
}

func (*fakeConversation) GetInteraction(context.Context, agentengine.ConversationKey, string) (agentengine.InteractionRequest, error) {
	return agentengine.InteractionRequest{}, &agentengine.TurnError{Code: agentengine.ErrorInteractionNotFound, Message: "no interaction in this test fixture"}
}
