package im

import (
	"os"

	"golang.org/x/sys/windows"
)

const sessionNonblockingRead = 0

func lockSessionPublications(f *os.File, exclusive bool) error {
	var flags uint32
	if exclusive {
		flags = windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	return windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &windows.Overlapped{})
}
