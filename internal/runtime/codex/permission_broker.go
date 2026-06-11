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
	defaultPermissionTimeout  = time.Minute
	defaultPermissionCacheTTL = 10 * time.Minute
)

var (
	ErrPermissionNotFound       = activity.ErrActionNotFound
	ErrPermissionInvalidOption  = activity.ErrActionInvalidOption
	ErrPermissionAlreadyDecided = activity.ErrActionAlreadyDecided
	ErrPermissionGone           = activity.ErrActionGone
)

type PermissionStatus = activity.ActionStatus

const (
	PermissionStatusPending  = activity.ActionStatusPending
	PermissionStatusAllowed  = activity.ActionStatusAllowed
	PermissionStatusRejected = activity.ActionStatusRejected
	PermissionStatusExpired  = activity.ActionStatusExpired
	PermissionStatusCanceled = activity.ActionStatusCanceled
)

const PermissionKindPermission = activity.ActionKindPermission

type PermissionOptionSnapshot = activity.ActionOptionSnapshot

type PermissionDecisionSnapshot = activity.ActionDecisionSnapshot

type PermissionSnapshot = activity.ActivitySnapshot

type PendingPermissionRequest struct {
	ExecutionRef activity.ExecutionRef
	ToolTitle    string
	Options      []PermissionOptionSnapshot
	RequestedAt  time.Time
	Timeout      time.Duration
}

type PermissionDecision struct {
	Snapshot PermissionSnapshot
}

type PermissionBroker interface {
	Request(ctx context.Context, req PendingPermissionRequest) (PermissionDecision, error)
	Decide(ctx context.Context, requestID string, optionID string) (PermissionSnapshot, error)
	Get(requestID string) (PermissionSnapshot, bool)
	CancelSession(runtimeID string, sessionID string)
}

type PermissionDecider interface {
	Decide(ctx context.Context, requestID string, optionID string) (PermissionSnapshot, error)
}

type MemoryPermissionBroker struct {
	mu        sync.Mutex
	nextID    int
	idPrefix  string
	timeout   time.Duration
	cacheTTL  time.Duration
	eventSink SessionEventSink
	pending   map[string]*pendingPermission
	completed map[string]completedPermission
}

type pendingPermission struct {
	state permissionState
	done  chan permissionState
}

type completedPermission struct {
	state     permissionState
	expiresAt time.Time
}

type permissionState struct {
	snapshot  PermissionSnapshot
	execution activity.ExecutionRef
}

func NewPermissionBroker(eventSink SessionEventSink) *MemoryPermissionBroker {
	return &MemoryPermissionBroker{
		idPrefix:  fmt.Sprintf("perm-%d-%d", os.Getpid(), time.Now().UTC().UnixNano()),
		timeout:   defaultPermissionTimeout,
		cacheTTL:  defaultPermissionCacheTTL,
		eventSink: eventSink,
		pending:   make(map[string]*pendingPermission),
		completed: make(map[string]completedPermission),
	}
}

func (b *MemoryPermissionBroker) Request(ctx context.Context, req PendingPermissionRequest) (PermissionDecision, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := req.RequestedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = b.timeout
	}
	if timeout <= 0 {
		timeout = defaultPermissionTimeout
	}
	snapshot := PermissionSnapshot{
		ID:          b.nextRequestID(),
		Kind:        PermissionKindPermission,
		Title:       firstPermissionText(req.ToolTitle, "Run tool"),
		Status:      PermissionStatusPending,
		RequestedAt: now,
		ExpiresAt:   now.Add(timeout),
		Options:     normalizedPermissionOptions(req.Options),
	}
	state := permissionState{
		snapshot:  snapshot,
		execution: normalizedExecutionRef(req.ExecutionRef),
	}

	pending := &pendingPermission{state: state, done: make(chan permissionState, 1)}
	b.mu.Lock()
	b.pending[snapshot.ID] = pending
	b.mu.Unlock()

	b.publish(permissionRequestEvent(state))

	if len(snapshot.Options) == 0 {
		decided := b.finish(snapshot.ID, PermissionStatusCanceled, nil)
		b.publish(permissionDecisionEvent(decided))
		return PermissionDecision{Snapshot: decided.snapshot}, nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case decided := <-pending.done:
		b.publish(permissionDecisionEvent(decided))
		return PermissionDecision{Snapshot: decided.snapshot}, nil
	case <-timer.C:
		decided := b.finish(snapshot.ID, PermissionStatusExpired, nil)
		b.publish(permissionDecisionEvent(decided))
		return PermissionDecision{Snapshot: decided.snapshot}, nil
	case <-ctx.Done():
		decided := b.finish(snapshot.ID, PermissionStatusCanceled, nil)
		b.publish(permissionDecisionEvent(decided))
		return PermissionDecision{Snapshot: decided.snapshot}, nil
	}
}

func (b *MemoryPermissionBroker) Decide(_ context.Context, requestID string, optionID string) (PermissionSnapshot, error) {
	requestID = strings.TrimSpace(requestID)
	optionID = strings.TrimSpace(optionID)
	if requestID == "" || optionID == "" {
		return PermissionSnapshot{}, ErrPermissionInvalidOption
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now().UTC()
	b.pruneCompletedLocked(now)

	if snapshot, ok := b.completedSnapshotLocked(requestID, now); ok {
		if snapshot.Status == PermissionStatusExpired || snapshot.Status == PermissionStatusCanceled {
			return snapshot, ErrPermissionGone
		}
		return snapshot, ErrPermissionAlreadyDecided
	}
	pending := b.pending[requestID]
	if pending == nil {
		return PermissionSnapshot{}, ErrPermissionNotFound
	}
	if now.After(pending.state.snapshot.ExpiresAt) {
		state := b.finishLocked(requestID, PermissionStatusExpired, nil)
		return state.snapshot, ErrPermissionGone
	}

	option, ok := findPermissionOption(pending.state.snapshot.Options, optionID)
	if !ok {
		return pending.state.snapshot, ErrPermissionInvalidOption
	}
	status := PermissionStatusRejected
	if permissionOptionAllows(option.Kind) {
		status = PermissionStatusAllowed
	}
	decision := &PermissionDecisionSnapshot{
		OptionID:  option.ID,
		Kind:      option.Kind,
		DecidedAt: time.Now().UTC(),
	}
	return b.finishLocked(requestID, status, decision).snapshot, nil
}

func (b *MemoryPermissionBroker) Get(requestID string) (PermissionSnapshot, bool) {
	requestID = strings.TrimSpace(requestID)
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now().UTC()
	b.pruneCompletedLocked(now)
	if pending := b.pending[requestID]; pending != nil {
		return pending.state.snapshot, true
	}
	return b.completedSnapshotLocked(requestID, now)
}

func (b *MemoryPermissionBroker) CancelSession(runtimeID string, sessionID string) {
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
		b.finishLocked(id, PermissionStatusCanceled, nil)
	}
	b.mu.Unlock()
}

func (b *MemoryPermissionBroker) nextRequestID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	return fmt.Sprintf("%s-%d", b.idPrefix, b.nextID)
}

func (b *MemoryPermissionBroker) finish(requestID string, status PermissionStatus, decision *PermissionDecisionSnapshot) permissionState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.finishLocked(requestID, status, decision)
}

func (b *MemoryPermissionBroker) finishLocked(requestID string, status PermissionStatus, decision *PermissionDecisionSnapshot) permissionState {
	now := time.Now().UTC()
	b.pruneCompletedLocked(now)
	pending := b.pending[requestID]
	if pending == nil {
		if snapshot, ok := b.completedSnapshotLocked(requestID, now); ok {
			return permissionState{snapshot: snapshot}
		}
		return permissionState{snapshot: PermissionSnapshot{ID: requestID, Status: status, Decision: decision}}
	}

	state := pending.state
	snapshot := state.snapshot
	snapshot.Status = status
	snapshot.Decision = decision
	state.snapshot = snapshot
	delete(b.pending, requestID)
	b.completed[requestID] = completedPermission{
		state:     state,
		expiresAt: now.Add(b.cacheTTL),
	}
	pending.done <- state
	close(pending.done)
	return state
}

func (b *MemoryPermissionBroker) completedSnapshotLocked(requestID string, now time.Time) (PermissionSnapshot, bool) {
	completed, ok := b.completed[requestID]
	if !ok {
		return PermissionSnapshot{}, false
	}
	if !completed.expiresAt.IsZero() && !now.Before(completed.expiresAt) {
		delete(b.completed, requestID)
		return PermissionSnapshot{}, false
	}
	return completed.state.snapshot, true
}

func (b *MemoryPermissionBroker) pruneCompletedLocked(now time.Time) {
	for id, completed := range b.completed {
		if !completed.expiresAt.IsZero() && !now.Before(completed.expiresAt) {
			delete(b.completed, id)
		}
	}
}

func (b *MemoryPermissionBroker) publish(event SessionEvent) {
	if b != nil && b.eventSink != nil {
		b.eventSink.Publish(event)
	}
}

const (
	PermissionOptionKindAllowOnce   = "allow_once"
	PermissionOptionKindAllowAlways = "allow_always"
)

type ExternalPermissionOption struct {
	ID    string
	Kind  string
	Label string
}

func NormalizePermissionOptions(options []ExternalPermissionOption) []PermissionOptionSnapshot {
	out := make([]PermissionOptionSnapshot, 0, len(options))
	for _, option := range options {
		kind := strings.TrimSpace(option.Kind)
		out = append(out, PermissionOptionSnapshot{
			ID:    strings.TrimSpace(option.ID),
			Kind:  kind,
			Label: firstPermissionText(option.Label, kind, option.ID),
			Scope: permissionOptionScope(kind),
		})
	}
	return normalizedPermissionOptions(out)
}

func normalizedPermissionOptions(options []PermissionOptionSnapshot) []PermissionOptionSnapshot {
	out := make([]PermissionOptionSnapshot, 0, len(options))
	for _, option := range options {
		id := strings.TrimSpace(option.ID)
		if id == "" {
			continue
		}
		kind := strings.TrimSpace(option.Kind)
		scope := strings.TrimSpace(option.Scope)
		if scope == "" {
			scope = permissionOptionScope(kind)
		}
		out = append(out, PermissionOptionSnapshot{
			ID:    id,
			Kind:  kind,
			Label: firstPermissionText(option.Label, kind, id),
			Scope: scope,
		})
	}
	return out
}

func normalizedExecutionRef(ref activity.ExecutionRef) activity.ExecutionRef {
	return activity.ExecutionRef{
		RuntimeKind: strings.TrimSpace(ref.RuntimeKind),
		RuntimeID:   strings.TrimSpace(ref.RuntimeID),
		SessionID:   strings.TrimSpace(ref.SessionID),
		ToolCallID:  strings.TrimSpace(ref.ToolCallID),
		ToolKind:    strings.TrimSpace(ref.ToolKind),
	}
}

func findPermissionOption(options []PermissionOptionSnapshot, optionID string) (PermissionOptionSnapshot, bool) {
	for _, option := range options {
		if option.ID == optionID {
			return option, true
		}
	}
	return PermissionOptionSnapshot{}, false
}

func permissionOptionAllows(kind string) bool {
	switch strings.TrimSpace(kind) {
	case PermissionOptionKindAllowOnce, PermissionOptionKindAllowAlways:
		return true
	default:
		return false
	}
}

func permissionOptionScope(kind string) string {
	if strings.TrimSpace(kind) == PermissionOptionKindAllowAlways {
		return activity.ActionOptionScopeAgent
	}
	return ""
}

func firstPermissionText(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
