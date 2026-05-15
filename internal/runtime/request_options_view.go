package runtime

// RedactNotifierDetailsForAPI returns a copy of notifier detail map with secret token fields removed.
func RedactNotifierDetailsForAPI(nd map[string]any) map[string]any {
	if len(nd) == 0 {
		return nil
	}
	out := CloneAnyMap(nd)
	delete(out, "webhook_token")
	delete(out, "remote_token")
	if len(out) == 0 {
		return nil
	}
	return out
}

// RedactedRequestOptionsForAPIView returns a copy of request_options safe for JSON responses:
// nested notifier webhook_token and remote_token are removed (secrets live on agent.runtime_options).
func RedactedRequestOptionsForAPIView(ro map[string]any) map[string]any {
	if len(ro) == 0 {
		return nil
	}
	out := CloneAnyMap(ro)
	raw, ok := out["notifier"]
	if !ok || raw == nil {
		return out
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return out
	}
	nm := RedactNotifierDetailsForAPI(m)
	if len(nm) == 0 {
		delete(out, "notifier")
	} else {
		out["notifier"] = nm
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
