package local

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"unicode/utf8"
)

type FileChange struct {
	Path               string `json:"path"`
	Kind               string `json:"kind"`
	Before             string `json:"before,omitempty"`
	After              string `json:"after,omitempty"`
	PreviewUnavailable bool   `json:"previewUnavailable,omitempty"`
}

// PreviewArchive compares the proposed package with the actual local copy.
// It does not install files or retain historical versions.
func PreviewArchive(root, name, filename string, archive []byte) ([]FileChange, error) {
	if _, err := NormalizeName(name); err != nil {
		return nil, err
	}
	if len(archive) > int(maxArchiveBytes) {
		return nil, ErrSkillArchiveInvalid
	}
	temp, err := os.MkdirTemp("", "csgclaw-skill-preview-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temp)
	if err := extractArchive(archive, temp); err != nil {
		return nil, err
	}
	next, extractedName, err := resolveExtractedSkillDir(temp, filename)
	if err != nil {
		return nil, err
	}
	if extractedName != name {
		return nil, fmt.Errorf("%w: archive skill name changed", ErrSkillArchiveInvalid)
	}
	previous, err := readBundle(filepath.Join(root, name))
	if err != nil {
		return nil, err
	}
	proposed, err := readBundle(next)
	if err != nil {
		return nil, err
	}
	names := make(map[string]bool)
	for name := range previous {
		names[name] = true
	}
	for name := range proposed {
		names[name] = true
	}
	changes := make([]FileChange, 0)
	var previewBytes int
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	slices.Sort(ordered)
	for _, name := range ordered {
		before, existed := previous[name]
		after, exists := proposed[name]
		if existed && exists && bytes.Equal(before, after) {
			continue
		}
		kind := "modified"
		if !existed {
			kind = "added"
		} else if !exists {
			kind = "deleted"
		}
		change := FileChange{Path: name, Kind: kind}
		if len(before)+len(after) <= 64*1024 && previewBytes+len(before)+len(after) <= 512*1024 && utf8.Valid(before) && utf8.Valid(after) && !bytes.ContainsRune(before, 0) && !bytes.ContainsRune(after, 0) {
			change.Before, change.After = string(before), string(after)
			previewBytes += len(before) + len(after)
		} else {
			change.PreviewUnavailable = true
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func readBundle(dir string) (map[string][]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	out := make(map[string][]byte)
	var total int64
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported skill file")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if total > maxArchiveBytes || len(out) >= 10000 {
			return fmt.Errorf("skill contents exceed preview limits")
		}
		data, err := root.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = data
		return nil
	})
	return out, err
}
