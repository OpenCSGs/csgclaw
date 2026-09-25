package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"

	"csgclaw/internal/agentengine"
	channeltypes "csgclaw/internal/channel"
	feishuctx "csgclaw/internal/channel/feishu/context"
	"csgclaw/internal/channel/feishu/presentation"
	feishustate "csgclaw/internal/channel/feishu/state"
	"csgclaw/internal/channel/feishu/transport"
	"csgclaw/internal/slashcommand"
)

const (
	processingPinEmoji       = "Pin"
	maxFeishuOutputFileCount = 8
	maxFeishuOutputFileTotal = int64(50 << 20)
)

type FilePreparer interface {
	Prepare(context.Context, channeltypes.InboundMessage) ([]agentengine.InputPart, func(), error)
}

type Notifier interface {
	Notify()
}

type runnerState interface {
	Delivery(string) (channeltypes.DeliveryIntent, bool)
	ResolveControlTarget(feishustate.ControlQuery) (feishustate.ControlTarget, bool)
	MarkCanceling(string)
	RetryCOTCompletion(string) error
	Put(channeltypes.TurnRecord) error
	Get(string) (channeltypes.TurnRecord, bool)
	Enqueue(channeltypes.DeliveryIntent) error
	BeginTurn(channeltypes.TurnRecord) error
	AppendTurnDeliveries(string, uint64, ...channeltypes.DeliveryIntent) error
	FinishTurn(string, channeltypes.TurnStatus, ...channeltypes.DeliveryIntent) error
}

type RunnerOptions struct {
	Engine   agentengine.Interface
	State    *feishustate.Store
	Files    FilePreparer
	Notifier Notifier
}

// Runner is the only Feishu component allowed to invoke Agent Engine Run. It
// records only process-local presentation dependencies and never performs
// Feishu network calls.
type Runner struct {
	engine   agentengine.Interface
	state    runnerState
	files    FilePreparer
	notifier Notifier

	controlMu      sync.Mutex
	controls       map[string]*conversationControl
	interactionMu  sync.Mutex
	interactions   map[string]*pendingInteraction
	latest         map[string]string
	workerContexts map[string]context.Context
	mu             sync.Mutex
	active         map[string]*activeRun
	runs           map[*activeRun]struct{}
}

type activeRun struct {
	agentID       string
	key           string
	turnID        string
	engineEntered bool // guarded by Runner.mu
	cancel        context.CancelFunc
	done          chan struct{}
}

func NewRunner(options RunnerOptions) (*Runner, error) {
	if options.Engine == nil {
		return nil, fmt.Errorf("feishu runner: agent engine is required")
	}
	if options.State == nil {
		return nil, fmt.Errorf("feishu runner: memory state is required")
	}
	return &Runner{
		engine:         options.Engine,
		state:          options.State,
		files:          options.Files,
		notifier:       options.Notifier,
		active:         make(map[string]*activeRun),
		controls:       make(map[string]*conversationControl),
		interactions:   make(map[string]*pendingInteraction),
		latest:         make(map[string]string),
		workerContexts: make(map[string]context.Context),
		runs:           make(map[*activeRun]struct{}),
	}, nil
}

// Submit starts an Engine Run from the binding worker context. It does not
// queue conversations; Agent Engine AdmissionSupersede owns replacement.
func (r *Runner) Submit(ctx context.Context, message channeltypes.InboundMessage) error {
	release, err := r.acquireControl(ctx, message.ConversationKey)
	if err != nil {
		return err
	}
	defer release()
	return r.submit(ctx, message)
}
func (r *Runner) submit(ctx context.Context, message channeltypes.InboundMessage) error {
	if err := validateMessage(message); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	r.interactionMu.Lock()
	r.latest[message.ConversationKey] = message.TurnID
	r.workerContexts[message.ConversationKey] = ctx
	r.interactionMu.Unlock()
	record := turnRecord(message, channeltypes.TurnAccepted)
	if err := r.state.Put(record); err != nil {
		return fmt.Errorf("record accepted Feishu turn: %w", err)
	}
	if isChatReply(message) {
		if err := r.enqueueProcessStart(message); err != nil {
			return err
		}
		if err := r.enqueueProcessingReaction(message); err != nil {
			return err
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	active := &activeRun{
		agentID: message.AgentID,
		key:     message.ConversationKey,
		turnID:  message.TurnID,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
	r.mu.Lock()
	previous := r.active[message.ConversationKey]
	r.active[message.ConversationKey] = active
	r.runs[active] = struct{}{}
	// The channel owns only work that has not crossed into Agent Engine. The
	// transition to engineEntered uses this same lock, so a replacement either
	// cancels preflight or leaves an Engine-visible Turn to AdmissionSupersede.
	canceledPreviousPreflight := previous != nil && !previous.engineEntered
	if canceledPreviousPreflight {
		previous.cancel()
	}
	r.mu.Unlock()
	slog.Debug("accepted Feishu turn",
		messageLogAttrs(message,
			"canceled_previous_preflight", canceledPreviousPreflight,
		)...)
	go r.run(runCtx, active, message)
	return nil
}

func (r *Runner) run(ctx context.Context, active *activeRun, message channeltypes.InboundMessage) {
	process := presentation.NewProcess(message.TurnID, message.ConversationKey)
	defer func() {
		active.cancel()
		close(active.done)
		r.mu.Lock()
		if r.active[active.key] == active {
			delete(r.active, active.key)
		}
		delete(r.runs, active)
		r.mu.Unlock()
	}()
	if isChatReply(message) {
		defer r.finishProcess(message, process)
	}
	input := make([]agentengine.InputPart, 0, len(message.Files)+1)
	if prompt := feishuctx.MessagePrompt(message); prompt != "" {
		input = append(input, agentengine.InputPart{Kind: agentengine.InputPartText, Text: prompt})
	}
	cleanupFiles := func() {}
	if len(message.Files) > 0 {
		if r.files == nil {
			slog.Warn("prepare Feishu turn files failed",
				messageLogAttrs(message,
					"error_code", agentengine.ErrorFileUnavailable,
					"error", "Feishu attachment handling is unavailable",
				)...)
			if err := r.finalize(message, agentengine.TurnResult{
				Status: agentengine.TurnFailed,
				Error:  &agentengine.TurnError{Code: agentengine.ErrorFileUnavailable, Message: "Feishu attachment handling is unavailable"},
			}, true, presentation.Rendered{}); err != nil {
				r.logFinalizeError(message, err)
			}
			return
		}
		files, cleanup, err := r.files.Prepare(ctx, message)
		if cleanup != nil {
			cleanupFiles = cleanup
		}
		if err != nil {
			cleanupFiles()
			slog.Warn("prepare Feishu turn files failed",
				messageLogAttrs(message,
					"error_code", agentengine.ErrorCodeOf(err),
					"error", err,
				)...)
			if finalizeErr := r.finalize(message, resultFromError(err), true, presentation.Rendered{}); finalizeErr != nil {
				r.logFinalizeError(message, finalizeErr)
			}
			return
		}
		input = append(input, files...)
	}
	defer cleanupFiles()
	if len(input) == 0 {
		slog.Warn("reject Feishu turn without supported input",
			messageLogAttrs(message,
				"error_code", agentengine.ErrorInvalidRequest,
			)...)
		if err := r.finalize(message, agentengine.TurnResult{
			Status: agentengine.TurnFailed,
			Error:  &agentengine.TurnError{Code: agentengine.ErrorInvalidRequest, Message: "Feishu message contains no supported text or attachment"},
		}, true, presentation.Rendered{}); err != nil {
			r.logFinalizeError(message, err)
		}
		return
	}
	if err := r.state.BeginTurn(turnRecord(message, channeltypes.TurnRunning)); err != nil {
		slog.Error("record running Feishu turn failed", messageLogAttrs(message, "error", err)...)
		return
	}
	if !r.markEngineEntered(ctx, active) {
		slog.Debug("cancel Feishu turn before Agent Engine run", messageLogAttrs(message)...)
		if err := r.finalize(message, resultFromError(context.Canceled), true, presentation.Rendered{}); err != nil {
			r.logFinalizeError(message, err)
		}
		return
	}

	slog.Debug("start Feishu Agent Engine run",
		messageLogAttrs(message,
			"input_part_count", len(input),
		)...)
	progress := presentation.NewProgress()
	result := r.engine.Conversations(message.AgentID).Run(ctx, agentengine.TurnRequest{
		ID:              agentengine.TurnID(message.TurnID),
		ConversationKey: agentengine.ConversationKey(message.ConversationKey),
		Input:           input,
		Admission:       agentengine.AdmissionSupersede,
		Continuation:    agentengine.ContinuationCreateOrResume,
		Interaction:     interactionPolicy(message),
	}, agentengine.EventSinkFunc(func(_ context.Context, event agentengine.TurnEvent) error {
		if !isChatReply(message) {
			return nil
		}
		if err := r.observeInteraction(ctx, message, event); err != nil {
			return err
		}
		if err := r.enqueueProcess(message, event.Sequence, process.Observe(event)); err != nil {
			return err
		}
		rendered, flush := progress.Observe(event)
		if !flush {
			return nil
		}
		intents := r.replyIntents(message, event.Sequence, false, rendered)
		if err := r.state.AppendTurnDeliveries(message.TurnID, event.Sequence, intents...); err != nil {
			return err
		}
		r.notify()
		return nil
	}))
	result = normalizeTerminalResult(result)
	for _, item := range result.Interactions {
		if err := r.registerInteraction(ctx, message, item); err != nil {
			r.logFinalizeError(message, err)
		}
	}
	r.logTerminalResult(message, result)
	if err := r.finalize(message, result, true, progress.Finalize(presentationResult(result))); err != nil {
		r.logFinalizeError(message, err)
	}
}

// Reset handles /new through Engine's atomic active-Turn Reset operation.
func (r *Runner) Reset(ctx context.Context, message channeltypes.InboundMessage) error {
	release, err := r.acquireControl(ctx, message.ConversationKey)
	if err != nil {
		return err
	}
	defer release()
	if err := validateMessage(message); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.interactionMu.Lock()
	r.latest[message.ConversationKey] = message.TurnID
	r.interactionMu.Unlock()
	if err := r.state.Put(turnRecord(message, channeltypes.TurnAccepted)); err != nil {
		return fmt.Errorf("record accepted Feishu reset: %w", err)
	}
	slog.Debug("accepted Feishu reset", messageLogAttrs(message)...)
	if active := r.current(message.ConversationKey); active != nil {
		// Engine cannot see attachment preparation, so cancel it before crossing
		// the atomic Reset boundary. A late Run observes its canceled context.
		slog.Debug("cancel active Feishu turn before reset",
			messageLogAttrs(message,
				"active_turn_id", active.turnID,
			)...)
		active.cancel()
	}
	if err := r.state.BeginTurn(turnRecord(message, channeltypes.TurnRunning)); err != nil {
		return fmt.Errorf("record running Feishu reset: %w", err)
	}
	err = r.engine.Conversations(message.AgentID).Reset(ctx, agentengine.ConversationKey(message.ConversationKey))
	if err != nil {
		result := resultFromError(err)
		slog.Warn("Feishu Agent Engine reset failed",
			messageLogAttrs(message, resultLogAttrs(result)...,
			)...)
		return r.finalize(message, result, true, presentation.Rendered{})
	}
	intent := r.messageCardIntent(message, message.TurnID+":reset", 1, "Cleared my internal history for this conversation. The IM room messages were not cleared.")
	if err := r.state.FinishTurn(message.TurnID, channeltypes.TurnSucceeded, intent); err != nil {
		return fmt.Errorf("record terminal Feishu reset: %w", err)
	}
	slog.Debug("Feishu Agent Engine reset completed", messageLogAttrs(message, "status", channeltypes.TurnSucceeded)...)
	r.notify()
	return nil
}

// Cancel stops channel-side preparation for the exact active Turn and then
// delegates cancellation to Agent Engine. The local cancellation matters in
// the short interval before Engine.Run is entered (for example while an
// attachment is downloading); it never substitutes a Runtime call.
func (r *Runner) Cancel(ctx context.Context, agentID, conversationKey, turnID string) error {
	agentID = strings.TrimSpace(agentID)
	conversationKey = strings.TrimSpace(conversationKey)
	turnID = strings.TrimSpace(turnID)
	if agentID == "" || conversationKey == "" || turnID == "" {
		return &agentengine.TurnError{Code: agentengine.ErrorInvalidRequest, Message: "agent, conversation, and turn IDs are required"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	slog.Info("cancel Feishu Agent Engine turn requested",
		"agent_id", agentID,
		"conversation_key", conversationKey,
		"turn_id", turnID,
	)
	active := r.current(conversationKey)
	if active != nil && active.agentID == agentID && active.turnID == turnID {
		active.cancel()
	}
	if err := r.engine.Conversations(agentID).Cancel(ctx,
		agentengine.ConversationKey(conversationKey), agentengine.TurnID(turnID)); err != nil {
		return err
	}
	if active != nil && active.agentID == agentID && active.turnID == turnID {
		select {
		case <-active.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (r *Runner) finalize(message channeltypes.InboundMessage, result agentengine.TurnResult, includePresentation bool, terminal presentation.Rendered) error {
	result = normalizeTerminalResult(result)
	status := channeltypes.TurnFailed
	switch result.Status {
	case agentengine.TurnSucceeded:
		status = channeltypes.TurnSucceeded
	case agentengine.TurnCanceled:
		status = channeltypes.TurnCanceled
	case agentengine.TurnFailed:
		status = channeltypes.TurnFailed
	}
	if result.Error != nil {
		if status == channeltypes.TurnSucceeded {
			status = channeltypes.TurnFailed
		}
	}
	finalSequence := uint64(1)
	if record, ok := r.state.Get(message.TurnID); ok {
		finalSequence = record.LastSequence + 1
	}

	intents := make([]channeltypes.DeliveryIntent, 0, 3)
	if status != channeltypes.TurnCanceled && isCommentReply(message) {
		text := strings.TrimSpace(result.Output)
		if status == channeltypes.TurnFailed {
			text = userFacingError(result.Error)
		} else if text == "" {
			text = "Done."
		}
		intents = append(intents, r.commentIntent(message, message.TurnID+":final", finalSequence, text))
	}
	if includePresentation && isChatReply(message) {
		if len(terminal.Cards) == 0 {
			terminal = presentation.Terminal(presentationResult(result))
		}
		intents = append(intents, r.replyIntents(message, finalSequence, true, terminal)...)
		if status == channeltypes.TurnSucceeded {
			intents = append(intents, r.fileDeliveryIntents(message, result.Files, finalSequence+1)...)
		}
		if cleanup, ok := r.reactionCleanupIntent(message); ok {
			intents = append(intents, cleanup)
		}
	}
	// Agent Engine currently returns terminal state only as TurnResult, not a
	// terminal TurnEvent. Record the result and terminal deliveries together in
	// process memory without inventing an Engine event.
	if err := r.state.FinishTurn(message.TurnID, status, intents...); err != nil {
		return fmt.Errorf("record terminal Feishu turn: %w", err)
	}
	r.notify()
	return nil
}

func normalizeTerminalResult(result agentengine.TurnResult) agentengine.TurnResult {
	switch result.Status {
	case agentengine.TurnSucceeded:
		if result.Error != nil {
			result.Status = agentengine.TurnFailed
		}
	case agentengine.TurnFailed, agentengine.TurnCanceled:
	default:
		result.Status = agentengine.TurnFailed
		result.Error = &agentengine.TurnError{
			Code:    agentengine.ErrorRuntimeFailed,
			Message: "Agent Engine returned no terminal status",
		}
	}
	return result
}

func presentationResult(result agentengine.TurnResult) agentengine.TurnResult {
	result = normalizeTerminalResult(result)
	if result.Status == agentengine.TurnFailed {
		result.Error = &agentengine.TurnError{
			Code:    agentengine.ErrorCodeOf(result.Error),
			Message: userFacingError(result.Error),
		}
	}
	return result
}

func (r *Runner) commentIntent(message channeltypes.InboundMessage, id string, sequence uint64, text string) channeltypes.DeliveryIntent {
	intent := baseIntent(message, id, sequence)
	intent.Kind = channeltypes.DeliveryCommentReply
	intent.Text = truncateRunes(strings.TrimSpace(text), 2000)
	return intent
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if limit <= 0 {
		return ""
	}
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}

func (r *Runner) messageCardIntent(message channeltypes.InboundMessage, id string, sequence uint64, text string) channeltypes.DeliveryIntent {
	text = strings.TrimSpace(text)
	intent := baseIntent(message, id, sequence)
	intent.Kind = channeltypes.DeliveryCard
	intent.Card = presentation.Card(text)
	return intent
}

func (r *Runner) fileDeliveryIntents(message channeltypes.InboundMessage, files []agentengine.OutputFile, startSequence uint64) []channeltypes.DeliveryIntent {
	if len(files) == 0 {
		return nil
	}
	intents := make([]channeltypes.DeliveryIntent, 0, len(files)+1)
	var totalSize int64
	rejected := 0
	for index, file := range files {
		fileID := strings.TrimSpace(file.ID)
		if reason := outputFileRejection(file, len(intents), totalSize); reason != "" {
			slog.Warn("skip unsupported Feishu output file delivery",
				messageLogAttrs(message,
					"file_id", fileID,
					"file_name", file.Name,
					"file_media_type", file.MediaType,
					"file_size_bytes", file.SizeBytes,
					"reason", reason,
				)...)
			rejected++
			continue
		}
		intent := baseIntent(message, fileDeliveryID(message.TurnID, index), startSequence+uint64(len(intents)))
		intent.Kind = channeltypes.DeliveryFile
		intent.FileID = fileID
		intents = append(intents, intent)
		totalSize += file.SizeBytes
	}
	if rejected > 0 {
		warning := baseIntent(message, strings.TrimSpace(message.TurnID)+":files:warning", startSequence+uint64(len(intents)))
		warning.Kind = channeltypes.DeliveryCard
		warning.Text = fmt.Sprintf(
			"Feishu could not send %d generated file(s) because their metadata or delivery limits were invalid (maximum %d files, %d MiB per file, %d MiB total).",
			rejected, maxFeishuOutputFileCount, transport.FileUploadLimitBytes>>20, maxFeishuOutputFileTotal>>20,
		)
		warning.Card = presentation.Card(warning.Text)
		warning.Text = ""
		intents = append(intents, warning)
	}
	return intents
}

func outputFileRejection(file agentengine.OutputFile, accepted int, acceptedBytes int64) string {
	if strings.TrimSpace(file.ID) == "" {
		return "Engine file ID is empty"
	}
	if file.SizeBytes <= 0 {
		return "file size must be positive"
	}
	if file.SizeBytes > transport.FileUploadLimitBytes {
		return "file exceeds Feishu's per-file upload limit"
	}
	if accepted >= maxFeishuOutputFileCount {
		return "Turn exceeds the Feishu output file count limit"
	}
	if file.SizeBytes > maxFeishuOutputFileTotal-acceptedBytes {
		return "Turn exceeds the Feishu output file total size limit"
	}
	return ""
}

func fileDeliveryID(turnID string, index int) string {
	return strings.TrimSpace(turnID) + fmt.Sprintf(":file:%d", index+1)
}

func (r *Runner) deliveryExists(id string) bool {
	_, found := r.state.Delivery(id)
	return found
}

func (r *Runner) enqueueProcessingReaction(message channeltypes.InboundMessage) error {
	if strings.TrimSpace(message.Source.MessageID) == "" {
		return nil
	}
	intent := baseIntent(message, processingReactionID(message.TurnID), 0)
	intent.Kind = channeltypes.DeliveryReactionAdd
	intent.MessageID = message.Source.MessageID
	intent.EmojiType = processingPinEmoji
	if err := r.state.Enqueue(intent); err != nil {
		return fmt.Errorf("queue Feishu processing reaction: %w", err)
	}
	r.notify()
	return nil
}

func (r *Runner) reactionCleanupIntent(message channeltypes.InboundMessage) (channeltypes.DeliveryIntent, bool) {
	if strings.TrimSpace(message.Source.MessageID) == "" || !r.deliveryExists(processingReactionID(message.TurnID)) {
		return channeltypes.DeliveryIntent{}, false
	}
	intent := baseIntent(message, message.TurnID+":reaction:delete", math.MaxUint64)
	intent.Kind = channeltypes.DeliveryReactionDelete
	intent.MessageID = message.Source.MessageID
	intent.RelatedID = processingReactionID(message.TurnID)
	return intent, true
}

func baseIntent(message channeltypes.InboundMessage, id string, sequence uint64) channeltypes.DeliveryIntent {
	threadID := strings.TrimSpace(message.Source.ThreadID)
	replyTo := ""
	if threadID != "" {
		replyTo = firstNonEmpty(message.Source.RootID, message.Source.ParentID, message.Source.MessageID)
	} else if strings.TrimSpace(message.Source.RootID) != "" || strings.TrimSpace(message.Source.ParentID) != "" {
		// Reply to an ordinary quoted message without asking Feishu to create a
		// topic. ReplyInThread is derived from the real ThreadID downstream.
		replyTo = strings.TrimSpace(message.Source.MessageID)
	}
	intent := channeltypes.DeliveryIntent{
		RequesterID: message.Source.SenderID,
		ID:          id,
		BindingID:   message.Source.BindingID,
		TurnID:      message.TurnID,
		Sequence:    sequence,
		Status:      channeltypes.DeliveryPending,
		ChatID:      message.Source.ChatID,
		ReplyTo:     replyTo,
		ThreadID:    threadID,
	}
	if target := message.ReplyTarget; target != nil {
		intent.ResourceID = strings.TrimSpace(target.ResourceID)
		intent.ResourceType = strings.TrimSpace(target.ResourceType)
		intent.ParentID = strings.TrimSpace(target.ParentID)
		intent.TopLevel = target.TopLevel
	}
	return intent
}

func turnRecord(message channeltypes.InboundMessage, status channeltypes.TurnStatus) channeltypes.TurnRecord {
	return channeltypes.TurnRecord{
		TurnID:          message.TurnID,
		AgentID:         message.AgentID,
		BindingID:       message.Source.BindingID,
		ConversationKey: message.ConversationKey,
		Status:          status,
	}
}

func validateMessage(message channeltypes.InboundMessage) error {
	hasChat := strings.TrimSpace(message.Source.ChatID) != ""
	hasComment := isCommentReply(message) && strings.TrimSpace(message.ReplyTarget.ResourceID) != "" &&
		strings.TrimSpace(message.ReplyTarget.ResourceType) != "" && strings.TrimSpace(message.ReplyTarget.ParentID) != ""
	if strings.TrimSpace(message.AgentID) == "" || strings.TrimSpace(message.Source.BindingID) == "" ||
		strings.TrimSpace(message.Source.EventID) == "" || (!hasChat && !hasComment) ||
		strings.TrimSpace(message.ConversationKey) == "" || strings.TrimSpace(message.TurnID) == "" {
		return fmt.Errorf("feishu runner: agent, binding, source event, delivery target, conversation, and turn IDs are required")
	}
	return nil
}

func isCommentReply(message channeltypes.InboundMessage) bool {
	return message.ReplyTarget != nil && strings.TrimSpace(message.ReplyTarget.Kind) == channeltypes.ReplyTargetComment
}

func isChatReply(message channeltypes.InboundMessage) bool {
	return !isCommentReply(message)
}

func (r *Runner) current(key string) *activeRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active[strings.TrimSpace(key)]
}

// markEngineEntered makes the preflight-to-Engine ownership transfer atomic
// with Submit's decision to cancel an older preflight.
func (r *Runner) markEngineEntered(ctx context.Context, active *activeRun) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx.Err() != nil {
		return false
	}
	active.engineEntered = true
	return true
}

func (r *Runner) ActiveTurn(key string) string {
	if active := r.current(key); active != nil {
		return active.turnID
	}
	return ""
}

// Wait blocks until all Runs owned by this binding runner have observed
// cancellation and completed their in-memory cleanup.
func (r *Runner) Wait(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		r.mu.Lock()
		active := make([]*activeRun, 0, len(r.runs))
		for turn := range r.runs {
			active = append(active, turn)
		}
		r.mu.Unlock()
		if len(active) == 0 {
			return nil
		}
		for _, turn := range active {
			turn.cancel()
			select {
			case <-turn.done:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (r *Runner) notify() {
	if r.notifier != nil {
		r.notifier.Notify()
	}
}

func (r *Runner) logFinalizeError(message channeltypes.InboundMessage, err error) {
	slog.Error("record terminal Feishu turn failed",
		messageLogAttrs(message, "error", err)...)
}

func (r *Runner) logTerminalResult(message channeltypes.InboundMessage, result agentengine.TurnResult) {
	attrs := messageLogAttrs(message, resultLogAttrs(result)...)
	switch result.Status {
	case agentengine.TurnSucceeded:
		slog.Info("Feishu Agent Engine run completed", attrs...)
	case agentengine.TurnCanceled:
		slog.Info("Feishu Agent Engine run canceled", attrs...)
	default:
		slog.Warn("Feishu Agent Engine run failed", attrs...)
	}
}

func (r *Runner) IsResetCommand(text string) bool {
	fields := strings.Fields(text)
	if len(fields) > 0 && strings.EqualFold(fields[0], "/new") {
		// Feishu renders the canonical command as slash text, so messages typed
		// in Feishu arrive as /new rather than the internal XML envelope.
		return true
	}
	command, ok, err := slashcommand.Parse(text)
	return err == nil && ok && slashcommand.IsNewConversationCommand(command)
}

func resultFromError(err error) agentengine.TurnResult {
	if errors.Is(err, context.Canceled) {
		return agentengine.TurnResult{
			Status: agentengine.TurnCanceled,
			Error:  &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: "Feishu turn was canceled"},
		}
	}
	var turnErr *agentengine.TurnError
	if errors.As(err, &turnErr) {
		status := agentengine.TurnFailed
		if turnErr.Code == agentengine.ErrorCanceled {
			status = agentengine.TurnCanceled
		}
		return agentengine.TurnResult{Status: status, Error: turnErr}
	}
	return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: err.Error()}}
}

func userFacingError(turnErr *agentengine.TurnError) string {
	if turnErr == nil {
		return "Agent execution failed. Please try again."
	}
	switch turnErr.Code {
	case agentengine.ErrorAgentUnavailable:
		return "Agent is currently unavailable. Start it and try again."
	case agentengine.ErrorRuntimeAdapterUnavailable:
		return "This Agent runtime does not support direct Feishu execution yet."
	case agentengine.ErrorConversationBusy:
		return "This conversation is already processing another message. Please try again shortly."
	case agentengine.ErrorConversationNotResumable:
		return "This conversation can no longer be resumed. Use /new to start a fresh conversation."
	case agentengine.ErrorFileUnavailable:
		return "The Feishu attachment could not be made available to the Agent."
	case agentengine.ErrorInteractionUnsupported:
		return "This Feishu channel cannot answer the requested interaction yet."
	case agentengine.ErrorInvalidRequest:
		return "The Feishu message could not be processed."
	default:
		return "Agent execution failed. Please try again."
	}
}

func processingReactionID(turnID string) string {
	return strings.TrimSpace(turnID) + ":reaction:add"
}
