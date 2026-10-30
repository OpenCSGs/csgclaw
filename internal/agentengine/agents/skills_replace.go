package agents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	skillinstall "csgclaw/internal/skill/installstate"
	skilllocal "csgclaw/internal/skill/local"
	skillsystem "csgclaw/internal/skill/system"
)

type preparedSkillReplacement struct {
	targetRoot string
	stageRoot  string
	names      []string
	origins    map[string]*skillinstall.Record
	keepBackup bool
}

func validateSkillReplacements(names, desired []string) error {
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		normalized, err := skilllocal.NormalizeName(name)
		if err != nil || normalized != name || seen[name] || !slices.Contains(desired, name) {
			return fmt.Errorf("%w: replacement %q must name a distinct desired skill", ErrAgentSkillInvalid, name)
		}
		seen[name] = true
	}
	return nil
}

// Prepare all source files before changing the Agent's files or enablement.
func (s *Controller) prepareSkillReplacement(item Agent, names []string) (*preparedSkillReplacement, error) {
	globalRoot, err := skilllocal.SkillsRoot()
	if err != nil {
		return nil, err
	}
	targetRoot, err := s.agentSkillsRoot(item.ID, item.RuntimeKind)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(targetRoot), 0o755); err != nil {
		return nil, err
	}
	stageRoot, err := os.MkdirTemp(filepath.Dir(targetRoot), ".skills-replace-")
	if err != nil {
		return nil, fmt.Errorf("create staged skills directory: %w", err)
	}
	prepared := &preparedSkillReplacement{targetRoot: targetRoot, stageRoot: stageRoot, names: slices.Clone(names), origins: make(map[string]*skillinstall.Record, len(names))}
	err = skillinstall.ReadSnapshot(globalRoot, func(origins map[string]skillinstall.Record) error {
		for _, name := range names {
			source, err := resolveAgentSkillSource(globalRoot, name)
			if err == nil {
				err = copyWorkspaceFS(source.FS, source.RootPath, filepath.Join(stageRoot, "next", name), "skill", true)
			}
			if err != nil {
				if errors.Is(err, skilllocal.ErrSkillInvalid) {
					err = errors.Join(ErrAgentSkillInvalid, err)
				}
				return fmt.Errorf("prepare skill %q: %w", name, err)
			}
			origin := origins[name]
			if found, err := skilllocal.Discovered(globalRoot, name); err != nil {
				return err
			} else if found != nil {
				origin = skillinstall.Record{Source: skillinstall.SourceDiscovered, SourceURL: found.Path, Version: found.Version}
			}
			if source.Source == skillsystem.SkillSourceSystem {
				origin = skillinstall.Record{Source: skillinstall.SourceBuiltin}
			}
			prepared.origins[name] = &origin
		}
		return nil
	})
	if err != nil {
		prepared.cleanup()
		return nil, err
	}
	return prepared, nil
}

func (p *preparedSkillReplacement) cleanup() {
	if !p.keepBackup {
		_ = os.RemoveAll(p.stageRoot)
	}
}

func (p *preparedSkillReplacement) apply() error {
	backupRoot := filepath.Join(p.stageRoot, "previous")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return err
	}
	return skillinstall.UpdateMany(p.targetRoot, p.origins, func() (err error) {
		type movedSkill struct {
			name      string
			backedUp  bool
			activated bool
		}
		moved := make([]movedSkill, 0, len(p.names))
		defer func() {
			if err == nil {
				return
			}
			for i := len(moved) - 1; i >= 0; i-- {
				entry := moved[i]
				target := filepath.Join(p.targetRoot, entry.name)
				if entry.activated {
					if restoreErr := os.RemoveAll(target); restoreErr != nil {
						p.keepBackup = true
						err = errors.Join(err, fmt.Errorf("remove failed replacement %q: %w", entry.name, restoreErr))
						continue
					}
				}
				if entry.backedUp {
					if restoreErr := os.Rename(filepath.Join(backupRoot, entry.name), target); restoreErr != nil {
						p.keepBackup = true
						err = errors.Join(err, fmt.Errorf("restore skill %q from %q: %w", entry.name, backupRoot, restoreErr))
					}
				}
			}
		}()
		for _, name := range p.names {
			target := filepath.Join(p.targetRoot, name)
			entry := movedSkill{name: name}
			if info, statErr := os.Lstat(target); statErr == nil {
				if info.Mode()&os.ModeSymlink != 0 {
					return fmt.Errorf("%w: %s", ErrWorkspaceSymlinkDenied, name)
				}
				if !info.IsDir() {
					return fmt.Errorf("%w: %s", ErrAgentSkillInvalid, name)
				}
				if err := os.Rename(target, filepath.Join(backupRoot, name)); err != nil {
					return fmt.Errorf("backup skill %q: %w", name, err)
				}
				entry.backedUp = true
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}
			moved = append(moved, entry)
			if err := os.Rename(filepath.Join(p.stageRoot, "next", name), target); err != nil {
				return fmt.Errorf("activate skill %q: %w", name, err)
			}
			moved[len(moved)-1].activated = true
		}
		return nil
	})
}

func (s *Controller) restoreSkillReplacementRecord(previous Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putAgentLocked(previous.ID, previous)
	s.syncRuntimeRecordLocked(previous)
	return s.saveLocked()
}
