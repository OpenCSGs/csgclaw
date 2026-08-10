package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func localDirectoryPickerAvailable() bool {
	return directoryPickerAvailable(runtime.GOOS, exec.LookPath, os.Getenv, connectDisplay)
}

type displayConnector func(network, address string) bool

func connectDisplay(network, address string) bool {
	conn, err := net.DialTimeout(network, address, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func directoryPickerAvailable(
	goos string,
	lookPath func(string) (string, error),
	getenv func(string) string,
	connect displayConnector,
) bool {
	switch goos {
	case "darwin":
		_, err := lookPath("osascript")
		return err == nil
	case "linux":
		if !linuxDisplayAvailable(getenv, connect) {
			return false
		}
		for _, name := range []string{"zenity", "kdialog", "yad"} {
			if _, err := lookPath(name); err == nil {
				return true
			}
		}
		return false
	case "windows":
		_, err := lookPath("powershell")
		return err == nil
	default:
		return false
	}
}

func linuxDisplayAvailable(getenv func(string) string, connect displayConnector) bool {
	waylandDisplay := strings.TrimSpace(getenv("WAYLAND_DISPLAY"))
	if waylandDisplay != "" {
		runtimeDir := strings.TrimSpace(getenv("XDG_RUNTIME_DIR"))
		if runtimeDir != "" && connect("unix", filepath.Join(runtimeDir, waylandDisplay)) {
			return true
		}
	}

	display := strings.TrimSpace(getenv("DISPLAY"))
	if display == "" {
		return false
	}
	host, displayNumber, ok := parseX11Display(display)
	if !ok {
		return false
	}
	if host == "" || host == "unix" {
		return connect("unix", filepath.Join("/tmp/.X11-unix", "X"+strconv.Itoa(displayNumber)))
	}
	return connect("tcp", net.JoinHostPort(host, strconv.Itoa(6000+displayNumber)))
}

func parseX11Display(display string) (string, int, bool) {
	separator := strings.LastIndex(display, ":")
	if separator < 0 || separator == len(display)-1 {
		return "", 0, false
	}
	host := strings.Trim(display[:separator], "[]")
	numberText := display[separator+1:]
	if dot := strings.Index(numberText, "."); dot >= 0 {
		numberText = numberText[:dot]
	}
	number, err := strconv.Atoi(numberText)
	if err != nil || number < 0 {
		return "", 0, false
	}
	return host, number, true
}

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
	var failures []error
	for _, command := range commands {
		out, err := runDirectoryPickerCommand(ctx, command.name, command.args...)
		if err == nil {
			return normalizePickedDirectoryPath(string(out))
		}
		if directoryPickerCanceled(err) {
			return "", errDirectorySelectionCanceled
		}
		if errors.Is(err, exec.ErrNotFound) {
			continue
		}
		failures = append(failures, fmt.Errorf("run %s: %w", command.name, err))
	}
	if len(failures) > 0 {
		return "", fmt.Errorf("%w: %v", errDirectoryPickerUnsupported, errors.Join(failures...))
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
