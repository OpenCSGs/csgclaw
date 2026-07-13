package agent

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

const (
	RuntimeKindPicoClawSandbox = agentruntime.KindPicoClawSandbox
	RuntimeKindOpenClawSandbox = agentruntime.KindOpenClawSandbox
	RuntimeKindCodex           = agentruntime.KindCodex
	RuntimeNamePicoClaw        = agentruntime.NamePicoClaw
	RuntimeNameOpenClaw        = agentruntime.NameOpenClaw
	RuntimeNameCodex           = agentruntime.NameCodex
)

type RuntimeRecord struct {
	ID        string             `json:"id,omitempty"`
	Kind      string             `json:"kind"`
	State     agentruntime.State `json:"state,omitempty"`
	AgentIDs  []string           `json:"agent_ids,omitempty"`
	SandboxID string             `json:"sandbox_id,omitempty"`
	Options   map[string]any     `json:"options,omitempty"`
	CreatedAt time.Time          `json:"created_at,omitempty"`
}

func (rt RuntimeRecord) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	if strings.TrimSpace(rt.ID) != "" {
		out["id"] = rt.ID
	}
	if strings.TrimSpace(rt.Kind) != "" {
		out["kind"] = rt.Kind
	}
	if rt.State != "" {
		out["state"] = rt.State
	}
	if len(rt.AgentIDs) > 0 {
		out["agent_ids"] = rt.AgentIDs
	}
	if strings.TrimSpace(rt.SandboxID) != "" {
		out["sandbox_id"] = rt.SandboxID
	}
	if len(rt.Options) > 0 {
		out["options"] = rt.Options
	}
	if !rt.CreatedAt.IsZero() {
		out["created_at"] = rt.CreatedAt
	}
	return json.Marshal(out)
}

func normalizeRuntimeID(runtimeID, agentID string) string {
	runtimeID = strings.TrimSpace(runtimeID)
	agentID = canonicalAgentID(agentID)
	if runtimeID == "" {
		return runtimeIDForAgentID(agentID)
	}
	if strings.HasPrefix(runtimeID, "rt-u-") {
		legacyAgentID := strings.TrimPrefix(runtimeID, "rt-")
		if migrated := runtimeIDForAgentID(canonicalAgentID(legacyAgentID)); migrated != "" {
			return migrated
		}
	}
	if runtimeID == "rt-"+ManagerName {
		return runtimeIDForAgentID(ManagerUserID)
	}
	if strings.HasPrefix(runtimeID, "rt-"+AgentIDPrefix) {
		return runtimeID
	}
	if agentID != "" && strings.TrimPrefix(runtimeID, "rt-") == strings.TrimPrefix(agentID, AgentIDPrefix) {
		return runtimeIDForAgentID(agentID)
	}
	return runtimeID
}

func runtimeIDForAgentID(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	return "rt-" + agentID
}

func runtimeIDLookupAliases(runtimeID string) []string {
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		return nil
	}
	aliases := []string{runtimeID}
	switch {
	case strings.HasPrefix(runtimeID, "rt-u-"):
		if migrated := runtimeIDForAgentID(canonicalAgentID(strings.TrimPrefix(runtimeID, "rt-"))); migrated != "" {
			aliases = append(aliases, migrated)
		}
	case runtimeID == "rt-"+ManagerName:
		aliases = append(aliases, runtimeIDForAgentID(ManagerUserID))
	case strings.HasPrefix(runtimeID, "rt-") && !strings.HasPrefix(runtimeID, "rt-"+AgentIDPrefix):
		suffix := strings.TrimPrefix(runtimeID, "rt-")
		if suffix != "" {
			aliases = append(aliases, runtimeIDForAgentID(AgentIDPrefix+suffix))
		}
	}
	return aliases
}

func isGatewayRuntimeKind(kind string) bool {
	switch kind {
	case RuntimeKindPicoClawSandbox, RuntimeKindOpenClawSandbox:
		return true
	default:
		return false
	}
}

func normalizeRuntimeName(name string) string {
	return agentruntime.NormalizeRuntimeName(name)
}

func runtimeNameForKind(kind string) string {
	return agentruntime.RuntimeConfigForKind(kind).Name
}

func sandboxEnabledForKind(kind string) bool {
	return agentruntime.SandboxEnabledForKind(kind)
}

func resolveRuntimeSelection(kind, name string, sandboxEnabled bool) (string, string, bool, error) {
	cfg, err := agentruntime.RuntimeConfigFromSelection(kind, name, sandboxEnabled)
	if err != nil {
		return "", "", false, err
	}
	return cfg.LegacyKind(), cfg.Name, cfg.Sandboxed, nil
}

func runtimeKindForGatewayRuntime(runtime string) string {
	switch agentruntime.RuntimeConfigForKind(runtime).LegacyKind() {
	case RuntimeKindPicoClawSandbox, RuntimeKindOpenClawSandbox:
		return agentruntime.RuntimeConfigForKind(runtime).LegacyKind()
	default:
		return ""
	}
}

func normalizeRuntimeRecord(rt RuntimeRecord) RuntimeRecord {
	rt.ID = strings.TrimSpace(rt.ID)
	rt.Kind = strings.TrimSpace(rt.Kind)
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
	runtimeOptions := a.RuntimeOptions
	return normalizeRuntimeRecord(RuntimeRecord{
		ID:        normalizeRuntimeID(a.RuntimeID, a.ID),
		Kind:      a.RuntimeConfig().LegacyKind(),
		State:     agentruntime.State(strings.TrimSpace(a.Status)),
		AgentIDs:  []string{strings.TrimSpace(a.ID)},
		SandboxID: strings.TrimSpace(a.BoxID),
		Options:   runtimeOptions,
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
