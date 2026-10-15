//go:build windows

package filebrowse

import "os"

func openNonblocking(root *os.Root, name string) (*os.File, error) {
	return root.Open(name)
}
