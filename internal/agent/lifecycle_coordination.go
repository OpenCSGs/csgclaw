package agent

import (
	"context"
	"fmt"
	"strings"
)

type agentLifecycleGate struct {
	token chan struct{}
}

type agentLifecycleLease struct {
	service *Service
	agentID string
	parent  *agentLifecycleLease
}

type agentLifecycleLeaseContextKey struct{}

func newAgentLifecycleGate() *agentLifecycleGate {
	gate := &agentLifecycleGate{token: make(chan struct{}, 1)}
	gate.token <- struct{}{}
	return gate
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

	s.agentLifecycleMu.Lock()
	if s.agentLifecycleGates == nil {
		s.agentLifecycleGates = make(map[string]*agentLifecycleGate)
	}
	gate := s.agentLifecycleGates[agentID]
	if gate == nil {
		gate = newAgentLifecycleGate()
		s.agentLifecycleGates[agentID] = gate
	}
	s.agentLifecycleMu.Unlock()

	select {
	case <-ctx.Done():
		return ctx, nil, ctx.Err()
	case <-gate.token:
	}

	parent, _ := ctx.Value(agentLifecycleLeaseContextKey{}).(*agentLifecycleLease)
	lease := &agentLifecycleLease{service: s, agentID: agentID, parent: parent}
	return context.WithValue(ctx, agentLifecycleLeaseContextKey{}, lease), func() {
		gate.token <- struct{}{}
	}, nil
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
