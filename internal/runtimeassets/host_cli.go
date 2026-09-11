package runtimeassets

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// HostCLIPath binds host agents to the server's companion CLI, not an unrelated
// installation on PATH. Return the expected path even if the bundle is incomplete:
// a missing companion must fail visibly instead of silently running an old CLI.
func HostCLIPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return hostCLIPath(exe, runtime.GOOS)
}

func hostCLIPath(exe, goos string) string {
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	name := "csgclaw-cli"
	if strings.EqualFold(goos, "windows") {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(exe), name)
}
