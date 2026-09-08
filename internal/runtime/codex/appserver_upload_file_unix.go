//go:build !windows

package codex

import (
	"os"
	"syscall"
)

func openAppServerUploadFile(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
