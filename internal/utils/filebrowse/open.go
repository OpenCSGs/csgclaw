package filebrowse

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OpenFile opens a regular file beneath root without following symlinks.
// The returned handle owns the source independently of later path replacements.
func OpenFile(root, relativePath string) (*os.File, error) {
	path, err := cleanFilePath(relativePath)
	if err != nil {
		return nil, err
	}
	return openPath(root, path, false)
}

func openPath(root, path string, directory bool) (*os.File, error) {
	bounded, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer bounded.Close()
	return openBoundedPath(bounded, path, directory)
}

func openBoundedPath(bounded *os.Root, path string, directory bool) (*os.File, error) {
	var err error
	current := ""
	var info os.FileInfo
	for _, part := range strings.Split(filepath.FromSlash(path), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err = bounded.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("workspace path is a symlink")
		}
	}
	if path == "" {
		path = "."
		info, err = bounded.Lstat(path)
		if err != nil {
			return nil, err
		}
	}
	if (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		return nil, fmt.Errorf("workspace path has unsupported file type")
	}
	file, err := openNonblocking(bounded, filepath.FromSlash(path))
	if err != nil {
		return nil, err
	}
	info, err = file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		file.Close()
		return nil, fmt.Errorf("workspace path has unsupported file type")
	}
	return file, nil
}
