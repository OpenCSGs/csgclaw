package sandboxproviders

import (
	"fmt"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"csgclaw/internal/config"
	"csgclaw/internal/sandbox/boxlitecli"
)

// Availability checks whether the configured sandbox provider CLI is present.
// It does not contact daemons, start sandboxes, or pull images.
func Availability(cfg config.SandboxConfig) error {
	cfg = cfg.Resolved()
	switch strings.TrimSpace(cfg.Provider) {
	case config.BoxLiteProvider:
		if goruntime.GOOS == "windows" {
			return fmt.Errorf("sandbox provider %q is not supported on Windows; switch [sandbox].provider to %q", config.BoxLiteProvider, config.DockerProvider)
		}
		return ensureBoxLiteAvailable(boxlitecli.ResolvePath(""))
	case config.DockerProvider:
		return ensureDockerAvailable(cfg.EffectiveDockerCLIPath())
	default:
		if _, ok := factories[cfg.Provider]; !ok {
			return fmt.Errorf("unsupported sandbox provider %q; supported values are %s", cfg.Provider, SupportedProvidersText())
		}
		return nil
	}
}

func ensureDockerAvailable(resolvedPath string) error {
	resolvedPath = strings.TrimSpace(resolvedPath)
	if resolvedPath == "" {
		return fmt.Errorf("sandbox provider %q is configured, but the docker executable path is empty", config.DockerProvider)
	}
	if executablePathExists(resolvedPath) {
		return nil
	}
	return fmt.Errorf("sandbox provider %q is configured, but %q is not available on PATH or at the configured path", config.DockerProvider, resolvedPath)
}

func executablePathExists(resolvedPath string) bool {
	if filepath.Base(resolvedPath) != resolvedPath {
		if info, err := statPath(resolvedPath); err == nil && !info.IsDir() {
			return true
		}
		return false
	}
	_, err := lookPath(resolvedPath)
	return err == nil
}
