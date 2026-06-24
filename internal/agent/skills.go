package agent

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	skilllocal "csgclaw/internal/skill/local"
)

var (
	ErrAgentSkillAlreadyExists = errors.New("skill already exists")
	ErrAgentSkillInvalid       = errors.New("skill directory must contain SKILL.md")
)

const agentSkillFileName = "SKILL.md"

func (s *Service) AddSkill(agentID, skillName string) error {
	return s.BatchAddSkills(agentID, []string{skillName})
}

func (s *Service) BatchAddSkills(agentID string, skillNames []string) error {
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
	targetRoot, err := s.agentSkillsRoot(got.Name, got.RuntimeKind)
	if err != nil {
		return err
	}

	type copyPlan struct {
		name string
		src  string
		dst  string
	}
	plans := make([]copyPlan, 0, len(skillNames))
	seen := make(map[string]struct{}, len(skillNames))
	for _, rawName := range skillNames {
		name, err := normalizeAgentSkillName(rawName)
		if err != nil {
			return err
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate skill name %q", name)
		}
		seen[name] = struct{}{}

		srcDir, err := resolveValidSkillDir(globalRoot, name)
		if err != nil {
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
		plans = append(plans, copyPlan{name: name, src: srcDir, dst: dstDir})
	}

	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return fmt.Errorf("create agent skills root %q: %w", targetRoot, err)
	}
	for _, plan := range plans {
		if err := copyWorkspaceFS(os.DirFS(plan.src), ".", plan.dst, "skill", true); err != nil {
			_ = os.RemoveAll(plan.dst)
			return fmt.Errorf("copy skill %q: %w", plan.name, err)
		}
	}
	return nil
}

func (s *Service) DeleteSkill(agentID, skillName string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("agent id is required")
	}
	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return fmt.Errorf("agent %q not found", agentID)
	}
	targetRoot, err := s.agentSkillsRoot(got.Name, got.RuntimeKind)
	if err != nil {
		return err
	}
	return skilllocal.Delete(targetRoot, skillName)
}

func normalizeAgentSkillName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("skill name is required")
	}
	cleanName := filepath.Clean(name)
	if cleanName == "." || cleanName == ".." || cleanName != filepath.Base(cleanName) {
		return "", fmt.Errorf("invalid skill name %q", name)
	}
	return cleanName, nil
}

func resolveValidSkillDir(root, name string) (string, error) {
	name, err := normalizeAgentSkillName(name)
	if err != nil {
		return "", err
	}
	skillDir := filepath.Join(strings.TrimSpace(root), name)
	info, err := os.Stat(skillDir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", ErrAgentSkillInvalid
	}
	skillFile := filepath.Join(skillDir, agentSkillFileName)
	fileInfo, err := os.Stat(skillFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrAgentSkillInvalid
		}
		return "", err
	}
	if !fileInfo.Mode().IsRegular() || fileInfo.IsDir() || fileInfo.Mode()&fs.ModeSymlink != 0 {
		return "", ErrAgentSkillInvalid
	}
	return skillDir, nil
}
