package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/conv"
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/channel/csgclaw/files"
	"csgclaw/internal/channelbridge"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/slashcommand"
	"csgclaw/internal/worklease"
)

// Engine is the narrow Agent Engine surface used by the built-in IM adapter.
type Engine interface {
	Conversations(agentID string) agentengine.ConversationInterface
}

type inboundKind string

const (
	inboundRun   inboundKind = "run"
	inboundReset inboundKind = "reset"
	inboundSkip  inboundKind = "skip"
)

type idGenerator func() (agentengine.TurnID, error)

// Adapter converts built-in IM events into runtime-neutral Agent Engine turns.
type Adapter struct {
	engine      Engine
	attachments files.Resolver
	renderer    delivery.Renderer
	newTurnID   idGenerator
	work        workOptions
}

// builtInIMAdmissionPolicy deliberately preserves queued, in-order delivery
// for consecutive messages in one conversation. A later message does not
// cancel an earlier turn; explicit Stop and /new remain the cancellation paths.
const builtInIMAdmissionPolicy = agentengine.AdmissionWait

type Option func(*Adapter)

func WithAttachmentResolver(resolver files.Resolver) Option {
	return func(adapter *Adapter) {
		adapter.attachments = resolver
	}
}

func WithTurnIDGenerator(generator func() (agentengine.TurnID, error)) Option {
	return func(adapter *Adapter) {
		if generator != nil {
			adapter.newTurnID = generator
		}
	}
}

// WithParticipantWorkReporter enables the built-in IM working/idle lease
// lifecycle for each Engine turn.
func WithParticipantWorkReporter(reporter worklease.ParticipantWorkReporter) Option {
	return func(adapter *Adapter) {
		adapter.work.reporter = reporter
	}
}

// WithTurnControllerRegistrar exposes active built-in IM turns to the
// participant work stop API. The stop capability is advertised only when a
// registrar is configured.
func WithTurnControllerRegistrar(registrar agentruntime.TurnControllerRegistrar) Option {
	return func(adapter *Adapter) {
		adapter.work.turnControls = registrar
	}
}

func New(engine Engine, renderer delivery.Renderer, opts ...Option) (*Adapter, error) {
	if engine == nil {
		return nil, fmt.Errorf("agent engine is required")
	}
	adapter := &Adapter{
		engine:   engine,
		renderer: renderer,
		work:     defaultWorkOptions(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	return adapter, nil
}

// Classify reports whether a source event should run, reset, or be skipped.
func Classify(event channelbridge.BotEvent) string {
	return string(classifyInbound(event))
}

func classifyInbound(event channelbridge.BotEvent) inboundKind {
	text := strings.TrimSpace(event.Text)
	if cmd, ok, err := slashcommand.Parse(text); err == nil && ok && slashcommand.IsNewConversationCommand(cmd) {
		return inboundReset
	}
	if text == "" && len(event.Attachments) == 0 {
		return inboundSkip
	}
	return inboundRun
}

// Handle classifies an already-routed source event and either runs a turn or
// invokes the available Engine control capability.
func (a *Adapter) Handle(ctx context.Context, binding channel.Binding, event channelbridge.BotEvent) (channel.Outcome, error) {
	switch classifyInbound(event) {
	case inboundSkip:
		return channel.Outcome{}, nil
	case inboundReset:
		return a.reset(ctx, binding, event)
	default:
		return a.Run(ctx, binding, event)
	}
}

// Reset delegates to the Agent Engine conversation selected by agentID.
func (a *Adapter) Reset(ctx context.Context, agentID string, key agentengine.ConversationKey) error {
	conversation, err := a.conversation(agentID)
	if err != nil {
		return err
	}
	return conversation.Reset(ctx, key)
}

// Cancel stops exactly one Engine turn for the selected Agent conversation.
func (a *Adapter) Cancel(ctx context.Context, agentID string, key agentengine.ConversationKey, turnID agentengine.TurnID) error {
	conversation, err := a.conversation(agentID)
	if err != nil {
		return err
	}
	return conversation.Cancel(ctx, key, turnID)
}

// Resolve answers one pending Engine interaction for the selected Agent.
func (a *Adapter) Resolve(ctx context.Context, agentID string, resolution agentengine.InteractionResolution) error {
	conversation, err := a.conversation(agentID)
	if err != nil {
		return err
	}
	return conversation.Resolve(ctx, resolution)
}

func (a *Adapter) conversation(agentID string) (agentengine.ConversationInterface, error) {
	if a == nil || a.engine == nil {
		return nil, &agentengine.TurnError{Code: agentengine.ErrorAgentUnavailable, Message: "agent engine is unavailable"}
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, &agentengine.TurnError{Code: agentengine.ErrorInvalidRequest, Message: "agent ID is required"}
	}
	conversation := a.engine.Conversations(agentID)
	if conversation == nil {
		return nil, &agentengine.TurnError{Code: agentengine.ErrorAgentUnavailable, Message: "agent conversation is unavailable"}
	}
	return conversation, nil
}

func (a *Adapter) reset(ctx context.Context, binding channel.Binding, event channelbridge.BotEvent) (outcome channel.Outcome, err error) {
	turn, err := a.turnContext(binding, event)
	if err != nil {
		return channel.Outcome{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, finishWork := a.startWork(ctx, turn)
	defer func() { finishWork(outcome.Result) }()
	if err := a.Reset(ctx, turn.AgentID, turn.ConversationKey); err != nil {
		code := agentengine.ErrorCodeOf(err)
		if code == "" {
			code = agentengine.ErrorRuntimeFailed
		}
		result := failed(code, err.Error())
		return a.complete(ctx, turn, result)
	}
	return a.complete(ctx, turn, agentengine.TurnResult{
		Status: agentengine.TurnSucceeded,
		Output: "Cleared my internal history for this conversation. The IM room messages were not cleared.",
	})
}

// Run executes one already-routed and already-deduplicated built-in IM event.
func (a *Adapter) Run(ctx context.Context, binding channel.Binding, event channelbridge.BotEvent) (outcome channel.Outcome, err error) {
	turn, err := a.turnContext(binding, event)
	if err != nil {
		return channel.Outcome{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, finishWork := a.startWork(ctx, turn)
	defer func() { finishWork(outcome.Result) }()

	input, release, inputErr := a.input(ctx, binding, event)
	if release != nil {
		defer release()
	}
	if inputErr != nil {
		result := failed(agentengine.ErrorFileUnavailable, inputErr.Error())
		return a.complete(ctx, turn, result)
	}
	if len(input) == 0 {
		result := failed(agentengine.ErrorInvalidRequest, "message text or attachments are required")
		return a.complete(ctx, turn, result)
	}

	conversation := a.engine.Conversations(turn.AgentID)
	if conversation == nil {
		result := failed(agentengine.ErrorAgentUnavailable, "agent conversation is unavailable")
		return a.complete(ctx, turn, result)
	}
	result := conversation.Run(ctx, agentengine.TurnRequest{
		ID:              turn.TurnID,
		ConversationKey: turn.ConversationKey,
		Input:           input,
		Admission:       builtInIMAdmissionPolicy,
		Continuation:    agentengine.ContinuationCreateOrResume,
		Interaction:     agentengine.InteractionResolve,
	}, rendererSink{renderer: a.renderer, turn: turn})
	return a.complete(ctx, turn, result)
}

func (a *Adapter) turnContext(binding channel.Binding, event channelbridge.BotEvent) (channel.TurnContext, error) {
	if a == nil || a.engine == nil {
		return channel.TurnContext{}, fmt.Errorf("built-in IM adapter is not configured")
	}
	agentID := strings.TrimSpace(binding.AgentID)
	participantID := strings.TrimSpace(binding.ParticipantID)
	messageID := strings.TrimSpace(event.MessageID)
	if agentID == "" || participantID == "" || messageID == "" {
		return channel.TurnContext{}, fmt.Errorf("agent id, participant id, and source message id are required")
	}
	key, err := conv.ConversationKey(binding, event)
	if err != nil {
		return channel.TurnContext{}, err
	}
	turnID := sourceTurnID(binding, event)
	if a.newTurnID != nil {
		turnID, err = a.newTurnID()
		if err != nil {
			return channel.TurnContext{}, fmt.Errorf("generate turn id: %w", err)
		}
	}
	if strings.TrimSpace(string(turnID)) == "" {
		return channel.TurnContext{}, fmt.Errorf("generated turn id is empty")
	}
	return channel.TurnContext{
		BindingID:       binding.StableID(),
		ParticipantID:   participantID,
		AgentID:         agentID,
		RoomID:          strings.TrimSpace(event.RoomID),
		Locale:          strings.TrimSpace(event.Locale),
		ChatType:        strings.TrimSpace(event.ChatType),
		ThreadRootID:    strings.TrimSpace(event.ThreadRootID),
		SourceMessageID: messageID,
		ConversationKey: key,
		TurnID:          turnID,
	}, nil
}

func (a *Adapter) input(ctx context.Context, binding channel.Binding, event channelbridge.BotEvent) ([]agentengine.InputPart, func(), error) {
	if a.attachments == nil && len(event.Attachments) > 0 {
		return nil, nil, fmt.Errorf("attachment resolver is not configured")
	}

	releases := make([]func(), 0, len(event.Attachments))
	releaseAll := func() {
		for index := len(releases) - 1; index >= 0; index-- {
			if releases[index] != nil {
				releases[index]()
			}
		}
	}
	if contextResolver, ok := a.attachments.(files.ContextResolver); ok {
		resolved, release, err := contextResolver.ResolveContext(ctx, binding, event)
		if err != nil {
			return nil, nil, err
		}
		event = resolved
		releases = append(releases, release)
	}
	input := conv.TextInput(binding, event)
	for _, attachment := range event.Attachments {
		file, release, err := a.attachments.Resolve(ctx, binding, event, attachment)
		if err != nil {
			releaseAll()
			return nil, nil, fmt.Errorf("resolve attachment %q: %w", strings.TrimSpace(attachment.ID), err)
		}
		releases = append(releases, release)
		input = append(input, agentengine.InputPart{Kind: agentengine.InputPartFile, File: &file})
	}
	if len(releases) == 0 {
		return input, nil, nil
	}
	return input, releaseAll, nil
}

func (a *Adapter) complete(ctx context.Context, turn channel.TurnContext, result agentengine.TurnResult) (channel.Outcome, error) {
	outcome := channel.Outcome{Turn: turn, Result: result}
	if a.renderer == nil {
		return outcome, nil
	}
	if err := a.renderer.Complete(ctx, turn, result); err != nil {
		return outcome, fmt.Errorf("render turn result: %w", err)
	}
	return outcome, nil
}

func failed(code agentengine.ErrorCode, message string) agentengine.TurnResult {
	return agentengine.TurnResult{
		Status: agentengine.TurnFailed,
		Error:  &agentengine.TurnError{Code: code, Message: message},
	}
}

func sourceTurnID(binding channel.Binding, event channelbridge.BotEvent) agentengine.TurnID {
	identity := string(binding.StableID()) + "\x00" + strings.TrimSpace(event.MessageID)
	sum := sha256.Sum256([]byte(identity))
	return agentengine.TurnID("turn-csgclaw-" + hex.EncodeToString(sum[:16]))
}

type rendererSink struct {
	renderer delivery.Renderer
	turn     channel.TurnContext
}

func (s rendererSink) Emit(ctx context.Context, event agentengine.TurnEvent) error {
	if s.renderer == nil {
		return nil
	}
	return s.renderer.Emit(ctx, s.turn, event)
}
