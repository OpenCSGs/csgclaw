package api

import (
	"context"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apps"
	cliprojection "csgclaw/internal/runtimeextension/connectorcli"
	projection "csgclaw/internal/runtimeextension/larkcli"
	skill "csgclaw/internal/skill/state"
)

type connectorRuntime struct{ h *Handler }

func (b connectorRuntime) State(_ context.Context, id, kind string) (bool, error) {
	if b.h.svc == nil {
		return false, fmt.Errorf("Agent service is unavailable")
	}
	a, ok := b.h.svc.Agent(id)
	if !ok {
		return false, apps.ErrNotFound
	}
	if kind == apps.KindCLI && (a.SandboxEnabled || (a.RuntimeKind != agent.RuntimeKindCodex && a.RuntimeKind != agent.RuntimeKindDSH)) {
		return false, fmt.Errorf("CLI connectors require a Codex or DSH host runtime")
	}
	return a.Status == "running", nil
}
func (b connectorRuntime) Update(ctx context.Context, id string, resume bool, apply func(context.Context) error) error {
	owner, ok := b.h.agentEngine.(interface {
		UpdateRuntimeConfiguration(context.Context, string, bool, func(context.Context) error) error
	})
	if !ok {
		return fmt.Errorf("runtime configuration service is unavailable")
	}
	return owner.UpdateRuntimeConfiguration(ctx, id, resume, apply)
}
func (b connectorRuntime) Configure(ctx context.Context, id, resourceID string, p *apps.CLIProjection) error {
	extensions := b.h.agentEngine.RuntimeExtensions(id)
	name, err := b.h.apps.CLIExtensionName(resourceID)
	if err != nil {
		return err
	}
	if p == nil {
		err := extensions.Delete(ctx, name)
		if agentengine.ErrorCodeOf(err) == agentengine.ErrorRuntimeExtensionNotFound {
			return nil
		}
		return err
	}
	_, err = extensions.Apply(ctx, agentengine.RuntimeExtensionApplyRequest{Spec: agentengine.RuntimeExtensionSpec{Name: name, Kind: cliprojection.Kind, Source: agentengine.RuntimeExtensionSourceRef{Provider: cliprojection.SourceProvider, Ref: p.ResourceID}, FailurePolicy: agentengine.RuntimeExtensionBlockRuntime}})
	return err
}
func (b connectorRuntime) PlanSkills(id, bundle, version string, previous []skill.ManagedSkill, enabled bool) (skill.SkillPlan, error) {
	owner, ok := b.h.svc.(interface {
		PlanConnectorSkills(string, string, string, []skill.ManagedSkill, bool) (skill.SkillPlan, error)
	})
	if !ok {
		return skill.SkillPlan{}, fmt.Errorf("connector Skills service is unavailable")
	}
	return owner.PlanConnectorSkills(id, bundle, version, previous, enabled)
}
func (b connectorRuntime) ApplySkills(ctx context.Context, id string, plan skill.SkillPlan) error {
	owner, ok := b.h.svc.(interface {
		ApplyConnectorSkills(context.Context, string, skill.SkillPlan) error
	})
	if !ok {
		return fmt.Errorf("connector Skills service is unavailable")
	}
	return owner.ApplyConnectorSkills(ctx, id, plan)
}

type connectorCLISource struct {
	service *apps.Service
	h       *Handler
}

func (s connectorCLISource) Resolve(_ context.Context, agentID, ref string) (agentengine.ResolvedRuntimeExtension, error) {
	p, revision, err := s.service.CLIProjection(agentID, ref)
	if err != nil {
		return agentengine.ResolvedRuntimeExtension{}, err
	}
	if p == nil || p.ResourceID != ref {
		return agentengine.ResolvedRuntimeExtension{}, fmt.Errorf("CLI connector binding is unavailable")
	}
	if p.ConnectorID == "feishu" && s.h != nil {
		var status struct {
			AppID string `json:"appId"`
		}
		_ = json.Unmarshal(p.Status, &status)
		if info, err := s.h.feishuBotAppInfoForAgent(agentID); err == nil && info.AppID == status.AppID && status.AppID != "" {
			p.Instructions = projection.HistoricalAttachmentInstructions()
		}
	}
	payload, err := json.Marshal(p.Payload)
	return agentengine.ResolvedRuntimeExtension{SourceRevision: strconv.FormatUint(revision, 10) + "-" + strconv.FormatBool(p.Instructions != ""), Payload: payload}, err
}
func (h *Handler) handleConnectorCLI(w http.ResponseWriter, r *http.Request) {
	if h.apps == nil {
		writeCodedAPIError(w, http.StatusServiceUnavailable, "apps_unavailable", "App service is unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	id := pathValue(r, "resource_id")
	session := pathValue(r, "session_id")
	if session != "" {
		var result apps.CLIOperation
		var err error
		if r.Method == http.MethodDelete {
			result, err = h.apps.CancelCLI(r.Context(), id, session)
		} else {
			result, err = h.apps.CLISession(r.Context(), id, session)
		}
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	var req apps.CLIRequest
	if !decodeConnectorRequest(w, r, &req) {
		return
	}
	req.Action = pathValue(r, "cli_action")
	result, err := h.apps.StartCLI(r.Context(), id, req)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (b connectorRuntime) RestoreLegacy(ctx context.Context, id string, raw json.RawMessage) error {
	if len(raw) == 0 {
		err := b.h.agentEngine.RuntimeExtensions(id).Delete(ctx, projection.Name)
		if agentengine.ErrorCodeOf(err) == agentengine.ErrorRuntimeExtensionNotFound {
			return nil
		}
		return err
	}
	var previous agentengine.RuntimeExtension
	if err := json.Unmarshal(raw, &previous); err != nil {
		return err
	}
	_, err := b.h.agentEngine.RuntimeExtensions(id).Apply(ctx, agentengine.RuntimeExtensionApplyRequest{Spec: previous.Spec})
	return err
}
func (b connectorRuntime) Verify(ctx context.Context, id string) (bool, error) {
	item, err := b.h.agentEngine.RuntimeExtensions(id).Get(ctx, projection.Name)
	if agentengine.ErrorCodeOf(err) == agentengine.ErrorRuntimeExtensionNotFound {
		// A running worker also verifies a migration that removed the extension.
		return b.State(ctx, id, apps.KindCLI)
	}
	if err != nil {
		return false, err
	}
	return item.Status.State == agentengine.RuntimeExtensionConfigured && item.Status.RuntimeLoaded, nil
}

func (h *Handler) PrepareConnectorMigration(ctx context.Context) error {
	if h.apps == nil || h.agentEngine == nil || h.apps.CLIMigrationPrepared() {
		return nil
	}
	owner, ok := h.svc.(interface {
		ListRecords() []agent.Agent
		RuntimeConfigurationProjections(string) ([]agentruntime.ExtensionProjection, error)
	})
	if !ok {
		return nil
	}
	var candidates []apps.LegacyWorker
	for _, item := range owner.ListRecords() {
		if item.SandboxEnabled || (item.RuntimeKind != agent.RuntimeKindCodex && item.RuntimeKind != agent.RuntimeKindDSH) {
			continue
		}
		candidate := apps.LegacyWorker{AgentID: item.ID, Running: legacyWorkerRunning(item)}
		extension, err := h.agentEngine.RuntimeExtensions(item.ID).Get(ctx, projection.Name)
		found := false
		if err == nil {
			if extension.Spec.Source.Provider == cliprojection.SourceProvider {
				continue
			}
			candidate.Extension, err = json.Marshal(extension)
			if err != nil {
				return err
			}
			found = true
		} else if agentengine.ErrorCodeOf(err) != agentengine.ErrorRuntimeExtensionNotFound {
			return err
		}
		if h.participant != nil && len(h.participant.List(participant.ListOptions{Channel: participant.ChannelFeishu, AgentID: item.ID, Type: participant.TypeAgent})) > 0 {
			found = true
		}
		projections, err := owner.RuntimeConfigurationProjections(item.ID)
		if err != nil {
			return err
		}
		for _, current := range projections {
			if current.Name != projection.Name {
				continue
			}
			candidate.Projection = &current
			candidate.ConfigFile = filepath.Join(current.Environment["LARKSUITE_CLI_CONFIG_DIR"], projection.WorkspaceName, projection.ConfigFileName)
			found = true
			break
		}
		if found {
			candidates = append(candidates, candidate)
		}
	}
	return h.apps.PrepareCLIMigration(ctx, candidates)
}

func legacyWorkerRunning(item agent.Agent) bool {
	switch item.DesiredState {
	case agent.DesiredStateRunning:
		return true
	case agent.DesiredStateStopped:
		return false
	default:
		return item.Status == "running"
	}
}

func (h *Handler) RestoreConnectorMigration(ctx context.Context) error {
	if h.apps == nil {
		return nil
	}
	return h.apps.RestoreCLIMigration(ctx)
}

type legacyConnectorSource struct {
	h      *Handler
	source agentengine.RuntimeExtensionSource
}

func (s legacyConnectorSource) Resolve(ctx context.Context, id, ref string) (agentengine.ResolvedRuntimeExtension, error) {
	if s.h.apps != nil {
		if p, revision, ok := s.h.apps.LegacyCLIProjection(id); ok {
			payload, err := projection.Encode(*p)
			return agentengine.ResolvedRuntimeExtension{SourceRevision: revision, Payload: payload}, err
		}
	}
	return s.source.Resolve(ctx, id, ref)
}

func (h *Handler) connectorRuntimeStatus(id, resourceID string) string {
	if h.agentEngine == nil {
		return "unavailable"
	}
	if h.apps != nil {
		if err := h.apps.CheckCLIReady(id); err != nil {
			return "pending"
		}
	}
	name, err := h.apps.CLIExtensionName(resourceID)
	if err != nil {
		return "unconfigured"
	}
	current, err := h.agentEngine.RuntimeExtensions(id).Get(context.Background(), name)
	if err != nil {
		return "unconfigured"
	}
	if current.Status.State != agentengine.RuntimeExtensionConfigured {
		return "error"
	}
	if current.Status.RuntimeLoaded {
		return "loaded"
	}
	return "configured"
}
