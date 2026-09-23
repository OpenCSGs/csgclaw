package codex

import (
	"encoding/json"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/modelcap"
)

func (m *appServerManager) publishContextUsage(runtimeID string, live *liveSession, threadID string, params map[string]any, method string) {
	turnID := appServerNotificationTurnID(params)
	if waiter := live.appServerTurnWaiter(threadID); waiter != nil && turnID != "" && !waiter.matchesTurn(turnID) {
		return
	}
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
