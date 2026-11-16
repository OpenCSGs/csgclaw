//go:build !windows

package diagnostics

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Go raises SIGPIPE on a failed write to fd 1. A separate descriptor lets a
// disconnected stdout collector fail independently of the running workers.
func workerStdout() io.Writer {
	fd, err := unix.FcntlInt(os.Stdout.Fd(), unix.F_DUPFD_CLOEXEC, 3)
	if err != nil {
		return io.Discard
	}
	return os.NewFile(uintptr(fd), "worker stdout")
}
