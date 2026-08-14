//go:build !windows

package upgrade

import (
	"errors"
	"os"
	"syscall"
)

func processRunning(pid int) (running bool, stale bool, err error) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, false, err
	}
	err = proc.Signal(syscall.Signal(0))
	switch {
	case err == nil:
		return true, false, nil
	case errors.Is(err, syscall.ESRCH), errors.Is(err, os.ErrProcessDone):
		return false, true, nil
	case errors.Is(err, syscall.EPERM):
		return true, false, nil
	default:
		return false, false, err
	}
}
