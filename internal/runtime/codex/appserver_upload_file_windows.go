//go:build windows

package codex

import "os"

func openAppServerUploadFile(root *os.Root, name string) (*os.File, error) {
	return root.Open(name)
}
