//go:build !windows

package serve

import (
	"os"
	"syscall"
)

func stopServerProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}
