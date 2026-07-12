package codexcli

import (
	"fmt"
	"path/filepath"
	"strings"
)

func isWindowsCommandShimPath(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".cmd", ".bat":
		return true
	default:
		return false
	}
}

// windowsBatchAppServerCommandLine builds the exact command line consumed by
// cmd.exe. App-server arguments are fixed, so only the validated shim path is
// interpolated into the shell command.
func windowsBatchAppServerCommandLine(binaryPath string) (string, error) {
	binaryPath = strings.TrimSpace(binaryPath)
	if binaryPath == "" {
		return "", fmt.Errorf("Codex command shim path is required")
	}
	if strings.ContainsAny(binaryPath, "\x00\r\n\"") || strings.Contains(binaryPath, "%") {
		return "", fmt.Errorf("Codex command shim path %q contains characters that cannot be passed safely to cmd.exe", binaryPath)
	}
	return `/d /s /v:off /c ""` + binaryPath + `" ` + strings.Join(AppServerArgs(), " ") + `"`, nil
}
