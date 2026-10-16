package larkcli

import (
	agentruntime "csgclaw/internal/runtime"
	"path/filepath"
	"strings"
)

func IsConfigurationEnv(key string) bool {
	key = agentruntime.CanonicalEnvironmentKey(key)
	if strings.HasPrefix(key, "LARK_") || strings.HasPrefix(key, "LARKSUITE_") {
		return true
	}
	switch key {
	case "OPENCLAW_CLI", "OPENCLAW_HOME", "OPENCLAW_STATE_DIR", "OPENCLAW_CONFIG_PATH", "OPENCLAW_SERVICE_MARKER", "OPENCLAW_SERVICE_VERSION", "OPENCLAW_GATEWAY_PORT", "OPENCLAW_SHELL", "HERMES_HOME", "HERMES_QUIET", "HERMES_EXEC_ASK", "HERMES_GATEWAY_TOKEN", "HERMES_SESSION_KEY":
		return true
	}
	return false
}

func DisabledConfigDir(home string) string {
	return filepath.Join(home, "connectors", "feishu-disabled")
}
