package agentengine

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

const maxCompletedTurns = 1024

type conversationRuntimeResolver interface {
	conversationRuntime(ctx context.Context, agentID string) (conversationRuntimeAdapter, func(), *TurnError)
}

type conversationRuntimeAdapter interface {
	Run(ctx context.Context, request TurnRequest, sink EventSink) TurnResult
	Reset(ctx context.Context, key ConversationKey) *TurnError
	Resolve(ctx context.Context, request InteractionRequest, resolution InteractionResolution) *TurnError
}

// Engine owns normalized Agent operations, conversation admission, active Turn
// identity, pending interactions, replay-safe event delivery, and terminal
// result idempotency.
type Engine struct {
	agents   AgentInterface
	runtimes conversationRuntimeResolver
	files    *FileStore

	mu             sync.Mutex
	active         map[conversationIdentity]*activeTurn
	controls       map[conversationIdentity]*conversationControl
	completed      map[turnIdentity]completedTurn
	completedOrder []turnIdentity
}

type conversationIdentity struct {
	agentID string
	key     ConversationKey
}

type turnIdentity struct {
	conversationIdentity
	id TurnID
}

type conversationControl struct {
	done chan struct{}
}

type interactionState uint8

const (
	interactionPending interactionState = iota
	interactionResolving
)

type pendingInteraction struct {
	request InteractionRequest
	state   interactionState
}

type activeTurn struct {
	request      TurnRequest
	ctx          context.Context
	cancel       context.CancelFunc
	done         chan struct{}
	runtime      conversationRuntimeAdapter
	interactions map[string]*pendingInteraction
	sequence     uint64
	events       []TurnEvent
	result       TurnResult
}

type completedTurn struct {
	request TurnRequest
	events  []TurnEvent
	result  TurnResult
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

func (c *conversations) Files() FileInterface {
	if c == nil || c.engine == nil || c.engine.files == nil {
		return unavailableFiles{}
	}
	return c.engine.files.Scope(c.agentID)
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
	request.Admission = normalizedAdmission(request.Admission)
	request.Continuation = normalizedContinuation(request.Continuation)
	request.Interaction = normalizedInteraction(request.Interaction)
	if c == nil || c.engine == nil || c.engine.runtimes == nil || c.agentID == "" || request.ID == "" || request.ConversationKey == "" || len(request.Input) == 0 {
		return failedResult(ErrorInvalidRequest, "agent ID, turn ID, conversation key, and input are required")
	}
	if request.Admission != AdmissionRejectIfBusy && request.Admission != AdmissionWait && request.Admission != AdmissionSupersede {
		return failedResult(ErrorInvalidRequest, fmt.Sprintf("unsupported admission policy %q", request.Admission))
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

	identity := conversationIdentity{agentID: c.agentID, key: request.ConversationKey}
	turn, replay, existing, admissionResult := c.engine.admit(ctx, identity, request)
	if replay != nil {
		return replayCompleted(ctx, sink, *replay)
	}
	if existing != nil {
		return awaitActiveTurn(ctx, sink, existing)
	}
	if admissionResult != nil {
		return *admissionResult
	}
	runtimeRequest, inputErr := c.engine.resolveInputFiles(c.agentID, request)
	if inputErr != nil {
		result := TurnResult{Status: TurnFailed, Error: inputErr}
		turn.cancel()
		c.engine.complete(identity, turn, result)
		return result
	}

	runtimeAdapter, releaseRuntime, resolveErr := c.engine.runtimes.conversationRuntime(turn.ctx, c.agentID)
	if resolveErr != nil {
		result := TurnResult{Status: TurnFailed, Error: resolveErr}
		turn.cancel()
		c.engine.complete(identity, turn, result)
		return result
	}
	if releaseRuntime == nil {
		releaseRuntime = func() {}
	}
	c.engine.setRuntime(identity, turn, runtimeAdapter)

	orderedSink := EventSinkFunc(func(eventCtx context.Context, event TurnEvent) error {
		return c.engine.recordAndEmit(eventCtx, turn, sink, event)
	})
	result := runtimeAdapter.Run(turn.ctx, runtimeRequest, orderedSink)
	if result.Status != TurnSucceeded {
		result.Files = nil
		result.files = nil
	} else if len(result.files) > 0 {
		result.Files = c.engine.files.registerTurnFiles(c.agentID, request.ConversationKey, request.ID, result.files)
		result.files = nil
	}
	turn.cancel()
	releaseRuntime()
	c.engine.complete(identity, turn, result)
	return result
}

func (e *Engine) resolveInputFiles(agentID string, request TurnRequest) (TurnRequest, *TurnError) {
	resolved := cloneTurnRequest(request)
	for index := range resolved.Input {
		part := &resolved.Input[index]
		if part.Kind != InputPartFile {
			continue
		}
		file, err := e.files.resolve(agentID, part.File.ID)
		if err != nil {
			return TurnRequest{}, err
		}
		part.File.file = file
	}
	return resolved, nil
}

func (c *conversations) Cancel(ctx context.Context, key ConversationKey, turnID TurnID) error {
	if ctx == nil {
		ctx = context.Background()
	}
	key = ConversationKey(strings.TrimSpace(string(key)))
	turnID = TurnID(strings.TrimSpace(string(turnID)))
	if c == nil || c.engine == nil || c.agentID == "" || key == "" || turnID == "" {
		return &TurnError{Code: ErrorInvalidRequest, Message: "agent ID, conversation key, and turn ID are required"}
	}
	identity := conversationIdentity{agentID: c.agentID, key: key}
	c.engine.mu.Lock()
	turn := c.engine.active[identity]
	if turn == nil || turn.request.ID != turnID {
		c.engine.mu.Unlock()
		return nil
	}
	turn.cancel()
	done := turn.done
	c.engine.mu.Unlock()
	return waitForCompletion(ctx, done)
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
	control, active, ok := c.engine.beginControl(identity, true)
	if !ok {
		return &TurnError{Code: ErrorConversationBusy, Message: "conversation already has a control operation"}
	}
	defer c.engine.endControl(identity, control)
	if active != nil {
		if err := waitForCompletion(ctx, active.done); err != nil {
			return err
		}
	}
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
	resolution.ResponderID = strings.TrimSpace(resolution.ResponderID)
	if c == nil || c.engine == nil || c.agentID == "" || resolution.ConversationKey == "" || resolution.InteractionID == "" {
		return &TurnError{Code: ErrorInvalidRequest, Message: "agent ID, conversation key, and interaction ID are required"}
	}
	identity := conversationIdentity{agentID: c.agentID, key: resolution.ConversationKey}
	c.engine.mu.Lock()
	turn := c.engine.active[identity]
	var pending *pendingInteraction
	if turn != nil {
		pending = turn.interactions[resolution.InteractionID]
	}
	if pending == nil || pending.state != interactionPending || turn.runtime == nil {
		c.engine.mu.Unlock()
		return &TurnError{Code: ErrorInteractionNotFound, Message: "pending interaction was not found"}
	}
	pending.state = interactionResolving
	request := pending.request
	runtimeAdapter := turn.runtime
	c.engine.mu.Unlock()

	if err := runtimeAdapter.Resolve(ctx, request, resolution); err != nil {
		c.engine.mu.Lock()
		if current := c.engine.active[identity]; current == turn {
			if currentPending := turn.interactions[resolution.InteractionID]; currentPending == pending && currentPending.state == interactionResolving {
				currentPending.state = interactionPending
			}
		}
		c.engine.mu.Unlock()
		return err
	}
	c.engine.mu.Lock()
	if current := c.engine.active[identity]; current == turn {
		if currentPending := turn.interactions[resolution.InteractionID]; currentPending == pending {
			delete(turn.interactions, resolution.InteractionID)
		}
	}
	c.engine.mu.Unlock()
	return nil
}

func (e *Engine) admit(ctx context.Context, identity conversationIdentity, request TurnRequest) (*activeTurn, *completedTurn, *activeTurn, *TurnResult) {
	for {
		e.mu.Lock()
		e.ensureState()
		turnKey := turnIdentity{conversationIdentity: identity, id: request.ID}
		if completed, ok := e.completed[turnKey]; ok {
			if !sameTurnRequest(completed.request, request) {
				e.mu.Unlock()
				result := failedResult(ErrorInvalidRequest, "turn ID was already used with a different request")
				return nil, nil, nil, &result
			}
			copy := cloneCompletedTurn(completed)
			e.mu.Unlock()
			return nil, &copy, nil, nil
		}
		if current := e.active[identity]; current != nil && current.request.ID == request.ID {
			if !sameTurnRequest(current.request, request) {
				e.mu.Unlock()
				result := failedResult(ErrorInvalidRequest, "turn ID is active with a different request")
				return nil, nil, nil, &result
			}
			e.mu.Unlock()
			return nil, nil, current, nil
		}
		control := e.controls[identity]
		current := e.active[identity]
		if control == nil && current == nil {
			turn := newActiveTurn(ctx, request)
			e.active[identity] = turn
			e.mu.Unlock()
			return turn, nil, nil, nil
		}
		switch request.Admission {
		case AdmissionRejectIfBusy:
			e.mu.Unlock()
			result := failedResult(ErrorConversationBusy, "conversation already has an active turn or control operation")
			return nil, nil, nil, &result
		case AdmissionWait:
			done := controlDone(control, current)
			e.mu.Unlock()
			if err := waitForCompletion(ctx, done); err != nil {
				result := resultFromContext(ctx, err)
				return nil, nil, nil, &result
			}
		case AdmissionSupersede:
			if control != nil {
				done := control.done
				e.mu.Unlock()
				if err := waitForCompletion(ctx, done); err != nil {
					result := resultFromContext(ctx, err)
					return nil, nil, nil, &result
				}
				continue
			}
			owned := &conversationControl{done: make(chan struct{})}
			e.controls[identity] = owned
			current.cancel()
			done := current.done
			e.mu.Unlock()
			if err := waitForCompletion(ctx, done); err != nil {
				e.endControl(identity, owned)
				result := resultFromContext(ctx, err)
				return nil, nil, nil, &result
			}
			turn := newActiveTurn(ctx, request)
			if !e.promoteControl(identity, owned, turn) {
				turn.cancel()
				result := failedResult(ErrorConversationBusy, "conversation admission changed while superseding")
				return nil, nil, nil, &result
			}
			return turn, nil, nil, nil
		}
	}
}

func newActiveTurn(ctx context.Context, request TurnRequest) *activeTurn {
	runCtx, cancel := context.WithCancel(ctx)
	return &activeTurn{
		request:      cloneTurnRequest(request),
		ctx:          runCtx,
		cancel:       cancel,
		done:         make(chan struct{}),
		interactions: make(map[string]*pendingInteraction),
	}
}

func (e *Engine) ensureState() {
	if e.active == nil {
		e.active = make(map[conversationIdentity]*activeTurn)
	}
	if e.controls == nil {
		e.controls = make(map[conversationIdentity]*conversationControl)
	}
	if e.completed == nil {
		e.completed = make(map[turnIdentity]completedTurn)
	}
	if e.files == nil {
		e.files = NewFileStore()
	}
}

func (e *Engine) setRuntime(identity conversationIdentity, turn *activeTurn, runtimeAdapter conversationRuntimeAdapter) {
	e.mu.Lock()
	if e.active[identity] == turn {
		turn.runtime = runtimeAdapter
	}
	e.mu.Unlock()
}

func (e *Engine) beginControl(identity conversationIdentity, cancelActive bool) (*conversationControl, *activeTurn, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ensureState()
	if e.controls[identity] != nil {
		return nil, nil, false
	}
	control := &conversationControl{done: make(chan struct{})}
	e.controls[identity] = control
	active := e.active[identity]
	if cancelActive && active != nil {
		active.cancel()
	}
	return control, active, true
}

func (e *Engine) promoteControl(identity conversationIdentity, control *conversationControl, turn *activeTurn) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.controls[identity] != control || e.active[identity] != nil {
		return false
	}
	e.active[identity] = turn
	delete(e.controls, identity)
	close(control.done)
	return true
}

func (e *Engine) endControl(identity conversationIdentity, control *conversationControl) {
	e.mu.Lock()
	if e.controls[identity] == control {
		delete(e.controls, identity)
		close(control.done)
	}
	e.mu.Unlock()
}

func (e *Engine) recordAndEmit(ctx context.Context, turn *activeTurn, sink EventSink, event TurnEvent) error {
	e.mu.Lock()
	turn.sequence++
	event.TurnID = turn.request.ID
	event.Sequence = turn.sequence
	if event.Interaction != nil {
		interaction := *event.Interaction
		interaction.ID = strings.TrimSpace(interaction.ID)
		if interaction.ID != "" {
			turn.interactions[interaction.ID] = &pendingInteraction{request: interaction, state: interactionPending}
			event.Interaction = &interaction
		}
	}
	turn.events = append(turn.events, cloneTurnEvent(event))
	e.mu.Unlock()
	return emitTurnEvent(ctx, sink, cloneTurnEvent(event))
}

func (e *Engine) complete(identity conversationIdentity, turn *activeTurn, result TurnResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	turn.result = cloneTurnResult(result)
	if e.active[identity] == turn {
		delete(e.active, identity)
	}
	if result.Dispatched {
		e.ensureState()
		key := turnIdentity{conversationIdentity: identity, id: turn.request.ID}
		e.completed[key] = completedTurn{request: cloneTurnRequest(turn.request), events: cloneTurnEvents(turn.events), result: cloneTurnResult(result)}
		e.completedOrder = append(e.completedOrder, key)
		if len(e.completedOrder) > maxCompletedTurns {
			oldest := e.completedOrder[0]
			e.completedOrder = e.completedOrder[1:]
			delete(e.completed, oldest)
			e.files.deleteTurn(oldest.agentID, oldest.key, oldest.id)
		}
	}
	close(turn.done)
}

func awaitActiveTurn(ctx context.Context, sink EventSink, turn *activeTurn) TurnResult {
	if err := waitForCompletion(ctx, turn.done); err != nil {
		return resultFromContext(ctx, err)
	}
	events := cloneTurnEvents(turn.events)
	result := cloneTurnResult(turn.result)
	return replayEvents(ctx, sink, events, result)
}

func replayCompleted(ctx context.Context, sink EventSink, completed completedTurn) TurnResult {
	return replayEvents(ctx, sink, completed.events, completed.result)
}

func replayEvents(ctx context.Context, sink EventSink, events []TurnEvent, result TurnResult) TurnResult {
	for _, event := range events {
		if err := emitTurnEvent(ctx, sink, cloneTurnEvent(event)); err != nil {
			failed := failedResult(ErrorRuntimeFailed, err.Error())
			failed.Dispatched = result.Dispatched
			return failed
		}
	}
	return cloneTurnResult(result)
}

func waitForCompletion(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func controlDone(control *conversationControl, turn *activeTurn) <-chan struct{} {
	if control != nil {
		return control.done
	}
	return turn.done
}

func sameTurnRequest(left, right TurnRequest) bool {
	left.Admission = ""
	right.Admission = ""
	return reflect.DeepEqual(left, right)
}

func cloneCompletedTurn(input completedTurn) completedTurn {
	input.request = cloneTurnRequest(input.request)
	input.events = cloneTurnEvents(input.events)
	input.result = cloneTurnResult(input.result)
	return input
}

func cloneTurnRequest(input TurnRequest) TurnRequest {
	input.Input = append([]InputPart(nil), input.Input...)
	for index := range input.Input {
		if input.Input[index].File != nil {
			fileCopy := *input.Input[index].File
			input.Input[index].File = &fileCopy
		}
	}
	return input
}

func cloneTurnEvents(input []TurnEvent) []TurnEvent {
	output := make([]TurnEvent, len(input))
	for index, event := range input {
		output[index] = cloneTurnEvent(event)
	}
	return output
}

func cloneTurnEvent(input TurnEvent) TurnEvent {
	if input.Tool != nil {
		copy := *input.Tool
		input.Tool = &copy
	}
	if input.Activity != nil {
		copy := *input.Activity
		input.Activity = &copy
	}
	if input.Interaction != nil {
		copy := *input.Interaction
		input.Interaction = &copy
	}
	if input.Output != nil {
		copy := *input.Output
		input.Output = &copy
	}
	return input
}

func cloneTurnResult(input TurnResult) TurnResult {
	input.Files = cloneOutputFiles(input.Files)
	input.files = nil
	if input.Error != nil {
		errorCopy := *input.Error
		input.Error = &errorCopy
	}
	return input
}

func cloneOutputFiles(input []OutputFile) []OutputFile {
	if input == nil {
		return nil
	}
	output := make([]OutputFile, len(input))
	for index, file := range input {
		output[index] = file.metadata()
	}
	return output
}

func normalizedAdmission(policy AdmissionPolicy) AdmissionPolicy {
	if strings.TrimSpace(string(policy)) == "" {
		return AdmissionRejectIfBusy
	}
	return AdmissionPolicy(strings.TrimSpace(strings.ToLower(string(policy))))
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
			if strings.TrimSpace(part.File.ID) == "" {
				return &TurnError{Code: ErrorInvalidRequest, Message: "file ID is required"}
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
