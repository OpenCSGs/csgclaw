package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeParentProcess struct {
	readyFilePath string
	events        *[]string
	waitError     error
}

func (process *fakeParentProcess) Wait(time.Duration) error {
	marker, err := os.ReadFile(process.readyFilePath)
	if err != nil || string(marker) != readyMarker {
		return errors.New("ready marker was not written before parent wait")
	}
	*process.events = append(*process.events, "wait-parent")
	return process.waitError
}

func (process *fakeParentProcess) Close() error {
	*process.events = append(*process.events, "close-parent")
	return nil
}

func TestRunCoordinatorWaitsThenInstallsAndRelaunches(t *testing.T) {
	options := coordinatorTestOptions(t)
	var events []string
	var log bytes.Buffer
	dependencies := coordinatorDependencies{
		openParent: func(pid uint32) (parentProcess, error) {
			events = append(events, "open-parent")
			return &fakeParentProcess{
				readyFilePath: options.readyFilePath,
				events:        &events,
			}, nil
		},
		runInstaller: func(path string, output io.Writer) (int, error) {
			events = append(events, "run-installer")
			return 0, nil
		},
		startExecutable: func(path string) error {
			events = append(events, "start-root")
			return nil
		},
		sleep: func(time.Duration) {
			events = append(events, "post-exit-delay")
		},
	}

	exitCode := runCoordinator(
		options,
		dependencies,
		&eventLogger{writer: &log, now: time.Now},
	)
	if exitCode != 0 {
		t.Fatalf("runCoordinator() exit code = %d, want 0", exitCode)
	}
	wantEvents := []string{
		"open-parent",
		"wait-parent",
		"post-exit-delay",
		"run-installer",
		"start-root",
		"close-parent",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", events, wantEvents)
	}
	for _, event := range []string{
		"parent-handle-opened",
		"coordinator-ready",
		"parent-exited",
		"installer-exited",
		"relaunch-requested",
	} {
		if !strings.Contains(log.String(), event) {
			t.Fatalf("log does not contain %q:\n%s", event, log.String())
		}
	}
}

func TestRunCoordinatorRelaunchesAfterInstallerFailure(t *testing.T) {
	options := coordinatorTestOptions(t)
	var events []string
	dependencies := coordinatorDependencies{
		openParent: func(pid uint32) (parentProcess, error) {
			return &fakeParentProcess{
				readyFilePath: options.readyFilePath,
				events:        &events,
			}, nil
		},
		runInstaller: func(path string, output io.Writer) (int, error) {
			return 23, errors.New("installer failed")
		},
		startExecutable: func(path string) error {
			events = append(events, "start-root")
			return nil
		},
		sleep: func(time.Duration) {},
	}

	exitCode := runCoordinator(
		options,
		dependencies,
		&eventLogger{writer: io.Discard, now: time.Now},
	)
	if exitCode != exitInstallerFailed {
		t.Fatalf(
			"runCoordinator() exit code = %d, want %d",
			exitCode,
			exitInstallerFailed,
		)
	}
	if !reflect.DeepEqual(events, []string{"wait-parent", "start-root", "close-parent"}) {
		t.Fatalf("events = %#v", events)
	}
}

func coordinatorTestOptions(t *testing.T) coordinatorOptions {
	t.Helper()
	directory := t.TempDir()
	installerPath := filepath.Join(directory, "Setup.exe")
	rootExecutablePath := filepath.Join(directory, "CSGClaw.exe")
	for _, filePath := range []string{installerPath, rootExecutablePath} {
		if err := os.WriteFile(filePath, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return coordinatorOptions{
		parentPID:          9040,
		installerPath:      installerPath,
		rootExecutablePath: rootExecutablePath,
		readyFilePath:      filepath.Join(directory, "coordinator.ready"),
		logFilePath:        filepath.Join(directory, "coordinator.log"),
	}
}
