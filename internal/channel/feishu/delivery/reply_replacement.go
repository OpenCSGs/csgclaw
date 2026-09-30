package delivery

import (
	"log/slog"

	channel "csgclaw/internal/channel"
)

// A failed terminal snapshot gets one independent create. In particular, never
// point the replacement at a failed streaming create or previous page. Its
// stable ID is also the transport idempotency key for bounded network retries.
// Replacement cards share a per-turn delivery lane: a retry pauses subsequent
// replacements, while a terminal failure lets the remaining pages proceed.
func (d *Dispatcher) enqueueFinalReplyReplacement(intent channel.DeliveryIntent) {
	if !intent.FinalReply || (intent.Kind != channel.DeliveryCard && intent.Kind != channel.DeliveryCardUpdate) {
		return
	}
	replacement := intent
	replacement.ID += ":replacement"
	replacement.Kind = channel.DeliveryCard
	replacement.FinalReply = false // A replacement cannot recursively replace itself.
	replacement.RelatedID = ""
	replacement.MessageID = ""
	replacement.Attempts = 0
	replacement.NextAttemptAt = nil
	replacement.LastError = ""
	if err := d.state.Enqueue(replacement); err != nil {
		slog.Error("enqueue Feishu final reply replacement failed", intentLogAttrs(intent, "error", err)...)
		return
	}
	d.Notify()
}
