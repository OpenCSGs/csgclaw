package api

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestDirectoryPickerAvailable(t *testing.T) {
	found := func(available ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			if slices.Contains(available, name) {
				return "/usr/bin/" + name, nil
			}
			return "", exec.ErrNotFound
		}
	}
	env := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}
	connect := func(available ...string) displayConnector {
		return func(network, address string) bool {
			return slices.Contains(available, network+":"+address)
		}
	}

	tests := []struct {
		name      string
		goos      string
		commands  []string
		env       map[string]string
		displays  []string
		available bool
	}{
		{name: "linux x11 with zenity", goos: "linux", commands: []string{"zenity"}, env: map[string]string{"DISPLAY": ":0"}, displays: []string{"unix:/tmp/.X11-unix/X0"}, available: true},
		{name: "linux remote x11 with yad", goos: "linux", commands: []string{"yad"}, env: map[string]string{"DISPLAY": "localhost:10.0"}, displays: []string{"tcp:localhost:6010"}, available: true},
		{name: "linux protocol-qualified unix x11", goos: "linux", commands: []string{"zenity"}, env: map[string]string{"DISPLAY": "unix/:0"}, displays: []string{"unix:/tmp/.X11-unix/X0"}, available: true},
		{name: "linux protocol-qualified tcp x11", goos: "linux", commands: []string{"yad"}, env: map[string]string{"DISPLAY": "tcp/host.example:2"}, displays: []string{"tcp:host.example:6002"}, available: true},
		{name: "linux wayland with kdialog", goos: "linux", commands: []string{"kdialog"}, env: map[string]string{"WAYLAND_DISPLAY": "wayland-0", "XDG_RUNTIME_DIR": "/run/user/1000"}, displays: []string{"unix:/run/user/1000/wayland-0"}, available: true},
		{name: "linux wayland with absolute socket", goos: "linux", commands: []string{"kdialog"}, env: map[string]string{"WAYLAND_DISPLAY": "/run/user/1000/custom-wayland"}, displays: []string{"unix:/run/user/1000/custom-wayland"}, available: true},
		{name: "linux without display", goos: "linux", commands: []string{"zenity"}, available: false},
		{name: "linux with unreachable display", goos: "linux", commands: []string{"zenity"}, env: map[string]string{"DISPLAY": ":0"}, available: false},
		{name: "linux wayland without runtime dir", goos: "linux", commands: []string{"kdialog"}, env: map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, available: false},
		{name: "linux without picker command", goos: "linux", env: map[string]string{"DISPLAY": ":0"}, displays: []string{"unix:/tmp/.X11-unix/X0"}, available: false},
		{name: "darwin with osascript", goos: "darwin", commands: []string{"osascript"}, available: true},
		{name: "windows without powershell", goos: "windows", available: false},
		{name: "unsupported platform", goos: "freebsd", available: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := directoryPickerAvailable(tt.goos, found(tt.commands...), env(tt.env), connect(tt.displays...)); got != tt.available {
				t.Fatalf("directoryPickerAvailable() = %v, want %v", got, tt.available)
			}
		})
	}
}

func TestPickDirectoryLinuxContinuesAfterPickerFailure(t *testing.T) {
	originalRunner := runDirectoryPickerCommand
	t.Cleanup(func() { runDirectoryPickerCommand = originalRunner })

	var commands []string
	runDirectoryPickerCommand = func(_ context.Context, name string, _ ...string) ([]byte, error) {
		commands = append(commands, name)
		if name == "zenity" {
			return nil, errors.New("display connection failed")
		}
		if name == "kdialog" {
			return []byte("/tmp/project\n"), nil
		}
		return nil, exec.ErrNotFound
	}

	path, err := pickDirectoryLinux(context.Background())
	if err != nil {
		t.Fatalf("pickDirectoryLinux() error = %v", err)
	}
	if path != "/tmp/project" {
		t.Fatalf("pickDirectoryLinux() = %q, want /tmp/project", path)
	}
	if !slices.Equal(commands, []string{"zenity", "kdialog"}) {
		t.Fatalf("commands = %v, want zenity then kdialog", commands)
	}
}

func TestPickDirectoryLinuxReturnsUnavailableAfterAllPickersFail(t *testing.T) {
	originalRunner := runDirectoryPickerCommand
	t.Cleanup(func() { runDirectoryPickerCommand = originalRunner })

	var commands []string
	runDirectoryPickerCommand = func(_ context.Context, name string, _ ...string) ([]byte, error) {
		commands = append(commands, name)
		return nil, errors.New("display connection failed")
	}

	_, err := pickDirectoryLinux(context.Background())
	if !errors.Is(err, errDirectoryPickerUnsupported) {
		t.Fatalf("pickDirectoryLinux() error = %v, want errDirectoryPickerUnsupported", err)
	}
	if !slices.Equal(commands, []string{"zenity", "kdialog", "yad"}) {
		t.Fatalf("commands = %v, want all Linux pickers", commands)
	}
}

func directoryPickerCommandError(script string) error {
	_, err := exec.Command("sh", "-c", script).Output()
	return err
}

func TestDirectoryPickerCanceled(t *testing.T) {
	t.Run("empty stderr exit is treated as canceled", func(t *testing.T) {
		err := directoryPickerCommandError("exit 1")
		if !directoryPickerCanceled(err) {
			t.Fatal("directoryPickerCanceled() = false, want true")
		}
	})

	t.Run("apple script cancel code is treated as canceled", func(t *testing.T) {
		err := directoryPickerCommandError("printf 'execution error: User canceled. (-128)\n' >&2; exit 1")
		if !directoryPickerCanceled(err) {
			t.Fatal("directoryPickerCanceled() = false, want true for AppleScript cancel")
		}
	})

	t.Run("other stderr is not treated as canceled", func(t *testing.T) {
		err := directoryPickerCommandError("printf 'permission denied\n' >&2; exit 1")
		if directoryPickerCanceled(err) {
			t.Fatal("directoryPickerCanceled() = true, want false")
		}
	})

	t.Run("non exit errors are not treated as canceled", func(t *testing.T) {
		if directoryPickerCanceled(errors.New("boom")) {
			t.Fatal("directoryPickerCanceled() = true, want false")
		}
	})
}

func TestPickDirectoryWindowsUsesSeparatedSTACommands(t *testing.T) {
	originalRunner := runDirectoryPickerCommand
	t.Cleanup(func() {
		runDirectoryPickerCommand = originalRunner
	})

	var commandName string
	var commandArgs []string
	runDirectoryPickerCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		commandName = name
		commandArgs = slices.Clone(args)
		return []byte(`C:\workspace`), nil
	}

	path, err := pickDirectoryWindows(context.Background())
	if err != nil {
		t.Fatalf("pickDirectoryWindows() error = %v", err)
	}
	if path != `C:\workspace` {
		t.Fatalf("pickDirectoryWindows() = %q, want %q", path, `C:\workspace`)
	}
	if commandName != "powershell" {
		t.Fatalf("command name = %q, want powershell", commandName)
	}
	if !slices.Contains(commandArgs, "-STA") {
		t.Fatalf("command args = %q, want -STA", commandArgs)
	}
	if len(commandArgs) < 2 || commandArgs[len(commandArgs)-2] != "-Command" {
		t.Fatalf("command args = %q, want -Command followed by script", commandArgs)
	}
	script := commandArgs[len(commandArgs)-1]
	if !strings.Contains(script, "System.Windows.Forms\n$dialog =") {
		t.Fatalf("PowerShell statements are not separated by a newline: %q", script)
	}
}
