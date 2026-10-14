//go:build !windows

package filebrowse

import (
	"os"
	"syscall"
)

func openNonblocking(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
