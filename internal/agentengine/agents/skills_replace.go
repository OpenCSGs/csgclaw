package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"csgclaw/internal/agentengine/contract"

	skillinstall "csgclaw/internal/skill/installstate"
	skilllocal "csgclaw/internal/skill/local"
	"csgclaw/internal/skill/skillpath"
)

type preparedSkillReplacement struct {
	targetRoot    string
	stageRoot     string
	names         []string
	origins       map[string]*skillinstall.Record
	keepBackup    bool
	workerInstall *skillinstall.WorkerInstall
}

func (s *Controller) installWorkerCatalog(ctx context.Context, id string, in contract.CatalogInstall) error {
	name := in.Name
	if clean, err := skilllocal.NormalizeName(name); err != nil || clean != name || !filepath.IsLocal(name) || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) {
		return ErrAgentSkillInvalid
	}
	id = canonicalAgentID(id)
	return s.LifecycleCoordinator().TryUpdateFiles(ctx, id, func() error {
		item, ok := s.agentSnapshot(id)
		if !ok || item.ArchivedAt != nil {
			return os.ErrNotExist
		}
		if item.Role != RoleWorker || !isHostRuntimeKind(item.RuntimeKind) {
			return ErrSkillEnablementUnsupported
		}
		if in.Archive != nil {
			root, err := s.agentSkillsRoot(id, item.RuntimeKind)
			if err != nil {
				return err
			}
			_, err = skilllocal.InstallArchiveWithOptions(root, name+".zip", in.Archive, skilllocal.InstallArchiveOptions{Origin: in.Origin, ExpectedName: name, Replace: in.Replace, Worker: &skillinstall.WorkerInstall{Replace: in.Replace, Confirmation: in.Confirmation}})
			return err
		}
		if in.Selection == "" {
			return fmt.Errorf("global skill selection is required; search again")
		}
		p, err := s.prepareSelectedSkills(ctx, item, []string{name}, in.Selection)
		if err != nil {
			return err
		}
		defer p.cleanup()
		p.workerInstall = &skillinstall.WorkerInstall{Replace: in.Replace, Confirmation: in.Confirmation}
		return p.apply()
	})
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
	return s.prepareSelectedSkills(context.Background(), item, names, "")
}

func (s *Controller) prepareSelectedSkills(ctx context.Context, item Agent, names []string, selection string) (*preparedSkillReplacement, error) {
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
			bundle, err := skilllocal.ResolveLibraryBundle(globalRoot, name, origins[name])
			stage := filepath.Join(stageRoot, "next", name)
			if err == nil {
				err = copyWorkspaceFS(bundle.Tree, ".", stage, "skill", true)
			}
			if err != nil {
				if errors.Is(err, skilllocal.ErrSkillInvalid) {
					err = errors.Join(ErrAgentSkillInvalid, err)
				}
				return fmt.Errorf("prepare skill %q: %w", name, err)
			}
			if selection != "" {
				staged := skilllocal.LibraryBundle{Tree: os.DirFS(stage), Origin: bundle.Origin}
				actual, err := staged.Selection(ctx, name)
				if err != nil {
					return err
				}
				if actual != selection {
					return fmt.Errorf("global skill %q changed since selection; search again", name)
				}
				if bundle.Origin.SourceURL == "" && (bundle.Origin.SourceKind() == skillinstall.SourceUnknown || bundle.Origin.SourceKind() == skillinstall.SourceLocal) {
					bundle.Origin.Source = skillinstall.SourceLocal
					bundle.Origin.SourceURL = filepath.Join(globalRoot, name)
				}
				bundle.Origin.SHA256, err = skillinstall.BundleSHA256(stage)
				if err != nil {
					return err
				}
			}
			origin := bundle.Origin
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
	apply := func() (err error) {
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
				if err := skillpath.CheckReplacement(target); err != nil {
					if errors.Is(err, os.ErrExist) {
						return fmt.Errorf("%w: %w", ErrAgentSkillAlreadyExists, err)
					}
					return err
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
	}
	if p.workerInstall != nil {
		name := p.names[0]
		return p.workerInstall.Commit(p.targetRoot, name, filepath.Join(p.stageRoot, "next", name), p.origins[name], apply)
	}
	return skillinstall.UpdateMany(p.targetRoot, p.origins, apply)
}

func (s *Controller) restoreSkillReplacementRecord(previous Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putAgentLocked(previous.ID, previous)
	s.syncRuntimeRecordLocked(previous)
	return s.saveLocked()
}
