package agents

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	skilllocal "csgclaw/internal/skill/local"
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
	targetRoot, err := s.agentSkillsRoot(got.ID, got.RuntimeKind)
	if err != nil {
		return err
	}

	type copyPlan struct {
		name    string
		srcFS   fs.FS
		srcRoot string
		dst     string
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

		srcFS, srcRoot, err := resolveAgentSkillSource(globalRoot, name)
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
		plans = append(plans, copyPlan{name: name, srcFS: srcFS, srcRoot: srcRoot, dst: dstDir})
	}

	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return fmt.Errorf("create agent skills root %q: %w", targetRoot, err)
	}
	for _, plan := range plans {
		if err := copyWorkspaceFS(plan.srcFS, plan.srcRoot, plan.dst, "skill", true); err != nil {
			_ = os.RemoveAll(plan.dst)
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
	return skilllocal.Delete(targetRoot, skillName)
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
	entries, err := os.ReadDir(targetRoot)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read agent skills root %q: %w", targetRoot, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := skilllocal.ResolveDir(targetRoot, entry.Name()); err == nil {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return names, nil
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

	globalRoot, err := skilllocal.SkillsRoot()
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(skillNames))
	for _, rawName := range skillNames {
		name, err := skilllocal.NormalizeName(rawName)
		if err != nil {
			return err
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate skill name %q", name)
		}
		seen[name] = struct{}{}
		srcFS, srcRoot, err := resolveAgentSkillSource(globalRoot, name)
		if err != nil {
			if errors.Is(err, skilllocal.ErrSkillInvalid) {
				return fmt.Errorf("%w: %s", ErrAgentSkillInvalid, name)
			}
			return err
		}
		if err := copyWorkspaceFS(srcFS, srcRoot, filepath.Join(stageRoot, name), "skill", true); err != nil {
			return fmt.Errorf("stage skill %q: %w", name, err)
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

func resolveAgentSkillSource(root, name string) (fs.FS, string, error) {
	if skillDir, err := skilllocal.ResolveDir(root, name); err == nil {
		return os.DirFS(skillDir), ".", nil
	} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, skilllocal.ErrSkillInvalid) {
		return nil, "", err
	} else if !skillsystem.IsName(name) {
		return nil, "", err
	}
	source, err := skillsystem.ResolveSource(name)
	if err != nil {
		return nil, "", err
	}
	return source.FS, source.RootPath, nil
}

func (s *Controller) installDefaultSystemSkills(agentID, runtimeKind string) error {
	if !isGatewayRuntimeKind(strings.TrimSpace(runtimeKind)) {
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
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("remove default system skill %q: %w", name, err)
		}
		if err := copyWorkspaceFS(src.FS, src.RootPath, dst, "system skill", true); err != nil {
			_ = os.RemoveAll(dst)
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
