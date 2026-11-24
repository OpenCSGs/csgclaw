package agents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/skill/catalog"
	"csgclaw/internal/skill/installstate"
	"csgclaw/internal/skill/local"
	"csgclaw/internal/skill/state"
)

type WorkerSkillStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Enabled   bool   `json:"enabled"`
	DiskSync  string `json:"disk_sync"`
	Source    string `json:"source,omitempty"`
	URL       string `json:"url,omitempty"`
	Version   string `json:"version,omitempty"`
	Ref       string `json:"ref,omitempty"`
}

func (s *WorkspaceService) SkillStatus(ctx context.Context, id, name string) (WorkerSkillStatus, error) {
	out := WorkerSkillStatus{Name: name, DiskSync: "not_applicable"}
	if n, err := local.NormalizeName(name); err != nil || n != name || !filepath.IsLocal(name) || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) {
		return out, ErrAgentSkillInvalid
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	a, ok := s.agentSnapshot(id)
	if !ok || a.ArchivedAt != nil {
		return out, os.ErrNotExist
	}
	layout, err := s.agentLayout(a.ID, a.RuntimeKind)
	if err != nil {
		return out, err
	}
	dir, err := os.Lstat(filepath.Join(layout.SkillsRoot, name))
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if !dir.IsDir() {
		return out, ErrAgentSkillInvalid
	}
	if _, err := local.ResolveDir(layout.SkillsRoot, name); err != nil {
		return out, err
	}
	rec, err := installstate.Lookup(layout.SkillsRoot, name)
	if err != nil {
		return out, err
	}
	out.Installed = true
	out.Enabled = state.Enabled(a.SkillStates, name)
	out.DiskSync = "unknown"
	out.Source = rec.SourceKind()
	out.Version = rec.Version
	out.Ref = rec.Ref
	out.URL = rec.DisplayURL()
	if out.Source == installstate.SourceLocal && catalog.ValidPublicURL(out.URL) {
		out.Source = catalog.Public
	}
	rt, err := s.registry.Get(a.RuntimeKind)
	if err != nil {
		return out, err
	}
	if reader, ok := rt.(agentruntime.SkillDiskInspector); ok {
		out.DiskSync, err = reader.SkillDiskSync(layout, name, out.Enabled, a.RuntimeOptions)
		if err != nil {
			out.DiskSync = "unknown"
		} // installed is still true even if the projection is unreadable
	}
	return out, nil
}
