package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	readyMarker          = "coordinator-ready\n"
	parentWaitTimeout    = 60 * time.Second
	postParentExitDelay  = 750 * time.Millisecond
	exitInvalidArguments = 2
	exitLogUnavailable   = 3
	exitPreflightFailed  = 4
	exitReadyFailed      = 5
	exitParentWaitFailed = 6
	exitInstallerFailed  = 7
	exitRelaunchFailed   = 8
)

var errParentWaitTimeout = errors.New("timed out waiting for parent process")

type coordinatorOptions struct {
	parentPID          uint32
	installerPath      string
	rootExecutablePath string
	readyFilePath      string
	logFilePath        string
}

type parentProcess interface {
	Wait(timeout time.Duration) error
	Close() error
}

type relaunchResult struct {
	startedApplication   bool
	foundExistingWindow  bool
	windowDetected       bool
	windowProcessID      uint32
	windowRestored       bool
	windowBroughtToFront bool
	windowForeground     bool
	windowFlashed        bool
	windowLookupError    string
}

type coordinatorDependencies struct {
	openParent          func(pid uint32) (parentProcess, error)
	runInstaller        func(path string, output io.Writer) (int, error)
	relaunchApplication func(path string) (relaunchResult, error)
	sleep               func(duration time.Duration)
}

type eventLogger struct {
	writer io.Writer
	now    func() time.Time
}

func main() {
	os.Exit(realMain(os.Args[1:]))
}

func realMain(args []string) int {
	options, err := parseCoordinatorOptions(args)
	if err != nil {
		return exitInvalidArguments
	}
	if err := os.MkdirAll(filepath.Dir(options.logFilePath), 0o700); err != nil {
		return exitLogUnavailable
	}
	logFile, err := os.OpenFile(options.logFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return exitLogUnavailable
	}
	defer logFile.Close()

	logger := &eventLogger{writer: logFile, now: time.Now}
	logger.Event(
		"coordinator-started",
		"helper-pid",
		os.Getpid(),
		"parent-pid",
		options.parentPID,
		"installer",
		options.installerPath,
		"root-executable",
		options.rootExecutablePath,
	)
	exitCode := runCoordinator(options, platformDependencies(), logger)
	logger.Event("coordinator-finished", "code", exitCode)
	return exitCode
}

func parseCoordinatorOptions(args []string) (coordinatorOptions, error) {
	var options coordinatorOptions
	var parentPID uint64
	flags := flag.NewFlagSet("csgclaw-desktop-update-helper", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Uint64Var(&parentPID, "parent-pid", 0, "PID of the CSGClaw desktop process")
	flags.StringVar(&options.installerPath, "installer", "", "downloaded channel installer path")
	flags.StringVar(&options.rootExecutablePath, "root-executable", "", "Squirrel root executable path")
	flags.StringVar(&options.readyFilePath, "ready-file", "", "readiness marker path")
	flags.StringVar(&options.logFilePath, "log-file", "", "coordinator log path")
	if err := flags.Parse(args); err != nil {
		return coordinatorOptions{}, err
	}
	if flags.NArg() != 0 {
		return coordinatorOptions{}, fmt.Errorf("unexpected positional arguments")
	}
	if parentPID == 0 || parentPID > math.MaxUint32 {
		return coordinatorOptions{}, fmt.Errorf("invalid parent PID")
	}
	options.parentPID = uint32(parentPID)
	for name, value := range map[string]string{
		"installer":       options.installerPath,
		"root executable": options.rootExecutablePath,
		"ready file":      options.readyFilePath,
		"log file":        options.logFilePath,
	} {
		if strings.TrimSpace(value) == "" {
			return coordinatorOptions{}, fmt.Errorf("%s path is required", name)
		}
		if !filepath.IsAbs(value) {
			return coordinatorOptions{}, fmt.Errorf("%s path must be absolute", name)
		}
	}
	return options, nil
}

func runCoordinator(
	options coordinatorOptions,
	dependencies coordinatorDependencies,
	logger *eventLogger,
) int {
	for name, filePath := range map[string]string{
		"installer":       options.installerPath,
		"root executable": options.rootExecutablePath,
	} {
		if err := requireFile(filePath); err != nil {
			logger.Event("preflight-failed", "file", name, "error", err)
			return exitPreflightFailed
		}
	}

	parent, err := dependencies.openParent(options.parentPID)
	if err != nil {
		logger.Event("parent-open-failed", "error", err)
		return exitParentWaitFailed
	}
	defer parent.Close()
	logger.Event("parent-handle-opened", "parent-pid", options.parentPID)

	if err := writeReadyMarker(options.readyFilePath); err != nil {
		logger.Event("coordinator-ready-failed", "error", err)
		return exitReadyFailed
	}
	logger.Event("coordinator-ready")

	if err := parent.Wait(parentWaitTimeout); err != nil {
		if errors.Is(err, errParentWaitTimeout) {
			logger.Event("parent-wait-timeout", "timeout", parentWaitTimeout)
		} else {
			logger.Event("parent-wait-failed", "error", err)
		}
		return exitParentWaitFailed
	}
	logger.Event("parent-exited")
	dependencies.sleep(postParentExitDelay)

	logger.Event("installer-started")
	installerExitCode, installerErr := dependencies.runInstaller(
		options.installerPath,
		logger.writer,
	)
	if installerErr != nil {
		logger.Event("installer-exited", "code", installerExitCode, "error", installerErr)
	} else {
		logger.Event("installer-exited", "code", installerExitCode)
	}

	logger.Event("relaunch-started")
	relaunch, err := dependencies.relaunchApplication(options.rootExecutablePath)
	if err != nil {
		logger.Event("relaunch-failed", "error", err)
		return exitRelaunchFailed
	}
	if relaunch.startedApplication {
		logger.Event("relaunch-requested")
	}
	if relaunch.windowLookupError != "" {
		logger.Event(
			"relaunch-window-detection-failed",
			"error",
			relaunch.windowLookupError,
		)
	}
	if relaunch.foundExistingWindow {
		logger.Event(
			"relaunch-existing-window-detected",
			"process-id",
			relaunch.windowProcessID,
		)
	}
	if relaunch.windowDetected {
		logger.Event(
			"relaunch-window-activation",
			"process-id",
			relaunch.windowProcessID,
			"restored",
			relaunch.windowRestored,
			"brought-to-front",
			relaunch.windowBroughtToFront,
			"foreground",
			relaunch.windowForeground,
			"flashed",
			relaunch.windowFlashed,
		)
	} else {
		logger.Event("relaunch-window-not-detected")
	}
	if installerErr != nil || installerExitCode != 0 {
		return exitInstallerFailed
	}
	return 0
}

func requireFile(filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", filePath)
	}
	return nil
}

func writeReadyMarker(filePath string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		return err
	}
	temporaryPath := fmt.Sprintf("%s.%d.tmp", filePath, os.Getpid())
	defer os.Remove(temporaryPath)
	if err := os.WriteFile(temporaryPath, []byte(readyMarker), 0o600); err != nil {
		return err
	}
	return os.Rename(temporaryPath, filePath)
}

func (logger *eventLogger) Event(name string, fields ...any) {
	_, _ = fmt.Fprintf(
		logger.writer,
		"%s %s",
		logger.now().UTC().Format(time.RFC3339Nano),
		name,
	)
	for index := 0; index+1 < len(fields); index += 2 {
		_, _ = fmt.Fprintf(
			logger.writer,
			" %v=%q",
			fields[index],
			fmt.Sprint(fields[index+1]),
		)
	}
	_, _ = fmt.Fprintln(logger.writer)
	if syncer, ok := logger.writer.(interface{ Sync() error }); ok {
		_ = syncer.Sync()
	}
}
