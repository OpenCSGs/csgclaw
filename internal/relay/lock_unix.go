//go:build !windows

package relay

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func acquireStateLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("relay executor state is already in use")
	}
	return f, nil
}
