package notification

import (
	"strings"

	"csgclaw/internal/utils"
)

// MetadataKeyNotificationProfile is the metadata key for derived API summary.
const MetadataKeyNotificationProfile = "notification_profile"

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

// ProfileViewSummaryForMetadata returns nil when no configuration is present.
func ProfileViewSummaryForMetadata(metadata map[string]any) *ProfileViewSummary {
	flat := FlatFromMetadataMap(metadata)
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

// RedactMetadataForAPI returns metadata safe for JSON responses.
func RedactMetadataForAPI(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	out := utils.CloneAnyMap(metadata)
	if out == nil {
		return nil
	}
	delete(out, MetadataKeyNotificationProfile)
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

// ViewMetadataForAPI returns redacted metadata plus notification_profile summary.
func ViewMetadataForAPI(metadata map[string]any) map[string]any {
	base := RedactMetadataForAPI(metadata)
	summaryMap := profileViewSummaryToMap(ProfileViewSummaryForMetadata(metadata))
	out := utils.CloneAnyMap(base)
	if out == nil {
		out = make(map[string]any)
	}
	if summaryMap != nil {
		out[MetadataKeyNotificationProfile] = summaryMap
	}
	flat := FlatFromMetadataMap(metadata)
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

// MergeMetadataPatch merges patch metadata onto stored options.
func MergeMetadataPatch(baseMetadata, patchMetadata map[string]any) map[string]any {
	base := utils.CloneAnyMap(baseMetadata)
	incoming := StripViewOnlyMetadataKeys(patchMetadata)
	return MergeFlatPatchKeys(base, incoming)
}
