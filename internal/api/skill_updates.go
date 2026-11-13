package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/skill/installstate"
	skilllocal "csgclaw/internal/skill/local"
	"csgclaw/internal/skill/updates"
)

func (h *Handler) skillUpdateConnection(r *http.Request) (updates.Connection, error) {
	api, token, err := h.remoteSkillsHubConnection(r)
	return updates.Connection{API: api, Token: token, Site: h.currentOpenCSGEnvironment(r).OpenCSGBaseURL}, err
}

func (h *Handler) skillUpdateTargets(ctx context.Context, name, site string) ([]updates.Target, error) {
	if _, err := skilllocal.NormalizeName(name); err != nil {
		return nil, err
	}
	root, err := skilllocal.SkillsRoot()
	if err != nil {
		return nil, err
	}
	records, err := installstate.Read(root)
	if err != nil {
		return nil, err
	}
	path, err := updates.SourcePath(records[name], site)
	if err != nil {
		return nil, err
	}
	if discovered, err := skilllocal.Discovered(root, name); err != nil {
		return nil, err
	} else if discovered != nil {
		return nil, errors.New("skill_update_source_unavailable")
	}
	targets := []updates.Target{{ID: "library", Root: root}}
	if h.agentEngine == nil || h.workspace == nil {
		return targets, nil
	}
	agents, err := h.agentEngine.Agents().List(ctx, agentengine.AgentListOptions{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(agents, func(a, b agentengine.Agent) int { return strings.Compare(a.ID, b.ID) })
	for _, agent := range agents {
		if !slices.Contains(agent.Spec.Skills, name) {
			continue
		}
		layout, err := h.workspace.AgentLayout(agent.ID)
		if err != nil {
			return nil, err
		}
		dir := layout.SkillsRoot
		origins, err := installstate.Read(dir)
		if err != nil {
			return nil, err
		}
		originPath, err := updates.SourcePath(origins[name], site)
		// Independently authored or differently sourced copies are not this installation.
		if err != nil || originPath != path {
			continue
		}
		targets = append(targets, updates.Target{ID: agent.ID, Name: agent.Spec.Name, Root: dir})
	}
	return targets, nil
}

func (h *Handler) checkSkillUpdate(r *http.Request, conn updates.Connection, name string, preview bool) (updates.Info, error) {
	targets, err := h.skillUpdateTargets(r.Context(), name, conn.Site)
	if err != nil {
		return updates.Info{Name: name}, err
	}
	return updates.Check(r.Context(), conn, name, targets, preview)
}

func (h *Handler) handleSkillUpdateCheck(w http.ResponseWriter, r *http.Request) {
	conn, err := h.skillUpdateConnection(r)
	if err != nil {
		writeRemoteSkillsHubError(w, err)
		return
	}
	info, err := h.checkSkillUpdate(r, conn, pathValue(r, "name"), r.URL.Query().Get("preview") == "true")
	if err != nil {
		writeSkillUpdateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (h *Handler) handleSkillUpdates(w http.ResponseWriter, r *http.Request) {
	conn, err := h.skillUpdateConnection(r)
	if err != nil {
		writeRemoteSkillsHubError(w, err)
		return
	}
	root, err := skilllocal.SkillsRoot()
	if err != nil {
		writeSkillUpdateError(w, err)
		return
	}
	records, err := installstate.Read(root)
	if err != nil {
		writeSkillUpdateError(w, err)
		return
	}
	names := []string{}
	for name, record := range records {
		if record.SourceKind() == installstate.SourceMarket {
			if discovered, err := skilllocal.Discovered(root, name); err == nil && discovered != nil {
				continue
			}
			names = append(names, name)
		}
	}
	slices.Sort(names)
	items := make([]updates.Info, len(names))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(4, len(names)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				info, err := h.checkSkillUpdate(r, conn, names[i], false)
				if err != nil {
					info = updates.Info{Name: names[i], Error: "skill_update_check_failed"}
				}
				items[i] = info
			}
		}()
	}
	for i := range names {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	writeJSON(w, http.StatusOK, struct {
		Items     []updates.Info `json:"items"`
		CheckedAt time.Time      `json:"checkedAt"`
	}{items, time.Now().UTC()})
}

func (h *Handler) handleSkillUpdatesApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []updates.Decision `json:"items"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || len(req.Items) == 0 || len(req.Items) > 100 {
		http.Error(w, "invalid skill update request", http.StatusBadRequest)
		return
	}
	seen := map[string]bool{}
	for _, item := range req.Items {
		if item.Name == "" || seen[item.Name] || item.State == "" || item.Commit == "" || item.Version == "" || (item.Strategy != "update" && item.Strategy != "keep-local") {
			http.Error(w, "invalid skill update decision", http.StatusBadRequest)
			return
		}
		seen[item.Name] = true
	}
	conn, err := h.skillUpdateConnection(r)
	if err != nil {
		writeRemoteSkillsHubError(w, err)
		return
	}
	results := []updates.Result{}
	for _, decision := range req.Items {
		info, err := h.checkSkillUpdate(r, conn, decision.Name, false)
		if err == nil {
			var applied []updates.Result
			applied, err = updates.Apply(r.Context(), conn, decision, info, func(target updates.Target, apply func() error) error {
				if target.ID == "library" {
					return apply()
				}
				if h.agentRuntime == nil {
					return errors.New("agent service is unavailable")
				}
				return h.agentRuntime.WithAgentLifecycle(r.Context(), target.ID, func(context.Context) error {
					current, err := h.agentEngine.Agents().Get(r.Context(), target.ID, agentengine.AgentGetOptions{})
					if err != nil || !slices.Contains(current.Spec.Skills, decision.Name) {
						return installstate.ErrInstallationChanged
					}
					return apply()
				})
			})
			results = append(results, applied...)
		}
		if err != nil {
			results = append(results, updates.Result{Name: decision.Name, Status: "failed", Error: skillUpdateErrorCode(err)})
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Results []updates.Result `json:"results"`
	}{results})
}

func skillUpdateErrorCode(err error) string {
	if errors.Is(err, installstate.ErrInstallationChanged) {
		return "skill_installation_changed"
	}
	for _, code := range []string{"skill_update_source_unavailable", "skill_release_changed"} {
		if strings.Contains(err.Error(), code) {
			return code
		}
	}
	return "skill_update_failed"
}

func writeSkillUpdateError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, os.ErrNotExist) {
		status = http.StatusNotFound
	}
	code := skillUpdateErrorCode(err)
	if code == "skill_installation_changed" || code == "skill_release_changed" {
		status = http.StatusConflict
	}
	if code == "skill_update_source_unavailable" {
		status = http.StatusBadRequest
	}
	writeCodedAPIError(w, status, code, code)
}
