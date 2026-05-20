package notification_bot

import "csgclaw/internal/utils"

var viewOnlyRuntimeOptionRootKeys = []string{
	RuntimeOptionKeyNotificationProfile,
	"relay_pull_messages_url",
	"relay_pull_ack_url",
	"relay_webhook_ingress_url",
}

// StripViewOnlyRuntimeOptionKeys removes API-only keys that must never be persisted.
func StripViewOnlyRuntimeOptionKeys(ext map[string]any) map[string]any {
	if len(ext) == 0 {
		return nil
	}
	needsCopy := false
	for _, k := range viewOnlyRuntimeOptionRootKeys {
		if _, ok := ext[k]; ok {
			needsCopy = true
			break
		}
	}
	if !needsCopy {
		return ext
	}
	out := utils.CloneAnyMap(ext)
	for _, k := range viewOnlyRuntimeOptionRootKeys {
		delete(out, k)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// RedactDetailsForAPI returns a copy with secret token fields removed.
func RedactDetailsForAPI(nd map[string]any) map[string]any {
	if len(nd) == 0 {
		return nil
	}
	out := utils.CloneAnyMap(nd)
	delete(out, "webhook_token")
	delete(out, "remote_token")
	if len(out) == 0 {
		return nil
	}
	return out
}
