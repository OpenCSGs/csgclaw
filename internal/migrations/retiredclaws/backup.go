package retiredclaws

import (
	"path/filepath"
	"strings"
)

// GeneratedBackupEntry excludes only unreadable old container-generated plugin
// copies. Backup errors for user workspaces remain fatal.
func GeneratedBackupEntry(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	return len(parts) >= 4 && parts[0] == "agents" && parts[2] == ".openclaw" && parts[3] == "plugin-skills"
}
