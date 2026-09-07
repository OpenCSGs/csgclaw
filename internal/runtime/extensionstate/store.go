// Package extensionstate owns named Runtime projection generations.
// Active manifests switch atomically; initialization never writes active files.
package extensionstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"csgclaw/internal/runtime"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

func ValidName(name string) bool { return namePattern.MatchString(name) }

type Store struct{ root string }

func New(root string) (*Store, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) == string(filepath.Separator) {
		return nil, fmt.Errorf("private extension root is required")
	}
	return &Store{root: filepath.Clean(root)}, nil
}

func (s *Store) scope(name string) (string, error) {
	if s == nil || !ValidName(name) {
		return "", fmt.Errorf("invalid extension name")
	}
	dir := filepath.Join(s.root, name)
	for _, path := range []string{s.root, dir} {
		if info, err := os.Lstat(path); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return "", fmt.Errorf("extension root must be a private directory")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return dir, nil
}

func (s *Store) Load(name string) (runtime.ExtensionProjection, bool, error) {
	dir, err := s.scope(name)
	if err != nil {
		return runtime.ExtensionProjection{}, false, err
	}
	file, err := os.Open(filepath.Join(dir, "active.json"))
	if errors.Is(err, os.ErrNotExist) {
		return runtime.ExtensionProjection{}, false, nil
	}
	if err != nil {
		return runtime.ExtensionProjection{}, false, err
	}
	defer file.Close()
	var item runtime.ExtensionProjection
	if err := json.NewDecoder(io.LimitReader(file, 1<<20)).Decode(&item); err != nil {
		return item, false, err
	}
	if item.Name != name || !generationName(item.Root) || item.Digest == "" || item.Digest != Digest(item) {
		return item, false, fmt.Errorf("invalid active extension manifest")
	}
	return item, true, nil
}

func (s *Store) List() ([]runtime.ExtensionProjection, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []runtime.ExtensionProjection
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !entry.IsDir() || !ValidName(entry.Name()) {
			return nil, fmt.Errorf("invalid extension directory")
		}
		item, found, err := s.Load(entry.Name())
		if err != nil {
			return nil, err
		}
		if found {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) Directory(item runtime.ExtensionProjection) (string, error) {
	dir, err := s.scope(item.Name)
	if err != nil {
		return "", err
	}
	if !generationName(item.Root) {
		return "", fmt.Errorf("invalid extension generation")
	}
	path := filepath.Join(dir, item.Root)
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("extension generation must not be a symlink")
	}
	return path, nil
}

func generationName(name string) bool {
	return strings.HasPrefix(name, "generation-") && filepath.Base(name) == name && filepath.IsLocal(name)
}

// Stage allocates an unreferenced generation. Its eventual path is stable during
// preparation, including commands that persist absolute paths inside that root.
func (s *Store) Stage(name string) (*Change, error) {
	dir, err := s.scope(name)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	previous, found, err := s.Load(name)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp(dir, "generation-")
	if err != nil {
		return nil, err
	}
	change := &Change{store: s, name: name, directory: root, ownsDirectory: true}
	if found {
		change.previous = &previous
	}
	change.current = runtime.ExtensionProjection{Name: name, Root: filepath.Base(root)}
	return change, nil
}

// Revise records a new desired generation whose effective projection is unchanged.
func (s *Store) Revise(item runtime.ExtensionProjection) (*Change, error) {
	previous, found, err := s.Load(item.Name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, os.ErrNotExist
	}
	dir, err := s.Directory(item)
	if err != nil {
		return nil, err
	}
	return &Change{store: s, name: item.Name, directory: dir, previous: &previous, current: item}, nil
}

type Change struct {
	store         *Store
	name          string
	directory     string
	ownsDirectory bool
	previous      *runtime.ExtensionProjection
	current       runtime.ExtensionProjection
	active        bool
	deleting      bool
	trash         string
}

func (c *Change) Directory() string { return c.directory }
func (c *Change) Projection() runtime.ExtensionProjection {
	item := c.current
	item.Environment = maps.Clone(item.Environment)
	return item
}

func (c *Change) SetProjection(item runtime.ExtensionProjection) {
	item.Name = c.name
	item.Root = filepath.Base(c.directory)
	item.Environment = maps.Clone(item.Environment)
	item.Digest = Digest(item)
	c.current = item
}

// Digest identifies the effective configuration, not an Apply counter. A retry
// may acknowledge a new generation already loaded with identical contributions.
func Digest(item runtime.ExtensionProjection) string {
	item.Generation = 0
	item.Digest = ""
	data, _ := json.Marshal(item)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (c *Change) Activate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.active {
		return nil
	}
	dir, err := c.store.scope(c.name)
	if err != nil {
		return err
	}
	if c.deleting {
		if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
			c.active = true
			return nil
		} else if err != nil {
			return err
		}
		trash, err := os.MkdirTemp(c.store.root, tombstonePrefix(c.name))
		if err != nil {
			return err
		}
		if err := os.Remove(trash); err != nil {
			return err
		}
		if err := os.Rename(dir, trash); err != nil {
			return err
		}
		c.trash = trash
	} else {
		if c.current.Name != c.name || !generationName(c.current.Root) {
			return fmt.Errorf("invalid staged projection")
		}
		if err := writeManifest(filepath.Join(dir, "active.json"), c.current); err != nil {
			return err
		}
	}
	c.active = true
	return nil
}

func (c *Change) Rollback(_ context.Context) error {
	if !c.active {
		return nil
	}
	dir, err := c.store.scope(c.name)
	if err != nil {
		return err
	}
	if c.deleting {
		if c.trash != "" {
			if err := os.Rename(c.trash, dir); err != nil {
				return err
			}
			c.trash = ""
		}
	} else if c.previous != nil {
		if err := writeManifest(filepath.Join(dir, "active.json"), *c.previous); err != nil {
			return err
		}
	} else if err := os.Remove(filepath.Join(dir, "active.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	c.active = false
	return nil
}

func (c *Change) Cleanup(_ context.Context) error {
	if c.deleting {
		if c.trash != "" {
			return os.RemoveAll(c.trash)
		}
		if c.active {
			return c.store.cleanTombstones(c.name)
		}
		return nil
	}
	if !c.active && c.ownsDirectory {
		return os.RemoveAll(c.directory)
	}
	if c.active {
		return c.store.prune(c.current)
	}
	return nil
}

func (s *Store) Delete(name string) (*Change, error) {
	if _, err := s.scope(name); err != nil {
		return nil, err
	}
	change := &Change{store: s, name: name, deleting: true, current: runtime.ExtensionProjection{Name: name}}
	return change, nil
}

func (s *Store) prune(active runtime.ExtensionProjection) error {
	dir, err := s.scope(active.Name)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if generationName(entry.Name()) && entry.Name() != active.Root {
			if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return s.cleanTombstones(active.Name)
}

func (s *Store) cleanTombstones(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid extension name")
	}
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), tombstonePrefix(name)) {
			if err := os.RemoveAll(filepath.Join(s.root, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func tombstonePrefix(name string) string { return ".deleted-" + hex.EncodeToString([]byte(name)) + "-" }

func writeManifest(path string, item runtime.ExtensionProjection) error {
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".manifest-")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
