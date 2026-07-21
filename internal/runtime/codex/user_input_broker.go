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

type DetachedUserInputContext struct {
	Channel         string
	RoomID          string
	ThreadRootID    string
	SourceMessageID string
}

type DetachedUserInputResolution struct {
	Context   DetachedUserInputContext
	Execution activity.ExecutionRef
	Snapshot  activity.UserInputSnapshot
	Response  CodexUserInputResponse
}

type DetachedUserInputHandler func(DetachedUserInputResolution)

type CodexUserInputAnswer = activity.RequestUserInputAnswer

type CodexUserInputResponse = activity.RequestUserInputResponse

type UserInputDecision struct {
	Snapshot activity.UserInputSnapshot
	Response CodexUserInputResponse
}

type UserInputBroker interface {
	activity.UserInputResponder
	Request(ctx context.Context, req PendingUserInputRequest) (UserInputDecision, error)
	CreateDetached(req PendingUserInputRequest, detached DetachedUserInputContext) (activity.UserInputSnapshot, error)
	AddDetachedHandler(handler DetachedUserInputHandler)
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
	detached  []DetachedUserInputHandler
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
	detachedContext *DetachedUserInputContext
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
	pending, err := b.start(req, nil, true)
	if err != nil {
		return UserInputDecision{}, err
	}
	snapshotID := pending.state.snapshot.ID

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
		return UserInputDecision{Snapshot: publicUserInputSnapshot(resolved.snapshot), Response: response}, resolved.err
	case <-timerC:
		resolved := b.finish(snapshotID, activity.UserInputStatusExpired, CodexUserInputResponse{Answers: map[string]CodexUserInputAnswer{}}, "", nil, nil)
		b.publish(userInputResolvedEvent(resolved))
		response := resolved.response
		b.clearCompletedResponse(snapshotID)
		return UserInputDecision{Snapshot: publicUserInputSnapshot(resolved.snapshot), Response: response}, resolved.err
	case <-ctx.Done():
		resolved := b.finish(snapshotID, activity.UserInputStatusCanceled, CodexUserInputResponse{}, "", nil, ctx.Err())
		b.publish(userInputResolvedEvent(resolved))
		response := resolved.response
		b.clearCompletedResponse(snapshotID)
		return UserInputDecision{Snapshot: publicUserInputSnapshot(resolved.snapshot), Response: response}, resolved.err
	}
}

func (b *MemoryUserInputBroker) start(req PendingUserInputRequest, detached *DetachedUserInputContext, publish bool) (*pendingUserInput, error) {
	questions, err := normalizeUserInputQuestions(req.Questions)
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
	if detached != nil {
		copy := *detached
		copy.Channel = strings.TrimSpace(copy.Channel)
		copy.RoomID = strings.TrimSpace(copy.RoomID)
		copy.ThreadRootID = strings.TrimSpace(copy.ThreadRootID)
		copy.SourceMessageID = strings.TrimSpace(copy.SourceMessageID)
		if copy.Channel == "" || copy.RoomID == "" {
			return nil, fmt.Errorf("%w: detached user input requires channel and room", ErrUserInputInvalidResponse)
		}
		detached = &copy
		snapshot.Channel = copy.Channel
		snapshot.RoomID = copy.RoomID
	}
	if req.AutoResolve > 0 {
		deadline := now.Add(req.AutoResolve)
		snapshot.AutoResolveAt = &deadline
	}
	state := userInputState{
		snapshot:        snapshot,
		execution:       normalizedExecutionRef(req.Execution),
		serverRequestID: strings.TrimSpace(req.ServerRequestID),
		detachedContext: detached,
	}
	pending := &pendingUserInput{state: state, done: make(chan userInputState, 1)}
	b.mu.Lock()
	b.pending[snapshot.ID] = pending
	b.mu.Unlock()
	if publish {
		b.publish(userInputRequestEvent(state))
	}
	return pending, nil
}

func (b *MemoryUserInputBroker) CreateDetached(req PendingUserInputRequest, detached DetachedUserInputContext) (activity.UserInputSnapshot, error) {
	pending, err := b.start(req, &detached, false)
	if err != nil {
		return activity.UserInputSnapshot{}, err
	}
	snapshot := publicUserInputSnapshot(pending.state.snapshot)
	if req.AutoResolve > 0 {
		requestID := snapshot.ID
		go func() {
			timer := time.NewTimer(req.AutoResolve)
			defer timer.Stop()
			<-timer.C
			resolved := b.finish(requestID, activity.UserInputStatusExpired, CodexUserInputResponse{Answers: map[string]CodexUserInputAnswer{}}, "", nil, nil)
			if resolved.detachedContext == nil || resolved.snapshot.Status != activity.UserInputStatusExpired {
				return
			}
			b.notifyDetached(resolved)
		}()
	}
	return snapshot, nil
}

func (b *MemoryUserInputBroker) AddDetachedHandler(handler DetachedUserInputHandler) {
	if handler == nil {
		return
	}
	b.mu.Lock()
	b.detached = append(b.detached, handler)
	b.mu.Unlock()
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

func (b *MemoryUserInputBroker) notifyDetached(state userInputState) {
	if state.detachedContext == nil {
		return
	}
	b.mu.Lock()
	handlers := append([]DetachedUserInputHandler(nil), b.detached...)
	b.mu.Unlock()
	resolution := DetachedUserInputResolution{
		Context:   *state.detachedContext,
		Execution: state.execution,
		Snapshot:  publicUserInputSnapshot(state.snapshot),
		Response:  state.response,
	}
	if len(handlers) == 0 {
		b.clearCompletedResponse(state.snapshot.ID)
		return
	}
	go func() {
		for _, handler := range handlers {
			handler(resolution)
		}
		b.clearCompletedResponse(state.snapshot.ID)
	}()
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
	now := time.Now().UTC()
	b.pruneCompletedLocked(now)
	if snapshot, ok := b.completedSnapshotLocked(req.ActivityID, now); ok {
		b.mu.Unlock()
		if snapshot.Status == activity.UserInputStatusExpired ||
			snapshot.Status == activity.UserInputStatusCanceled ||
			snapshot.Status == activity.UserInputStatusInterrupted {
			return publicUserInputSnapshot(snapshot), ErrUserInputGone
		}
		return publicUserInputSnapshot(snapshot), ErrUserInputAlreadyResolved
	}
	pending := b.pending[req.ActivityID]
	if pending == nil {
		b.mu.Unlock()
		return activity.UserInputSnapshot{}, ErrUserInputNotFound
	}
	if pending.state.snapshot.Channel == "" ||
		pending.state.snapshot.Channel != req.Channel ||
		pending.state.snapshot.RoomID != req.RoomID {
		b.mu.Unlock()
		return activity.UserInputSnapshot{}, ErrUserInputNotFound
	}
	if deadline := pending.state.snapshot.AutoResolveAt; deadline != nil && !now.Before(*deadline) {
		state := b.finishLocked(req.ActivityID, activity.UserInputStatusExpired, CodexUserInputResponse{Answers: map[string]CodexUserInputAnswer{}}, "", nil, nil)
		b.mu.Unlock()
		if state.detachedContext != nil {
			b.notifyDetached(state)
		}
		return publicUserInputSnapshot(state.snapshot), ErrUserInputGone
	}

	status, response, answers, err := buildUserInputResponse(pending.state.snapshot.Questions, req.Response)
	if err != nil {
		b.mu.Unlock()
		return publicUserInputSnapshot(pending.state.snapshot), err
	}
	state := b.finishLocked(req.ActivityID, status, response, req.ResponderID, answers, nil)
	b.mu.Unlock()
	if state.detachedContext != nil {
		b.notifyDetached(state)
	}
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
	var detached []userInputState
	b.mu.Lock()
	for id, pending := range b.pending {
		if runtimeID != "" && pending.state.execution.RuntimeID != runtimeID {
			continue
		}
		if sessionID != "" && pending.state.execution.SessionID != sessionID {
			continue
		}
		state := b.finishLocked(id, activity.UserInputStatusInterrupted, CodexUserInputResponse{}, "", nil, context.Canceled)
		if state.detachedContext != nil {
			detached = append(detached, state)
		}
	}
	b.mu.Unlock()
	for _, state := range detached {
		b.notifyDetached(state)
	}
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
	if len(questions) < 1 || len(questions) > maxStructuredOutputQuestions {
		return nil, fmt.Errorf("%w: expected 1 to %d questions", ErrUserInputInvalidResponse, maxStructuredOutputQuestions)
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
		if len(question.Options) > maxStructuredOutputQuestionOptions {
			return nil, fmt.Errorf("%w: question %q has more than %d options", ErrUserInputInvalidResponse, question.ID, maxStructuredOutputQuestionOptions)
		}
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

func buildUserInputResponse(questions []activity.UserInputQuestionSnapshot, response CodexUserInputResponse) (activity.UserInputStatus, CodexUserInputResponse, map[string]activity.UserInputAnswerSnapshot, error) {
	if len(response.Answers) == 0 {
		return activity.UserInputStatusSkipped, CodexUserInputResponse{Answers: map[string]CodexUserInputAnswer{}}, nil, nil
	}
	known := make(map[string]activity.UserInputQuestionSnapshot, len(questions))
	for _, question := range questions {
		known[question.ID] = question
	}
	for id := range response.Answers {
		if _, ok := known[id]; !ok {
			return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: unknown question id %q", ErrUserInputInvalidResponse, id)
		}
	}
	normalized := CodexUserInputResponse{Answers: make(map[string]CodexUserInputAnswer, len(questions))}
	snapshots := make(map[string]activity.UserInputAnswerSnapshot, len(questions))
	answered := false
	for _, question := range questions {
		input, ok := response.Answers[question.ID]
		if !ok {
			return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: missing answer for question %q", ErrUserInputInvalidResponse, question.ID)
		}
		if len(input.Answers) == 0 {
			normalized.Answers[question.ID] = CodexUserInputAnswer{Answers: []string{}}
			snapshots[question.ID] = activity.UserInputAnswerSnapshot{Skipped: true, Secret: question.IsSecret}
			continue
		}
		if len(input.Answers) > 2 {
			return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: question %q accepts at most one option and one user note", ErrUserInputInvalidResponse, question.ID)
		}
		values := make([]string, 0, len(input.Answers))
		snapshot := activity.UserInputAnswerSnapshot{Answered: true, Secret: question.IsSecret}
		for _, rawValue := range input.Answers {
			value := strings.TrimSpace(rawValue)
			if value == "" {
				return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: question %q contains an empty answer", ErrUserInputInvalidResponse, question.ID)
			}
			if strings.HasPrefix(value, "user_note:") {
				if snapshot.Text != "" {
					return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: question %q contains multiple user notes", ErrUserInputInvalidResponse, question.ID)
				}
				note := strings.TrimSpace(strings.TrimPrefix(value, "user_note:"))
				if note == "" {
					return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: question %q contains an empty user note", ErrUserInputInvalidResponse, question.ID)
				}
				values = append(values, "user_note: "+note)
				if question.IsSecret {
					snapshot.Text = "******"
				} else {
					snapshot.Text = note
				}
				continue
			}
			if snapshot.OptionIndex != 0 {
				return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: question %q contains multiple option labels", ErrUserInputInvalidResponse, question.ID)
			}
			optionIndex := 0
			for index, option := range question.Options {
				if value == option.Label {
					optionIndex = index + 1
					break
				}
			}
			if optionIndex == 0 && question.IsOther && value == userInputOtherLabel {
				optionIndex = len(question.Options) + 1
			}
			if optionIndex == 0 {
				return "", CodexUserInputResponse{}, nil, fmt.Errorf("%w: unknown option label %q for question %q", ErrUserInputInvalidResponse, value, question.ID)
			}
			values = append(values, value)
			snapshot.OptionIndex = optionIndex
			snapshot.OptionLabel = value
		}
		normalized.Answers[question.ID] = CodexUserInputAnswer{Answers: values}
		snapshots[question.ID] = snapshot
		answered = true
	}
	status := activity.UserInputStatusSkipped
	if answered {
		status = activity.UserInputStatusAnswered
	}
	return status, normalized, snapshots, nil
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
