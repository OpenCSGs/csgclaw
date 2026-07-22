package api

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	errDirectoryPickerUnsupported = errors.New("directory picker is not supported on this host")
	errDirectorySelectionCanceled = errors.New("directory selection canceled")
)

type directoryPickerCommandRunner func(context.Context, string, ...string) ([]byte, error)

var runDirectoryPickerCommand directoryPickerCommandRunner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func selectLocalDirectory(ctx context.Context) (string, error) {
	pickerCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	switch runtime.GOOS {
	case "darwin":
		return pickDirectoryDarwin(pickerCtx)
	case "linux":
		return pickDirectoryLinux(pickerCtx)
	case "windows":
		return pickDirectoryWindows(pickerCtx)
	default:
		return "", errDirectoryPickerUnsupported
	}
}

func pickDirectoryDarwin(ctx context.Context) (string, error) {
	out, err := runDirectoryPickerCommand(ctx, "osascript", "-e", `POSIX path of (choose folder with prompt "Select a directory for CSGClaw")`)
	if err != nil {
		if directoryPickerCanceled(err) {
			return "", errDirectorySelectionCanceled
		}
		return "", fmt.Errorf("run osascript: %w", err)
	}
	return normalizePickedDirectoryPath(string(out))
}

func pickDirectoryLinux(ctx context.Context) (string, error) {
	commands := []struct {
		name string
		args []string
	}{
		{name: "zenity", args: []string{"--file-selection", "--directory", "--title=Select a directory for CSGClaw"}},
		{name: "kdialog", args: []string{"--getexistingdirectory", "", "--title", "Select a directory for CSGClaw"}},
		{name: "yad", args: []string{"--file-selection", "--directory", "--title=Select a directory for CSGClaw"}},
	}
	var unsupported bool
	for _, command := range commands {
		out, err := runDirectoryPickerCommand(ctx, command.name, command.args...)
		if err == nil {
			return normalizePickedDirectoryPath(string(out))
		}
		if directoryPickerCanceled(err) {
			return "", errDirectorySelectionCanceled
		}
		if errors.Is(err, exec.ErrNotFound) {
			unsupported = true
			continue
		}
		return "", fmt.Errorf("run %s: %w", command.name, err)
	}
	if unsupported {
		return "", errDirectoryPickerUnsupported
	}
	return "", errDirectoryPickerUnsupported
}

func pickDirectoryWindows(ctx context.Context) (string, error) {
	script := strings.Join([]string{
		`Add-Type -AssemblyName System.Windows.Forms`,
		`$dialog = New-Object System.Windows.Forms.FolderBrowserDialog`,
		`$dialog.Description = 'Select a directory for CSGClaw'`,
		`if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {`,
		`  [Console]::Out.Write($dialog.SelectedPath)`,
		`} else {`,
		`  exit 1`,
		`}`,
	}, "\n")
	out, err := runDirectoryPickerCommand(ctx, "powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	if err != nil {
		if directoryPickerCanceled(err) {
			return "", errDirectorySelectionCanceled
		}
		return "", fmt.Errorf("run powershell: %w", err)
	}
	return normalizePickedDirectoryPath(string(out))
}

func directoryPickerCanceled(err error) bool {
	if err == nil {
		return false
	}
	if exitErr := new(exec.ExitError); errors.As(err, &exitErr) {
		message := strings.ToLower(strings.TrimSpace(string(exitErr.Stderr)))
		if message == "" {
			return true
		}
		return strings.Contains(message, "user canceled") ||
			strings.Contains(message, "user cancelled") ||
			strings.Contains(message, "user canceled.") ||
			strings.Contains(message, "user cancelled.") ||
			strings.Contains(message, "(-128)") ||
			strings.Contains(message, "error number -128") ||
			strings.Contains(message, "cancelled") ||
			strings.Contains(message, "canceled")
	}
	return false
}

func normalizePickedDirectoryPath(raw string) (string, error) {
	path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "file://"))
	if path == "" {
		return "", errDirectorySelectionCanceled
	}
	return filepath.Clean(path), nil
}
