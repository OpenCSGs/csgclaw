package codexcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const BinaryName = "codex"

// Provider resolves the Codex CLI shipped beside the CSGClaw executable. It
// intentionally never consults environment overrides or PATH: every official
// CSGClaw bundle carries the Codex build it was tested with.
type Provider struct {
	Locator BundleLocator
}

func (p Provider) Ensure(_ context.Context) (string, error) {
	return p.Locator.Locate()
}

// BundledPath returns the Codex CLI located beside the running CSGClaw binary.
func BundledPath() (string, error) {
	return (BundleLocator{}).Locate()
}

// BundleLocator locates the private Codex runtime packaged in csgclaw/bin.
// The injectable functions make the package layout independently testable.
type BundleLocator struct {
	GOOS           string
	ExecutablePath func() (string, error)
	EvalSymlinks   func(string) (string, error)
	Stat           func(string) (os.FileInfo, error)
}

func (l BundleLocator) Locate() (string, error) {
	executable, err := l.executablePath()
	if err != nil {
		return "", fmt.Errorf("resolve CSGClaw executable for bundled Codex CLI: %w", err)
	}

	candidate := filepath.Join(filepath.Dir(l.executablePathCandidate(executable)), l.binaryName())
	if l.isBundledBinary(candidate) {
		return candidate, nil
	}
	return "", fmt.Errorf("bundled Codex CLI not found next to CSGClaw executable: %w", os.ErrNotExist)
}

func (l BundleLocator) executablePath() (string, error) {
	if l.ExecutablePath != nil {
		path, err := l.ExecutablePath()
		return strings.TrimSpace(path), err
	}
	return os.Executable()
}

func (l BundleLocator) executablePathCandidate(executable string) string {
	executable = strings.TrimSpace(executable)
	if executable == "" {
		return ""
	}
	if evaluate := l.evalSymlinks(); evaluate != nil {
		if resolved, err := evaluate(executable); err == nil {
			resolved = strings.TrimSpace(resolved)
			if resolved != "" {
				return resolved
			}
		}
	}
	return executable
}

func (l BundleLocator) isBundledBinary(path string) bool {
	info, err := l.stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if l.goos() == "windows" {
		return strings.EqualFold(filepath.Ext(path), ".exe")
	}
	return info.Mode()&0o111 != 0
}

func (l BundleLocator) binaryName() string {
	if l.goos() == "windows" {
		return BinaryName + ".exe"
	}
	return BinaryName
}

func (l BundleLocator) goos() string {
	if value := strings.TrimSpace(l.GOOS); value != "" {
		return strings.ToLower(value)
	}
	return runtime.GOOS
}

func (l BundleLocator) evalSymlinks() func(string) (string, error) {
	if l.EvalSymlinks != nil {
		return l.EvalSymlinks
	}
	return filepath.EvalSymlinks
}

func (l BundleLocator) stat(path string) (os.FileInfo, error) {
	if l.Stat != nil {
		return l.Stat(path)
	}
	return os.Stat(path)
}

func AppServerArgs() []string {
	return []string{"app-server", "--listen", "stdio://"}
}
