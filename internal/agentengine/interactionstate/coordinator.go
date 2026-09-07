package interactionstate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine/contract"
)

// Coordinator owns pending and terminal interaction state independently
// of Runtime-native request brokers. It is also used by the in-memory client.
// Only opaque Agent/conversation/turn identities cross this boundary.
type Coordinator struct {
	mu    sync.Mutex
	items map[interactionIdentity]*interactionRecord
}

type interactionIdentity struct {
	agentID string
	key     contract.ConversationKey
	id      string
}
type interactionRecord struct {
	request       contract.InteractionRequest
	turnID        contract.TurnID
	resolve       func(context.Context, contract.InteractionRequest, contract.InteractionResolution) *contract.TurnError
	notify        func(contract.InteractionRequest)
	resolving     bool
	resolveCancel context.CancelFunc
	terminal      bool
	finishedAt    time.Time
	timer         *time.Timer
}

// Register records an interaction emitted by the selected Runtime Adapter.
func (c *Coordinator) Register(agentID string, key contract.ConversationKey, turnID contract.TurnID, request contract.InteractionRequest, resolve func(context.Context, contract.InteractionRequest, contract.InteractionResolution) *contract.TurnError, notify func(contract.InteractionRequest)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = make(map[interactionIdentity]*interactionRecord)
	}
	c.pruneLocked()
	id := interactionIdentity{agentID, key, strings.TrimSpace(request.ID)}
	if c.items[id] != nil {
		return
	}
	record := &interactionRecord{request: Clone(request), turnID: turnID, resolve: resolve, notify: notify}
	c.items[id] = record
	var deadline time.Time
	switch snapshot := request.Payload.(type) {
	case activity.UserInputSnapshot:
		if snapshot.AutoResolveAt != nil {
			deadline = *snapshot.AutoResolveAt
		}
	case activity.ActivitySnapshot:
		deadline = snapshot.ExpiresAt
	}
	if request.Detached && !deadline.IsZero() {
		record.timer = time.AfterFunc(time.Until(deadline), func() { c.expire(id) })
	}
}

// CreateDetached activates validated output only after its Turn succeeded.
// It does not create an IM identity, access a Runtime session, or execute a Turn.
func (c *Coordinator) CreateDetached(agentID string, key contract.ConversationKey, turnID contract.TurnID, args activity.RequestUserInputArgs, notify func(contract.InteractionRequest)) (contract.InteractionRequest, error) {
	questions := make([]activity.UserInputQuestionSnapshot, 0, len(args.Questions))
	for _, q := range args.Questions {
		item := activity.UserInputQuestionSnapshot{ID: q.ID, Header: q.Header, Question: q.Question, IsOther: q.IsOther, IsSecret: q.IsSecret}
		for _, option := range q.Options {
			item.Options = append(item.Options, activity.UserInputOptionSnapshot{Label: option.Label, Description: option.Description})
		}
		questions = append(questions, item)
	}
	questions, err := activity.NormalizeUserInputQuestions(questions)
	if err != nil {
		return contract.InteractionRequest{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return contract.InteractionRequest{}, err
	}
	now := time.Now().UTC()
	snapshot := activity.UserInputSnapshot{ID: "question-" + hex.EncodeToString(random[:]), Status: activity.UserInputStatusPending, Questions: questions, RequestedAt: now}
	if args.AutoResolutionMS != nil && *args.AutoResolutionMS > 0 {
		if *args.AutoResolutionMS > uint64((24*time.Hour)/time.Millisecond) {
			return contract.InteractionRequest{}, fmt.Errorf("interaction deadline exceeds 24 hours")
		}
		deadline := now.Add(time.Duration(*args.AutoResolutionMS) * time.Millisecond)
		snapshot.AutoResolveAt = &deadline
	}
	request := contract.InteractionRequest{ID: snapshot.ID, Kind: contract.InteractionUserInput, Detached: true, Title: "User input required", Payload: snapshot}
	c.Register(agentID, key, turnID, request, nil, notify)
	return Clone(request), nil
}

func (c *Coordinator) Get(agentID string, key contract.ConversationKey, id string) (contract.InteractionRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked()
	item := c.items[interactionIdentity{agentID, key, strings.TrimSpace(id)}]
	if item == nil {
		return contract.InteractionRequest{}, &contract.TurnError{Code: contract.ErrorInteractionNotFound, Message: "interaction was not found"}
	}
	return Clone(item.request), nil
}

// Resolve reserves the request before validating side effects. Secret response
// values are passed to the Runtime only and never retained in the record.
func (c *Coordinator) Resolve(ctx context.Context, agentID string, resolution contract.InteractionResolution) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	id := interactionIdentity{agentID, resolution.ConversationKey, strings.TrimSpace(resolution.InteractionID)}
	c.mu.Lock()
	item := c.items[id]
	if item == nil {
		c.mu.Unlock()
		return &contract.TurnError{Code: contract.ErrorInteractionNotFound, Message: "interaction was not found"}
	}
	if item.terminal || item.resolving {
		code := contract.ErrorInteractionAlreadyResolved
		if interactionGone(item.request) {
			code = contract.ErrorInteractionGone
		}
		c.mu.Unlock()
		return &contract.TurnError{Code: code, Message: "interaction is no longer pending"}
	}
	current := Clone(item.request)
	next, err := resolvedInteraction(current, resolution)
	if err != nil {
		c.mu.Unlock()
		if contract.ErrorCodeOf(err) != "" {
			return err
		}
		return &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: err.Error()}
	}
	item.resolving = true
	resolveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	item.resolveCancel = cancel
	resolve := item.resolve
	c.mu.Unlock()
	if resolution.BeforeResolve != nil {
		err = resolution.BeforeResolve(resolveCtx, Clone(next))
	}
	c.mu.Lock()
	interrupted := item.terminal
	c.mu.Unlock()
	if interrupted {
		err = &contract.TurnError{Code: contract.ErrorInteractionGone, Message: "interaction was interrupted"}
	}
	if err == nil {
		err = resolveCtx.Err()
	}
	if err == nil && resolve != nil {
		if runtimeErr := resolve(resolveCtx, current, resolution); runtimeErr != nil {
			err = runtimeErr
		}
	}
	c.mu.Lock()
	item.resolving = false
	item.resolveCancel = nil
	if item.terminal {
		c.mu.Unlock()
		return &contract.TurnError{Code: contract.ErrorInteractionGone, Message: "interaction was interrupted"}
	}
	if err == nil {
		err = resolveCtx.Err()
	}
	if err != nil {
		c.mu.Unlock()
		return err
	}
	item.request = next
	c.finishLocked(item)
	notify := item.notify
	c.mu.Unlock()
	if notify != nil {
		notify(Clone(next))
	}
	return nil
}

func resolvedInteraction(request contract.InteractionRequest, resolution contract.InteractionResolution) (contract.InteractionRequest, error) {
	now := time.Now().UTC()
	switch request.Kind {
	case contract.InteractionUserInput:
		snapshot, ok := request.Payload.(activity.UserInputSnapshot)
		if !ok {
			if !request.Detached {
				return request, nil
			} // Opaque native payloads are validated by their Adapter.
			return request, fmt.Errorf("invalid user input payload")
		}
		if snapshot.AutoResolveAt != nil && !now.Before(*snapshot.AutoResolveAt) {
			return request, &contract.TurnError{Code: contract.ErrorInteractionGone, Message: "interaction expired"}
		}
		answers := make(map[string]activity.RequestUserInputAnswer, len(resolution.Answers))
		for id, answer := range resolution.Answers {
			values := append([]string(nil), answer.Values...)
			if answer.Skipped {
				values = []string{}
			}
			answers[id] = activity.RequestUserInputAnswer{Answers: values}
		}
		status, _, snapshots, err := activity.BuildUserInputResponse(snapshot.Questions, activity.RequestUserInputResponse{Answers: answers})
		if err != nil {
			return request, err
		}
		snapshot.Status = status
		snapshot.Answers = snapshots
		snapshot.ResolvedAt = &now
		snapshot.ResponderID = resolution.ResponderID
		request.Payload = activity.PublicUserInputSnapshot(snapshot)
	case contract.InteractionPermission:
		snapshot, ok := request.Payload.(activity.ActivitySnapshot)
		if !ok {
			return request, nil // Opaque native payloads are validated by their Adapter.
		}
		if !snapshot.ExpiresAt.IsZero() && !now.Before(snapshot.ExpiresAt) {
			return request, &contract.TurnError{Code: contract.ErrorInteractionGone, Message: "permission expired"}
		}
		var option *activity.ActionOptionSnapshot
		for i := range snapshot.Options {
			if snapshot.Options[i].ID == resolution.OptionID {
				option = &snapshot.Options[i]
				break
			}
		}
		if option == nil {
			return request, activity.ErrActionInvalidOption
		}
		snapshot.Status = activity.ActionStatusRejected
		if option.Kind == "allow_once" || option.Kind == "allow_always" {
			snapshot.Status = activity.ActionStatusAllowed
		}
		snapshot.Decision = &activity.ActionDecisionSnapshot{OptionID: option.ID, Kind: option.Kind, DecidedAt: now}
		request.Payload = snapshot
	default:
		return request, fmt.Errorf("unsupported interaction kind")
	}
	return request, nil
}

// Observe accepts authoritative terminal events from the Runtime Adapter.
func (c *Coordinator) Observe(agentID string, key contract.ConversationKey, event contract.TurnEvent) {
	if event.Activity == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	item := c.items[interactionIdentity{agentID, key, event.Activity.ID}]
	if item == nil || item.terminal {
		return
	}
	item.request.Payload = event.Activity.Payload
	// Resolve may still be inside the Runtime call when its terminal event arrives.
	if !item.resolving {
		c.finishLocked(item)
	}
}

// Interrupt invalidates requests in the chosen scope; empty key/turn matches all.
func (c *Coordinator) Interrupt(agentID string, key contract.ConversationKey, turnID contract.TurnID, detachedOnly bool) {
	c.interrupt(agentID, key, turnID, detachedOnly, false)
}

// CompleteTurn abandons unanswered native requests after successful execution.
// A native response can unblock its Turn before the response call returns, so
// successful completion must allow that reserved response to finish.
func (c *Coordinator) CompleteTurn(agentID string, key contract.ConversationKey, turnID contract.TurnID) {
	c.interrupt(agentID, key, turnID, false, true)
}

func (c *Coordinator) interrupt(agentID string, key contract.ConversationKey, turnID contract.TurnID, detachedOnly, completed bool) {
	var notifications []func()
	c.mu.Lock()
	for id, item := range c.items {
		if id.agentID != agentID || (key != "" && id.key != key) || (turnID != "" && item.turnID != turnID) || item.terminal || (detachedOnly && !item.request.Detached) {
			continue
		}
		if completed && item.resolving && !item.request.Detached {
			continue
		}
		c.interruptLocked(item, false)
		if item.notify != nil {
			request, notify := Clone(item.request), item.notify
			notifications = append(notifications, func() { notify(request) })
		}
	}
	c.mu.Unlock()
	for _, notify := range notifications {
		notify()
	}
}
func (c *Coordinator) expire(id interactionIdentity) {
	c.mu.Lock()
	item := c.items[id]
	if item == nil || item.terminal {
		c.mu.Unlock()
		return
	}
	c.interruptLocked(item, true)
	request, notify := Clone(item.request), item.notify
	c.mu.Unlock()
	if notify != nil {
		notify(request)
	}
}
func (c *Coordinator) interruptLocked(item *interactionRecord, expired bool) {
	// Invalidation also cancels a reserved response. Its transcript callback
	// may still be returning, but it must not release a native request or
	// publish a detached continuation after Cancel, Reset or lifecycle changes.
	if item.resolveCancel != nil {
		item.resolveCancel()
	}
	now := time.Now().UTC()
	switch snapshot := item.request.Payload.(type) {
	case activity.UserInputSnapshot:
		snapshot.Status = activity.UserInputStatusInterrupted
		if expired {
			snapshot.Status = activity.UserInputStatusExpired
		}
		snapshot.ResolvedAt = &now
		item.request.Payload = snapshot
	case activity.ActivitySnapshot:
		snapshot.Status = activity.ActionStatusCanceled
		if expired {
			snapshot.Status = activity.ActionStatusExpired
		}
		item.request.Payload = snapshot
	}
	c.finishLocked(item)
}
func (c *Coordinator) finishLocked(item *interactionRecord) {
	item.terminal = true
	item.finishedAt = time.Now()
	if item.timer != nil {
		item.timer.Stop()
	}
	item.resolve = nil
}
func (c *Coordinator) pruneLocked() {
	for id, item := range c.items {
		if item.terminal && time.Since(item.finishedAt) > 10*time.Minute {
			delete(c.items, id)
		}
	}
}
func interactionGone(request contract.InteractionRequest) bool {
	switch snapshot := request.Payload.(type) {
	case activity.UserInputSnapshot:
		return snapshot.Status == activity.UserInputStatusCanceled || snapshot.Status == activity.UserInputStatusInterrupted || snapshot.Status == activity.UserInputStatusExpired
	case activity.ActivitySnapshot:
		return snapshot.Status == activity.ActionStatusCanceled || snapshot.Status == activity.ActionStatusExpired
	}
	return false
}
func Clone(request contract.InteractionRequest) contract.InteractionRequest {
	switch snapshot := request.Payload.(type) {
	case activity.UserInputSnapshot:
		request.Payload = activity.PublicUserInputSnapshot(snapshot)
	case activity.ActivitySnapshot:
		snapshot.Options = append([]activity.ActionOptionSnapshot(nil), snapshot.Options...)
		if snapshot.Decision != nil {
			decision := *snapshot.Decision
			snapshot.Decision = &decision
		}
		request.Payload = snapshot
	}
	return request
}
