package dshcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

const (
	DefaultRegistry   = "https://registry.npmmirror.com"
	DefaultVersion    = "0.1.5-rc.2"
	DefaultPackage    = "@deepseek-ai/dsh@" + DefaultVersion
	InstallRootEnv    = "CSGCLAW_DSH_INSTALL_ROOT"
	InstallPackageEnv = "CSGCLAW_DSH_PACKAGE"

	InstallStageChecking    = "checking_environment"
	InstallStageInstalling  = "installing_packages"
	InstallStageLinking     = "creating_launcher"
	InstallStageVerifying   = "verifying_installation"
	InstallStageConfiguring = "configuring_path"
	InstallStageCompleted   = "completed"
)

var (
	ErrNodeNotFound       = errors.New("Node.js is required to install DSH")
	ErrNodeUnsupported    = errors.New("Node.js version is unsupported by DSH")
	ErrNPMNotFound        = errors.New("npm is required to install DSH")
	ErrInstallFailed      = errors.New("DSH installation failed")
	ErrVerificationFailed = errors.New("DSH installation verification failed")
)

type InstallProgress struct {
	Stage         string
	ActivityCount int
}

type ProgressReporter func(InstallProgress)

// Installer installs DSH into the current user's home directory. It deliberately
// avoids npm's system-wide prefix so installation never requires sudo.
type Installer struct {
	LookPath    func(string) (string, error)
	Run         CommandRunner
	UserHomeDir func() (string, error)
	GOOS        string
}

func (i Installer) Install(ctx context.Context) (Info, error) {
	return i.InstallWithProgress(ctx, nil)
}

func (i Installer) InstallWithProgress(ctx context.Context, report ProgressReporter) (Info, error) {
	reportProgress(report, InstallStageChecking, 0)
	lookPath := i.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	nodePath, err := lookPath("node")
	if err != nil {
		return Info{}, fmt.Errorf("%w: install Node.js 24 LTS first", ErrNodeNotFound)
	}
	nodeVersionOutput, err := i.run(ctx, nodePath, "--version")
	if err != nil {
		return Info{}, fmt.Errorf("%w: run node --version: %v%s", ErrNodeUnsupported, err, commandOutputSuffix(nodeVersionOutput))
	}
	if !supportedNodeVersion(string(nodeVersionOutput)) {
		return Info{}, fmt.Errorf("%w: found %s; install Node.js 24 LTS", ErrNodeUnsupported, strings.TrimSpace(string(nodeVersionOutput)))
	}
	npmPath, err := lookPath("npm")
	if err != nil {
		return Info{}, fmt.Errorf("%w: install Node.js and npm first", ErrNPMNotFound)
	}

	homeDir := i.UserHomeDir
	if homeDir == nil {
		homeDir = os.UserHomeDir
	}
	home, err := homeDir()
	if err != nil {
		return Info{}, fmt.Errorf("%w: resolve user home directory: %v", ErrInstallFailed, err)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return Info{}, fmt.Errorf("%w: user home directory is empty", ErrInstallFailed)
	}
	installRoot := strings.TrimSpace(os.Getenv(InstallRootEnv))
	if installRoot == "" {
		installRoot = filepath.Join(home, ".local", "share", "deepseek-harness")
	}
	packageName := strings.TrimSpace(os.Getenv(InstallPackageEnv))
	if packageName == "" {
		packageName = DefaultPackage
	}
	registry := strings.TrimSpace(os.Getenv("NPM_CONFIG_REGISTRY"))
	if registry == "" {
		registry = DefaultRegistry
	}

	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		return Info{}, fmt.Errorf("%w: prepare install directory: %v", ErrInstallFailed, err)
	}
	reportProgress(report, InstallStageInstalling, 0)
	out, err := i.runNPM(
		ctx,
		npmPath,
		report,
		"install",
		"-g",
		"--prefix="+installRoot,
		"--registry="+registry,
		"--loglevel=http",
		"--no-progress",
		"--no-audit",
		"--no-fund",
		"--prefer-offline",
		packageName,
	)
	if err != nil {
		return Info{}, fmt.Errorf("%w: npm: %v%s", ErrInstallFailed, err, commandOutputSuffix(out))
	}

	goos := strings.TrimSpace(i.GOOS)
	if goos == "" {
		goos = runtime.GOOS
	}
	actualPath := filepath.Join(installRoot, "bin", BinaryName)
	launcherPath := filepath.Join(home, ".local", "bin", BinaryName)
	if goos == "windows" {
		actualPath = filepath.Join(installRoot, "dsh.cmd")
		launcherPath = filepath.Join(home, ".local", "bin", "dsh.cmd")
	}
	reportProgress(report, InstallStageLinking, 0)
	if _, err := validateExecutable(actualPath); err != nil {
		return Info{}, fmt.Errorf("%w: %v", ErrVerificationFailed, err)
	}
	if err := createLauncher(actualPath, launcherPath, goos); err != nil {
		return Info{}, err
	}
	reportProgress(report, InstallStageVerifying, 0)
	versionOut, err := i.run(ctx, actualPath, "--version")
	if err != nil {
		return Info{}, fmt.Errorf("%w: run dsh --version: %v%s", ErrVerificationFailed, err, commandOutputSuffix(versionOut))
	}
	version, _ := ParseVersion(string(versionOut))
	reportProgress(report, InstallStageConfiguring, 0)
	// The managed launcher is resolved directly by CSGClaw, so a read-only shell
	// profile must not turn a verified installation into a failure.
	_ = persistLauncherPath(home, goos)
	reportProgress(report, InstallStageCompleted, 0)
	return Info{Path: launcherPath, Version: version}, nil
}

func supportedNodeVersion(raw string) bool {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(raw), "v"), ".")
	if len(parts) < 2 {
		return false
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil {
		return false
	}
	return major > 24 || major == 24 || (major == 22 && minor >= 19)
}

func reportProgress(report ProgressReporter, stage string, activityCount int) {
	if report != nil {
		report(InstallProgress{Stage: stage, ActivityCount: activityCount})
	}
}

func persistLauncherPath(home, goos string) error {
	if goos == "windows" {
		// CSGClaw resolves the managed Windows launcher directly. Avoid invoking
		// setx here because it can truncate an existing user PATH.
		return nil
	}
	profileName := ".profile"
	switch filepath.Base(strings.TrimSpace(os.Getenv("SHELL"))) {
	case "zsh":
		profileName = ".zprofile"
	case "bash":
		profileName = ".bash_profile"
	}
	profilePath := filepath.Join(home, profileName)
	const pathLine = `export PATH="$HOME/.local/bin:$PATH"`
	contents, err := os.ReadFile(profilePath)
	if err == nil && strings.Contains(string(contents), pathLine) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: read shell profile: %v", ErrInstallFailed, err)
	}
	prefix := ""
	if len(contents) > 0 && contents[len(contents)-1] != '\n' {
		prefix = "\n"
	}
	entry := prefix + "# Added by CSGClaw for DeepSeek Harness\n" + pathLine + "\n"
	file, err := os.OpenFile(profilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("%w: update shell profile: %v", ErrInstallFailed, err)
	}
	defer file.Close()
	if _, err := file.WriteString(entry); err != nil {
		return fmt.Errorf("%w: update shell profile: %v", ErrInstallFailed, err)
	}
	return nil
}

func (i Installer) run(ctx context.Context, path string, args ...string) ([]byte, error) {
	if i.Run != nil {
		return i.Run(ctx, path, args...)
	}
	return exec.CommandContext(ctx, path, args...).CombinedOutput()
}

func (i Installer) runNPM(
	ctx context.Context,
	path string,
	report ProgressReporter,
	args ...string,
) ([]byte, error) {
	if i.Run != nil {
		return i.Run(ctx, path, args...)
	}
	collector := &installOutputCollector{report: report}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = collector
	cmd.Stderr = collector
	err := cmd.Run()
	collector.flushProgress()
	return collector.outputBytes(), err
}

type installOutputCollector struct {
	mu            sync.Mutex
	output        []byte
	pending       string
	activityCount int
	reportedCount int
	report        ProgressReporter
}

func (w *installOutputCollector) Write(data []byte) (int, error) {
	w.mu.Lock()
	w.output = append(w.output, data...)
	const outputLimit = 64 * 1024
	if len(w.output) > outputLimit {
		w.output = append([]byte(nil), w.output[len(w.output)-outputLimit:]...)
	}
	lines := strings.Split(w.pending+string(data), "\n")
	w.pending = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		if strings.Contains(line, "http fetch GET") {
			w.activityCount++
		}
	}
	activityCount := w.activityCount
	shouldReport := activityCount > 0 && (w.reportedCount == 0 || activityCount-w.reportedCount >= 10)
	if shouldReport {
		w.reportedCount = activityCount
	}
	w.mu.Unlock()
	if shouldReport {
		reportProgress(w.report, InstallStageInstalling, activityCount)
	}
	return len(data), nil
}

func (w *installOutputCollector) flushProgress() {
	w.mu.Lock()
	activityCount := w.activityCount
	shouldReport := activityCount > w.reportedCount
	w.reportedCount = activityCount
	w.mu.Unlock()
	if shouldReport {
		reportProgress(w.report, InstallStageInstalling, activityCount)
	}
}

func (w *installOutputCollector) outputBytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.output...)
}

func createLauncher(actualPath, launcherPath, goos string) error {
	if err := os.MkdirAll(filepath.Dir(launcherPath), 0o755); err != nil {
		return fmt.Errorf("%w: prepare launcher directory: %v", ErrInstallFailed, err)
	}
	if info, err := os.Lstat(launcherPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(launcherPath)
			if readErr == nil && filepath.Clean(target) == filepath.Clean(actualPath) {
				return nil
			}
		}
		return fmt.Errorf("%w: launcher already exists at %s", ErrInstallFailed, launcherPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: inspect launcher: %v", ErrInstallFailed, err)
	}

	if goos == "windows" {
		contents := []byte("@echo off\r\n\"" + actualPath + "\" %*\r\n")
		if err := os.WriteFile(launcherPath, contents, 0o755); err != nil {
			return fmt.Errorf("%w: create launcher: %v", ErrInstallFailed, err)
		}
		return nil
	}
	if err := os.Symlink(actualPath, launcherPath); err != nil {
		return fmt.Errorf("%w: create launcher: %v", ErrInstallFailed, err)
	}
	return nil
}

func commandOutputSuffix(output []byte) string {
	value := strings.TrimSpace(string(output))
	if value == "" {
		return ""
	}
	const limit = 4096
	if len(value) > limit {
		value = value[len(value)-limit:]
	}
	return ": " + value
}
