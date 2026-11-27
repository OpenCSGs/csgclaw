package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/url"
	"os"
	"reflect"
	"strings"
	"unicode/utf8"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/knowledgebase"
	"csgclaw/internal/mcp"
	"csgclaw/internal/mcpschema"
	agentruntime "csgclaw/internal/runtime"
	runtimeinstructions "csgclaw/internal/runtime/instructions"
	hub "csgclaw/internal/template"
	"csgclaw/internal/utils"
)

func (s *Controller) refreshWorkerTemplate(ctx context.Context, id, ref string) (_ contract.Agent, retErr error) {
	ctx, release, err := s.acquireAgentLifecycle(ctx, id)
	if err != nil {
		return contract.Agent{}, err
	}
	defer release()
	service, _ := ctx.Value(localTemplateServiceContextKey{}).(*hub.Service)
	if service == nil {
		s.mu.RLock()
		service, _ = s.hub.(*hub.Service)
		s.mu.RUnlock()
	}
	if service == nil {
		return contract.Agent{}, fmt.Errorf("hub service is required for template refresh")
	}
	overlay, err := service.FetchWorkerOverlay(ctx, ref)
	if err != nil {
		return contract.Agent{}, err
	}
	if overlay.Workspace.Temporary {
		defer os.RemoveAll(overlay.Workspace.Path)
	}
	current, ok := s.Agent(id)
	if !ok || current.Role != RoleWorker || current.ArchivedAt != nil {
		return contract.Agent{}, fmt.Errorf("active worker %q is required for template refresh", id)
	}
	if !strings.EqualFold(current.RuntimeKind, overlay.RuntimeKind) {
		return contract.Agent{}, fmt.Errorf("template runtime %q differs from worker runtime %q", overlay.RuntimeKind, current.RuntimeKind)
	}
	update, err := s.templateRefreshConfig(ctx, current, overlay)
	if err != nil {
		return contract.Agent{}, err
	}
	layout, err := s.Workspace().AgentLayout(id)
	if err != nil {
		return contract.Agent{}, err
	}
	home, err := s.Workspace().agentHomeDir(id)
	if err != nil {
		return contract.Agent{}, err
	}
	if err := os.MkdirAll(home, 0700); err != nil {
		return contract.Agent{}, err
	}
	s.mu.RLock()
	seedMemory := s.memoryTemplate
	s.mu.RUnlock()
	files, err := newTemplateFileUpdate(home)
	if err != nil {
		return contract.Agent{}, err
	}
	defer files.close()
	instructions, err := files.prepare(ctx, overlay.Workspace, layout, seedMemory != nil)
	if err != nil {
		return contract.Agent{}, err
	}
	if instructions != nil {
		begin, _ := runtimeinstructions.AgentsInstructionsBlockMarkers()
		if strings.Contains(*instructions, begin) {
			value := runtimeinstructions.ExtractUserInstructionsFromAgentsDocument(*instructions)
			if value != current.Instructions || files.instructionsChanged {
				update.Instructions = &value
			}
		} else if files.instructionsChanged {
			update.Instructions = &current.Instructions
		}
		if update.Instructions != nil {
			update.FieldMask = append(update.FieldMask, "instructions")
		}
	}
	if err := files.apply(); err != nil {
		return contract.Agent{}, err
	}
	previous := current
	// The normal configuration path may commit desired state before a runtime
	// call fails. Keep its matching files and retry live reconciliation on replay.
	committed := false
	defer func() {
		if retErr != nil && committed {
			_, pendingErr := s.markResourceRestart(id)
			retErr = errors.Join(retErr, pendingErr)
		}
	}()
	if len(update.FieldMask) != 0 {
		current, err = s.update(context.WithValue(ctx, deferResourceRestartKey{}, true), id, update)
		if err != nil {
			actual, ok := s.Agent(id)
			committed = ok && actual.UpdatedAt != previous.UpdatedAt
			if committed {
				return contract.Agent{}, err
			}
			return contract.Agent{}, errors.Join(err, files.rollback())
		}
	}
	committed = true
	if update.MCPServersSet {
		if err := s.reconcileMCPServers(ctx, previous, current); err != nil {
			return contract.Agent{}, err
		}
	}
	if seedMemory != nil && overlay.Workspace.MemoryPath != "" {
		memory, err := os.ReadFile(overlay.Workspace.MemoryPath)
		if err != nil {
			return contract.Agent{}, err
		}
		if err := seedMemory(id, string(memory)); err != nil {
			return contract.Agent{}, err
		}
	}
	skillsChanged, err := files.applySkills(ctx, layout)
	if err != nil {
		return contract.Agent{}, err
	}
	if len(files.skills) > 0 {
		if err := s.reconcileSkillStates(ctx, current); err != nil {
			return contract.Agent{}, err
		}
	}
	needsRestart := skillsChanged || len(files.entries) > 0 || previous.AgentProfile.EnvRestartRequired || current.AgentProfile.EnvRestartRequired || update.Instructions != nil
	if needsRestart && current.DesiredState == DesiredStateRunning {
		current, err = s.restartRuntime(agentruntime.WithInstalledSkills(ctx), id)
		if err != nil {
			return contract.Agent{}, err
		}
	}
	return s.convert(current)
}

func (s *Controller) templateRefreshConfig(ctx context.Context, current Agent, overlay hub.WorkerOverlay) (UpdateRequest, error) {
	var update UpdateRequest
	if overlay.Description != nil && *overlay.Description != current.Description {
		update.Description = overlay.Description
		update.FieldMask = append(update.FieldMask, "description")
	}
	if overlay.Avatar != nil && *overlay.Avatar != current.Avatar {
		update.Avatar = overlay.Avatar
		update.FieldMask = append(update.FieldMask, "avatar")
	}
	if overlay.Image.Ref != nil && *overlay.Image.Ref != current.Image {
		update.Image = overlay.Image.Ref
		update.FieldMask = append(update.FieldMask, "image")
	}
	opts := utils.OverlayAnyMap(maps.Clone(current.RuntimeOptions), overlay.RuntimeOptions)
	if !reflect.DeepEqual(opts, current.RuntimeOptions) {
		update.RuntimeOptions = &opts
		update.FieldMask = append(update.FieldMask, "runtime_options")
	}
	profile := cloneProfile(current.AgentProfile)
	if overlay.Model.ID != nil {
		profile.ModelID = *overlay.Model.ID
	}
	if overlay.Model.ReasoningEffort != nil {
		profile.ReasoningEffort = *overlay.Model.ReasoningEffort
	}
	for _, env := range overlay.Image.Env {
		if env.Default == nil {
			continue
		}
		if len(profile.Env) == 0 {
			profile.Env = map[string]string{}
		}
		profile.Env[strings.TrimSpace(env.Name)] = *env.Default
	}
	if !reflect.DeepEqual(profile, current.AgentProfile) {
		update.AgentProfile = &profile
		update.FieldMask = append(update.FieldMask, "agent_profile")
	}
	servers, err := templateServers(ctx, overlay.Workspace.MCPServersJSON)
	if err != nil {
		return UpdateRequest{}, err
	}
	if len(servers) > 0 {
		stored, err := s.currentMCPServersForManagement(ctx, current)
		if err != nil {
			return UpdateRequest{}, err
		}
		servers, err = mcpschema.TemplateServerIDs(stored, servers)
		if err != nil {
			return UpdateRequest{}, err
		}
		merged := mergeTemplateMCPServers(stored, servers)
		if !reflect.DeepEqual(stored, merged) || current.AgentProfile.EnvRestartRequired {
			update.MCPServers, update.MCPServersSet = &merged, true
			update.FieldMask = append(update.FieldMask, "mcpServers")
		}
	}
	return update, nil
}

// Authentication inherited from a worker belongs to its original HTTP origin.
// A template can supply authentication for a new endpoint explicitly.
func mergeTemplateMCPServers(stored, incoming map[string]any) map[string]any {
	base := maps.Clone(stored)
	for id, raw := range incoming {
		patch, _ := raw.(map[string]any)
		nextURL, supplied := patch["url"]
		current, ok := stored[id].(map[string]any)
		if !ok || !supplied {
			continue
		}
		if origin := mcpHTTPOrigin(current["url"]); origin != "" && origin == mcpHTTPOrigin(nextURL) {
			continue
		}
		current = maps.Clone(current)
		for _, key := range []string{"headers", "http_headers", "env_http_headers", "bearer_token_env_var", "oauth_client_id", "oauth_resource"} {
			delete(current, key)
		}
		base[id] = current
	}
	return overlayConfigMap(base, incoming)
}

func mcpHTTPOrigin(raw any) string {
	address, _ := raw.(string)
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	host := u.Hostname()
	// DNS case folding is safe for ASCII names. Keep Unicode names and IPv6
	// zone identifiers exact so distinct hosts cannot acquire the same origin.
	if !strings.Contains(host, "%") && len(host) == utf8.RuneCountInString(host) {
		host = strings.ToLower(host)
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	return u.Scheme + "://" + net.JoinHostPort(host, port)
}

func overlayConfigMap(current, overlay map[string]any) map[string]any {
	out := maps.Clone(current)
	if out == nil {
		out = make(map[string]any, len(overlay))
	}
	for name, value := range overlay {
		if fields, ok := value.(map[string]any); ok {
			existing, _ := out[name].(map[string]any)
			value = overlayConfigMap(existing, fields)
		}
		out[name] = value
	}
	return out
}

func templateServers(ctx context.Context, document string) (map[string]any, error) {
	if document == "" {
		return nil, nil
	}
	var servers map[string]any
	if err := json.Unmarshal([]byte(document), &servers); err != nil {
		return nil, fmt.Errorf("decode template mcp servers: %w", err)
	}
	servers, err := knowledgebase.HydrateTemplateServers(ctx, servers)
	if err != nil {
		return nil, err
	}
	if availability, ok := ctx.Value(templateMCPAvailabilityContextKey{}).(templateMCPAvailabilityContext); ok {
		var skipped []mcp.SkippedTemplateResource
		servers, skipped = availability.service.FilterAvailableTemplateServers(ctx, servers)
		*availability.skipped = append(*availability.skipped, skipped...)
	}
	return servers, nil
}
