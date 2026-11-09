package mcpschema

import (
	"path/filepath"
	"strings"
)

// ResolveWorkspaceArg expands the workspace placeholders supported in Codex MCP
// arguments. Cleanup uses this same rule before retiring referenced files.
func ResolveWorkspaceArg(arg, workspaceDir string) string {
	if workspaceDir == "" {
		return arg
	}
	for _, placeholder := range []string{"${workspace}", "${workspaceDir}", "{workspace}", "{workspaceDir}"} {
		if arg == placeholder {
			return workspaceDir
		}
		if strings.HasPrefix(arg, placeholder+"/") {
			return filepath.Join(workspaceDir, strings.TrimPrefix(arg, placeholder+"/"))
		}
	}
	return arg
}
