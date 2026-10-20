package conversationresource

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/apps"
	"csgclaw/internal/knowledgebase"
	"csgclaw/internal/mcpschema"
	"csgclaw/internal/resourcequery"
	skillstate "csgclaw/internal/skill/state"
)

type AgentReader interface {
	Get(context.Context, string, contract.AgentGetOptions) (contract.Agent, error)
}
type AppReader interface {
	List(context.Context, string) ([]apps.Installation, error)
	CallableTools(context.Context, string, string) ([]apps.CallableTool, error)
}
type Service struct {
	agents AgentReader
	apps   AppReader
}

func New(agents AgentReader, apps AppReader) *Service { return &Service{agents: agents, apps: apps} }

func (s *Service) getAgent(ctx context.Context, agentID string, options contract.AgentGetOptions) (contract.Agent, error) {
	agent, err := s.agents.Get(ctx, agentID, options)
	if err != nil {
		return contract.Agent{}, readError(err)
	}
	if agent.Spec.Runtime.Adapter != "codex" && agent.Spec.Runtime.Adapter != "dsh" {
		return contract.Agent{}, &Error{Code: InvalidRequest}
	}
	return agent, nil
}

func (s *Service) List(ctx context.Context, agentID string, kind apitypes.ResourceType, query resourcequery.Query) ([]apitypes.AgentResourceSummary, resourcequery.Metadata, error) {
	if kind != apitypes.ResourceTypeSkill && kind != apitypes.ResourceTypeConnector && kind != apitypes.ResourceTypeKnowledgeBase {
		return nil, resourcequery.Metadata{}, resourcequery.ErrInvalid
	}
	options := contract.AgentGetOptions{AdoptMCPServers: kind != apitypes.ResourceTypeSkill}
	if kind == apitypes.ResourceTypeSkill {
		q := query
		q.Exclude = nil
		for _, key := range query.Exclude {
			if strings.HasPrefix(key, "skill:") {
				q.Exclude = append(q.Exclude, strings.TrimPrefix(key, "skill:"))
			}
		}
		options.IncludeSkillSummaries, options.SkillQuery = true, &q
	}
	agent, err := s.getAgent(ctx, agentID, options)
	if err != nil {
		return nil, resourcequery.Metadata{}, err
	}
	if kind == apitypes.ResourceTypeSkill {
		items := []apitypes.AgentResourceSummary{}
		for _, summary := range agent.Status.SkillSummaries {
			items = append(items, resourceSummary(apitypes.ResourceSourceSkill, summary.Name, kind, summary.Name, summary.Description, summary.Enabled))
		}
		if agent.Status.SkillPage == nil {
			return nil, resourcequery.Metadata{}, &Error{Code: ResourceUnavailable}
		}
		return items, *agent.Status.SkillPage, nil
	}
	items := mcpSummaries(agent)
	appItems, _, err := s.appSummaries(ctx, agentID)
	if err != nil {
		return nil, resourcequery.Metadata{}, readError(err)
	}
	items = append(items, appItems...)
	entries := []resourcequery.Entry{}
	byID := map[string]apitypes.AgentResourceSummary{}
	for _, item := range items {
		key := string(item.Source) + ":" + item.ID
		if item.Type != kind || (query.Enabled != nil && item.Enabled != *query.Enabled) || slices.Contains(query.Exclude, key) {
			continue
		}
		if !resourcequery.Match(item.Name, query.Search) && !resourcequery.Match(item.ID, query.Search) && !resourcequery.Match(item.Description, query.Search) {
			continue
		}
		byID[key] = item
		entries = append(entries, resourcequery.Entry{Name: key, Enabled: item.Enabled})
	}
	scope, _ := json.Marshal(struct {
		Agent   string
		Type    apitypes.ResourceType
		Search  string
		Enabled *bool
		Exclude []string
	}{agentID, kind, query.Search, query.Enabled, query.Exclude})
	query.Search, query.Enabled, query.Exclude = "", nil, nil
	keys, meta, err := resourcequery.Select(entries, query, string(scope))
	if err != nil {
		return nil, meta, err
	}
	result := make([]apitypes.AgentResourceSummary, 0, len(keys))
	for _, key := range keys {
		result = append(result, byID[key])
	}
	return result, meta, nil
}

func (s *Service) Resolve(ctx context.Context, agentID string, refs []apitypes.ResourceReference) (ResolvedSelection, error) {
	input, err := Normalize("", refs)
	if err != nil {
		return ResolvedSelection{}, err
	}
	result := ResolvedSelection{AgentID: agentID, Version: 1, Resources: []ResolvedResource{}}
	if len(input.Refs) == 0 {
		return result, nil
	}
	agent, err := s.getAgent(ctx, agentID, contract.AgentGetOptions{
		AdoptMCPServers: slices.ContainsFunc(input.Refs, func(ref apitypes.ResourceReference) bool { return ref.Source == apitypes.ResourceSourceMCP }),
	})
	if err != nil {
		return result, err
	}
	candidates := map[apitypes.ResourceReference]apitypes.AgentResourceSummary{}
	for _, slug := range agent.Spec.Skills {
		item := resourceSummary(apitypes.ResourceSourceSkill, slug, apitypes.ResourceTypeSkill, slug, "", skillstate.Enabled(agent.Spec.SkillStates, slug))
		candidates[item.ResourceReference] = item
	}
	for _, item := range mcpSummaries(agent) {
		candidates[item.ResourceReference] = item
	}
	toolNames := map[string][]string{}
	if slices.ContainsFunc(input.Refs, func(ref apitypes.ResourceReference) bool { return ref.Source == apitypes.ResourceSourceApp }) {
		items, names, err := s.appSummaries(ctx, agentID)
		if err != nil {
			return result, readError(err)
		}
		toolNames = names
		for _, item := range items {
			candidates[item.ResourceReference] = item
		}
	}
	for _, ref := range input.Refs {
		item, ok := candidates[ref]
		if !ok {
			return result, &Error{Code: ResourceNotAttached, Reference: ref}
		}
		if !item.Enabled {
			return result, &Error{Code: ResourceDisabled, Reference: ref}
		}
		if !item.Available {
			return result, &Error{Code: ResourceUnavailable, Reference: ref}
		}
		resolved := ResolvedResource{ResourceSnapshot: item.ResourceSnapshot}
		if ref.Source == apitypes.ResourceSourceApp {
			resolved.ToolNames = append([]string(nil), toolNames[ref.ID]...)
		}
		result.Resources = append(result.Resources, resolved)
	}
	return result, nil
}

func resourceSummary(source apitypes.ResourceSource, id string, kind apitypes.ResourceType, name, description string, enabled bool) apitypes.AgentResourceSummary {
	item := apitypes.AgentResourceSummary{ResourceSnapshot: apitypes.ResourceSnapshot{ResourceReference: apitypes.ResourceReference{Source: source, ID: id}, Type: kind, Name: name}, Description: description, Enabled: enabled, Available: enabled}
	if !enabled {
		item.UnavailableReason = "disabled"
	}
	return item
}

func mcpSummaries(agent contract.Agent) []apitypes.AgentResourceSummary {
	result := []apitypes.AgentResourceSummary{}
	for key, config := range agent.Spec.MCPServers {
		kind := apitypes.ResourceTypeConnector
		if _, ok := knowledgebase.ManagedMetadataFromServer(map[string]any(config)); ok {
			kind = apitypes.ResourceTypeKnowledgeBase
		}
		name, _ := config["display_name"].(string)
		if strings.TrimSpace(name) == "" {
			name = key
		}
		description, _ := config["description"].(string)
		result = append(result, resourceSummary(apitypes.ResourceSourceMCP, key, kind, name, description, mcpschema.ServerEnabled(config)))
	}
	return result
}

func (s *Service) appSummaries(ctx context.Context, agentID string) ([]apitypes.AgentResourceSummary, map[string][]string, error) {
	result := []apitypes.AgentResourceSummary{}
	names := map[string][]string{}
	if s.apps == nil {
		return result, names, nil
	}
	items, err := s.apps.List(ctx, agentID)
	if err != nil {
		return nil, nil, readError(err)
	}
	for _, app := range items {
		kind := apitypes.ResourceTypeConnector
		if app.AppID == "llm-wiki" {
			kind = apitypes.ResourceTypeKnowledgeBase
		}
		item := resourceSummary(apitypes.ResourceSourceApp, app.InstallationID, kind, app.Name, app.AppID, app.Enabled && app.ResourceEnabled)
		if item.Enabled {
			item.Available = false
			switch {
			case app.Disconnected:
				item.UnavailableReason = "disconnected"
			case app.Status != "connected":
				item.UnavailableReason = app.Status
				if item.UnavailableReason == "" || item.UnavailableReason == "error" {
					item.UnavailableReason = "connection_error"
				}
			default:
				tools, err := s.apps.CallableTools(ctx, agentID, app.InstallationID)
				if err != nil {
					return nil, nil, readError(err)
				}
				for _, tool := range tools {
					names[app.InstallationID] = append(names[app.InstallationID], tool.Name)
				}
				item.Available = len(tools) > 0
				if !item.Available {
					item.UnavailableReason = "no_tools"
				}
			}
		}
		result = append(result, item)
	}
	return result, names, nil
}
