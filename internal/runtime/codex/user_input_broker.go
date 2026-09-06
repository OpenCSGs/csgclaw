package codex

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/activity"
)

const (
	defaultUserInputCacheTTL = 10 * time.Minute
)

var (
	ErrUserInputNotFound        = activity.ErrUserInputNotFound
	ErrUserInputInvalidResponse = activity.ErrUserInputInvalidResponse
	ErrUserInputAlreadyResolved = activity.ErrUserInputAlreadyResolved
	ErrUserInputGone            = activity.ErrUserInputGone
)

type PendingUserInputRequest struct {
	Execution       activity.ExecutionRef
	ServerRequestID string
	Questions       []activity.UserInputQuestionSnapshot
	RequestedAt     time.Time
	AutoResolve     time.Duration
}

type CodexUserInputAnswer = activity.RequestUserInputAnswer

type CodexUserInputResponse = activity.RequestUserInputResponse

type UserInputDecision struct {
	Snapshot activity.UserInputSnapshot
	Response CodexUserInputResponse
}

type UserInputBroker interface {
	RespondDirect(context.Context, string, string, activity.RequestUserInputResponse) (activity.UserInputSnapshot, error)
	Request(ctx context.Context, req PendingUserInputRequest) (UserInputDecision, error)
	Get(requestID string) (activity.UserInputSnapshot, bool)
	CancelSession(runtimeID, sessionID string)
	CancelServerRequest(runtimeID, sessionID, serverRequestID string)
}

type MemoryUserInputBroker struct {
	mu        sync.Mutex
	nextID    int
	idPrefix  string
	cacheTTL  time.Duration
	eventSink SessionEventSink
	pending   map[string]*pendingUserInput
	completed map[string]completedUserInput
}

type pendingUserInput struct {
	id    string
	state userInputState
	done  chan userInputState
}

type completedUserInput struct {
	state     userInputState
	expiresAt time.Time
}

type userInputState struct {
	snapshot        activity.UserInputSnapshot
	execution       activity.ExecutionRef
	serverRequestID string
	response        CodexUserInputResponse
	err             error
}

func NewUserInputBroker(eventSink SessionEventSink) *MemoryUserInputBroker {
	return &MemoryUserInputBroker{
		idPrefix:  fmt.Sprintf("question-%d-%d", os.Getpid(), time.Now().UTC().UnixNano()),
		cacheTTL:  defaultUserInputCacheTTL,
		eventSink: eventSink,
		pending:   make(map[string]*pendingUserInput),
		completed: make(map[string]completedUserInput),
	}
}

func (b *MemoryUserInputBroker) Request(ctx context.Context, req PendingUserInputRequest) (UserInputDecision, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	pending, err := b.start(req)
	if err != nil {
		return UserInputDecision{}, err
	}
	snapshotID := pending.id

	var timer *time.Timer
	var timerC <-chan time.Time
	if req.AutoResolve > 0 {
		timer = time.NewTimer(req.AutoResolve)
		timerC = timer.C
		defer timer.Stop()
	}

	select {
	case resolved := <-pending.done:
		b.publish(userInputResolvedEvent(resolved))
		response := resolved.response
		b.clearCompletedResponse(snapshotID)
		return UserInputDecision{Snapshot: activity.PublicUserInputSnapshot(resolved.snapshot), Response: response}, resolved.err
	case <-timerC:
		resolved := b.finish(snapshotID, activity.UserInputStatusExpired, CodexUserInputResponse{Answers: map[string]CodexUserInputAnswer{}}, "", nil, nil)
		b.publish(userInputResolvedEvent(resolved))
		response := resolved.response
		b.clearCompletedResponse(snapshotID)
		return UserInputDecision{Snapshot: activity.PublicUserInputSnapshot(resolved.snapshot), Response: response}, resolved.err
	case <-ctx.Done():
		resolved := b.finish(snapshotID, activity.UserInputStatusCanceled, CodexUserInputResponse{}, "", nil, ctx.Err())
		b.publish(userInputResolvedEvent(resolved))
		response := resolved.response
		b.clearCompletedResponse(snapshotID)
		return UserInputDecision{Snapshot: activity.PublicUserInputSnapshot(resolved.snapshot), Response: response}, resolved.err
	}
}

func (b *MemoryUserInputBroker) start(req PendingUserInputRequest) (*pendingUserInput, error) {
	questions, err := activity.NormalizeUserInputQuestions(req.Questions)
	if err != nil {
		return nil, err
	}
	now := req.RequestedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	snapshot := activity.UserInputSnapshot{
		ID:          b.nextRequestID(),
		Status:      activity.UserInputStatusPending,
		Questions:   questions,
		RequestedAt: now,
	}
	if req.AutoResolve > 0 {
		deadline := now.Add(req.AutoResolve)
		snapshot.AutoResolveAt = &deadline
	}
	state := userInputState{
		snapshot:        snapshot,
		execution:       normalizedExecutionRef(req.Execution),
		serverRequestID: strings.TrimSpace(req.ServerRequestID),
	}
	pending := &pendingUserInput{id: snapshot.ID, state: state, done: make(chan userInputState, 1)}
	b.mu.Lock()
	b.pending[snapshot.ID] = pending
	b.mu.Unlock()
	b.publish(userInputRequestEvent(state))
	return pending, nil
}

func (b *MemoryUserInputBroker) clearCompletedResponse(requestID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	completed, ok := b.completed[requestID]
	if !ok {
		return
	}
	completed.state.response = CodexUserInputResponse{}
	b.completed[requestID] = completed
}

func (b *MemoryUserInputBroker) RespondDirect(ctx context.Context, requestID, responderID string, input activity.RequestUserInputResponse) (activity.UserInputSnapshot, error) {
	if ctx != nil && ctx.Err() != nil {
		return activity.UserInputSnapshot{}, ctx.Err()
	}
	requestID, responderID = strings.TrimSpace(requestID), strings.TrimSpace(responderID)
	if requestID == "" || responderID == "" {
		return activity.UserInputSnapshot{}, ErrUserInputInvalidResponse
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now().UTC()
	b.pruneCompletedLocked(now)
	if snapshot, ok := b.completedSnapshotLocked(requestID, now); ok {
		if snapshot.Status == activity.UserInputStatusExpired || snapshot.Status == activity.UserInputStatusCanceled || snapshot.Status == activity.UserInputStatusInterrupted {
			return activity.PublicUserInputSnapshot(snapshot), ErrUserInputGone
		}
		return activity.PublicUserInputSnapshot(snapshot), ErrUserInputAlreadyResolved
	}
	pending := b.pending[requestID]
	if pending == nil {
		return activity.UserInputSnapshot{}, ErrUserInputNotFound
	}
	if deadline := pending.state.snapshot.AutoResolveAt; deadline != nil && !now.Before(*deadline) {
		state := b.finishLocked(requestID, activity.UserInputStatusExpired, CodexUserInputResponse{Answers: map[string]CodexUserInputAnswer{}}, "", nil, nil)
		return activity.PublicUserInputSnapshot(state.snapshot), ErrUserInputGone
	}
	status, response, answers, err := activity.BuildUserInputResponse(pending.state.snapshot.Questions, input)
	if err != nil {
		return activity.PublicUserInputSnapshot(pending.state.snapshot), err
	}
	state := b.finishLocked(requestID, status, response, responderID, answers, nil)
	return activity.PublicUserInputSnapshot(state.snapshot), nil
}

func (b *MemoryUserInputBroker) Get(requestID string) (activity.UserInputSnapshot, bool) {
	requestID = strings.TrimSpace(requestID)
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now().UTC()
	b.pruneCompletedLocked(now)
	if pending := b.pending[requestID]; pending != nil {
		return activity.PublicUserInputSnapshot(pending.state.snapshot), true
	}
	snapshot, ok := b.completedSnapshotLocked(requestID, now)
	return activity.PublicUserInputSnapshot(snapshot), ok
}

func (b *MemoryUserInputBroker) CancelSession(runtimeID, sessionID string) {
	runtimeID = strings.TrimSpace(runtimeID)
	sessionID = strings.TrimSpace(sessionID)
	if runtimeID == "" && sessionID == "" {
		return
	}
	b.mu.Lock()
	for id, pending := range b.pending {
		if runtimeID != "" && pending.state.execution.RuntimeID != runtimeID {
			continue
		}
		if sessionID != "" && pending.state.execution.SessionID != sessionID {
			continue
		}
		b.finishLocked(id, activity.UserInputStatusInterrupted, CodexUserInputResponse{}, "", nil, context.Canceled)
	}
	b.mu.Unlock()
}

func (b *MemoryUserInputBroker) CancelServerRequest(runtimeID, sessionID, serverRequestID string) {
	runtimeID = strings.TrimSpace(runtimeID)
	sessionID = strings.TrimSpace(sessionID)
	serverRequestID = strings.TrimSpace(serverRequestID)
	if serverRequestID == "" {
		return
	}
	b.mu.Lock()
	for id, pending := range b.pending {
		if pending.state.serverRequestID != serverRequestID {
			continue
		}
		if runtimeID != "" && pending.state.execution.RuntimeID != runtimeID {
			continue
		}
		if sessionID != "" && pending.state.execution.SessionID != sessionID {
			continue
		}
		b.finishLocked(id, activity.UserInputStatusCanceled, CodexUserInputResponse{}, "", nil, context.Canceled)
	}
	b.mu.Unlock()
}

func (b *MemoryUserInputBroker) nextRequestID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	return fmt.Sprintf("%s-%d", b.idPrefix, b.nextID)
}

func (b *MemoryUserInputBroker) finish(requestID string, status activity.UserInputStatus, response CodexUserInputResponse, responderID string, answers map[string]activity.UserInputAnswerSnapshot, err error) userInputState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.finishLocked(requestID, status, response, responderID, answers, err)
}

func (b *MemoryUserInputBroker) finishLocked(requestID string, status activity.UserInputStatus, response CodexUserInputResponse, responderID string, answers map[string]activity.UserInputAnswerSnapshot, err error) userInputState {
	now := time.Now().UTC()
	b.pruneCompletedLocked(now)
	pending := b.pending[requestID]
	if pending == nil {
		if completed, ok := b.completed[requestID]; ok {
			return completed.state
		}
		return userInputState{snapshot: activity.UserInputSnapshot{ID: requestID, Status: status}, response: response, err: err}
	}
	state := pending.state
	state.snapshot.Status = status
	state.snapshot.ResolvedAt = &now
	state.snapshot.ResponderID = strings.TrimSpace(responderID)
	state.snapshot.Answers = answers
	state.response = response
	state.err = err
	delete(b.pending, requestID)
	b.completed[requestID] = completedUserInput{state: state, expiresAt: now.Add(b.cacheTTL)}
	pending.done <- state
	close(pending.done)
	return state
}

func (b *MemoryUserInputBroker) completedSnapshotLocked(requestID string, now time.Time) (activity.UserInputSnapshot, bool) {
	completed, ok := b.completed[requestID]
	if !ok {
		return activity.UserInputSnapshot{}, false
	}
	if !completed.expiresAt.IsZero() && !now.Before(completed.expiresAt) {
		delete(b.completed, requestID)
		return activity.UserInputSnapshot{}, false
	}
	return completed.state.snapshot, true
}

func (b *MemoryUserInputBroker) pruneCompletedLocked(now time.Time) {
	for id, completed := range b.completed {
		if !completed.expiresAt.IsZero() && !now.Before(completed.expiresAt) {
			delete(b.completed, id)
		}
	}
}

func (b *MemoryUserInputBroker) publish(event SessionEvent) {
	if b != nil && b.eventSink != nil {
		b.eventSink.Publish(event)
	}
}
