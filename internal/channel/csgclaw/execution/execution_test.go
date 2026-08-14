package execution

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/channelbridge"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/worklease"
)

type fakeEngine struct {
	agentID     string
	request     agentengine.TurnRequest
	run         func(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult
	reset       func(context.Context, agentengine.ConversationKey) error
	resetKey    agentengine.ConversationKey
	cancel      func(context.Context, agentengine.ConversationKey, agentengine.TurnID) error
	cancelCalls atomic.Int32
	resolve     func(context.Context, agentengine.InteractionResolution) error
}

func (f *fakeEngine) Conversations(agentID string) agentengine.ConversationInterface {
	f.agentID = agentID
	return fakeConversation{engine: f}
}

type fakeConversation struct {
	engine *fakeEngine
}

func (fakeConversation) Files() agentengine.FileInterface {
	return nil
}

func (f fakeConversation) Run(ctx context.Context, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
	f.engine.request = request
	if f.engine.run != nil {
		return f.engine.run(ctx, request, sink)
	}
	return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "done"}
}

func (f fakeConversation) Cancel(ctx context.Context, key agentengine.ConversationKey, turnID agentengine.TurnID) error {
	f.engine.cancelCalls.Add(1)
	if f.engine.cancel != nil {
		return f.engine.cancel(ctx, key, turnID)
	}
	return nil
}

func (f fakeConversation) Reset(ctx context.Context, key agentengine.ConversationKey) error {
	f.engine.resetKey = key
	if f.engine.reset != nil {
		return f.engine.reset(ctx, key)
	}
	return nil
}

func (f fakeConversation) Resolve(ctx context.Context, resolution agentengine.InteractionResolution) error {
	if f.engine.resolve != nil {
		return f.engine.resolve(ctx, resolution)
	}
	return nil
}

type fakeRenderer struct {
	events   []agentengine.TurnEvent
	complete []agentengine.TurnResult
}

func (f *fakeRenderer) Emit(_ context.Context, _ channel.TurnContext, event agentengine.TurnEvent) error {
	f.events = append(f.events, event)
	return nil
}

func (f *fakeRenderer) Complete(_ context.Context, _ channel.TurnContext, result agentengine.TurnResult) error {
	f.complete = append(f.complete, result)
	return nil
}

type fakeAttachmentResolver struct {
	released int
}

func (f *fakeAttachmentResolver) Resolve(
	_ context.Context,
	_ channel.Binding,
	_ channelbridge.BotEvent,
	attachment channelbridge.MessageAttachment,
) (agentengine.InputFile, func(), error) {
	return agentengine.InputFile{ID: attachment.ID}, func() { f.released++ }, nil
}

type failingRenderer struct {
	emitErr error
}

type recordingWorkReporter struct {
	mu           sync.Mutex
	starts       []worklease.ParticipantWorkLease
	statuses     []apitypes.ParticipantWorkStatusPatchRequest
	finishes     []string
	controller   agentruntime.TurnController
	unregisters  int
	firstStarted chan struct{}
	startOnce    sync.Once
}

func newRecordingWorkReporter() *recordingWorkReporter {
	return &recordingWorkReporter{firstStarted: make(chan struct{})}
}

func (r *recordingWorkReporter) StartOrRenew(_ context.Context, lease worklease.ParticipantWorkLease) (apitypes.ParticipantWorkUpdate, error) {
	r.mu.Lock()
	r.starts = append(r.starts, lease)
	r.mu.Unlock()
	r.startOnce.Do(func() { close(r.firstStarted) })
	return apitypes.ParticipantWorkUpdate{LeaseID: lease.LeaseID}, nil
}

func (r *recordingWorkReporter) Stop(context.Context, string, string) error {
	r.mu.Lock()
	r.finishes = append(r.finishes, apitypes.ParticipantWorkOutcomeReleased)
	r.mu.Unlock()
	return nil
}

func (r *recordingWorkReporter) Finish(_ context.Context, _, _ string, outcome string) error {
	r.mu.Lock()
	r.finishes = append(r.finishes, outcome)
	r.mu.Unlock()
	return nil
}

func (r *recordingWorkReporter) UpdateStatus(
	_ context.Context,
	_, _ string,
	request apitypes.ParticipantWorkStatusPatchRequest,
) (apitypes.ParticipantWorkUpdate, bool, error) {
	r.mu.Lock()
	r.statuses = append(r.statuses, request)
	r.mu.Unlock()
	return apitypes.ParticipantWorkUpdate{}, true, nil
}

func (r *recordingWorkReporter) RegisterTurnController(_ string, controller agentruntime.TurnController) func() {
	r.mu.Lock()
	r.controller = controller
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		if r.controller == controller {
			r.controller = nil
		}
		r.unregisters++
		r.mu.Unlock()
	}
}

func (r *recordingWorkReporter) snapshot() (
	[]worklease.ParticipantWorkLease,
	[]apitypes.ParticipantWorkStatusPatchRequest,
	[]string,
	agentruntime.TurnController,
	int,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]worklease.ParticipantWorkLease(nil), r.starts...),
		append([]apitypes.ParticipantWorkStatusPatchRequest(nil), r.statuses...),
		append([]string(nil), r.finishes...), r.controller, r.unregisters
}

func (f *failingRenderer) Emit(context.Context, channel.TurnContext, agentengine.TurnEvent) error {
	return f.emitErr
}

func (f *failingRenderer) Complete(context.Context, channel.TurnContext, agentengine.TurnResult) error {
	return nil
}

func TestAdapterRunBuildsRuntimeNeutralTurn(t *testing.T) {
	engine := &fakeEngine{}
	renderer := &fakeRenderer{}
	attachments := &fakeAttachmentResolver{}
	engine.run = func(ctx context.Context, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "answer"}); err != nil {
			t.Fatalf("Emit() error = %v", err)
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "answer"}
	}
	adapter, err := New(
		engine,
		renderer,
		WithAttachmentResolver(attachments),
		WithTurnIDGenerator(func() (agentengine.TurnID, error) { return "turn-fixed", nil }),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	outcome, err := adapter.Run(context.Background(), channel.Binding{
		ID: "binding:manager", ParticipantID: "manager", AgentID: "u-manager",
	}, channelbridge.BotEvent{
		Channel:       string(channel.ChannelCSGClaw),
		ParticipantID: "manager",
		MessageID:     "message-1",
		RoomID:        "room/1",
		ThreadRootID:  "root-1",
		Text:          "please inspect this",
		ThreadContext: &channelbridge.BotThreadContext{
			RootMessageID: "root-1",
			Context: []channelbridge.BotThreadContextMessage{{
				ID: "root-1", SenderID: "u-admin", Content: "original question",
			}},
		},
		Attachments: []channelbridge.MessageAttachment{{
			ID: "attachment-1", Name: "report.pdf", MediaType: "application/pdf", SizeBytes: 42, SHA256: "sha",
		}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.Result.Status != agentengine.TurnSucceeded || engine.agentID != "u-manager" {
		t.Fatalf("outcome = %+v, agentID = %q", outcome, engine.agentID)
	}
	if got, want := string(engine.request.ConversationKey), "csgclaw-im:binding%3Amanager:room:room%2F1:thread:root-1"; got != want {
		t.Fatalf("ConversationKey = %q, want %q", got, want)
	}
	if engine.request.ID != "turn-fixed" || outcome.Turn.SourceMessageID != "message-1" {
		t.Fatalf("turn = %+v, request = %+v", outcome.Turn, engine.request)
	}
	if engine.request.Admission != agentengine.AdmissionWait ||
		engine.request.Continuation != agentengine.ContinuationCreateOrResume ||
		engine.request.Interaction != agentengine.InteractionResolve {
		t.Fatalf("turn policies = %+v", engine.request)
	}
	if len(engine.request.Input) != 3 {
		t.Fatalf("Input = %#v, want hidden text, current text, and file", engine.request.Input)
	}
	if !strings.Contains(engine.request.Input[0].Text, "Hidden thread context") || !strings.Contains(engine.request.Input[1].Text, "Current thread message") {
		t.Fatalf("text input = %#v", engine.request.Input)
	}
	if file := engine.request.Input[2].File; file == nil || file.ID != "attachment-1" {
		t.Fatalf("file input = %#v", engine.request.Input[2])
	}
	if attachments.released != 1 {
		t.Fatalf("release count = %d, want 1", attachments.released)
	}
	if len(renderer.events) != 1 || len(renderer.complete) != 1 {
		t.Fatalf("renderer events = %#v, complete = %#v", renderer.events, renderer.complete)
	}
}

func TestAdapterRunReportsParticipantWorkLeaseLifecycle(t *testing.T) {
	reporter := newRecordingWorkReporter()
	adapter, err := New(
		&fakeEngine{},
		&fakeRenderer{},
		WithParticipantWorkReporter(reporter),
		WithTurnControllerRegistrar(reporter),
		WithTurnIDGenerator(func() (agentengine.TurnID, error) { return "turn-work", nil }),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	outcome, err := adapter.Run(context.Background(), channel.Binding{
		ParticipantID: "pt-worker", AgentID: "agent-worker",
	}, channelbridge.BotEvent{
		MessageID: "message-work", RoomID: "room-work", ThreadRootID: "thread-work", Text: "handle this",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.Result.Status != agentengine.TurnSucceeded {
		t.Fatalf("result = %+v", outcome.Result)
	}

	starts, statuses, finishes, controller, unregisters := reporter.snapshot()
	if len(starts) != 1 {
		t.Fatalf("StartOrRenew calls = %#v, want one start", starts)
	}
	lease := starts[0]
	if lease.ParticipantID != "pt-worker" || lease.RoomID != "room-work" || lease.ThreadRootID != "thread-work" ||
		lease.RequestID != "message-work" || lease.Kind != apitypes.ParticipantWorkKindAgentTurn ||
		!lease.TTLExplicit || lease.TTLSeconds != defaultWorkLeaseTTL || !worklease.ValidID(lease.LeaseID) {
		t.Fatalf("lease = %+v", lease)
	}
	if len(statuses) != 1 || statuses[0].Sequence != 1 || statuses[0].Phase != apitypes.ParticipantWorkPhaseWorking ||
		len(statuses[0].Capabilities) != 1 || statuses[0].Capabilities[0] != apitypes.ParticipantWorkCapabilityTurnStopV1 {
		t.Fatalf("status updates = %#v", statuses)
	}
	if len(finishes) != 1 || finishes[0] != apitypes.ParticipantWorkOutcomeReleased {
		t.Fatalf("finish outcomes = %#v", finishes)
	}
	if controller != nil || unregisters != 1 {
		t.Fatalf("controller = %T, unregisters = %d", controller, unregisters)
	}
}

func TestAdapterWorkStopCancelsTurnAndFinishesStopped(t *testing.T) {
	reporter := newRecordingWorkReporter()
	engineStarted := make(chan struct{})
	engine := &fakeEngine{run: func(ctx context.Context, _ agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		close(engineStarted)
		<-ctx.Done()
		return agentengine.TurnResult{Status: agentengine.TurnCanceled, Error: &agentengine.TurnError{
			Code: agentengine.ErrorRuntimeFailed, Message: ctx.Err().Error(),
		}}
	}}
	adapter, err := New(
		engine,
		&fakeRenderer{},
		WithParticipantWorkReporter(reporter),
		WithTurnControllerRegistrar(reporter),
		WithTurnIDGenerator(func() (agentengine.TurnID, error) { return "turn-stop", nil }),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result := make(chan channel.Outcome, 1)
	go func() {
		outcome, _ := adapter.Run(context.Background(), channel.Binding{
			ParticipantID: "pt-worker", AgentID: "agent-worker",
		}, channelbridge.BotEvent{MessageID: "message-stop", RoomID: "room-stop", Text: "keep working"})
		result <- outcome
	}()
	select {
	case <-engineStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Engine turn")
	}
	starts, _, _, controller, _ := reporter.snapshot()
	if len(starts) != 1 || controller == nil {
		t.Fatalf("starts = %#v, controller = %T", starts, controller)
	}
	lease := starts[0]
	if err := controller.StopTurn(context.Background(), agentruntime.TurnRef{
		ParticipantID: lease.ParticipantID,
		RoomID:        lease.RoomID,
		LeaseID:       lease.LeaseID,
		RequestID:     lease.RequestID,
	}); err != nil {
		t.Fatalf("StopTurn() error = %v", err)
	}

	select {
	case outcome := <-result:
		if outcome.Result.Status != agentengine.TurnCanceled {
			t.Fatalf("result = %+v", outcome.Result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stopped turn did not return")
	}
	_, _, finishes, controller, unregisters := reporter.snapshot()
	if len(finishes) != 1 || finishes[0] != apitypes.ParticipantWorkOutcomeStopped {
		t.Fatalf("finish outcomes = %#v", finishes)
	}
	if controller != nil || unregisters != 1 {
		t.Fatalf("controller = %T, unregisters = %d", controller, unregisters)
	}
	if engine.cancelCalls.Load() != 1 {
		t.Fatalf("Engine Cancel calls = %d, want 1", engine.cancelCalls.Load())
	}
}

func TestAdapterRenewsParticipantWorkLeaseUntilTurnCompletes(t *testing.T) {
	reporter := newRecordingWorkReporter()
	release := make(chan struct{})
	engine := &fakeEngine{run: func(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		<-release
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "done"}
	}}
	adapter, err := New(
		engine,
		&fakeRenderer{},
		WithParticipantWorkReporter(reporter),
		WithTurnIDGenerator(func() (agentengine.TurnID, error) { return "turn-renew", nil }),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	adapter.work.renewEvery = 5 * time.Millisecond

	done := make(chan struct{})
	go func() {
		_, _ = adapter.Run(context.Background(), channel.Binding{
			ParticipantID: "pt-worker", AgentID: "agent-worker",
		}, channelbridge.BotEvent{MessageID: "message-renew", RoomID: "room-renew", Text: "keep working"})
		close(done)
	}()
	select {
	case <-reporter.firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for work lease start")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		starts, _, _, _, _ := reporter.snapshot()
		if len(starts) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("StartOrRenew calls = %d, want at least one renewal", len(starts))
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("turn did not complete")
	}
	_, _, finishes, _, _ := reporter.snapshot()
	if len(finishes) != 1 || finishes[0] != apitypes.ParticipantWorkOutcomeReleased {
		t.Fatalf("finish outcomes = %#v", finishes)
	}
}

func TestAdapterRunRejectsAttachmentsWithoutResolver(t *testing.T) {
	engine := &fakeEngine{}
	renderer := &fakeRenderer{}
	adapter, err := New(engine, renderer, WithTurnIDGenerator(func() (agentengine.TurnID, error) { return "turn-fixed", nil }))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	outcome, err := adapter.Run(context.Background(), channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}, channelbridge.BotEvent{
		MessageID: "message-1",
		RoomID:    "room-1",
		Text:      "see file",
		Attachments: []channelbridge.MessageAttachment{{
			ID: "attachment-1", Name: "report.pdf",
		}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.Result.Error == nil || outcome.Result.Error.Code != agentengine.ErrorFileUnavailable {
		t.Fatalf("result = %+v", outcome.Result)
	}
	if engine.request.ID != "" {
		t.Fatalf("engine unexpectedly called with request %+v", engine.request)
	}
	if len(renderer.complete) != 1 {
		t.Fatalf("complete calls = %d, want 1", len(renderer.complete))
	}
}

func TestAdapterRunKeepsRendererFailureOnEnginePath(t *testing.T) {
	engine := &fakeEngine{}
	renderer := &failingRenderer{emitErr: errors.New("store unavailable")}
	engine.run = func(ctx context.Context, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "partial"}); err != nil {
			return agentengine.TurnResult{
				Status: agentengine.TurnFailed,
				Error:  &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: err.Error()},
			}
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
	}
	adapter, err := New(engine, renderer, WithTurnIDGenerator(func() (agentengine.TurnID, error) { return "turn-fixed", nil }))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	outcome, err := adapter.Run(context.Background(), channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}, channelbridge.BotEvent{
		MessageID: "message-1", RoomID: "room-1", Text: "hello",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.Result.Error == nil || !strings.Contains(outcome.Result.Error.Message, "store unavailable") {
		t.Fatalf("result = %+v", outcome.Result)
	}
}

func TestAdapterHandleResetUsesEngineConversation(t *testing.T) {
	engine := &fakeEngine{}
	adapter, err := New(engine, &fakeRenderer{}, WithTurnIDGenerator(func() (agentengine.TurnID, error) {
		return "turn-reset", nil
	}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	outcome, err := adapter.Handle(context.Background(), channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}, channelbridge.BotEvent{
		MessageID: "message-new",
		RoomID:    "room-1",
		Text:      `<slash-command name="new" arg="conversation"></slash-command>`,
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if outcome.Result.Status != agentengine.TurnSucceeded {
		t.Fatalf("result = %+v", outcome.Result)
	}
	if engine.request.ID != "" {
		t.Fatalf("engine unexpectedly ran request %+v", engine.request)
	}
	if string(engine.resetKey) == "" {
		t.Fatal("Reset conversation key is empty")
	}
}

func TestAdapterHandleResetMapsEngineError(t *testing.T) {
	engine := &fakeEngine{reset: func(context.Context, agentengine.ConversationKey) error {
		return &agentengine.TurnError{Code: agentengine.ErrorRuntimeAdapterUnavailable, Message: "reset unavailable"}
	}}
	renderer := &fakeRenderer{}
	adapter, err := New(engine, renderer, WithTurnIDGenerator(func() (agentengine.TurnID, error) {
		return "turn-reset", nil
	}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	outcome, err := adapter.Handle(context.Background(), channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}, channelbridge.BotEvent{
		MessageID: "message-new",
		RoomID:    "room-1",
		Text:      `<slash-command name="new" arg="conversation"></slash-command>`,
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if outcome.Result.Status != agentengine.TurnFailed || outcome.Result.Error == nil ||
		outcome.Result.Error.Code != agentengine.ErrorRuntimeAdapterUnavailable {
		t.Fatalf("result = %+v", outcome.Result)
	}
	if len(renderer.complete) != 1 {
		t.Fatalf("complete calls = %d, want 1", len(renderer.complete))
	}
}

func TestAdapterDerivesStableScopedTurnIDFromSourceMessage(t *testing.T) {
	adapter, err := New(&fakeEngine{}, &fakeRenderer{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	binding := channel.Binding{ID: "binding-a", ParticipantID: "pt-a", AgentID: "agent-a"}
	event := channelbridge.BotEvent{MessageID: "message-1", RoomID: "room-1", Text: "hello"}
	first, err := adapter.turnContext(binding, event)
	if err != nil {
		t.Fatal(err)
	}
	second, err := adapter.turnContext(binding, event)
	if err != nil {
		t.Fatal(err)
	}
	if first.TurnID == "" || first.TurnID != second.TurnID {
		t.Fatalf("Turn IDs = %q and %q, want one stable non-empty ID", first.TurnID, second.TurnID)
	}
	event.MessageID = "message-2"
	different, err := adapter.turnContext(binding, event)
	if err != nil {
		t.Fatal(err)
	}
	if different.TurnID == first.TurnID {
		t.Fatalf("different source messages shared Turn ID %q", first.TurnID)
	}
}

func TestAdapterHandleSkipsEmptyEvents(t *testing.T) {
	engine := &fakeEngine{}
	adapter, err := New(engine, &fakeRenderer{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	outcome, err := adapter.Handle(context.Background(), channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}, channelbridge.BotEvent{
		MessageID: "message-empty",
		RoomID:    "room-1",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if outcome.Turn.TurnID != "" || engine.request.ID != "" {
		t.Fatalf("empty event was executed: outcome=%+v request=%+v", outcome, engine.request)
	}
}
