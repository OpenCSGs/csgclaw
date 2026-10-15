package filebrowse

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// ArchiveDirectory snapshots a bounded directory as a ZIP, preserving its name
// and relative structure. The caller must close and remove the temporary file.
func ArchiveDirectory(root, relativePath string, maxBytes int64) (_ *os.File, resultErr error) {
	clean, err := cleanFilePath(relativePath)
	if err != nil {
		return nil, err
	}
	bounded, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer bounded.Close()
	directory, err := openBoundedPath(bounded, clean, true)
	if err != nil {
		return nil, err
	}
	directory.Close()
	snapshot, err := os.CreateTemp("", "csgclaw-directory-*.zip")
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			snapshot.Close()
			os.Remove(snapshot.Name())
		}
	}()
	writer := zip.NewWriter(&archiveLimitWriter{destination: snapshot, remaining: maxBytes})
	var total int64
	entries := 0
	err = fs.WalkDir(safeDirectoryFS{root: bounded}, clean, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entries++
		if entries > 10000 {
			return fmt.Errorf("directory contains too many entries")
		}
		// Never include symlinks or special files, even if their targets are in scope.
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("directory contains a symlink")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		rel := filename[len(clean):]
		header.Name = path.Base(clean) + rel
		if info.IsDir() {
			header.Name += "/"
		} else if !info.Mode().IsRegular() {
			return fmt.Errorf("directory contains an unsupported file type")
		}
		header.Method = zip.Store
		destination, err := writer.CreateHeader(header)
		if err != nil || info.IsDir() {
			return err
		}
		source, err := openBoundedPath(bounded, filename, false)
		if err != nil {
			return err
		}
		defer source.Close()
		copied, err := io.Copy(destination, io.LimitReader(source, maxBytes-total+1))
		total += copied
		if total > maxBytes {
			return fmt.Errorf("directory exceeds attachment size limit")
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	if _, err = snapshot.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// os.Root.FS opens directories without O_NONBLOCK by default.
// Retain type checks when a directory is replaced during traversal.
type safeDirectoryFS struct {
	root *os.Root
}

func (f safeDirectoryFS) Open(name string) (fs.File, error) {
	info, err := f.Stat(name)
	if err != nil {
		return nil, err
	}
	return openBoundedPath(f.root, name, info.IsDir())
}

func (f safeDirectoryFS) Stat(name string) (fs.FileInfo, error) {
	return f.root.Lstat(filepath.FromSlash(name))
}

func (f safeDirectoryFS) ReadDir(name string) ([]fs.DirEntry, error) {
	directory, err := openBoundedPath(f.root, name, true)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}

type archiveLimitWriter struct {
	destination io.Writer
	remaining   int64
}

func (w *archiveLimitWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		return 0, fmt.Errorf("directory archive exceeds attachment size limit")
	}
	count, err := w.destination.Write(data)
	w.remaining -= int64(count)
	return count, err
}
