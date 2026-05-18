package notifier

import (
	"strings"

	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/utils"
)

// ConfigFromRuntimeOptions parses Config from a runtime_options map (e.g. create payload before agent exists).
func ConfigFromRuntimeOptions(runtimeOptions map[string]any) Config {
	return ConfigFromStored(NotifierFlatFromRuntimeOptionsMap(runtimeOptions))
}

// RedactRuntimeOptionsForAPI returns a shallow copy of runtime_options with known secret-bearing subtrees redacted.
func RedactRuntimeOptionsForAPI(runtimeOptions map[string]any) map[string]any {
	if len(runtimeOptions) == 0 {
		return nil
	}
	out := utils.CloneAnyMap(runtimeOptions)
	if out == nil {
		return nil
	}
	delete(out, RuntimeOptionKeyNotifierProfile)
	// Flat notifier keys at map root (canonical storage).
	if len(copyNotifierKeysFromMap(out)) > 0 {
		redRoot := RedactDetailsForAPI(copyNotifierKeysFromMap(out))
		for _, k := range NotifierStorageKeys {
			delete(out, k)
		}
		for k, v := range redRoot {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// MatchesNotifierRuntimeKind reports whether kind is the in-server notifier worker runtime.
func MatchesNotifierRuntimeKind(kind string) bool {
	return strings.EqualFold(strings.TrimSpace(kind), agentruntime.KindNotifier)
}

// StripProfileLLMFieldsForRuntime returns baseURL and modelID unchanged except for the in-server notifier worker runtime,
// which does not use LLM gateway fields; those must not be persisted on notifier agents.
func StripProfileLLMFieldsForRuntime(runtimeKind, baseURL, modelID string) (string, string) {
	if MatchesNotifierRuntimeKind(runtimeKind) {
		return "", ""
	}
	return baseURL, modelID
}

// MergeFlatRuntimeOptionsForProfilePatch merges patch profile runtime_options onto a base options map (no agent-level storage yet).
func MergeFlatRuntimeOptionsForProfilePatch(baseRuntimeOptions, patchRuntimeOptions map[string]any) map[string]any {
	base := NotifierFlatFromRuntimeOptionsMap(baseRuntimeOptions)
	incoming := NotifierFlatFromRuntimeOptionsMap(patchRuntimeOptions)
	return MergeNotifierFlatPatch(base, incoming)
}

// ProfileDeliveryComplete reports whether notifier delivery is sufficiently configured from flat runtime storage.
func ProfileDeliveryComplete(flat map[string]any) bool {
	if len(flat) == 0 {
		return false
	}
	c := ParseNotifierDetails(flat)
	return c.AllowsWebhook() || c.AllowsPull()
}

// ProfileViewSummary is view-only API state derived from stored notifier configuration (never a source of truth on disk).
type ProfileViewSummary struct {
	DeliveryComplete bool `json:"delivery_complete,omitempty"`
	WebhookTokenSet  bool `json:"webhook_token_set,omitempty"`
	RemoteTokenSet   bool `json:"remote_token_set,omitempty"`
}

// ProfileViewSummaryForAPI returns nil when no notifier configuration is present in the given runtime_options map.
func ProfileViewSummaryForAPI(runtimeOptions map[string]any) *ProfileViewSummary {
	return ProfileViewSummaryForAgentStorage(NotifierFlatFromRuntimeOptionsMap(runtimeOptions))
}

// ProfileViewSummaryForAgentStorage builds a summary from agent-level notifier flat only.
func ProfileViewSummaryForAgentStorage(agentFlat map[string]any) *ProfileViewSummary {
	if len(agentFlat) == 0 {
		return nil
	}
	cfg := ConfigFromStored(agentFlat)
	if strings.TrimSpace(cfg.DeliveryMode) == "" && strings.TrimSpace(cfg.WebhookToken) == "" && strings.TrimSpace(cfg.RemoteURL) == "" {
		return nil
	}
	return &ProfileViewSummary{
		DeliveryComplete: ProfileDeliveryComplete(agentFlat),
		WebhookTokenSet:  strings.TrimSpace(cfg.WebhookToken) != "",
		RemoteTokenSet:   strings.TrimSpace(cfg.RemoteToken) != "",
	}
}

func profileViewSummaryToMap(s *ProfileViewSummary) map[string]any {
	if s == nil {
		return nil
	}
	m := make(map[string]any)
	if s.DeliveryComplete {
		m["delivery_complete"] = true
	}
	if s.WebhookTokenSet {
		m["webhook_token_set"] = true
	}
	if s.RemoteTokenSet {
		m["remote_token_set"] = true
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// ViewRuntimeOptionsForAPI returns runtime_options safe for JSON: redacted notifier subtree plus view-only notifier_profile summary.
// Options are treated as agent-level runtime_options (the only source of truth for notifier delivery).
func ViewRuntimeOptionsForAPI(agentRuntimeOptions map[string]any) map[string]any {
	return ViewRuntimeOptionsForAPIUnified(agentRuntimeOptions, nil)
}

// ViewRuntimeOptionsForAPIUnified merges agent-level and profile-level runtime_options before redacting and summarizing.
// Agent runtime_options is the only source of truth for delivery config.
func ViewRuntimeOptionsForAPIUnified(agentRuntimeOptions, profileRuntimeOptions map[string]any) map[string]any {
	profileRuntimeOptions = StripNotifierKeysForProfileRuntimeOptions(profileRuntimeOptions)
	merged := MergeRuntimeOptionMapsForView(agentRuntimeOptions, profileRuntimeOptions)
	base := RedactRuntimeOptionsForAPI(merged)
	agentFlat := NotifierFlatFromAgentRuntimeOptions(agentRuntimeOptions)
	summaryMap := profileViewSummaryToMap(ProfileViewSummaryForAgentStorage(agentFlat))
	if summaryMap == nil {
		if len(base) == 0 {
			return nil
		}
		return base
	}
	out := utils.CloneAnyMap(base)
	if out == nil {
		out = make(map[string]any)
	}
	out[RuntimeOptionKeyNotifierProfile] = summaryMap
	return out
}
