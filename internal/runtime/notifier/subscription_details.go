package notifier

import (
	"strings"
)

// EnsurePullRemoteSubscriptionInNotifierDetails sets remote_subscription_id on a flat notifier map
// when delivery_mode is remote_pull and the id is empty. Mutates nd in place.
func EnsurePullRemoteSubscriptionInNotifierDetails(nd map[string]any) map[string]any {
	if len(nd) == 0 {
		return nd
	}
	cfg := ParseNotifierDetails(nd)
	if cfg.normalizedDeliveryMode() != DeliveryRemotePull {
		return nd
	}
	if strings.TrimSpace(cfg.RemoteSubscriptionID) != "" {
		return nd
	}
	nd["remote_subscription_id"] = newPullSubscriptionID()
	return nd
}
