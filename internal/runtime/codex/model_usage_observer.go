package codex

import (
	"csgclaw/internal/activity"
	"strings"
)

// ObserveModelResponseUsage records completeness only; consumption still comes
// from native cumulative counters. Never assign a response by Agent alone.
func (r *Runtime) ObserveModelResponseUsage(agentID, threadID, turnID string, reported bool) {
	if reported {
		return
	}
	threadID, turnID = strings.TrimSpace(threadID), strings.TrimSpace(turnID)
	if threadID == "" || turnID == "" || len(threadID) > 256 || len(turnID) > 256 {
		return
	}
	manager, live, err := r.activeAgentSession(agentID)
	if err != nil {
		return
	}
	manager.observeModelResponseUsage(live, threadID, turnID)
}

func (m *appServerManager) observeModelResponseUsage(live *liveSession, threadID, turnID string) {
	live.mu.Lock()
	waiter := live.turnWaiters[threadID]
	if waiter == nil {
		live.mu.Unlock()
		return
	}
	expected := waiter.currentTurnID()
	if expected != "" && expected != turnID {
		live.mu.Unlock()
		return
	}
	if waiter.missingUsageTurns == nil {
		waiter.missingUsageTurns = make(map[string]bool)
	}
	// A response can beat turn/start's RPC reply. Retain its exact identity,
	// applying it only after a start notification or reply establishes the turn.
	if len(waiter.missingUsageTurns) < 64 {
		waiter.missingUsageTurns[turnID] = true
	}
	if expected == "" {
		live.mu.Unlock()
		return
	}
	waiter.tokenUsage.Partial = true
	usage := waiter.tokenUsage
	live.mu.Unlock()
	m.publishAppServerEvent(SessionEvent{RuntimeID: live.spec.RuntimeID, SessionID: threadID, TurnID: turnID, Kind: activity.RuntimeEventTokenUsage, Payload: usage})
}
