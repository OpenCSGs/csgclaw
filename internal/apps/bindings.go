package apps

import (
	"context"
	skill "csgclaw/internal/skill/state"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	skilllocal "csgclaw/internal/skill/local"
)

func (s *Service) packageSkills(r record) (string, error) {
	dirs, err := s.prepareInstallation(r)
	if err != nil {
		return "", err
	}
	ref := r.Package.Manifest.Skills
	root := filepath.Join(dirs.Plugin, strings.TrimPrefix(ref, "./"))
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if _, err := skilllocal.ResolveDir(root, entry.Name()); err != nil {
			return "", err
		}
	}
	return root, nil
}
func queueMCPSkills(r *record) {
	if r.Kind != KindMCP || r.AgentID == "" || len(r.Package.SkillFiles) == 0 {
		return
	}
	if r.CLIApply == nil {
		r.CLIApply = &cliBinding{}
	}
	r.CLIApply.Pending = true
}
func (s *Service) removeMCPSkills(ctx context.Context, agentID, id string) (bool, error) {
	s.mu.Lock()
	e, err := s.findLocked(agentID, id)
	if err != nil {
		s.mu.Unlock()
		return false, err
	}
	needed := len(e.record.Package.SkillFiles) > 0 || len(e.record.Skills) > 0
	s.mu.Unlock()
	if !needed {
		return false, nil
	}
	if _, err := s.Disconnect(ctx, agentID, id); err != nil {
		return true, err
	}
	s.cliWorkMu.Lock()
	defer s.cliWorkMu.Unlock()
	s.mu.Lock()
	e, err = s.findLocked(agentID, id)
	if err != nil {
		s.mu.Unlock()
		return true, err
	}
	next := copyJSON(e.record)
	queueMCPSkills(&next)
	next.CLIApply.Remove = true
	if err = s.persistLocked(id, &next); err == nil {
		e.record = next
	}
	s.mu.Unlock()
	if err != nil {
		return true, err
	}
	if err = s.applyBinding(ctx, id); err != nil {
		return true, err
	}
	return true, s.removeInstallationData(id)
}

func (s *Service) applyBinding(ctx context.Context, id string) (result error) {
	s.mu.Lock()
	e := s.entries[id]
	if e == nil || e.record.CLIApply == nil || !e.record.CLIApply.Pending {
		s.mu.Unlock()
		return nil
	}
	r := copyJSON(e.record)
	resource := copyJSON(s.entries[r.ResourceID].record)
	s.mu.Unlock()
	defer func() {
		if result == nil {
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := s.entries[id]; e != nil {
			e.record.LastErrorCode = "configuration_failed"
			e.record.LastError = "Connector configuration is incomplete; disconnect and reconnect."
			e.record.Status = "error"
			_ = s.persistLocked(id, &e.record)
		}
	}()
	if s.options.Runtime == nil {
		return errors.New("Connector runtime support is unavailable")
	}
	rt := s.options.Runtime
	if !r.CLIApply.ResumeRecorded {
		running, err := rt.State(ctx, r.AgentID, r.Kind)
		if err != nil {
			return err
		}
		r.CLIApply.Resume = running
		r.CLIApply.ResumeRecorded = true
		s.mu.Lock()
		e = s.entries[id]
		if e == nil {
			s.mu.Unlock()
			return ErrNotFound
		}
		e.record.CLIApply = r.CLIApply
		err = s.persistLocked(id, &e.record)
		s.mu.Unlock()
		if err != nil {
			return err
		}
	}
	configured := r.Enabled && !resource.Disconnected && cliReady(resource)
	bundle := ""
	if resource.CLI != nil {
		bundle = resource.CLI.SkillsDir
	}
	if r.Kind == KindMCP {
		configured = r.active() && !r.Disconnected
		if configured {
			var err error
			bundle, err = s.packageSkills(resource)
			if err != nil {
				return err
			}
		}
	}
	err := rt.Update(ctx, r.AgentID, r.CLIApply.Resume, func(ctx context.Context) error {
		previous := r.Skills
		if r.CLIApply.Plan != nil && (r.CLIApply.PlannedEnabled != configured || r.CLIApply.PlannedRevision != resource.Revision) {
			if err := rt.ApplySkills(ctx, r.AgentID, *r.CLIApply.Plan); err != nil {
				return err
			}
			previous = r.CLIApply.Plan.After
			r.CLIApply.Plan = nil
		}
		if r.CLIApply.Plan == nil {
			plan, err := rt.PlanSkills(r.AgentID, bundle, resource.SkillVersion, previous, configured)
			if err != nil {
				return err
			}
			if err := s.shareSkillOwnership(r.AgentID, id, &plan, configured); err != nil {
				return err
			}
			r.CLIApply.Plan = &plan
			r.CLIApply.PlannedEnabled = configured
			r.CLIApply.PlannedRevision = resource.Revision
			s.mu.Lock()
			e = s.entries[id]
			if e == nil {
				s.mu.Unlock()
				return ErrNotFound
			}
			e.record.CLIApply = r.CLIApply
			err = s.persistLocked(id, &e.record)
			s.mu.Unlock()
			if err != nil {
				return err
			}
		}
		if m := s.migrationSnapshot(); m != nil && m.Workers[r.AgentID] != nil && !m.Workers[r.AgentID].Committed {
			if err := s.editMigration(func(m *cliMigration) error {
				if w := m.Workers[r.AgentID]; w != nil {
					w.Plan = copyJSON(r.CLIApply.Plan)
				}
				return nil
			}); err != nil {
				return err
			}
		}
		if err := rt.ApplySkills(ctx, r.AgentID, *r.CLIApply.Plan); err != nil {
			return err
		}
		if r.Kind == KindMCP {
			return nil
		}
		var p *CLIProjection
		if configured {
			p = cliPayload(r.AgentID, resource)
		}
		return rt.Configure(ctx, r.AgentID, r.ResourceID, p)
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e = s.entries[id]
	if e == nil {
		return ErrNotFound
	}
	next := copyJSON(e.record)
	if next.CLIApply.Remove && !configured {
		if err := s.persistLocked(id, nil); err != nil {
			return err
		}
		delete(s.entries, id)
		return nil
	}
	next.AppliedRevision = resource.Revision
	next.Skills = r.CLIApply.Plan.After
	next.CLIApply = &cliBinding{}
	if r.Kind == KindMCP && (next.Enabled != r.Enabled || next.Disconnected != r.Disconnected || next.ResourceEnabled != r.ResourceEnabled) {
		next.CLIApply.Pending = true
		s.scheduleConnectorWork()
	}
	next.LastErrorCode = ""
	next.LastError = ""
	if r.Kind == KindCLI {
		next.Status = "needs_configuration"
		if configured {
			next.Status = "connected"
		}
	}
	next.UpdatedAt = time.Now().UTC()
	if err = s.persistLocked(id, &next); err != nil {
		return err
	}
	e.record = next
	return nil
}

// bindingNeedsConfiguration includes unfinished removal of a previous configuration.
func bindingNeedsConfiguration(r record) bool {
	return r.Enabled || (r.CLIApply != nil && r.CLIApply.Pending)
}

// Managed Skills may be referenced by multiple resources on the same worker.
func (s *Service) shareSkillOwnership(agentID, id string, plan *skill.SkillPlan, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	owned := map[string]string{}
	for otherID, e := range s.entries {
		if otherID == id || e.record.AgentID != agentID {
			continue
		}
		skills := e.record.Skills
		if e.record.CLIApply != nil && e.record.CLIApply.Plan != nil {
			skills = e.record.CLIApply.Plan.After
		}
		for _, item := range skills {
			if item.Managed {
				owned[item.Name] = item.Digest
			}
		}
	}
	for index, item := range plan.Before {
		if _, shared := owned[item.Name]; shared {
			plan.Before[index].Managed = false
		}
	}
	if enabled {
		for index, item := range plan.After {
			if digest, shared := owned[item.Name]; shared {
				if digest != item.Digest {
					return fmt.Errorf("skill_conflict: %s is used by another connector", item.Name)
				}
				plan.After[index].Managed = true
			}
		}
	}
	return nil
}
