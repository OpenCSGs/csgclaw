package codex

import (
	"encoding/json"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/modelusage"
)

func (m *appServerManager) publishContextUsage(runtimeID string, live *liveSession, threadID string, params map[string]any, method string) {
	turnID := appServerNotificationTurnID(params)
	if waiter := live.appServerTurnWaiter(threadID); waiter != nil && turnID != "" && !waiter.matchesTurn(turnID) {
		return
	}
	m.publishTokenUsage(runtimeID, live, threadID, turnID, params, method)
	metadata := live.spec.Profile.ModelMetadata.Normalized()
	live.mu.Lock()
	if live.contextUsage == nil {
		live.contextUsage = make(map[string]modelcap.ContextUsage)
	}
	usage, ok := live.contextUsage[threadID]
	if !ok || usage.ModelID != live.spec.Profile.ModelID {
		usage = modelcap.ContextUsage{SessionID: threadID, ModelID: live.spec.Profile.ModelID, ContextWindow: metadata.ContextWindow, ContextSource: metadata.ContextSource, AutoCompact: true, CompactThreshold: metadata.CompactThreshold(), Estimated: true}
	}
	// The model profile is the configured full window. Codex reports its
	// internal usable budget (normally 95%) as modelContextWindow; using that
	// as the denominator makes a 75% compaction threshold appear as 79%.
	// Keep the display, remaining tokens and threshold on the same full window.
	usage.ContextWindow = metadata.ContextWindow
	usage.ContextSource = metadata.ContextSource
	usage.CompactThreshold = metadata.CompactThreshold()
	switch method {
	case "item/started":
		usage.Compacting = true
	case "item/completed":
		usage.Compacting = false
		usage.UsedTokens = nil
	default:
		if raw, ok := params["tokenUsage"].(map[string]any); ok {
			if last, ok := raw["last"].(map[string]any); ok {
				if value, valid := contextTokenNumber(last["totalTokens"]); valid {
					usage.UsedTokens = &value
					usage.Estimated = false
				}
			}
		}
	}
	if method != "snapshot" {
		usage.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	live.contextUsage[threadID] = usage
	live.mu.Unlock()
	m.publishAppServerEvent(SessionEvent{RuntimeID: runtimeID, SessionID: threadID, TurnID: turnID, Kind: activity.RuntimeEventContextUsage, Payload: usage})
}

// Codex total is a thread-lifetime counter; last is one model request, not one
// user turn. Subtract the admitted turn's baseline so tools and retries count
// once, while duplicate notifications simply replace the same snapshot.
func (m *appServerManager) publishTokenUsage(runtimeID string, live *liveSession, threadID, turnID string, params map[string]any, method string) {
	live.mu.Lock()
	waiter := live.turnWaiters[threadID]
	if waiter == nil {
		live.mu.Unlock()
		return
	}
	if method != "snapshot" {
		if turnID == "" {
			live.mu.Unlock()
			return
		}
		raw, _ := params["tokenUsage"].(map[string]any)
		totalRaw, _ := raw["total"].(map[string]any)
		lastRaw, _ := raw["last"].(map[string]any)
		total := modelusage.Parse(totalRaw, live.spec.Profile.ModelID)
		last := modelusage.Parse(lastRaw, live.spec.Profile.ModelID)
		if total == nil {
			live.mu.Unlock()
			return
		}
		if waiter.tokenBaseline == nil {
			if last == nil {
				live.mu.Unlock()
				return
			}
			baseline := total.Sub(*last)
			if !baseline.Valid() {
				live.mu.Unlock()
				return
			}
			waiter.tokenBaseline = &baseline
		}
		current := total.Sub(*waiter.tokenBaseline)
		if !current.Valid() {
			live.mu.Unlock()
			return
		}
		current.Partial = current.Partial || waiter.tokenUsage.Partial
		waiter.tokenUsage = current
		if live.tokenTotals == nil {
			live.tokenTotals = make(map[string]modelusage.Usage)
		}
		live.tokenTotals[threadID] = *total
	}
	if turnID != "" && waiter.missingUsageTurns[turnID] {
		waiter.tokenUsage.Partial = true
	}
	usage := waiter.tokenUsage
	live.mu.Unlock()
	m.publishAppServerEvent(SessionEvent{RuntimeID: runtimeID, SessionID: threadID, TurnID: turnID, Kind: activity.RuntimeEventTokenUsage, Payload: usage})
}

func contextTokenNumber(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		if n >= 0 && n < 1e15 && n == float64(int64(n)) {
			return int64(n), true
		}
	case int64:
		return n, n >= 0
	case json.Number:
		i, err := n.Int64()
		return i, err == nil && i >= 0
	}
	return 0, false
}
