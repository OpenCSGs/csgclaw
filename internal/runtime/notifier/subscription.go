package notifier

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// EnsurePullRemoteSubscriptionInRequestOptions sets request_options.notifier.remote_subscription_id
// when delivery_mode is remote_pull and the id is empty. Mutates nested maps in place.
func EnsurePullRemoteSubscriptionInRequestOptions(ro map[string]any) map[string]any {
	if len(ro) == 0 {
		return ro
	}
	raw, ok := ro["notifier"]
	if !ok || raw == nil {
		return ro
	}
	m, ok := raw.(map[string]any)
	if !ok || m == nil {
		return ro
	}
	EnsurePullRemoteSubscriptionInNotifierDetails(m)
	ro["notifier"] = m
	return ro
}

func newPullSubscriptionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("sub-%d", time.Now().UnixNano())
	}
	return "sub-" + hex.EncodeToString(b[:])
}
