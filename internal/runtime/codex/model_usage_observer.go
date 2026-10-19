package codex

import (
	"csgclaw/internal/activity"
	"csgclaw/internal/modelusage"
	"strings"
)

// ObserveModelResponseUsage aggregates each upstream HTTP response exactly once.
// Native cumulative counters must never contribute to reply consumption.
func (r *Runtime) ObserveModelResponseUsage(agentID, threadID, turnID string, usage *modelusage.Usage) {
	threadID, turnID = strings.TrimSpace(threadID), strings.TrimSpace(turnID)
	if threadID == "" || turnID == "" || len(threadID) > 256 || len(turnID) > 256 {
		return
	}
	manager, live, err := r.activeAgentSession(agentID)
	if err != nil {
		return
	}
	manager.observeModelResponseUsage(live, threadID, turnID, usage)
}

func (m *appServerManager) observeModelResponseUsage(live *liveSession, threadID, turnID string, reported *modelusage.Usage) {
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
	usage := modelusage.Usage{Partial: true}
	if reported != nil && reported.Known && reported.Valid() {
		usage = *reported
	}
	// Preserve the configured selection, including auto, rather than the upstream model.
	usage.ModelID = live.spec.Profile.ModelID
	if expected == "" {
		if waiter.pendingUsage == nil {
			waiter.pendingUsage = make(map[string]modelusage.Usage)
		}
		if _, exists := waiter.pendingUsage[turnID]; exists || len(waiter.pendingUsage) < 64 {
			pending := waiter.pendingUsage[turnID]
			pending.Add(usage)
			waiter.pendingUsage[turnID] = pending
		}
		live.mu.Unlock()
		return
	}
	if pending, ok := waiter.pendingUsage[turnID]; ok {
		waiter.tokenUsage.Add(pending)
	}
	waiter.pendingUsage = nil
	waiter.tokenUsage.Add(usage)
	snapshot := waiter.tokenUsage
	live.mu.Unlock()
	m.publishAppServerEvent(SessionEvent{RuntimeID: live.spec.RuntimeID, SessionID: threadID, TurnID: turnID, Kind: activity.RuntimeEventTokenUsage, Payload: snapshot})
}
