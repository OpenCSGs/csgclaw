package notifier

import "strings"

const workerRole = "worker"

// IsDeliveryWorker reports a worker agent that uses the in-server notifier runtime (IM delivery).
func IsDeliveryWorker(role, runtimeKind string) bool {
	return strings.EqualFold(strings.TrimSpace(role), workerRole) && MatchesNotifierRuntimeKind(runtimeKind)
}
