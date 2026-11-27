// Package skillpath identifies installed Agent skills by their root-relative directory.
package skillpath

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

func Valid(id string) bool {
	return id != "." && fs.ValidPath(id) && filepath.IsLocal(filepath.FromSlash(id)) && !strings.ContainsAny(id, "\\\x00\r\n\t")
}

// IDs returns installed skills without reading their contents. A manifest ends
// discovery at that directory, including when its metadata cannot be read.
func IDs(ctx context.Context, root *os.Root, includeInvalid bool) ([]string, error) {
	ids := []string{}
	err := fs.WalkDir(root.FS(), ".", func(id string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() || id == "." {
			return nil
		}
		if !Valid(id) {
			return fs.SkipDir
		}
		info, err := root.Lstat(filepath.FromSlash(path.Join(id, "SKILL.md")))
		if errors.Is(err, fs.ErrNotExist) {
			// Hidden directories without a manifest contain runtime or staging data.
			if strings.HasPrefix(entry.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if includeInvalid || (err == nil && info.Mode().IsRegular()) {
			ids = append(ids, id)
		}
		return fs.SkipDir
	})
	slices.Sort(ids)
	return ids, err
}

// CheckReplacement protects installed descendants when a flat install name
// matches their group directory. Call it while holding the installation lock.
func CheckReplacement(dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.IsDir()) {
		return nil
	}
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err := root.Lstat("SKILL.md"); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	ids, err := IDs(context.Background(), root, true)
	if err != nil {
		return err
	}
	if len(ids) != 0 {
		return fmt.Errorf("%w: %s contains grouped skills", fs.ErrExist, filepath.Base(dir))
	}
	return nil
}

func Installed(root string) ([]string, error) {
	dir, err := os.OpenRoot(root)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return IDs(context.Background(), dir, false)
}
