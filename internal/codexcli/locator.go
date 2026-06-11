package codexcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const (
	BinaryName = "codex"

	EnvBinaryPath          = "CSGCLAW_CODEX_PATH"
	EnvLegacyACPBinaryPath = "CSGCLAW_CODEX_ACP_PATH"
)

type Provider struct {
	Locator Locator
}

func (p Provider) Ensure(_ context.Context) (string, error) {
	return p.Locator.Locate()
}

type Locator struct {
	ExplicitPath string
	GOOS         string

	LookPath func(string) (string, error)
	Stat     func(string) (os.FileInfo, error)
}

func (l Locator) Locate() (string, error) {
	explicit := strings.TrimSpace(l.resolvedExplicitPath())
	if explicit != "" {
		path, ok, err := l.executablePath(explicit)
		if err != nil {
			return "", err
		}
		if ok {
			return path, nil
		}
		return "", fmt.Errorf("codex binary %s: %w", explicit, os.ErrNotExist)
	}
	if lookPath := l.lookPath(); lookPath != nil {
		name := l.binaryName()
		if path, err := lookPath(name); err == nil {
			resolved, ok, statErr := l.executablePath(path)
			if statErr != nil {
				return "", statErr
			}
			if ok {
				return resolved, nil
			}
		}
	}
	return "", fmt.Errorf("codex binary not found; install Codex CLI or set %s: %w", EnvBinaryPath, os.ErrNotExist)
}

func (l Locator) resolvedExplicitPath() string {
	if path := strings.TrimSpace(l.ExplicitPath); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv(EnvBinaryPath)); path != "" {
		return path
	}
	return strings.TrimSpace(os.Getenv(EnvLegacyACPBinaryPath))
}

func (l Locator) executablePath(path string) (string, bool, error) {
	info, err := l.stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return "", false, nil
	}
	if info.Mode()&0o111 == 0 {
		return "", false, nil
	}
	return path, true, nil
}

func (l Locator) lookPath() func(string) (string, error) {
	if l.LookPath != nil {
		return l.LookPath
	}
	return exec.LookPath
}

func (l Locator) stat(path string) (os.FileInfo, error) {
	if l.Stat != nil {
		return l.Stat(path)
	}
	return os.Stat(path)
}

func (l Locator) binaryName() string {
	if strings.TrimSpace(l.GOOS) == "windows" || (strings.TrimSpace(l.GOOS) == "" && runtime.GOOS == "windows") {
		return BinaryName + ".exe"
	}
	return BinaryName
}

func AppServerArgs() []string {
	return []string{"app-server", "--listen", "stdio://"}
}
