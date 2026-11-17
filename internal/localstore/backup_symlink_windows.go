package localstore

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func createBackupSymlink(source fs.FileInfo, linkTarget, path string) error {
	// A directory link can be dangling or precede its target in the backup.
	// Use the source link's attributes to choose its type.
	attributes := source.Sys().(*syscall.Win32FileAttributeData).FileAttributes
	var flags uint32
	if attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		flags = windows.SYMBOLIC_LINK_FLAG_DIRECTORY
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	link, err := windows.UTF16PtrFromString(backupSymlinkLongPath(absolutePath))
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(backupSymlinkLongPath(filepath.FromSlash(linkTarget)))
	if err != nil {
		return err
	}
	const allowUnprivilegedCreate = 0x2 // SYMBOLIC_LINK_FLAG_ALLOW_UNPRIVILEGED_CREATE
	err = windows.CreateSymbolicLink(link, target, flags|allowUnprivilegedCreate)
	if err != nil {
		err = windows.CreateSymbolicLink(link, target, flags)
	}
	if err != nil {
		return &os.LinkError{Op: "symlink", Old: linkTarget, New: path, Err: err}
	}
	return nil
}

func backupSymlinkLongPath(path string) string {
	if len(path) < 248 || !filepath.IsAbs(path) ||
		strings.HasPrefix(path, "\\\\?\\") || strings.HasPrefix(path, "\\\\.\\") {
		return path
	}
	if unc, ok := strings.CutPrefix(path, "\\\\"); ok {
		return "\\\\?\\UNC\\" + unc
	}
	return "\\\\?\\" + path
}
