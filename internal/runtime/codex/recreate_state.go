package codex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var recreatePersistentPathPatterns = []string{
	workspaceDirName,
	sessionFileName,
	filepath.Join(homeDirName, "memories"),
	filepath.Join(homeDirName, "memories_*.sqlite*"),
	filepath.Join(homeDirName, "sessions"),
	filepath.Join(homeDirName, "state_*.sqlite*"),
	filepath.Join(homeDirName, "goals_*.sqlite*"),
	filepath.Join(homeDirName, "auth.json"),
	filepath.Join(homeDirName, "plugins"),
	filepath.Join(homeDirName, "rules"),
	filepath.Join(homeDirName, "hooks.json"),
	filepath.Join(homeDirName, "installation_id"),
}

type preservedRuntimeState struct {
	tempRoot string
	entries  []preservedRuntimeEntry
}

type preservedRuntimeEntry struct {
	source   string
	tempPath string
	restored bool
}

func preserveRuntimeState(runtimeDir string) (*preservedRuntimeState, error) {
	relativePaths, err := recreatePersistentPaths(runtimeDir)
	if err != nil {
		return nil, err
	}
	if len(relativePaths) == 0 {
		return nil, nil
	}
	tempRoot, err := os.MkdirTemp(filepath.Dir(runtimeDir), ".csgclaw-codex-state-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary Codex state preservation directory: %w", err)
	}
	preserved := &preservedRuntimeState{tempRoot: tempRoot}
	for _, relativePath := range relativePaths {
		entry := preservedRuntimeEntry{
			source:   filepath.Join(runtimeDir, relativePath),
			tempPath: filepath.Join(tempRoot, relativePath),
		}
		if err := os.MkdirAll(filepath.Dir(entry.tempPath), 0o755); err != nil {
			return nil, preserved.rollback(fmt.Errorf("prepare Codex state preservation for %s: %w", relativePath, err))
		}
		if err := os.Rename(entry.source, entry.tempPath); err != nil {
			return nil, preserved.rollback(fmt.Errorf("preserve Codex state %s: %w", relativePath, err))
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
			return nil, fmt.Errorf("resolve Codex recreate state pattern %q: %w", pattern, err)
		}
		for _, match := range matches {
			if _, err := os.Lstat(match); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return nil, fmt.Errorf("inspect Codex recreate state %s: %w", match, err)
			}
			relativePath, err := filepath.Rel(runtimeDir, match)
			if err != nil || relativePath == "." || relativePath == ".." || filepath.IsAbs(relativePath) {
				return nil, fmt.Errorf("resolve Codex recreate state path %s", match)
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

func (p *preservedRuntimeState) Restore() error {
	if p == nil {
		return nil
	}
	var restoreErr error
	for index := range p.entries {
		entry := &p.entries[index]
		if entry.restored {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(entry.source), 0o755); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("recreate parent for Codex state %s: %w", entry.source, err))
			continue
		}
		if _, err := os.Lstat(entry.source); err == nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("restore Codex state: destination %s already exists; preserved data remains at %s", entry.source, entry.tempPath))
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("inspect Codex state restore destination %s: %w", entry.source, err))
			continue
		}
		if err := os.Rename(entry.tempPath, entry.source); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("restore Codex state from %s: %w", entry.tempPath, err))
			continue
		}
		entry.restored = true
	}
	return restoreErr
}

func (p *preservedRuntimeState) rollback(cause error) error {
	restoreErr := p.Restore()
	p.Cleanup()
	return errors.Join(cause, restoreErr)
}

func (p *preservedRuntimeState) Cleanup() {
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
