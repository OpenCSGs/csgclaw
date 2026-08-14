//go:build windows

package upgrade

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

const waitTimeout = 0x00000102

func processRunning(pid int) (running bool, stale bool, err error) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		switch {
		case errors.Is(err, windows.ERROR_INVALID_PARAMETER):
			return false, true, nil
		case errors.Is(err, windows.ERROR_ACCESS_DENIED):
			return true, false, nil
		default:
			return false, false, err
		}
	}
	defer windows.CloseHandle(handle)

	result, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return false, false, err
	}
	switch result {
	case windows.WAIT_OBJECT_0:
		return false, true, nil
	case waitTimeout:
		return true, false, nil
	default:
		return false, false, fmt.Errorf("unexpected wait result %#x", result)
	}
}
