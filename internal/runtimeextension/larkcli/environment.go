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
	case "HERMES_HOME", "HERMES_QUIET", "HERMES_EXEC_ASK", "HERMES_GATEWAY_TOKEN", "HERMES_SESSION_KEY":
		return true
	}
	return false
}

func DisabledConfigDir(home string) string {
	return filepath.Join(home, "connectors", "feishu-disabled")
}
