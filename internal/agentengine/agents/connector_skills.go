package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	skilllocal "csgclaw/internal/skill/local"
	skill "csgclaw/internal/skill/state"
)

func (s *Controller) ConnectorSkillsRoot(id string) (string, error) {
	selected, ok := s.Agent(id)
	if !ok {
		return "", fmt.Errorf("agent %q not found", id)
	}
	return s.agentSkillsRoot(id, selected.RuntimeKind)
}
func (s *Controller) PlanConnectorSkills(id, bundle, version string, previous []skill.ManagedSkill, enabled bool) (skill.SkillPlan, error) {
	root, err := s.ConnectorSkillsRoot(id)
	if err != nil {
		return skill.SkillPlan{}, err
	}
	plan := skill.SkillPlan{Before: previous, Bundle: bundle}
	owned := map[string]skill.ManagedSkill{}
	for _, item := range previous {
		owned[item.Name] = item
	}
	if !enabled {
		for _, item := range previous {
			digest, err := skill.DirectoryDigest(filepath.Join(root, item.Name))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return plan, err
			}
			if !item.Managed || digest != item.Digest {
				item.Managed = false
				item.Digest = digest
				plan.After = append(plan.After, item)
			}
		}
		return plan, nil
	}
	entries, err := os.ReadDir(bundle)
	if err != nil {
		return plan, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() {
			continue
		}
		if _, err := skilllocal.ResolveDir(bundle, name); err != nil {
			return plan, err
		}
		digest, err := skill.DirectoryDigest(filepath.Join(bundle, name))
		if err != nil {
			return plan, err
		}
		item := skill.ManagedSkill{Name: name, Version: version, Digest: digest, Managed: true}
		actual, err := skill.DirectoryDigest(filepath.Join(root, name))
		if err == nil {
			old, known := owned[name]
			if !known || !old.Managed {
				if actual != digest {
					return plan, fmt.Errorf("skill_conflict: %s", name)
				}
				item.Managed = false
			} else if actual != old.Digest && actual != digest {
				return plan, fmt.Errorf("skill_conflict: %s", name)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return plan, err
		}
		plan.After = append(plan.After, item)
	}
	sort.Slice(plan.After, func(i, j int) bool { return plan.After[i].Name < plan.After[j].Name })
	return plan, nil
}
func (s *Controller) ApplyConnectorSkills(ctx context.Context, id string, plan skill.SkillPlan) error {
	return s.WithAgentLifecycle(ctx, id, func(ctx context.Context) error {
		root, err := s.ConnectorSkillsRoot(id)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(root, 0755); err != nil {
			return err
		}
		before := map[string]skill.ManagedSkill{}
		after := map[string]skill.ManagedSkill{}
		for _, item := range plan.Before {
			before[item.Name] = item
		}
		for _, item := range plan.After {
			after[item.Name] = item
		}
		names := map[string]bool{}
		for n := range before {
			names[n] = true
		}
		for n := range after {
			names[n] = true
		}
		ordered := make([]string, 0, len(names))
		for n := range names {
			ordered = append(ordered, n)
		}
		sort.Strings(ordered)
		for _, name := range ordered {
			if err := ctx.Err(); err != nil {
				return err
			}
			normalized, err := skilllocal.NormalizeName(name)
			if err != nil || normalized != name {
				return ErrAgentSkillInvalid
			}
			old, had := before[name]
			next, keep := after[name]
			if (keep && !next.Managed) || (!keep && had && !old.Managed) {
				continue
			}
			target := filepath.Join(root, name)
			backup := filepath.Join(filepath.Dir(root), ".connector-backup-"+name)
			actual, err := skill.DirectoryDigest(target)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if keep && err == nil && actual == next.Digest {
				if err := os.RemoveAll(backup); err != nil {
					return err
				}
				continue
			}
			if err == nil && (!had || !old.Managed || actual != old.Digest) {
				return fmt.Errorf("skill_conflict: %s", name)
			}
			if _, err := os.Lstat(backup); err == nil {
				digest, err := skill.DirectoryDigest(backup)
				if err != nil || !had || digest != old.Digest {
					return fmt.Errorf("skill_conflict: %s", name)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if keep {
				source := filepath.Join(plan.Bundle, name)
				digest, err := skill.DirectoryDigest(source)
				if err != nil {
					return err
				}
				if digest != next.Digest {
					return fmt.Errorf("skill source changed: %s", name)
				}
				stage, err := os.MkdirTemp(filepath.Dir(root), ".connector-stage-")
				if err != nil {
					return err
				}
				copyErr := copyWorkspaceFS(os.DirFS(plan.Bundle), name, stage, "skill", true)
				if copyErr != nil {
					_ = os.RemoveAll(stage)
					return copyErr
				}
				if actual != "" {
					if err := os.Rename(target, backup); err != nil {
						_ = os.RemoveAll(stage)
						return err
					}
				}
				if err := os.Rename(stage, target); err != nil {
					_ = os.RemoveAll(stage)
					return err
				}
			} else if actual != "" {
				if err := os.Rename(target, backup); err != nil {
					return err
				}
			}
			if err := os.RemoveAll(backup); err != nil {
				return err
			}
		}
		return nil
	})
}
