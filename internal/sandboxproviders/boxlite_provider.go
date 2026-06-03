package sandboxproviders

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/boxlitecli"
)

var (
	lookPath = exec.LookPath
	statPath = os.Stat
)

func LookPathForTest(fn func(string) (string, error)) func() {
	prev := lookPath
	lookPath = fn
	return func() {
		lookPath = prev
	}
}

func StatPathForTest(fn func(string) (os.FileInfo, error)) func() {
	prev := statPath
	statPath = fn
	return func() {
		statPath = prev
	}
}

// Non-SDK sandbox providers register unconditionally so they remain available
// in every csgclaw build.
func init() {
	Register(config.BoxLiteProvider, func(cfg config.SandboxConfig) (sandbox.Provider, error) {
		resolvedPath := boxlitecli.ResolvePath("")
		if err := ensureBoxLiteAvailable(resolvedPath); err != nil {
			return nil, err
		}

		opts := []boxlitecli.ProviderOption{boxlitecli.WithPath(resolvedPath)}
		for _, registry := range cfg.EffectiveDebianRegistries() {
			opts = append(opts, boxlitecli.WithRegistry(registry))
		}
		return boxlitecli.NewProvider(opts...), nil
	})
}

func ensureBoxLiteAvailable(resolvedPath string) error {
	resolvedPath = strings.TrimSpace(resolvedPath)
	if resolvedPath == "" {
		return fmt.Errorf("sandbox provider %q is configured, but the boxlite executable path is empty", config.BoxLiteProvider)
	}

	if filepath.Base(resolvedPath) != resolvedPath {
		if _, err := statPath(resolvedPath); err == nil {
			return nil
		}
	}
	if _, err := lookPath(resolvedPath); err == nil {
		return nil
	}

	return fmt.Errorf("sandbox provider %q is configured, but no bundled boxlite binary was found and %q is not available on PATH\nSwitch [sandbox].provider to %q, or install boxlite separately if your platform later supports it.", config.BoxLiteProvider, resolvedPath, config.DockerProvider)
}
