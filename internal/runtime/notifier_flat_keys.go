package runtime

// NotifierFlatStorageKeys are flat keys used for notifier delivery on runtime_options (must stay aligned with notifier package).
var NotifierFlatStorageKeys = []string{
	"delivery_mode",
	"webhook_token",
	"remote_url",
	"remote_messages_url",
	"remote_ack_url",
	"remote_subscription_id",
	"poll_interval",
	"remote_token",
}

// ExtensionsHaveNotifierFlatKeys reports whether m contains any notifier flat storage key at the map root.
func ExtensionsHaveNotifierFlatKeys(m map[string]any) bool {
	if len(m) == 0 {
		return false
	}
	for _, k := range NotifierFlatStorageKeys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}
