//go:build !windows

package localstore

import (
	"io/fs"
	"os"
)

func createBackupSymlink(_ fs.FileInfo, linkTarget, path string) error {
	return os.Symlink(linkTarget, path)
}
