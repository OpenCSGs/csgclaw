package dsh

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var recreatePersistentPathPatterns = []string{
	workspaceDirName,
	runtimeFileName,
	filepath.Join(homeDirName, "agents"),
	filepath.Join(homeDirName, sessionsDirName),
	filepath.Join(homeDirName, "skills"),
	filepath.Join(homeDirName, "runtime-extensions"),
}

type preservedRecreateState struct {
	tempRoot string
	entries  []preservedRecreateEntry
}

type preservedRecreateEntry struct {
	source   string
	tempPath string
	restored bool
}

func preserveRecreateState(runtimeDir string) (*preservedRecreateState, error) {
	paths, err := recreatePersistentPaths(runtimeDir)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}
	tempRoot, err := os.MkdirTemp(filepath.Dir(runtimeDir), ".csgclaw-dsh-state-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary DSH state preservation directory: %w", err)
	}
	preserved := &preservedRecreateState{tempRoot: tempRoot}
	for _, relativePath := range paths {
		entry := preservedRecreateEntry{
			source:   filepath.Join(runtimeDir, relativePath),
			tempPath: filepath.Join(tempRoot, relativePath),
		}
		if err := os.MkdirAll(filepath.Dir(entry.tempPath), 0o755); err != nil {
			return nil, preserved.rollback(fmt.Errorf("prepare DSH state preservation for %s: %w", relativePath, err))
		}
		if err := os.Rename(entry.source, entry.tempPath); err != nil {
			return nil, preserved.rollback(fmt.Errorf("preserve DSH state %s: %w", relativePath, err))
		}
		preserved.entries = append(preserved.entries, entry)
	}
	return preserved, nil
}

func recreatePersistentPaths(runtimeDir string) ([]string, error) {
	seen := make(map[string]struct{})
	for _, pattern := range recreatePersistentPathPatterns {
		matches, err := filepath.Glob(filepath.Join(runtimeDir, pattern))
		if err != nil {
			return nil, fmt.Errorf("resolve DSH recreate state pattern %q: %w", pattern, err)
		}
		for _, match := range matches {
			if _, err := os.Lstat(match); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return nil, fmt.Errorf("inspect DSH recreate state %s: %w", match, err)
			}
			relativePath, err := filepath.Rel(runtimeDir, match)
			if err != nil || relativePath == "." || relativePath == ".." || filepath.IsAbs(relativePath) {
				return nil, fmt.Errorf("resolve DSH recreate state path %s", match)
			}
			seen[relativePath] = struct{}{}
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func (p *preservedRecreateState) Restore() error {
	if p == nil {
		return nil
	}
	var result error
	for index := range p.entries {
		entry := &p.entries[index]
		if entry.restored {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(entry.source), 0o755); err != nil {
			result = errors.Join(result, fmt.Errorf("recreate parent for DSH state %s: %w", entry.source, err))
			continue
		}
		if _, err := os.Lstat(entry.source); err == nil {
			result = errors.Join(result, fmt.Errorf("restore DSH state: destination %s already exists; preserved data remains at %s", entry.source, entry.tempPath))
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, fmt.Errorf("inspect DSH state restore destination %s: %w", entry.source, err))
			continue
		}
		if err := os.Rename(entry.tempPath, entry.source); err != nil {
			result = errors.Join(result, fmt.Errorf("restore DSH state from %s: %w", entry.tempPath, err))
			continue
		}
		entry.restored = true
	}
	return result
}

func (p *preservedRecreateState) rollback(cause error) error {
	restoreErr := p.Restore()
	p.Cleanup()
	return errors.Join(cause, restoreErr)
}

func (p *preservedRecreateState) Cleanup() {
	if p == nil {
		return
	}
	for _, entry := range p.entries {
		if !entry.restored {
			return
		}
	}
	_ = os.RemoveAll(p.tempRoot)
}
