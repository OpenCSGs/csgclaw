package notifier

import (
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

// ProfileViewSummary is view-only API state derived from stored notifier configuration (never a source of truth on disk).
type ProfileViewSummary struct {
	DeliveryComplete bool `json:"delivery_complete,omitempty"`
	WebhookTokenSet  bool `json:"webhook_token_set,omitempty"`
	RemoteTokenSet   bool `json:"remote_token_set,omitempty"`
}

// ProfileViewSummaryForAPI returns nil when no notifier configuration is present (profile runtime_extensions only).
func ProfileViewSummaryForAPI(extensions map[string]any) *ProfileViewSummary {
	return ProfileViewSummaryForAgentStorage(NotifierFlatFromRuntimeExtensionsMap(extensions))
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

// requestOptionsForAgentProfileView returns request_options safe for JSON (secrets redacted).
// When agent-level notifier flat is present, nested request_options["notifier"] is omitted from the view
// to avoid duplicating what is summarized under runtime_extensions.notifier_profile.
func requestOptionsForAgentProfileView(agentExt map[string]any, requestOptions map[string]any) map[string]any {
	flat := NotifierFlatFromAgentStorage(agentExt)
	ro := agentruntime.RedactedRequestOptionsForAPIView(requestOptions)
	if len(flat) > 0 && ro != nil {
		delete(ro, "notifier")
		if len(ro) == 0 {
			ro = nil
		}
	}
	return ro
}

// RequestOptionsForAgentProfileView returns request_options safe for JSON (secrets redacted).
// When agent-level notifier flat is present, nested request_options["notifier"] is omitted from the view
// to avoid duplicating what is summarized under runtime_extensions.notifier_profile.
func RequestOptionsForAgentProfileView(agentExt map[string]any, requestOptions map[string]any) map[string]any {
	return requestOptionsForAgentProfileView(agentExt, requestOptions)
}

// ViewRuntimeExtensionsForAPI returns runtime_extensions safe for JSON: redacted notifier subtree plus view-only notifier_profile summary.
// Extensions are treated as agent-level runtime_extensions (the only source of truth for notifier delivery).
func ViewRuntimeExtensionsForAPI(extensions map[string]any) map[string]any {
	return ViewRuntimeExtensionsForAPIUnified(extensions, nil)
}

// ViewRuntimeExtensionsForAPIUnified merges agent-level and profile-level runtime_extensions before redacting and summarizing.
// Profile-level notifier payload is not merged (agent runtime_extensions is the only source of truth for delivery config).
func ViewRuntimeExtensionsForAPIUnified(agentExt, profileExt map[string]any) map[string]any {
	profileExt = ProfileRuntimeExtensionsWithoutNotifierPayload(profileExt)
	merged := MergeRuntimeExtensionMapsForView(agentExt, profileExt)
	base := RedactRuntimeExtensionsForAPI(merged)
	agentFlat := NotifierFlatFromAgentStorage(agentExt)
	summaryMap := profileViewSummaryToMap(ProfileViewSummaryForAgentStorage(agentFlat))
	if summaryMap == nil {
		if len(base) == 0 {
			return nil
		}
		return base
	}
	out := CloneAnyMap(base)
	if out == nil {
		out = make(map[string]any)
	}
	out[RuntimeExtensionKeyNotifierProfile] = summaryMap
	return out
}
