//go:build windows

package serve

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func stopServerProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return os.ErrProcessDone
		}
		return err
	}
	return proc.Kill()
}
