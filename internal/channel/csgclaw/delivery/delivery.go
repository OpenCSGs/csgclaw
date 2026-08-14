package delivery

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/channelbridge/runtimebridge"
)

const (
	completedTurnWindow = 1024
	thoughtTailBytes    = 1536
	thoughtFlushEvery   = 400 * time.Millisecond
)

// Renderer owns channel-specific event rendering and transcript delivery.
// Returning an error from Emit causes Agent Engine to cancel the active turn.
type Renderer interface {
	Emit(ctx context.Context, turn channel.TurnContext, event agentengine.TurnEvent) error
	Complete(ctx context.Context, turn channel.TurnContext, result agentengine.TurnResult) error
}

// TranscriptStore is the narrow IM write surface used by the renderer.
type TranscriptStore interface {
	DeliverMessage(ctx context.Context, turn channel.TurnContext, text string) error
	DeliverActivity(ctx context.Context, turn channel.TurnContext, event agentengine.TurnEvent) error
}

// ActivityDelivery is the legacy-compatible activity representation persisted
// by the built-in IM adapter. Event remains available for delivery metadata.
type ActivityDelivery struct {
	MessageID      string
	Text           string
	Metadata       map[string]any
	ThreadRootID   string
	EnsureTurnRoot bool
	Event          agentengine.TurnEvent
}

type renderedActivityStore interface {
	DeliverRenderedActivity(context.Context, channel.TurnContext, ActivityDelivery) error
}

type thoughtStore interface {
	DeliverThought(context.Context, channel.TurnContext, string) error
}

type failureStore interface {
	DeliverFailure(context.Context, channel.TurnContext, string) error
}

// UserInputBinder attaches a Runtime-native blocking question to its IM scope
// before the activity card becomes actionable in the Web UI.
type UserInputBinder interface {
	Bind(requestID, channel, roomID, threadRootID, requesterID string) (activity.UserInputSnapshot, error)
}

// StructuredUserInputActivator turns a validated request_user_input output
// item into a detached, actionable IM question.
type StructuredUserInputActivator interface {
	Activate(context.Context, channel.TurnContext, activity.RequestUserInputArgs) (activity.UserInputSnapshot, error)
}

type RendererOption func(*TranscriptRenderer)

func WithUserInputBinder(binder UserInputBinder) RendererOption {
	return func(renderer *TranscriptRenderer) {
		renderer.userInput = binder
	}
}

func WithStructuredUserInputActivator(activator StructuredUserInputActivator) RendererOption {
	return func(renderer *TranscriptRenderer) {
		renderer.structuredUserInput = activator
	}
}

// TranscriptRenderer maps Engine turn events onto the existing IM activity
// and transcript contract. Rendering state is isolated by the complete Turn
// identity so interleaved conversations cannot overwrite each other.
type TranscriptRenderer struct {
	store               TranscriptStore
	userInput           UserInputBinder
	structuredUserInput StructuredUserInputActivator

	mu             sync.Mutex
	turns          map[turnBufferKey]*turnRenderState
	completed      map[turnBufferKey]struct{}
	completedOrder []turnBufferKey
	now            func() time.Time
	thoughtLimit   int
	thoughtEvery   time.Duration
}

type turnRenderState struct {
	renderer            *runtimebridge.TurnRenderer
	thought             string
	thoughtDirty        bool
	lastThoughtFlush    time.Time
	lastSequence        uint64
	hasText             bool
	structuredUserInput *activity.RequestUserInputArgs
}

type turnBufferKey struct {
	bindingID       channel.BindingID
	agentID         string
	conversationKey agentengine.ConversationKey
	turnID          agentengine.TurnID
	sourceMessageID string
}

func NewTranscriptRenderer(store TranscriptStore, opts ...RendererOption) *TranscriptRenderer {
	renderer := &TranscriptRenderer{
		store:        store,
		now:          time.Now,
		thoughtLimit: thoughtTailBytes,
		thoughtEvery: thoughtFlushEvery,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(renderer)
		}
	}
	return renderer
}

func (r *TranscriptRenderer) Emit(ctx context.Context, turn channel.TurnContext, event agentengine.TurnEvent) error {
	if r == nil {
		return nil
	}
	state, ok := r.acceptEvent(turn, event)
	if !ok {
		return nil
	}

	switch event.Kind {
	case agentengine.TurnEventTextDelta:
		if event.Text == "" {
			return nil
		}
		r.mu.Lock()
		state.renderer.ApplyText(activity.RuntimeEvent{Kind: activity.RuntimeEventTextDelta, Text: event.Text})
		state.hasText = true
		r.mu.Unlock()
		return nil
	case agentengine.TurnEventThoughtDelta:
		if event.Thought == "" {
			return nil
		}
		thought, flush := r.appendThought(state, event.Thought)
		if store, ok := r.store.(thoughtStore); ok && flush && thought != "" {
			if err := store.DeliverThought(ctx, turn, thought); err != nil {
				r.markThoughtDirty(state)
				return err
			}
		}
		return nil
	case agentengine.TurnEventOutputItem:
		return r.captureOutputItem(state, event)
	case agentengine.TurnEventInteractionRequest:
		bound, err := r.bindInteraction(turn, event)
		if err != nil {
			return err
		}
		event = bound
	}

	runtimeEvent, renderable := normalizedRuntimeEvent(turn, event)
	if !renderable {
		if r.store != nil && (event.Kind == agentengine.TurnEventToolCallStart ||
			event.Kind == agentengine.TurnEventToolCallUpdate ||
			event.Kind == agentengine.TurnEventActivityUpdate ||
			event.Kind == agentengine.TurnEventInteractionRequest) {
			return r.store.DeliverActivity(ctx, turn, event)
		}
		return nil
	}
	r.mu.Lock()
	rendered, renderedOK := state.renderer.RenderActivity(runtimeEvent, string(channel.ChannelCSGClaw), turn.RoomID, turn.ParticipantID)
	r.mu.Unlock()
	if !renderedOK {
		if r.store != nil {
			return r.store.DeliverActivity(ctx, turn, event)
		}
		return nil
	}
	return r.deliverRenderedActivity(ctx, turn, activityDelivery(turn, event, rendered))
}

func (r *TranscriptRenderer) Complete(ctx context.Context, turn channel.TurnContext, result agentengine.TurnResult) error {
	if r == nil {
		return nil
	}
	state := r.finishState(turn)
	if r.store == nil || result.Status == agentengine.TurnCanceled {
		return nil
	}
	if state == nil {
		state = newTurnRenderState(turn.Locale)
	}
	if store, ok := r.store.(thoughtStore); ok {
		if thought := r.pendingThought(state); thought != "" {
			if err := store.DeliverThought(ctx, turn, thought); err != nil {
				return err
			}
		}
	}
	if result.Status == agentengine.TurnFailed {
		state.renderer.DiscardStructuredOutput()
		message := "turn failed"
		if result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
			message = strings.TrimSpace(result.Error.Message)
		}
		if store, ok := r.store.(failureStore); ok {
			return store.DeliverFailure(ctx, turn, message)
		}
		return r.store.DeliverMessage(ctx, turn, message)
	}

	if !state.hasText && strings.TrimSpace(result.Output) != "" {
		state.renderer.ApplyText(activity.RuntimeEvent{Kind: activity.RuntimeEventTextDelta, Text: result.Output})
	}
	if text := strings.TrimSpace(strings.Join(state.renderer.FinalMessages(), "\n\n")); text != "" {
		if err := r.store.DeliverMessage(ctx, turn, text); err != nil {
			return err
		}
	}
	if state.structuredUserInput == nil || r.structuredUserInput == nil {
		return nil
	}
	snapshot, err := r.structuredUserInput.Activate(ctx, turn, *cloneRequestUserInputArgs(state.structuredUserInput))
	if err != nil {
		return err
	}
	event := agentengine.TurnEvent{
		TurnID:   turn.TurnID,
		Kind:     agentengine.TurnEventInteractionRequest,
		Sequence: state.lastSequence + 1,
		Interaction: &agentengine.InteractionRequest{
			ID:      snapshot.ID,
			Kind:    agentengine.InteractionUserInput,
			Title:   "User input required",
			Payload: snapshot,
		},
	}
	runtimeEvent, ok := normalizedRuntimeEvent(turn, event)
	if !ok {
		return nil
	}
	rendered, ok := state.renderer.RenderActivity(runtimeEvent, string(channel.ChannelCSGClaw), turn.RoomID, turn.ParticipantID)
	if !ok {
		return nil
	}
	return r.deliverRenderedActivity(ctx, turn, activityDelivery(turn, event, rendered))
}

func (r *TranscriptRenderer) acceptEvent(turn channel.TurnContext, event agentengine.TurnEvent) (*turnRenderState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := bufferKey(turn)
	if _, completed := r.completed[key]; completed {
		return nil, false
	}
	if r.turns == nil {
		r.turns = make(map[turnBufferKey]*turnRenderState)
	}
	state := r.turns[key]
	if state == nil {
		state = newTurnRenderState(turn.Locale)
		r.turns[key] = state
	}
	if event.Sequence > 0 {
		if event.Sequence <= state.lastSequence {
			return nil, false
		}
		state.lastSequence = event.Sequence
	}
	return state, true
}

func newTurnRenderState(locale string) *turnRenderState {
	renderer := runtimebridge.NewTurnRenderer()
	renderer.SetLocale(locale)
	return &turnRenderState{renderer: renderer}
}

func (r *TranscriptRenderer) appendThought(state *turnRenderState, delta string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := r.thoughtLimit
	if limit <= 0 {
		limit = thoughtTailBytes
	}
	state.thought = tailUTF8(state.thought+delta, limit)
	state.thoughtDirty = true
	now := time.Now()
	if r.now != nil {
		now = r.now()
	}
	interval := r.thoughtEvery
	if interval <= 0 {
		interval = thoughtFlushEvery
	}
	if !state.lastThoughtFlush.IsZero() && now.Sub(state.lastThoughtFlush) < interval {
		return "", false
	}
	state.lastThoughtFlush = now
	state.thoughtDirty = false
	return strings.TrimSpace(state.thought), true
}

func (r *TranscriptRenderer) markThoughtDirty(state *turnRenderState) {
	r.mu.Lock()
	state.thoughtDirty = true
	r.mu.Unlock()
}

func (r *TranscriptRenderer) pendingThought(state *turnRenderState) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if state == nil || !state.thoughtDirty {
		return ""
	}
	state.thoughtDirty = false
	return strings.TrimSpace(state.thought)
}

func tailUTF8(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	start := len(value) - limit
	for start < len(value) && !utf8.RuneStart(value[start]) {
		start++
	}
	return value[start:]
}

func (r *TranscriptRenderer) captureOutputItem(state *turnRenderState, event agentengine.TurnEvent) error {
	if event.Output == nil {
		return nil
	}
	switch event.Output.Kind {
	case agentengine.OutputItemResourceLink:
		link, ok := event.Output.Payload.(activity.ResourceLink)
		if !ok {
			return nil
		}
		r.mu.Lock()
		state.renderer.ApplyStructuredOutput(activity.RuntimeEvent{
			Kind: activity.RuntimeEventStructuredOutput,
			Payload: activity.StructuredOutputArtifact{
				ResourceLinks: []activity.ResourceLink{link},
			},
		})
		r.mu.Unlock()
	case agentengine.OutputItemRequestUserInput:
		args, ok := requestUserInputArgs(event.Output.Payload)
		if !ok {
			return nil
		}
		r.mu.Lock()
		state.structuredUserInput = cloneRequestUserInputArgs(&args)
		state.renderer.ApplyStructuredOutput(activity.RuntimeEvent{
			Kind: activity.RuntimeEventStructuredOutput,
			Payload: activity.StructuredOutputArtifact{
				RequestUserInput: cloneRequestUserInputArgs(&args),
			},
		})
		r.mu.Unlock()
	}
	return nil
}

func (r *TranscriptRenderer) bindInteraction(turn channel.TurnContext, event agentengine.TurnEvent) (agentengine.TurnEvent, error) {
	if r.userInput == nil || event.Interaction == nil || event.Interaction.Kind != agentengine.InteractionUserInput {
		return event, nil
	}
	snapshot, ok := userInputSnapshot(event.Interaction.Payload)
	if !ok {
		return event, nil
	}
	bound, err := r.userInput.Bind(snapshot.ID, string(channel.ChannelCSGClaw), turn.RoomID, turn.ThreadRootID, turn.ParticipantID)
	if err != nil {
		return event, err
	}
	interaction := *event.Interaction
	interaction.Payload = bound
	event.Interaction = &interaction
	return event, nil
}

func (r *TranscriptRenderer) deliverRenderedActivity(ctx context.Context, turn channel.TurnContext, rendered ActivityDelivery) error {
	if store, ok := r.store.(renderedActivityStore); ok {
		return store.DeliverRenderedActivity(ctx, turn, rendered)
	}
	if r.store != nil {
		return r.store.DeliverActivity(ctx, turn, rendered.Event)
	}
	return nil
}

func (r *TranscriptRenderer) finishState(turn channel.TurnContext) *turnRenderState {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := bufferKey(turn)
	state := r.turns[key]
	delete(r.turns, key)
	if r.completed == nil {
		r.completed = make(map[turnBufferKey]struct{})
	}
	if _, exists := r.completed[key]; exists {
		return state
	}
	r.completed[key] = struct{}{}
	r.completedOrder = append(r.completedOrder, key)
	if len(r.completedOrder) <= completedTurnWindow {
		return state
	}
	oldest := r.completedOrder[0]
	r.completedOrder = r.completedOrder[1:]
	delete(r.completed, oldest)
	return state
}

func normalizedRuntimeEvent(turn channel.TurnContext, event agentengine.TurnEvent) (activity.RuntimeEvent, bool) {
	value := activity.RuntimeEvent{
		RuntimeKind: "codex",
		RuntimeID:   string(turn.BindingID),
		SessionID:   activitySessionID(turn),
		TurnID:      string(turn.TurnID),
		ReceivedAt:  time.Now().UTC(),
	}
	switch event.Kind {
	case agentengine.TurnEventToolCallStart, agentengine.TurnEventToolCallUpdate:
		if event.Tool == nil {
			return value, false
		}
		if event.Kind == agentengine.TurnEventToolCallStart {
			value.Kind = activity.RuntimeEventToolCallStart
		} else {
			value.Kind = activity.RuntimeEventToolCallUpdate
		}
		value.ToolCallID = event.Tool.ID
		value.ToolKind = event.Tool.Kind
		value.ToolTitle = event.Tool.Title
		value.ToolStatus = event.Tool.Status
		value.ToolInputSummary = event.Tool.InputSummary
		value.ToolOutputSummary = event.Tool.OutputSummary
		value.Payload = event.Tool.Payload
	case agentengine.TurnEventActivityUpdate:
		if event.Activity == nil {
			return value, false
		}
		value.Payload = event.Activity.Payload
		switch activity.RuntimeEventKind(strings.TrimSpace(event.Activity.Kind)) {
		case activity.RuntimeEventActionDecision:
			value.Kind = activity.RuntimeEventActionDecision
			value.ActionID = event.Activity.ID
			value.ActionStatus = event.Activity.Status
		case activity.RuntimeEventUserInputResolved:
			value.Kind = activity.RuntimeEventUserInputResolved
			value.UserInputID = event.Activity.ID
			value.UserInputStatus = event.Activity.Status
		default:
			return value, false
		}
	case agentengine.TurnEventInteractionRequest:
		if event.Interaction == nil {
			return value, false
		}
		value.Payload = event.Interaction.Payload
		switch event.Interaction.Kind {
		case agentengine.InteractionPermission:
			value.Kind = activity.RuntimeEventActionRequest
			value.ActionID = event.Interaction.ID
		case agentengine.InteractionUserInput:
			value.Kind = activity.RuntimeEventUserInputRequest
			value.UserInputID = event.Interaction.ID
		default:
			return value, false
		}
	default:
		return value, false
	}
	return value, true
}

func activitySessionID(turn channel.TurnContext) string {
	conversationKey := strings.TrimSpace(string(turn.ConversationKey))
	turnID := strings.TrimSpace(string(turn.TurnID))
	if turnID == "" {
		return conversationKey
	}
	// Activity IDs must be stable within one Turn and distinct across Turns,
	// even if a Runtime reuses a tool-call ID in the same conversation.
	return conversationKey + "\x00" + turnID
}

func activityDelivery(turn channel.TurnContext, event agentengine.TurnEvent, rendered runtimebridge.RenderedActivity) ActivityDelivery {
	delivery := ActivityDelivery{
		MessageID: rendered.MessageID,
		Text:      rendered.Text,
		Metadata:  rendered.Metadata,
		Event:     event,
	}
	switch event.Kind {
	case agentengine.TurnEventToolCallStart, agentengine.TurnEventToolCallUpdate:
		// Legacy tool activities are top-level timeline entries.
	case agentengine.TurnEventInteractionRequest:
		if event.Interaction != nil && event.Interaction.Kind == agentengine.InteractionUserInput {
			delivery.ThreadRootID = turn.ThreadRootID
		} else if turn.ThreadRootID != "" {
			delivery.ThreadRootID = turn.ThreadRootID
		} else {
			delivery.EnsureTurnRoot = true
		}
	case agentengine.TurnEventActivityUpdate:
		if event.Activity != nil && strings.TrimSpace(event.Activity.Kind) == string(activity.RuntimeEventUserInputResolved) {
			delivery.ThreadRootID = turn.ThreadRootID
		} else if turn.ThreadRootID != "" {
			delivery.ThreadRootID = turn.ThreadRootID
		} else {
			delivery.EnsureTurnRoot = true
		}
	}
	return delivery
}

func requestUserInputArgs(value any) (activity.RequestUserInputArgs, bool) {
	switch typed := value.(type) {
	case activity.RequestUserInputArgs:
		return typed, true
	case *activity.RequestUserInputArgs:
		if typed != nil {
			return *typed, true
		}
	}
	return activity.RequestUserInputArgs{}, false
}

func userInputSnapshot(value any) (activity.UserInputSnapshot, bool) {
	switch typed := value.(type) {
	case activity.UserInputSnapshot:
		return typed, true
	case *activity.UserInputSnapshot:
		if typed != nil {
			return *typed, true
		}
	}
	return activity.UserInputSnapshot{}, false
}

func cloneRequestUserInputArgs(input *activity.RequestUserInputArgs) *activity.RequestUserInputArgs {
	if input == nil {
		return nil
	}
	out := *input
	out.Questions = append([]activity.RequestUserInputQuestion(nil), input.Questions...)
	for index := range out.Questions {
		out.Questions[index].Options = append([]activity.RequestUserInputOption(nil), input.Questions[index].Options...)
	}
	if input.AutoResolutionMS != nil {
		value := *input.AutoResolutionMS
		out.AutoResolutionMS = &value
	}
	return &out
}

func bufferKey(turn channel.TurnContext) turnBufferKey {
	return turnBufferKey{
		bindingID:       turn.BindingID,
		agentID:         turn.AgentID,
		conversationKey: turn.ConversationKey,
		turnID:          turn.TurnID,
		sourceMessageID: turn.SourceMessageID,
	}
}
