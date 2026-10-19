//go:build !windows

package installstate

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func lockRecordsFile(file *os.File) error {
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX)
		if !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}
