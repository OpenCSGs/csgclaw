package api

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

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
