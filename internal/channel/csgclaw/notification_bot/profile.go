package notification_bot

import (
	"strings"

	"csgclaw/internal/utils"
)

// RuntimeOptionKeyNotificationProfile is the runtime_options key for derived API summary.
const RuntimeOptionKeyNotificationProfile = "notification_profile"

// ProfileViewSummary is view-only API state derived from stored configuration.
type ProfileViewSummary struct {
	DeliveryComplete bool `json:"delivery_complete,omitempty"`
	WebhookTokenSet  bool `json:"webhook_token_set,omitempty"`
	RemoteTokenSet   bool `json:"remote_token_set,omitempty"`
}

// ProfileDeliveryComplete reports whether delivery is sufficiently configured for the active mode(s).
func ProfileDeliveryComplete(flat map[string]any) bool {
	if len(flat) == 0 {
		return false
	}
	c := ConfigFromStored(flat)
	switch c.normalizedDeliveryMode() {
	case DeliveryRemotePull:
		return c.PullDeliveryComplete()
	case DeliveryBoth:
		return c.WebhookDeliveryComplete() && c.PullDeliveryComplete()
	default:
		return c.WebhookDeliveryComplete()
	}
}

// ProfileViewSummaryForRuntimeOptions returns nil when no configuration is present.
func ProfileViewSummaryForRuntimeOptions(runtimeOptions map[string]any) *ProfileViewSummary {
	flat := FlatFromRuntimeOptionsMap(runtimeOptions)
	if len(flat) == 0 {
		return nil
	}
	cfg := ConfigFromStored(flat)
	if strings.TrimSpace(cfg.DeliveryMode) == "" && strings.TrimSpace(cfg.WebhookToken) == "" && strings.TrimSpace(cfg.RemoteURL) == "" {
		return nil
	}
	return &ProfileViewSummary{
		DeliveryComplete: ProfileDeliveryComplete(flat),
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

// RedactRuntimeOptionsForAPI returns runtime_options safe for JSON responses.
func RedactRuntimeOptionsForAPI(runtimeOptions map[string]any) map[string]any {
	if len(runtimeOptions) == 0 {
		return nil
	}
	out := utils.CloneAnyMap(runtimeOptions)
	if out == nil {
		return nil
	}
	delete(out, RuntimeOptionKeyNotificationProfile)
	if len(copyStorageKeysFromMap(out)) > 0 {
		redRoot := RedactDetailsForAPI(copyStorageKeysFromMap(out))
		for _, k := range StorageKeys {
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

// ViewRuntimeOptionsForAPI returns redacted runtime_options plus notification_profile summary.
func ViewRuntimeOptionsForAPI(runtimeOptions map[string]any) map[string]any {
	base := RedactRuntimeOptionsForAPI(runtimeOptions)
	summaryMap := profileViewSummaryToMap(ProfileViewSummaryForRuntimeOptions(runtimeOptions))
	out := utils.CloneAnyMap(base)
	if out == nil {
		out = make(map[string]any)
	}
	if summaryMap != nil {
		out[RuntimeOptionKeyNotificationProfile] = summaryMap
	}
	flat := FlatFromRuntimeOptionsMap(runtimeOptions)
	remoteURL := strings.TrimSpace(ParseNotifierDetails(flat).RemoteURL)
	if remoteURL != "" {
		if msg, ack, ingress, err := ResolveRelayRoutes(remoteURL); err == nil {
			out["relay_pull_messages_url"] = msg
			out["relay_pull_ack_url"] = ack
			out["relay_webhook_ingress_url"] = ingress
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// MergeRuntimeOptionsPatch merges patch runtime_options onto stored options.
func MergeRuntimeOptionsPatch(baseRuntimeOptions, patchRuntimeOptions map[string]any) map[string]any {
	base := utils.CloneAnyMap(baseRuntimeOptions)
	incoming := StripViewOnlyRuntimeOptionKeys(patchRuntimeOptions)
	return MergeFlatPatchKeys(base, incoming)
}
