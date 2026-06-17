package api

import (
	"errors"
	"os/exec"
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
