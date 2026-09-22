package modelcap

import (
	"fmt"
)

const DefaultContextWindow = 200000
const ContextUsageKind = "context_usage"

// Metadata describes a model or an explicit override. Zero values mean automatic.
type Metadata struct {
	ContextWindow int64 `json:"context_window,omitempty"`
}

type Resolved struct {
	ContextWindow int64  `json:"context_window"`
	ContextSource string `json:"context_source"`
}

// ContextUsage is a snapshot, never a sum of request usage across a session.
type ContextUsage struct {
	SessionID        string `json:"session_id"`
	ModelID          string `json:"model_id"`
	UsedTokens       *int64 `json:"used_tokens"`
	ContextWindow    int64  `json:"context_window"`
	ContextSource    string `json:"context_source"`
	AutoCompact      bool   `json:"auto_compact"`
	CompactThreshold int64  `json:"compact_threshold"`
	Compacting       bool   `json:"compacting"`
	Estimated        bool   `json:"estimated"`
	UpdatedAt        string `json:"updated_at"`
}

func (m Metadata) Validate() error {
	if m.ContextWindow < 0 || m.ContextWindow > 1_000_000_000 {
		return fmt.Errorf("context_window must be between 1 and 1000000000, or 0 for automatic")
	}
	return nil
}

func Clone(values map[string]Metadata) map[string]Metadata {
	if values == nil {
		return nil
	}
	out := make(map[string]Metadata, len(values))
	for id, v := range values {
		out[id] = v
	}
	return out
}

func Resolve(provider, endpoint, model string, discovered, override Metadata) Resolved {
	out := Resolved{ContextWindow: DefaultContextWindow, ContextSource: "default"}
	known := catalog(provider, endpoint, model)
	for _, entry := range []struct {
		value  Metadata
		source string
	}{{known, "catalog"}, {discovered, "provider"}, {override, "user"}} {
		if entry.value.Validate() != nil {
			continue
		}
		if entry.value.ContextWindow > 0 {
			out.ContextWindow = entry.value.ContextWindow
			out.ContextSource = entry.source
		}
	}
	return out
}

func (r Resolved) Normalized() Resolved {
	if r.ContextWindow <= 0 {
		r.ContextWindow = DefaultContextWindow
		r.ContextSource = "default"
	}
	return r
}

func (r Resolved) CompactThreshold() int64 { return max(int64(1), r.Normalized().ContextWindow*3/4) }

func CloneUsage(source *ContextUsage) *ContextUsage {
	if source == nil {
		return nil
	}
	out := *source
	if source.UsedTokens != nil {
		n := *source.UsedTokens
		out.UsedTokens = &n
	}
	return &out
}
func (u ContextUsage) Valid() bool {
	return u.ContextWindow > 0 && u.ContextWindow <= 1_000_000_000 && (u.UsedTokens == nil || (*u.UsedTokens >= 0 && *u.UsedTokens < 1_000_000_000_000)) && len(u.ModelID) < 1024 && len(u.SessionID) < 1024 && len(u.UpdatedAt) < 64 && u.CompactThreshold >= 0 && u.CompactThreshold <= 1_000_000_000 && (u.ContextSource == "runtime" || u.ContextSource == "default" || u.ContextSource == "user" || u.ContextSource == "provider" || u.ContextSource == "catalog")
}
