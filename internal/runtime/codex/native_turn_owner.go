package codex

// nativeTurnOwner finds the unique runtime that owns the Agent's active thread
// and turn. A pending start may own the thread before its native ID is known.
func (r *Runtime) nativeTurnOwner(agentID, threadID, turnID string) (*appServerManager, *liveSession) {
	manager, ok := r.currentSessionManager().(*appServerManager)
	if !ok {
		return nil, nil
	}
	agentID = canonicalRuntimeAgentID(agentID)
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	var owner *liveSession
	for _, live := range manager.sessions {
		if live.spec.AgentID != agentID {
			continue
		}
		live.mu.Lock()
		waiter := live.turnWaiters[threadID]
		matches := waiter != nil && waiter.matchesTurn(turnID)
		live.mu.Unlock()
		if !matches {
			continue
		}
		if owner != nil && owner != live {
			return nil, nil
		}
		owner = live
	}
	return manager, owner
}
