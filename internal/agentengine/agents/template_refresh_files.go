package agents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	agentruntime "csgclaw/internal/runtime"
	runtimeinstructions "csgclaw/internal/runtime/instructions"
	"csgclaw/internal/skill/installstate"
	hub "csgclaw/internal/template"
)

// Stage only incoming files. Renaming the staged files avoids modifying other
// hard links to an existing file. The temporary backups last for one request.
type templateFileUpdate struct {
	root                *os.Root
	stage               string
	entries             []templateFileChange
	applied             int
	instructionsChanged bool
	skills              []templateSkillFile
}
type templateSkillFile struct {
	source, relative string
	mode             fs.FileMode
}

type templateFileChange struct {
	name, staged, backup string
}

func newTemplateFileUpdate(home string) (*templateFileUpdate, error) {
	root, err := os.OpenRoot(home)
	if err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(home, ".template-update-")
	if err != nil {
		root.Close()
		return nil, err
	}
	return &templateFileUpdate{root: root, stage: filepath.Base(stage)}, nil
}

func (f *templateFileUpdate) close() {
	_ = f.root.RemoveAll(f.stage)
	_ = f.root.Close()
}

func (f *templateFileUpdate) prepare(ctx context.Context, src hub.WorkspaceRef, layout agentruntime.Layout, managedMemory bool) (*string, error) {
	if src.Path == "" {
		return nil, nil
	}
	var instructions *string
	err := filepath.WalkDir(src.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("invalid template file %q", d.Name())
		}
		rel, err := filepath.Rel(src.Path, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode().Perm() | 0600
		if sub, ok := strings.CutPrefix(filepath.ToSlash(rel), "skills/"); ok {
			if strings.EqualFold(sub, installstate.RecordsLockName) {
				return nil
			}
			f.skills = append(f.skills, templateSkillFile{path, filepath.FromSlash(sub), mode})
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		switch {
		case path == src.MemoryPath:
			if managedMemory {
				return nil
			}
			return f.stageFile(filepath.Join(filepath.Dir(layout.SkillsRoot), "memories", "memory_summary.md"), data, 0600, false)
		case rel == "AGENTS.md":
			value := string(data)
			instructions = &value
			before := len(f.entries)
			target := layout.InstructionsPath
			if target == "" {
				target = filepath.Join(layout.WorkspaceRoot, rel)
			}
			err := f.stageFile(target, data, mode, true)
			f.instructionsChanged = len(f.entries) != before
			return err
		default:
			return f.stageFile(filepath.Join(layout.WorkspaceRoot, rel), data, mode, false)
		}
	})
	return instructions, err
}

// Inspect and publish both copies while CLI installations are excluded. Release
// the lock before projecting skills or restarting the runtime: both may read skills.
func (f *templateFileUpdate) applySkills(ctx context.Context, layout agentruntime.Layout) (bool, error) {
	if len(f.skills) == 0 {
		return false, nil
	}
	lock, err := f.destination(filepath.Join(layout.SkillsRoot, installstate.RecordsLockName))
	if err != nil {
		return false, err
	}
	if info, err := f.root.Lstat(lock); err == nil && !info.Mode().IsRegular() {
		return false, fmt.Errorf("invalid skill installation lock %q", lock)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	files, err := newTemplateFileUpdate(f.root.Name())
	if err != nil {
		return false, err
	}
	defer files.close()
	err = installstate.WithFilesLocked(layout.SkillsRoot, func() error {
		for _, skill := range f.skills {
			if err := ctx.Err(); err != nil {
				return err
			}
			data, err := os.ReadFile(skill.source)
			if err != nil {
				return err
			}
			workspace := filepath.Join(layout.WorkspaceRoot, "skills", skill.relative)
			if err := files.stageFile(workspace, data, skill.mode, false); err != nil {
				return err
			}
			target := filepath.Join(layout.SkillsRoot, skill.relative)
			if target != workspace {
				if err := files.stageFile(target, data, skill.mode, false); err != nil {
					return err
				}
			}
		}
		return files.apply()
	})
	return len(files.entries) > 0, err
}

func (f *templateFileUpdate) destination(path string) (string, error) {
	name, err := filepath.Rel(f.root.Name(), path)
	if err != nil || !filepath.IsLocal(name) {
		return "", fmt.Errorf("template destination outside worker home: %q", path)
	}
	dir := filepath.Dir(name)
	for parent := dir; parent != "."; parent = filepath.Dir(parent) {
		info, err := f.root.Lstat(parent)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("template destination %q is not a directory", parent)
		}
	}
	return name, nil
}

func (f *templateFileUpdate) stageFile(path string, data []byte, mode fs.FileMode, instructions bool) error {
	name, err := f.destination(path)
	if err != nil {
		return err
	}
	entry := templateFileChange{name: name, staged: filepath.Join(f.stage, fmt.Sprintf("new-%d", len(f.entries)))}
	info, err := f.root.Lstat(name)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return fmt.Errorf("template destination %q is not a regular file", name)
		}
		if info.Size() == int64(len(data)) || instructions {
			existing, err := f.root.ReadFile(name)
			if err != nil {
				return err
			}
			same := bytes.Equal(data, existing)
			begin, end := runtimeinstructions.AgentsInstructionsBlockMarkers()
			if instructions && !same && bytes.Contains(existing, []byte(begin)) && bytes.Contains(existing, []byte(end)) {
				same = instructionBase(string(existing)) == instructionBase(string(data))
			}
			if same && info.Mode().Perm()&0111 == mode&0111 {
				return nil
			}
		}
		entry.backup = filepath.Join(f.stage, fmt.Sprintf("old-%d", len(f.entries)))
		if err := f.root.Link(name, entry.backup); err != nil {
			return err
		}
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	if err := f.root.WriteFile(entry.staged, data, mode); err != nil {
		return err
	}
	f.entries = append(f.entries, entry)
	return nil
}

func (f *templateFileUpdate) apply() error {
	for _, entry := range f.entries {
		if err := f.root.MkdirAll(filepath.Dir(entry.name), 0755); err != nil {
			return errors.Join(err, f.rollback())
		}
		if err := f.root.Rename(entry.staged, entry.name); err != nil {
			return errors.Join(err, f.rollback())
		}
		f.applied++
	}
	return nil
}

func (f *templateFileUpdate) rollback() error {
	var result error
	for f.applied > 0 {
		f.applied--
		entry := f.entries[f.applied]
		if entry.backup == "" {
			result = errors.Join(result, f.root.Remove(entry.name))
		} else {
			result = errors.Join(result, f.root.Rename(entry.backup, entry.name))
		}
	}
	return result
}

func instructionBase(text string) string {
	begin, end := runtimeinstructions.AgentsInstructionsBlockMarkers()
	before, block, ok := strings.Cut(text, begin)
	if !ok {
		return strings.TrimSpace(text)
	}
	_, after, ok := strings.Cut(block, end)
	if !ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(before + after)
}
