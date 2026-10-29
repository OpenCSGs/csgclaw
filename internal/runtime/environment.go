package runtime

// IsManagedAgentEnvironmentKey identifies connection settings projected by the
// Controller. They must not be inherited from the server process or shadowed by
// differently cased profile keys on Windows.
func IsManagedAgentEnvironmentKey(key string) bool {
	switch CanonicalEnvironmentKey(key) {
	case "CSGCLAW_BASE_URL", "CSGCLAW_ACCESS_TOKEN", "CSGCLAW_CONNECTOR_CAPABILITY", "CSGCLAW_CALLER_AGENT_ID", "CSGCLAW_CLI":
		return true
	default:
		return false
	}
}
