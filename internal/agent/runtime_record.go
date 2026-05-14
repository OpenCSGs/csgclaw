package agent

import (
	"slices"
	"strings"
	"time"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

const (
	RuntimeKindPicoClawSandbox = agentruntime.KindPicoClawSandbox
	RuntimeKindOpenClawSandbox = agentruntime.KindOpenClawSandbox
	RuntimeKindCodex           = agentruntime.KindCodex
	RuntimeKindNotifier        = agentruntime.KindNotifier
)

type RuntimeRecord struct {
	ID        string             `json:"id"`
	Kind      string             `json:"kind"`
	State     agentruntime.State `json:"state,omitempty"`
	AgentIDs  []string           `json:"agent_ids,omitempty"`
	SandboxID string             `json:"sandbox_id,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
}

func normalizeRuntimeID(runtimeID, agentID string) string {
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID != "" {
		return runtimeID
	}
	return runtimeIDForAgentID(agentID)
}

func runtimeIDForAgentID(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	return "rt-" + agentID
}

func runtimeKindForAgent(a Agent) string {
	if kind := normalizeRuntimeKind(a.RuntimeKind); kind != "" {
		return kind
	}
	switch normalizeRole(a.Role) {
	case RoleManager, RoleWorker:
		return RuntimeKindPicoClawSandbox
	default:
		return RuntimeKindOpenClawSandbox
	}
}

func isGatewayRuntimeKind(kind string) bool {
	switch normalizeRuntimeKind(kind) {
	case RuntimeKindPicoClawSandbox, RuntimeKindOpenClawSandbox:
		return true
	default:
		return false
	}
}

func runtimeKindForGatewayRuntime(runtime string) string {
	switch kind := normalizeRuntimeKind(runtime); kind {
	case RuntimeKindPicoClawSandbox, RuntimeKindOpenClawSandbox:
		return kind
	default:
		return ""
	}
}

func managerImageForRuntimeKind(kind string) string {
	switch normalizeRuntimeKind(kind) {
	case RuntimeKindOpenClawSandbox, RuntimeKindPicoClawSandbox:
		return config.DefaultManagerImageForRuntimeKind(kind)
	default:
		return ""
	}
}

func normalizeRuntimeKind(kind string) string {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case RuntimeKindPicoClawSandbox:
		return RuntimeKindPicoClawSandbox
	case RuntimeKindOpenClawSandbox:
		return RuntimeKindOpenClawSandbox
	case RuntimeKindCodex:
		return RuntimeKindCodex
	case RuntimeKindNotifier:
		return RuntimeKindNotifier
	default:
		return strings.TrimSpace(kind)
	}
}

func normalizeRuntimeRecord(rt RuntimeRecord) RuntimeRecord {
	rt.ID = strings.TrimSpace(rt.ID)
	rt.Kind = strings.TrimSpace(rt.Kind)
	if rt.Kind == "" {
		rt.Kind = RuntimeKindPicoClawSandbox
	}
	rt.State = agentruntime.State(strings.TrimSpace(string(rt.State)))
	rt.SandboxID = strings.TrimSpace(rt.SandboxID)
	rt.AgentIDs = normalizeRuntimeAgentIDs(rt.AgentIDs)
	return rt
}

func normalizeRuntimeAgentIDs(agentIDs []string) []string {
	if len(agentIDs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(agentIDs))
	out := make([]string, 0, len(agentIDs))
	for _, id := range agentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

func runtimeRecordForAgent(a Agent) RuntimeRecord {
	createdAt := a.CreatedAt.UTC()
	if a.CreatedAt.IsZero() {
		createdAt = time.Time{}
	}
	return normalizeRuntimeRecord(RuntimeRecord{
		ID:        normalizeRuntimeID(a.RuntimeID, a.ID),
		Kind:      runtimeKindForAgent(a),
		State:     agentruntime.State(strings.TrimSpace(a.Status)),
		AgentIDs:  []string{strings.TrimSpace(a.ID)},
		SandboxID: strings.TrimSpace(a.BoxID),
		CreatedAt: createdAt,
	})
}

func sortedRuntimeRecordsFromMap(items map[string]RuntimeRecord) []RuntimeRecord {
	runtimes := make([]RuntimeRecord, 0, len(items))
	for _, rt := range items {
		runtimes = append(runtimes, normalizeRuntimeRecord(rt))
	}
	slices.SortFunc(runtimes, func(a, b RuntimeRecord) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			switch {
			case a.ID < b.ID:
				return -1
			case a.ID > b.ID:
				return 1
			default:
				return 0
			}
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	})
	return runtimes
}
