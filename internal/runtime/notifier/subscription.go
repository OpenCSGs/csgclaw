package notifier

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
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

func newPullSubscriptionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("sub-%d", time.Now().UnixNano())
	}
	return "sub-" + hex.EncodeToString(b[:])
}
