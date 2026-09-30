package apps

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// A recovery belongs to one binding and is canceled by every explicit lifecycle
// change. It restores sessions and tool discovery, never replays tool calls.
type connectionRecovery struct {
	ctx    context.Context
	cancel context.CancelFunc
	delay  time.Duration
}

func (s *Service) stopRecoveryLocked(e *entry) {
	if e.recovery != nil {
		e.recovery.cancel()
		e.recovery = nil
	}
}

func (s *Service) startRecoveryLocked(e *entry) {
	if s.ctx.Err() != nil || e.recovery != nil || e.connection != nil || e.pendingCancel != nil ||
		e.record.AgentID == "" || e.record.Config.Transport != "http" ||
		!e.record.active() || e.record.Disconnected || !e.record.ConnectRequested || e.record.Status != "error" {
		return
	}
	// A server that immediately drops every new session must not reset the
	// backoff merely by completing initialization successfully.
	if e.recoveryDelay == 0 || (!e.connectedAt.IsZero() && time.Since(e.connectedAt) >= time.Minute) {
		e.recoveryDelay = time.Second
	} else {
		e.recoveryDelay = min(2*e.recoveryDelay, 30*time.Second)
	}
	ctx, cancel := context.WithCancel(s.ctx)
	recovery := &connectionRecovery{ctx: ctx, cancel: cancel, delay: e.recoveryDelay}
	e.recovery = recovery
	go s.recoverConnection(e.record.AgentID, e.record.InstallationID, recovery)
}

func (s *Service) recoverConnection(agentID, id string, recovery *connectionRecovery) {
	defer recovery.cancel()
	defer func() {
		s.mu.Lock()
		if e, err := s.findLocked(agentID, id); err == nil && e.recovery == recovery {
			e.recovery = nil
		}
		s.mu.Unlock()
	}()
	delay := recovery.delay
	for attempt := 1; ; attempt++ {
		timer := time.NewTimer(delay)
		select {
		case <-recovery.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		item, err := s.connectWithRecovery(recovery.ctx, agentID, id, false, recovery)
		if err == nil && item.Status == "connected" {
			slog.Info("Agent App MCP connection recovered", "agent_id", agentID, "installation_id", id, "attempt", attempt)
		}
		if err == nil || recovery.ctx.Err() != nil || item.Status == "authorization_required" ||
			errors.Is(err, errAuthentication) || errors.Is(err, ErrInvalid) || errors.Is(err, ErrNotFound) ||
			errors.Is(err, ErrUnsupportedOAuth) || errors.Is(err, ErrChanged) {
			return
		}
		delay = min(2*delay, 30*time.Second)
		s.mu.Lock()
		if e, findErr := s.findLocked(agentID, id); findErr == nil && e.recovery == recovery {
			e.recoveryDelay = delay
		}
		s.mu.Unlock()
	}
}
