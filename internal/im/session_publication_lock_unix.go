//go:build !windows

package im

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

const sessionNonblockingRead = unix.O_NONBLOCK

func lockSessionPublications(f *os.File, exclusive bool) error {
	mode := unix.LOCK_SH
	if exclusive {
		mode = unix.LOCK_EX
	}
	for {
		err := unix.Flock(int(f.Fd()), mode)
		if !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}
