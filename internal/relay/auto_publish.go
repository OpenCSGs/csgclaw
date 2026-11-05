package relay

import (
	"context"
	"fmt"
	"time"
)

// QueuePublication durably records startup intent before any remote operation.
// Readiness and transient network/auth failures are retried by the server, not CLI.
func (m *Manager) QueuePublication(req PublishRequest, targets ...*CommunityTarget) error {
	if !safeID(req.SourceAgentID) || (req.Visibility != "public" && req.Visibility != "private") {
		return fmt.Errorf("invalid automatic publication")
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx.Err() != nil {
		return m.ctx.Err()
	}
	if m.manifest.Deleted[req.SourceAgentID] {
		return ErrLocalAgentDeleted
	}
	if m.local == nil {
		return fmt.Errorf("local agent API is not configured")
	}
	if m.manifest.Removing[req.SourceAgentID] {
		return fmt.Errorf("finish unpublishing this agent before publishing again")
	}
	oldTarget, hadTarget := m.manifest.Targets[req.SourceAgentID]
	if len(targets) > 0 && targets[0] != nil {
		if err := targets[0].validate(); err != nil {
			return err
		}
		for localID, existingTarget := range m.manifest.Targets {
			if localID != req.SourceAgentID && existingTarget == *targets[0] {
				return fmt.Errorf("platform instance is already bound to another local agent")
			}
		}
		if hadTarget && oldTarget != *targets[0] {
			return fmt.Errorf("platform instance target cannot change")
		}
		m.manifest.Targets[req.SourceAgentID] = *targets[0]
	}
	for id, published := range m.manifest.Agents {
		if published.SourceAgentID == req.SourceAgentID && !m.manifest.Pending[id] && (hadTarget || m.publicationTargetLocked(req.SourceAgentID) == nil) {
			if m.running[id] == nil {
				m.startLocked(id, published)
			}
			return nil
		}
	}
	previous, exists := m.manifest.AutoPublish[req.SourceAgentID]
	m.manifest.AutoPublish[req.SourceAgentID] = req
	if err := m.saveLocked(); err != nil {
		if hadTarget {
			m.manifest.Targets[req.SourceAgentID] = oldTarget
		} else {
			delete(m.manifest.Targets, req.SourceAgentID)
		}
		if exists {
			m.manifest.AutoPublish[req.SourceAgentID] = previous
		} else {
			delete(m.manifest.AutoPublish, req.SourceAgentID)
		}
		return err
	}
	m.startAutoPublicationLocked(req.SourceAgentID)
	return nil
}

func (m *Manager) startAutoPublicationLocked(localID string) {
	if m.autoRunning[localID] {
		return
	}
	m.autoRunning[localID] = true
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() { m.mu.Lock(); delete(m.autoRunning, localID); m.mu.Unlock() }()
		backoff := time.Second
		for m.ctx.Err() == nil {
			m.mu.Lock()
			_, desired := m.manifest.AutoPublish[localID]
			m.mu.Unlock()
			if !desired {
				return
			}
			ready := (&LocalExecutor{Client: m.local, AgentID: localID}).Ready(m.ctx)
			if ready {
				m.opMu.Lock()
				m.mu.Lock()
				req, desired := m.manifest.AutoPublish[localID]
				if !desired {
					m.mu.Unlock()
					m.opMu.Unlock()
					return
				}
				ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
				_, err := m.publishLocked(ctx, req)
				cancel()
				if err == nil {
					delete(m.manifest.AutoPublish, localID)
					if err = m.saveLocked(); err != nil {
						m.manifest.AutoPublish[localID] = req
					}
				}
				m.mu.Unlock()
				m.opMu.Unlock()
				if err == nil {
					return
				}
			}
			timer := time.NewTimer(backoff)
			select {
			case <-m.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			backoff *= 2
			if backoff > 15*time.Second {
				backoff = 15 * time.Second
			}
		}
	}()
}
