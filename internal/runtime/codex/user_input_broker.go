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
	userInputOtherLabel      = "None of the above"
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

type CodexUserInputAnswer struct {
	Answers []string `json:"answers"`
}

type CodexUserInputResponse struct {
	Answers map[string]CodexUserInputAnswer `json:"answers"`
}

type UserInputDecision struct {
	Snapshot activity.UserInputSnapshot
	Response CodexUserInputResponse
}

type UserInputBroker interface {
	activity.UserInputResponder
	Request(ctx context.Context, req PendingUserInputRequest) (UserInputDecision, error)
	Bind(requestID, channel, roomID string) (activity.UserInputSnapshot, error)
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
	questions, err := normalizeUserInputQuestions(req.Questions)
	if err != nil {
		return UserInputDecision{}, err
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
	pending := &pendingUserInput{state: state, done: make(chan userInputState, 1)}

	b.mu.Lock()
	b.pending[snapshot.ID] = pending
	b.mu.Unlock()
	b.publish(userInputRequestEvent(state))

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
		b.clearCompletedResponse(snapshot.ID)
		return UserInputDecision{Snapshot: publicUserInputSnapshot(resolved.snapshot), Response: response}, resolved.err
	case <-timerC:
		resolved := b.finish(snapshot.ID, activity.UserInputStatusExpired, CodexUserInputResponse{Answers: map[string]CodexUserInputAnswer{}}, "", nil, nil)
		b.publish(userInputResolvedEvent(resolved))
		response := resolved.response
		b.clearCompletedResponse(snapshot.ID)
		return UserInputDecision{Snapshot: publicUserInputSnapshot(resolved.snapshot), Response: response}, resolved.err
	case <-ctx.Done():
		resolved := b.finish(snapshot.ID, activity.UserInputStatusCanceled, CodexUserInputResponse{}, "", nil, ctx.Err())
		b.publish(userInputResolvedEvent(resolved))
		response := resolved.response
		b.clearCompletedResponse(snapshot.ID)
		return UserInputDecision{Snapshot: publicUserInputSnapshot(resolved.snapshot), Response: response}, resolved.err
	}
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

func (b *MemoryUserInputBroker) Bind(requestID, channel, roomID string) (activity.UserInputSnapshot, error) {
	requestID = strings.TrimSpace(requestID)
	channel = strings.TrimSpace(channel)
	roomID = strings.TrimSpace(roomID)
	if requestID == "" || channel == "" || roomID == "" {
		return activity.UserInputSnapshot{}, ErrUserInputInvalidResponse
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now().UTC()
	b.pruneCompletedLocked(now)
	if completed, ok := b.completedSnapshotLocked(requestID, now); ok {
		return publicUserInputSnapshot(completed), nil
	}
	pending := b.pending[requestID]
	if pending == nil {
		return activity.UserInputSnapshot{}, ErrUserInputNotFound
	}
	snapshot := pending.state.snapshot
	if snapshot.Channel != "" && (snapshot.Channel != channel || snapshot.RoomID != roomID) {
		return activity.UserInputSnapshot{}, ErrUserInputInvalidResponse
	}
	snapshot.Channel = channel
	snapshot.RoomID = roomID
	pending.state.snapshot = snapshot
	return publicUserInputSnapshot(snapshot), nil
}

func (b *MemoryUserInputBroker) Respond(_ context.Context, req activity.UserInputResponseRequest) (activity.UserInputSnapshot, error) {
	req.ActivityID = strings.TrimSpace(req.ActivityID)
	req.Channel = strings.TrimSpace(req.Channel)
	req.RoomID = strings.TrimSpace(req.RoomID)
	req.ResponderID = strings.TrimSpace(req.ResponderID)
	if req.ActivityID == "" || req.Channel == "" || req.RoomID == "" || req.ResponderID == "" {
		return activity.UserInputSnapshot{}, ErrUserInputInvalidResponse
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now().UTC()
	b.pruneCompletedLocked(now)
	if snapshot, ok := b.completedSnapshotLocked(req.ActivityID, now); ok {
		if snapshot.Status == activity.UserInputStatusExpired ||
			snapshot.Status == activity.UserInputStatusCanceled ||
			snapshot.Status == activity.UserInputStatusInterrupted {
			return publicUserInputSnapshot(snapshot), ErrUserInputGone
		}
		return publicUserInputSnapshot(snapshot), ErrUserInputAlreadyResolved
	}
	pending := b.pending[req.ActivityID]
	if pending == nil {
		return activity.UserInputSnapshot{}, ErrUserInputNotFound
	}
	if pending.state.snapshot.Channel == "" ||
		pending.state.snapshot.Channel != req.Channel ||
		pending.state.snapshot.RoomID != req.RoomID {
		return activity.UserInputSnapshot{}, ErrUserInputNotFound
	}
	if deadline := pending.state.snapshot.AutoResolveAt; deadline != nil && !now.Before(*deadline) {
		state := b.finishLocked(req.ActivityID, activity.UserInputStatusExpired, CodexUserInputResponse{Answers: map[string]CodexUserInputAnswer{}}, "", nil, nil)
		return publicUserInputSnapshot(state.snapshot), ErrUserInputGone
	}

	status, response, answers, err := buildUserInputResponse(pending.state.snapshot.Questions, req)
	if err != nil {
		return publicUserInputSnapshot(pending.state.snapshot), err
	}
	state := b.finishLocked(req.ActivityID, status, response, req.ResponderID, answers, nil)
	return publicUserInputSnapshot(state.snapshot), nil
}

func (b *MemoryUserInputBroker) Get(requestID string) (activity.UserInputSnapshot, bool) {
	requestID = strings.TrimSpace(requestID)
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now().UTC()
	b.pruneCompletedLocked(now)
	if pending := b.pending[requestID]; pending != nil {
		return publicUserInputSnapshot(pending.state.snapshot), true
	}
	snapshot, ok := b.completedSnapshotLocked(requestID, now)
	return publicUserInputSnapshot(snapshot), ok
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

func normalizeUserInputQuestions(questions []activity.UserInputQuestionSnapshot) ([]activity.UserInputQuestionSnapshot, error) {
	if len(questions) < 1 || len(questions) > 3 {
		return nil, fmt.Errorf("%w: expected 1 to 3 questions", ErrUserInputInvalidResponse)
	}
	seen := make(map[string]struct{}, len(questions))
	out := make([]activity.UserInputQuestionSnapshot, 0, len(questions))
	for _, question := range questions {
		question.ID = strings.TrimSpace(question.ID)
		question.Header = strings.TrimSpace(question.Header)
		question.Question = strings.TrimSpace(question.Question)
		if question.ID == "" || question.Header == "" || question.Question == "" {
			return nil, fmt.Errorf("%w: question id, header, and text are required", ErrUserInputInvalidResponse)
		}
		if _, ok := seen[question.ID]; ok {
			return nil, fmt.Errorf("%w: duplicate question id %q", ErrUserInputInvalidResponse, question.ID)
		}
		seen[question.ID] = struct{}{}
		options := make([]activity.UserInputOptionSnapshot, 0, len(question.Options))
		for _, option := range question.Options {
			option.Label = strings.TrimSpace(option.Label)
			option.Description = strings.TrimSpace(option.Description)
			if option.Label == "" {
				return nil, fmt.Errorf("%w: option labels are required", ErrUserInputInvalidResponse)
			}
			options = append(options, option)
		}
		question.Options = options
		out = append(out, question)
	}
	return out, nil
}

func buildUserInputResponse(questions []activity.UserInputQuestionSnapshot, req activity.UserInputResponseRequest) (activity.UserInputStatus, CodexUserInputResponse, map[string]activity.UserInputAnswerSnapshot, error) {
	if req.SkipAll {
		if len(req.Answers) > 0 {
			return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: skip_all cannot include answers", ErrUserInputInvalidResponse)
		}
		return activity.UserInputStatusSkipped, CodexUserInputResponse{Answers: map[string]CodexUserInputAnswer{}}, nil, nil
	}
	known := make(map[string]activity.UserInputQuestionSnapshot, len(questions))
	for _, question := range questions {
		known[question.ID] = question
	}
	for id := range req.Answers {
		if _, ok := known[id]; !ok {
			return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: unknown question id %q", ErrUserInputInvalidResponse, id)
		}
	}
	response := CodexUserInputResponse{Answers: make(map[string]CodexUserInputAnswer, len(questions))}
	snapshots := make(map[string]activity.UserInputAnswerSnapshot, len(questions))
	answered := false
	for _, question := range questions {
		input, ok := req.Answers[question.ID]
		if !ok {
			return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: missing answer for question %q", ErrUserInputInvalidResponse, question.ID)
		}
		input.Text = strings.TrimSpace(input.Text)
		if input.Skip {
			if input.OptionIndex != 0 || input.Text != "" {
				return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: skipped question %q cannot include an option or text", ErrUserInputInvalidResponse, question.ID)
			}
			response.Answers[question.ID] = CodexUserInputAnswer{Answers: []string{}}
			snapshots[question.ID] = activity.UserInputAnswerSnapshot{Skipped: true, Secret: question.IsSecret}
			continue
		}
		if input.OptionIndex == 0 && input.Text == "" {
			return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: question %q requires an option, text, or skip", ErrUserInputInvalidResponse, question.ID)
		}
		values := make([]string, 0, 2)
		snapshot := activity.UserInputAnswerSnapshot{Answered: true, Secret: question.IsSecret}
		if input.OptionIndex != 0 {
			optionCount := len(question.Options)
			if question.IsOther {
				optionCount++
			}
			if input.OptionIndex < 1 || input.OptionIndex > optionCount {
				return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: option index for question %q is out of range", ErrUserInputInvalidResponse, question.ID)
			}
			label := userInputOtherLabel
			if input.OptionIndex <= len(question.Options) {
				label = question.Options[input.OptionIndex-1].Label
			}
			values = append(values, label)
			snapshot.OptionIndex = input.OptionIndex
			snapshot.OptionLabel = label
		}
		if input.Text != "" {
			values = append(values, "user_note: "+input.Text)
			if question.IsSecret {
				snapshot.Text = "******"
			} else {
				snapshot.Text = input.Text
			}
		}
		response.Answers[question.ID] = CodexUserInputAnswer{Answers: values}
		snapshots[question.ID] = snapshot
		answered = true
	}
	status := activity.UserInputStatusSkipped
	if answered {
		status = activity.UserInputStatusAnswered
	}
	return status, response, snapshots, nil
}

func publicUserInputSnapshot(snapshot activity.UserInputSnapshot) activity.UserInputSnapshot {
	out := snapshot
	out.Questions = append([]activity.UserInputQuestionSnapshot(nil), snapshot.Questions...)
	for i := range out.Questions {
		out.Questions[i].Options = append([]activity.UserInputOptionSnapshot(nil), snapshot.Questions[i].Options...)
	}
	if snapshot.Answers != nil {
		out.Answers = make(map[string]activity.UserInputAnswerSnapshot, len(snapshot.Answers))
		for id, answer := range snapshot.Answers {
			if answer.Secret && answer.Answered && answer.Text != "" {
				answer.Text = "******"
			}
			out.Answers[id] = answer
		}
	}
	return out
}
