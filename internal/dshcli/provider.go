package dshcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	BinaryName       = "dsh"
	PathEnv          = "CSGCLAW_DSH_PATH"
	DocumentationURL = "https://github.com/deepseek-ai/deepseek-harness"
)

var versionPattern = regexp.MustCompile(`(?i)(?:^|[^0-9])v?(\d+)\.(\d+)\.(\d+)(?:-([0-9a-z.-]+))?`)

type CommandRunner func(context.Context, string, ...string) ([]byte, error)

// Provider resolves a user-installed DSH executable. ExplicitPath is used by
// per-Agent runtime options; CSGCLAW_DSH_PATH and PATH provide host defaults.
type Provider struct {
	ExplicitPath string
	LookPath     func(string) (string, error)
	Run          CommandRunner
}

type Info struct {
	Path    string
	Version string
}

func (p Provider) Ensure(ctx context.Context) (string, error) {
	info, err := p.Resolve(ctx)
	return info.Path, err
}

func (p Provider) Resolve(ctx context.Context) (Info, error) {
	path, err := p.resolvePath()
	if err != nil {
		return Info{}, err
	}
	// Version discovery is informational. Runtime compatibility is determined by
	// the ACP initialize handshake and advertised capabilities, so a newly released
	// DSH CLI is not rejected only because its version is outside a hard-coded range
	// or its --version output has changed.
	info := Info{Path: path}
	out, err := p.run(ctx, path, "--version")
	if err != nil {
		return info, nil
	}
	if version, parseErr := ParseVersion(string(out)); parseErr == nil {
		info.Version = version
	}
	return info, nil
}

func (p Provider) resolvePath() (string, error) {
	if path := strings.TrimSpace(p.ExplicitPath); path != "" {
		return validateExecutable(path)
	}
	if path := strings.TrimSpace(os.Getenv(PathEnv)); path != "" {
		return validateExecutable(path)
	}
	lookPath := p.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	path, err := lookPath(BinaryName)
	if err != nil {
		return "", fmt.Errorf("DSH CLI not found; install the current @deepseek-ai/dsh release or set %s: %w", PathEnv, err)
	}
	return filepath.Clean(path), nil
}

func validateExecutable(path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("DSH executable path %q must be absolute", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat DSH executable %q: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("DSH executable %q is a directory", path)
	}
	return path, nil
}

func (p Provider) run(ctx context.Context, path string, args ...string) ([]byte, error) {
	if p.Run != nil {
		return p.Run(ctx, path, args...)
	}
	return exec.CommandContext(ctx, path, args...).CombinedOutput()
}

func ParseVersion(output string) (string, error) {
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(output))
	if len(match) == 0 {
		return "", fmt.Errorf("unrecognized output %q", strings.TrimSpace(output))
	}
	version := strings.Join(match[1:4], ".")
	if match[4] != "" {
		version += "-" + strings.ToLower(match[4])
	}
	return version, nil
}
