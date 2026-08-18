package agentengine

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type conversationRuntimeResolver interface {
	conversationRuntime(ctx context.Context, agentID string) (conversationRuntimeAdapter, func(), *TurnError)
}

type conversationRuntimeAdapter interface {
	Run(ctx context.Context, request TurnRequest, sink EventSink) TurnResult
	Reset(ctx context.Context, key ConversationKey) *TurnError
	Resolve(ctx context.Context, request InteractionRequest, resolution InteractionResolution) *TurnError
}

// Engine owns normalized Agent operations, conversation admission, active Turn
// identity, pending interactions, and ordered event delivery.
type Engine struct {
	agents   AgentInterface
	runtimes conversationRuntimeResolver

	mu        sync.Mutex
	active    map[conversationIdentity]*activeTurn
	resetting map[conversationIdentity]struct{}
}

type conversationIdentity struct {
	agentID string
	key     ConversationKey
}

type activeTurn struct {
	id           TurnID
	cancel       context.CancelFunc
	done         chan struct{}
	runtime      conversationRuntimeAdapter
	interactions map[string]InteractionRequest
	sequence     uint64
}

func (e *Engine) Agents() AgentInterface {
	if e == nil || e.agents == nil {
		return unavailableAgents{}
	}
	return e.agents
}

func (e *Engine) Conversations(agentID string) ConversationInterface {
	return &conversations{engine: e, agentID: strings.TrimSpace(agentID)}
}

type conversations struct {
	engine  *Engine
	agentID string
}

func (c *conversations) Run(ctx context.Context, request TurnRequest, sink EventSink) TurnResult {
	if ctx == nil {
		ctx = context.Background()
	}
	request.ConversationKey = ConversationKey(strings.TrimSpace(string(request.ConversationKey)))
	request.ID = TurnID(strings.TrimSpace(string(request.ID)))
	request.Continuation = normalizedContinuation(request.Continuation)
	request.Interaction = normalizedInteraction(request.Interaction)
	if c == nil || c.engine == nil || c.engine.runtimes == nil || c.agentID == "" || request.ID == "" || request.ConversationKey == "" || len(request.Input) == 0 {
		return failedResult(ErrorInvalidRequest, "agent ID, turn ID, conversation key, and input are required")
	}
	if request.Continuation != ContinuationCreateOrResume && request.Continuation != ContinuationRequireExisting {
		return failedResult(ErrorInvalidRequest, fmt.Sprintf("unsupported continuation policy %q", request.Continuation))
	}
	if request.Interaction != InteractionResolve && request.Interaction != InteractionReject && request.Interaction != InteractionSkipUserInput {
		return failedResult(ErrorInvalidRequest, fmt.Sprintf("unsupported interaction policy %q", request.Interaction))
	}
	if err := validateInput(request.Input); err != nil {
		return TurnResult{Status: TurnFailed, Error: err}
	}
	if err := ctx.Err(); err != nil {
		return resultFromContext(ctx, err)
	}

	runtimeAdapter, releaseRuntime, resolveErr := c.engine.runtimes.conversationRuntime(ctx, c.agentID)
	if resolveErr != nil {
		return TurnResult{Status: TurnFailed, Error: resolveErr}
	}
	if releaseRuntime == nil {
		releaseRuntime = func() {}
	}

	runCtx, cancel := context.WithCancel(ctx)
	identity := conversationIdentity{agentID: c.agentID, key: request.ConversationKey}
	turn := &activeTurn{
		id:           request.ID,
		cancel:       cancel,
		done:         make(chan struct{}),
		runtime:      runtimeAdapter,
		interactions: make(map[string]InteractionRequest),
	}
	if !c.engine.register(identity, turn) {
		cancel()
		releaseRuntime()
		return failedResult(ErrorConversationBusy, "conversation already has an active turn")
	}
	defer func() {
		c.engine.release(identity, turn)
		releaseRuntime()
		close(turn.done)
	}()

	orderedSink := EventSinkFunc(func(eventCtx context.Context, event TurnEvent) error {
		c.engine.mu.Lock()
		turn.sequence++
		event.Sequence = turn.sequence
		if event.Interaction != nil {
			interaction := *event.Interaction
			interaction.ID = strings.TrimSpace(interaction.ID)
			if interaction.ID != "" {
				turn.interactions[interaction.ID] = interaction
				event.Interaction = &interaction
			}
		}
		c.engine.mu.Unlock()
		return emitTurnEvent(eventCtx, sink, event)
	})
	return runtimeAdapter.Run(runCtx, request, orderedSink)
}

func (c *conversations) Cancel(ctx context.Context, key ConversationKey, turnID TurnID) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil || c.engine == nil || c.agentID == "" || strings.TrimSpace(string(key)) == "" || strings.TrimSpace(string(turnID)) == "" {
		return &TurnError{Code: ErrorInvalidRequest, Message: "agent ID, conversation key, and turn ID are required"}
	}
	identity := conversationIdentity{agentID: c.agentID, key: ConversationKey(strings.TrimSpace(string(key)))}
	c.engine.mu.Lock()
	turn := c.engine.active[identity]
	if turn == nil || turn.id != TurnID(strings.TrimSpace(string(turnID))) {
		c.engine.mu.Unlock()
		return nil
	}
	turn.cancel()
	done := turn.done
	c.engine.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *conversations) Reset(ctx context.Context, key ConversationKey) error {
	if ctx == nil {
		ctx = context.Background()
	}
	key = ConversationKey(strings.TrimSpace(string(key)))
	if c == nil || c.engine == nil || c.engine.runtimes == nil || c.agentID == "" || key == "" {
		return &TurnError{Code: ErrorInvalidRequest, Message: "agent ID and conversation key are required"}
	}
	identity := conversationIdentity{agentID: c.agentID, key: key}
	if !c.engine.beginReset(identity) {
		return &TurnError{Code: ErrorConversationBusy, Message: "conversation already has an active turn"}
	}
	defer c.engine.endReset(identity)
	runtimeAdapter, releaseRuntime, resolveErr := c.engine.runtimes.conversationRuntime(ctx, c.agentID)
	if resolveErr != nil {
		return resolveErr
	}
	if releaseRuntime != nil {
		defer releaseRuntime()
	}
	if err := runtimeAdapter.Reset(ctx, key); err != nil {
		return err
	}
	return nil
}

func (c *conversations) Resolve(ctx context.Context, resolution InteractionResolution) error {
	if ctx == nil {
		ctx = context.Background()
	}
	resolution.ConversationKey = ConversationKey(strings.TrimSpace(string(resolution.ConversationKey)))
	resolution.InteractionID = strings.TrimSpace(resolution.InteractionID)
	if c == nil || c.engine == nil || c.agentID == "" || resolution.ConversationKey == "" || resolution.InteractionID == "" {
		return &TurnError{Code: ErrorInvalidRequest, Message: "agent ID, conversation key, and interaction ID are required"}
	}
	identity := conversationIdentity{agentID: c.agentID, key: resolution.ConversationKey}
	c.engine.mu.Lock()
	turn := c.engine.active[identity]
	request, ok := InteractionRequest{}, false
	if turn != nil {
		request, ok = turn.interactions[resolution.InteractionID]
	}
	c.engine.mu.Unlock()
	if !ok {
		return &TurnError{Code: ErrorInteractionNotFound, Message: "pending interaction was not found"}
	}
	if err := turn.runtime.Resolve(ctx, request, resolution); err != nil {
		return err
	}
	c.engine.mu.Lock()
	if current := c.engine.active[identity]; current == turn {
		delete(turn.interactions, resolution.InteractionID)
	}
	c.engine.mu.Unlock()
	return nil
}

func (e *Engine) register(identity conversationIdentity, turn *activeTurn) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.active == nil {
		e.active = make(map[conversationIdentity]*activeTurn)
	}
	if e.resetting == nil {
		e.resetting = make(map[conversationIdentity]struct{})
	}
	if e.active[identity] != nil {
		return false
	}
	if _, resetting := e.resetting[identity]; resetting {
		return false
	}
	e.active[identity] = turn
	return true
}

func (e *Engine) release(identity conversationIdentity, turn *activeTurn) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.active[identity] == turn {
		delete(e.active, identity)
	}
}

func (e *Engine) beginReset(identity conversationIdentity) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.resetting == nil {
		e.resetting = make(map[conversationIdentity]struct{})
	}
	if e.active[identity] != nil {
		return false
	}
	if _, exists := e.resetting[identity]; exists {
		return false
	}
	e.resetting[identity] = struct{}{}
	return true
}

func (e *Engine) endReset(identity conversationIdentity) {
	e.mu.Lock()
	delete(e.resetting, identity)
	e.mu.Unlock()
}

func normalizedContinuation(policy ContinuationPolicy) ContinuationPolicy {
	if strings.TrimSpace(string(policy)) == "" {
		return ContinuationCreateOrResume
	}
	return ContinuationPolicy(strings.TrimSpace(strings.ToLower(string(policy))))
}

func normalizedInteraction(policy InteractionPolicy) InteractionPolicy {
	if strings.TrimSpace(string(policy)) == "" {
		return InteractionResolve
	}
	return InteractionPolicy(strings.TrimSpace(strings.ToLower(string(policy))))
}

func validateInput(input []InputPart) *TurnError {
	for _, part := range input {
		switch part.Kind {
		case InputPartText:
			if part.File != nil || strings.TrimSpace(part.Text) == "" {
				return &TurnError{Code: ErrorInvalidRequest, Message: "text input must contain only non-empty text"}
			}
		case InputPartFile:
			if part.File == nil || strings.TrimSpace(part.Text) != "" {
				return &TurnError{Code: ErrorInvalidRequest, Message: "file input must include only a file"}
			}
			if strings.TrimSpace(part.File.ID) == "" || strings.TrimSpace(part.File.SourcePath) == "" || strings.TrimSpace(part.File.Name) == "" || strings.TrimSpace(part.File.MediaType) == "" || part.File.SizeBytes < 0 || len(strings.TrimSpace(part.File.SHA256)) != 64 {
				return &TurnError{Code: ErrorInvalidRequest, Message: "file ID, source path, name, media type, non-negative size, and SHA-256 are required"}
			}
		default:
			return &TurnError{Code: ErrorInvalidRequest, Message: fmt.Sprintf("unsupported input kind %q", part.Kind)}
		}
	}
	return nil
}

func failedResult(code ErrorCode, message string) TurnResult {
	return TurnResult{Status: TurnFailed, Error: &TurnError{Code: code, Message: message}}
}
