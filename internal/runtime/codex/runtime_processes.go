package codex

import (
	"bytes"
	"path/filepath"
	"strings"
)

func splitNullTerminated(data []byte) []string {
	parts := bytes.Split(data, []byte{0})
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 0 {
			values = append(values, string(part))
		}
	}
	return values
}

func isCodexAppServerCommand(args []string) bool {
	if len(args) < 2 {
		return false
	}
	binary := filepath.Base(strings.TrimSpace(args[0]))
	if binary != "codex" && !strings.HasPrefix(binary, "codex-") {
		return false
	}
	return strings.TrimSpace(args[1]) == "app-server"
}

func pathWithinRuntimeDir(runtimeDir, candidate string) bool {
	runtimeDir = strings.TrimSpace(runtimeDir)
	candidate = strings.TrimSpace(candidate)
	if runtimeDir == "" || candidate == "" {
		return false
	}
	runtimeDir, err := filepath.Abs(runtimeDir)
	if err != nil {
		return false
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(runtimeDir), filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
