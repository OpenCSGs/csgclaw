package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	agentruntime "csgclaw/internal/runtime"
)

// agentLifecycleGate is the single coordinator for lifecycle mutation and
// execution admission for one Agent. A future admission controller can wait or
// enforce broader limits before AcquireExecution without changing this gate.
type agentLifecycleGate struct {
	token chan struct{}

	mu              sync.Mutex
	admissionClosed bool
	active          int
	drained         chan struct{}
}

type agentLifecycleLease struct {
	service *Service
	agentID string
	parent  *agentLifecycleLease
}

type agentLifecycleLeaseContextKey struct{}

// ExecutionLease pins the Agent snapshot and concrete Runtime selected before
// dispatch. Release must be called after true Runtime termination.
type ExecutionLease struct {
	Agent   Agent
	Runtime agentruntime.Runtime

	once    sync.Once
	release func()
}

// Release returns this execution lease to the shared lifecycle gate.
func (l *ExecutionLease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.release != nil {
			l.release()
		}
	})
}

func newAgentLifecycleGate() *agentLifecycleGate {
	gate := &agentLifecycleGate{token: make(chan struct{}, 1), drained: make(chan struct{})}
	close(gate.drained)
	gate.token <- struct{}{}
	return gate
}

func (s *Service) lifecycleGate(agentID string) *agentLifecycleGate {
	s.agentLifecycleMu.Lock()
	defer s.agentLifecycleMu.Unlock()
	if s.agentLifecycleGates == nil {
		s.agentLifecycleGates = make(map[string]*agentLifecycleGate)
	}
	gate := s.agentLifecycleGates[agentID]
	if gate == nil {
		gate = newAgentLifecycleGate()
		s.agentLifecycleGates[agentID] = gate
	}
	return gate
}

// AcquireExecution admits one active execution and returns a pinned Agent and
// Runtime handle. Lifecycle mutations close admission and drain all such leases
// through this same gate.
func (s *Service) AcquireExecution(ctx context.Context, agentID string) (*ExecutionLease, error) {
	if s == nil {
		return nil, fmt.Errorf("agent service is required")
	}
	agentID = canonicalAgentID(strings.TrimSpace(agentID))
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	gate := s.lifecycleGate(agentID)
	gate.mu.Lock()
	if gate.admissionClosed {
		gate.mu.Unlock()
		return nil, fmt.Errorf("agent %q lifecycle change is in progress", agentID)
	}
	selected, ok := s.Agent(agentID)
	if !ok {
		gate.mu.Unlock()
		return nil, fmt.Errorf("agent %q not found", agentID)
	}
	runtimeImpl, err := s.runtimeForKind(strings.TrimSpace(selected.RuntimeKind))
	if err != nil {
		gate.mu.Unlock()
		return nil, err
	}
	if gate.active == 0 {
		gate.drained = make(chan struct{})
	}
	gate.active++
	gate.mu.Unlock()

	return &ExecutionLease{Agent: selected, Runtime: runtimeImpl, release: func() {
		gate.mu.Lock()
		if gate.active > 0 {
			gate.active--
			if gate.active == 0 {
				close(gate.drained)
			}
		}
		gate.mu.Unlock()
	}}, nil
}

func (s *Service) acquireAgentLifecycle(ctx context.Context, agentID string) (context.Context, func(), error) {
	if s == nil {
		return ctx, nil, fmt.Errorf("agent service is required")
	}
	agentID = canonicalAgentID(strings.TrimSpace(agentID))
	if agentID == "" {
		return ctx, nil, fmt.Errorf("agent id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if holdsAgentLifecycle(ctx, s, agentID) {
		return ctx, func() {}, nil
	}

	gate := s.lifecycleGate(agentID)
	select {
	case <-ctx.Done():
		return ctx, nil, ctx.Err()
	case <-gate.token:
	}

	gate.mu.Lock()
	gate.admissionClosed = true
	drained := gate.drained
	gate.mu.Unlock()
	select {
	case <-ctx.Done():
		gate.mu.Lock()
		gate.admissionClosed = false
		gate.mu.Unlock()
		gate.token <- struct{}{}
		return ctx, nil, ctx.Err()
	case <-drained:
	}

	parent, _ := ctx.Value(agentLifecycleLeaseContextKey{}).(*agentLifecycleLease)
	lease := &agentLifecycleLease{service: s, agentID: agentID, parent: parent}
	return context.WithValue(ctx, agentLifecycleLeaseContextKey{}, lease), func() {
		gate.mu.Lock()
		gate.admissionClosed = false
		gate.mu.Unlock()
		gate.token <- struct{}{}
	}, nil
}

// WithAgentLifecycle runs one compound lifecycle mutation while admission is
// closed and active executions are drained.
func (s *Service) WithAgentLifecycle(ctx context.Context, agentID string, operation func(context.Context) error) error {
	if operation == nil {
		return fmt.Errorf("lifecycle operation is required")
	}
	ctx, release, err := s.acquireAgentLifecycle(ctx, agentID)
	if err != nil {
		return err
	}
	defer release()
	return operation(ctx)
}

func holdsAgentLifecycle(ctx context.Context, service *Service, agentID string) bool {
	if ctx == nil || service == nil {
		return false
	}
	lease, _ := ctx.Value(agentLifecycleLeaseContextKey{}).(*agentLifecycleLease)
	for lease != nil {
		if lease.service == service && lease.agentID == agentID {
			return true
		}
		lease = lease.parent
	}
	return false
}
