package dshcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	BinaryName       = "dsh"
	DocumentationURL = "https://github.com/deepseek-ai/deepseek-harness"
	InstallCommand   = "npm install -g @deepseek-ai/dsh@latest --registry=https://registry.npmmirror.com"
)

var versionPattern = regexp.MustCompile(`(?i)(?:^|[^0-9])v?(\d+)\.(\d+)\.(\d+)(?:-([0-9a-z.-]+))?`)

type CommandRunner func(context.Context, string, ...string) ([]byte, error)

// Provider resolves a user-installed DSH executable from PATH and common
// user/system installation locations.
type Provider struct {
	LookPath    func(string) (string, error)
	Run         CommandRunner
	UserHomeDir func() (string, error)
	GOOS        string
}

type Info struct {
	Path    string
	Version string
}

func InstallGuidance() string {
	return fmt.Sprintf("run %q, then retry", InstallCommand)
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
	lookPath := p.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	path, err := lookPath(BinaryName)
	if err == nil {
		return filepath.Clean(path), nil
	}
	for _, candidate := range p.fallbackPaths() {
		if resolved, validateErr := validateExecutable(candidate); validateErr == nil {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("DSH CLI not found; %s: %w", InstallGuidance(), err)
}

func (p Provider) fallbackPaths() []string {
	homeDir := p.UserHomeDir
	if homeDir == nil {
		homeDir = os.UserHomeDir
	}
	home, _ := homeDir()
	goos := strings.TrimSpace(p.GOOS)
	if goos == "" {
		goos = runtime.GOOS
	}
	installRoot := strings.TrimSpace(os.Getenv(InstallRootEnv))
	if installRoot == "" && home != "" {
		installRoot = filepath.Join(home, ".local", "share", "deepseek-harness")
	}
	binaryName := BinaryName
	if goos == "windows" {
		binaryName = "dsh.cmd"
	}
	paths := make([]string, 0, 6)
	if home != "" {
		paths = append(paths,
			filepath.Join(home, "bin", binaryName),
			filepath.Join(home, ".local", "bin", binaryName),
		)
	}
	if goos == "windows" {
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			paths = append(paths, filepath.Join(appData, "npm", binaryName))
		}
		if installRoot != "" {
			paths = append(paths, filepath.Join(installRoot, binaryName))
		}
		return paths
	}
	paths = append(paths, "/opt/homebrew/bin/dsh", "/usr/local/bin/dsh")
	if installRoot != "" {
		paths = append(paths, filepath.Join(installRoot, "bin", BinaryName))
	}
	return paths
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
