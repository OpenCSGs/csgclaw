package codexcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"csgclaw/internal/config"
)

const (
	BinaryName = "codex"

	EnvBinaryPath          = "CSGCLAW_CODEX_PATH"
	EnvLegacyACPBinaryPath = "CSGCLAW_CODEX_ACP_PATH"
	ManagedBinDirName      = "bin"
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
	ManagedPath  string

	LookPath func(string) (string, error)
	Stat     func(string) (os.FileInfo, error)
}

func (l Locator) Locate() (string, error) {
	explicit := strings.TrimSpace(l.resolvedExplicitPath())
	if explicit != "" {
		if path, ok, err := l.windowsShimTarget(explicit); err != nil {
			return "", err
		} else if ok {
			return path, nil
		}
		if !l.isWindowsCommandShim(explicit) {
			path, ok, err := l.executablePath(explicit)
			if err != nil {
				return "", err
			}
			if ok {
				return path, nil
			}
			return "", fmt.Errorf("codex binary %s: %w", explicit, os.ErrNotExist)
		}
		// Batch shims cannot be passed directly to CreateProcess. Continue to a
		// native PATH or managed executable so startup installation can recover.
	}
	if lookPath := l.lookPath(); lookPath != nil {
		for _, name := range l.binaryNames() {
			path, err := lookPath(name)
			if err != nil {
				continue
			}
			resolved, ok, statErr := l.executablePath(path)
			if statErr != nil {
				return "", statErr
			}
			if ok {
				return resolved, nil
			}
		}
	}
	managedPath, err := l.resolvedManagedPath()
	if err != nil {
		return "", err
	}
	if managedPath != "" {
		path, ok, err := l.executablePath(managedPath)
		if err != nil {
			return "", err
		}
		if ok {
			return path, nil
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

func (l Locator) resolvedManagedPath() (string, error) {
	if path := strings.TrimSpace(l.ManagedPath); path != "" {
		return path, nil
	}
	return DefaultManagedPath(l.resolvedGOOS())
}

func DefaultManagedPath(goos string) (string, error) {
	dir, err := config.DefaultDomainDir(ManagedBinDirName)
	if err != nil {
		return "", err
	}
	name := BinaryName
	if strings.EqualFold(strings.TrimSpace(goos), "windows") {
		name += ".exe"
	}
	return filepath.Join(dir, name), nil
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
	if l.isWindows() {
		if !strings.EqualFold(filepath.Ext(path), ".exe") {
			return "", false, fmt.Errorf("codex binary %s is a script shim; set %s to a native codex.exe file", path, EnvBinaryPath)
		}
		return path, true, nil
	}
	if info.Mode()&0o111 == 0 {
		return "", false, nil
	}
	return path, true, nil
}

func (l Locator) windowsShimTarget(path string) (string, bool, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if !l.isWindows() || (ext != ".ps1" && ext != ".cmd" && ext != ".bat") {
		return "", false, nil
	}
	base := strings.TrimSuffix(path, filepath.Ext(path))
	resolved, ok, err := l.executablePath(base + ".exe")
	if err != nil {
		return "", false, err
	}
	if ok {
		return resolved, true, nil
	}
	return "", false, nil
}

func (l Locator) isWindowsCommandShim(path string) bool {
	if !l.isWindows() {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".cmd" || ext == ".bat"
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

func (l Locator) binaryNames() []string {
	if l.isWindows() {
		return []string{BinaryName + ".exe"}
	}
	return []string{BinaryName}
}

func (l Locator) isWindows() bool {
	return l.resolvedGOOS() == "windows"
}

func (l Locator) resolvedGOOS() string {
	if goos := strings.TrimSpace(l.GOOS); goos != "" {
		return strings.ToLower(goos)
	}
	return runtime.GOOS
}

func AppServerArgs() []string {
	return []string{"app-server", "--listen", "stdio://"}
}
