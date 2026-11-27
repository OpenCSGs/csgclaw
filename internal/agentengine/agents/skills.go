package agents

import (
	"context"
	skillbuiltin "csgclaw/internal/skill/builtin"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"csgclaw/internal/skill/discovery"
	skillinstall "csgclaw/internal/skill/installstate"
	skilllocal "csgclaw/internal/skill/local"
	"csgclaw/internal/skill/skillpath"
	skillsystem "csgclaw/internal/skill/system"
)

var (
	ErrAgentSkillAlreadyExists = errors.New("skill already exists")
	ErrAgentSkillInvalid       = errors.New("skill directory must contain SKILL.md")
)

func (s *Controller) AddSkill(agentID, skillName string) error {
	return s.BatchAddSkills(agentID, []string{skillName})
}

func (s *Controller) BatchAddSkills(agentID string, skillNames []string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("agent id is required")
	}
	if len(skillNames) == 0 {
		return fmt.Errorf("skill names are required")
	}

	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return fmt.Errorf("agent %q not found", agentID)
	}
	globalRoot, err := skilllocal.SkillsRoot()
	if err != nil {
		return err
	}
	origins, err := skillinstall.Read(globalRoot)
	if err != nil {
		return err
	}
	targetRoot, err := s.agentSkillsRoot(got.ID, got.RuntimeKind)
	if err != nil {
		return err
	}

	type copyPlan struct {
		name   string
		source skillsystem.SkillSource
		origin skillinstall.Record
		dst    string
	}
	plans := make([]copyPlan, 0, len(skillNames))
	seen := make(map[string]struct{}, len(skillNames))
	for _, rawName := range skillNames {
		name, err := skilllocal.NormalizeName(rawName)
		if err != nil {
			return err
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate skill name %q", name)
		}
		seen[name] = struct{}{}

		source, err := resolveAgentSkillSource(globalRoot, name)
		if err != nil {
			if errors.Is(err, skilllocal.ErrSkillInvalid) {
				return fmt.Errorf("%w: %s", ErrAgentSkillInvalid, name)
			}
			return err
		}
		dstDir := filepath.Join(targetRoot, name)
		if info, err := os.Stat(dstDir); err == nil {
			if info.IsDir() {
				return fmt.Errorf("%w: %s", ErrAgentSkillAlreadyExists, name)
			}
			return fmt.Errorf("%w: %s", ErrAgentSkillAlreadyExists, name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat agent skill destination %q: %w", dstDir, err)
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
		plans = append(plans, copyPlan{name: name, source: source, origin: origin, dst: dstDir})
	}

	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return fmt.Errorf("create agent skills root %q: %w", targetRoot, err)
	}
	for _, plan := range plans {
		if err := skillinstall.Update(targetRoot, plan.name, &plan.origin, func() error {
			if err := copyWorkspaceFS(plan.source.FS, plan.source.RootPath, plan.dst, "skill", true); err != nil {
				_ = os.RemoveAll(plan.dst)
				return err
			}
			return nil
		}); err != nil {
			return fmt.Errorf("copy skill %q: %w", plan.name, err)
		}
	}
	return nil
}

func (s *Controller) DeleteSkill(agentID, skillName string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("agent id is required")
	}
	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return fmt.Errorf("agent %q not found", agentID)
	}
	targetRoot, err := s.agentSkillsRoot(got.ID, got.RuntimeKind)
	if err != nil {
		return err
	}
	if !strings.Contains(skillName, "/") {
		return skilllocal.Delete(targetRoot, skillName)
	}
	if !skillpath.Valid(skillName) {
		return ErrAgentSkillInvalid
	}
	return skillinstall.Update(targetRoot, skillName, nil, func() error {
		root, err := os.OpenRoot(targetRoot)
		if err != nil {
			return err
		}
		defer root.Close()
		ids, err := skillpath.IDs(context.Background(), root, false)
		if err != nil {
			return err
		}
		if !slices.Contains(ids, skillName) {
			return os.ErrNotExist
		}
		return root.RemoveAll(filepath.FromSlash(skillName))
	})
}

// Skills returns the installed skill names for one Agent.
func (s *Controller) Skills(agentID string) ([]string, error) {
	agentID = strings.TrimSpace(agentID)
	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return nil, fmt.Errorf("agent %q not found", agentID)
	}
	targetRoot, err := s.agentSkillsRoot(got.ID, got.RuntimeKind)
	if err != nil {
		return nil, err
	}
	return skillpath.Installed(targetRoot)
}

// ReplaceSkills validates and stages the complete desired skill set before an
// atomic directory replacement under the shared lifecycle gate.
func (s *Controller) ReplaceSkills(ctx context.Context, agentID string, skillNames []string) error {
	ctx, release, err := s.acquireAgentLifecycle(ctx, agentID)
	if err != nil {
		return err
	}
	defer release()

	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return fmt.Errorf("agent %q not found", strings.TrimSpace(agentID))
	}
	targetRoot, err := s.agentSkillsRoot(got.ID, got.RuntimeKind)
	if err != nil {
		return err
	}
	parent := filepath.Dir(targetRoot)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create agent workspace %q: %w", parent, err)
	}
	stageRoot, err := os.MkdirTemp(parent, ".skills-stage-")
	if err != nil {
		return fmt.Errorf("create staged skills directory: %w", err)
	}
	defer os.RemoveAll(stageRoot)

	var grouped, flat []string
	seen := make(map[string]struct{}, len(skillNames))
	for _, rawName := range skillNames {
		name := strings.TrimSpace(rawName)
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate skill name %q", name)
		}
		seen[name] = struct{}{}
		if strings.Contains(name, "/") {
			grouped = append(grouped, name)
			continue
		}
		name, err := skilllocal.NormalizeName(name)
		if err != nil {
			return err
		}
		flat = append(flat, name)
	}
	for _, id := range grouped {
		parent, _, _ := strings.Cut(id, "/")
		if _, exists := seen[parent]; exists {
			return fmt.Errorf("%w: %q is both a skill and a group", ErrAgentSkillInvalid, parent)
		}
	}
	var globalRoot string
	var origins map[string]skillinstall.Record
	if len(flat) > 0 {
		globalRoot, err = skilllocal.SkillsRoot()
		if err != nil {
			return err
		}
		origins, err = skillinstall.Read(globalRoot)
		if err != nil {
			return err
		}
	}
	for _, name := range flat {
		source, err := resolveAgentSkillSource(globalRoot, name)
		if err != nil {
			if errors.Is(err, skilllocal.ErrSkillInvalid) {
				return fmt.Errorf("%w: %s", ErrAgentSkillInvalid, name)
			}
			return err
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
		if err := skillinstall.Update(stageRoot, name, &origin, func() error {
			return copyWorkspaceFS(source.FS, source.RootPath, filepath.Join(stageRoot, name), "skill", true)
		}); err != nil {
			return fmt.Errorf("stage skill %q: %w", name, err)
		}
	}

	if len(grouped) > 0 {
		if err := stageGroupedSkills(ctx, targetRoot, stageRoot, grouped); err != nil {
			return err
		}
	}

	backupRoot := targetRoot + ".replace-backup"
	if err := os.RemoveAll(backupRoot); err != nil {
		return fmt.Errorf("remove stale skill backup: %w", err)
	}
	targetExists := false
	if _, err := os.Stat(targetRoot); err == nil {
		targetExists = true
		if err := os.Rename(targetRoot, backupRoot); err != nil {
			return fmt.Errorf("backup current skills: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat current skills: %w", err)
	}
	if err := os.Rename(stageRoot, targetRoot); err != nil {
		if targetExists {
			_ = os.Rename(backupRoot, targetRoot)
		}
		return fmt.Errorf("activate staged skills: %w", err)
	}
	if err := os.RemoveAll(backupRoot); err != nil {
		return fmt.Errorf("remove previous skills: %w", err)
	}
	return nil
}

// Grouped skills may exist only in the Agent workspace. Preserve their installed
// files and provenance together when replacing that workspace's skill set.
func stageGroupedSkills(ctx context.Context, root, stage string, ids []string) error {
	origins := map[string]*skillinstall.Record{}
	err := skillinstall.ReadSnapshot(root, func(records map[string]skillinstall.Record) error {
		dir, err := os.OpenRoot(root)
		if err != nil {
			return err
		}
		defer dir.Close()
		installed, err := skillpath.IDs(ctx, dir, false)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, ok := slices.BinarySearch(installed, id); !ok {
				return fmt.Errorf("%w: %s", ErrAgentSkillInvalid, id)
			}
			if err := copyWorkspaceFS(discovery.BoundedFS(root), id, filepath.Join(stage, id), "skill", true); err != nil {
				return err
			}
			if record, ok := records[id]; ok {
				origins[id] = &record
			}
		}
		return nil
	})
	if err != nil || len(origins) == 0 {
		return err
	}
	return skillinstall.UpdateMany(stage, origins, nil)
}

func resolveAgentSkillSource(root, name string) (skillsystem.SkillSource, error) {
	if skillDir, err := skilllocal.ResolveDir(root, name); err == nil {
		return skillsystem.SkillSource{Name: name, Source: skillsystem.SkillSourceLocal, FS: discovery.BoundedFS(skillDir), RootPath: "."}, nil
	} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, skilllocal.ErrSkillInvalid) {
		return skillsystem.SkillSource{}, err
	} else if !skillsystem.IsName(name) {
		return skillsystem.SkillSource{}, err
	}
	return skillsystem.ResolveSource(name)
}

func (s *Controller) installDefaultSystemSkills(agentID, runtimeKind string) error {
	if !isGatewayRuntimeKind(strings.TrimSpace(runtimeKind)) && !isHostRuntimeKind(strings.TrimSpace(runtimeKind)) {
		return nil
	}
	names, err := defaultSystemSkillNames()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	targetRoot, err := s.agentSkillsRoot(agentID, runtimeKind)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return fmt.Errorf("create agent skills root %q: %w", targetRoot, err)
	}
	for _, name := range names {
		src, err := skillsystem.ResolveSource(name)
		if err != nil {
			return err
		}
		dst := filepath.Join(targetRoot, name)
		origin := skillinstall.Record{Source: skillinstall.SourceBuiltin}
		if err := skillinstall.UpdateIf(targetRoot, name, &origin, func() (bool, error) {
			err := skillpath.CheckReplacement(dst)
			if errors.Is(err, os.ErrExist) {
				return false, nil
			}
			return err == nil, err
		}, func() error {
			if err := os.RemoveAll(dst); err != nil {
				return err
			}
			if err := copyWorkspaceFS(src.FS, src.RootPath, dst, "system skill", true); err != nil {
				_ = os.RemoveAll(dst)
				return err
			}
			return nil
		}); err != nil {
			return fmt.Errorf("copy default system skill %q: %w", name, err)
		}
	}
	return nil
}

func defaultSystemSkillNames() ([]string, error) {
	names, err := skillsystem.DefaultNames()
	if err != nil {
		return nil, err
	}
	slices.Sort(names)
	return names, nil
}

// syncManagerBundledSkills must run after preserved skills and their origins have
// been restored, so a generated default cannot replace a user-owned name.
func (s *Controller) syncManagerBundledSkills() {
	root, err := s.agentSkillsRoot(ManagerUserID, RuntimeKindCodex)
	if err == nil {
		err = skillbuiltin.Sync(root)
	}
	if err != nil {
		slog.Warn("bundled manager skills unavailable; continuing runtime setup", "error", err)
	}
}
