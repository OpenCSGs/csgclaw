package relay

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"sync"
	"time"
)

// CatalogEntry is a browser-safe discovery record. ConnectionID is an opaque
// server/login reference; it never contains credentials.
type CatalogEntry struct {
	Recommended        bool   `json:"recommended"`
	RelayURL           string `json:"relay_url"`
	ConnectionID       string `json:"connection_id"`
	Agent              Agent  `json:"agent"`
	Avatar             string `json:"avatar,omitempty"`
	Source             string `json:"source,omitempty"`
	Type               string `json:"type,omitempty"`
	CreatorName        string `json:"creator_name,omitempty"`
	ModelID            string `json:"model_id,omitempty"`
	TemplateRepoPath   string `json:"template_repo_path,omitempty"`
	OwnedByCurrentUser bool   `json:"owned_by_current_user,omitempty"`
}

// Discover reads community instances and resolves their subscription IDs through
// Relay. The local catalog is retained for standalone protocol diagnostics.
func (m *Manager) Discover(ctx context.Context) ([]CatalogEntry, error) {
	return m.discover(ctx, "", nil)
}

// DiscoverCatalog probes only the requested IDs. An empty slice loads metadata only.
func (m *Manager) DiscoverCatalog(ctx context.Context, probeIDs []string) ([]CatalogEntry, error) {
	if probeIDs == nil {
		probeIDs = []string{}
	}
	return m.discover(ctx, "", probeIDs)
}

// DiscoverAgent probes only the requested registration.
func (m *Manager) DiscoverAgent(ctx context.Context, id, connectionID string) (CatalogEntry, error) {
	entries, err := m.discover(ctx, id, nil)
	if err != nil {
		return CatalogEntry{}, err
	}
	for _, entry := range entries {
		if entry.Agent.ID == id && entry.ConnectionID == connectionID {
			return entry, nil
		}
	}
	return CatalogEntry{}, ErrChatReference
}

func (m *Manager) discover(ctx context.Context, onlyID string, probeIDs []string) ([]CatalogEntry, error) {
	if m.community == nil {
		return m.catalog(ctx, onlyID, probeIDs), nil
	}
	m.mu.Lock()
	c, err := m.client(m.manifest.Endpoint, m.manifest.Token)
	connection, refErr := m.connectionReference()
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if refErr != nil {
		return nil, refErr
	}
	instances, err := m.community.List(ctx)
	if err != nil {
		return nil, err
	}
	if onlyID != "" {
		selected := make([]CommunityInstance, 0, 1)
		for _, item := range instances {
			if item.relayID() == onlyID {
				selected = append(selected, item)
			}
		}
		instances = selected
	}
	probeSet := make(map[string]bool, len(probeIDs))
	for _, id := range probeIDs {
		probeSet[id] = true
	}
	if len(probeSet) > 0 {
		selected := instances[:0]
		for _, item := range instances {
			if probeSet[item.relayID()] {
				selected = append(selected, item)
			}
		}
		instances = selected
	}
	entries := make([]CatalogEntry, len(instances))
	jobs := make(chan int)
	var wg sync.WaitGroup
	// Bound fanout while avoiding one slow registration blocking every later one.
	for range min(8, len(instances)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				item := instances[index]
				id := item.relayID()
				a := Agent{ID: id, Status: "offline"}
				if (probeIDs == nil || len(probeSet) > 0) && ctx.Err() == nil {
					probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
					var live Agent
					probeErr := c.Request(probeCtx, http.MethodGet, "/v1/agents/"+url.PathEscape(id), "", nil, &live)
					if probeErr == nil && probeCtx.Err() == nil && live.ID == id &&
						(live.Status == "online" || live.Status == "offline" || live.Status == "unhealthy") {
						a = live
					}
					cancel()
				}
				// Community controls discovery visibility, even if Relay's copy differs.
				a.Visibility = "private"
				if item.Public {
					a.Visibility = "public"
				}
				a.Name, a.Description = item.Name, item.Description
				entry := CatalogEntry{
					RelayURL: c.base.String(), ConnectionID: connection, Agent: a, Avatar: item.Metadata.Avatar,
					Source: item.Source, Type: item.Type, CreatorName: item.CreatorName, ModelID: item.Metadata.ProvisionRequest.LLM.Model,
					OwnedByCurrentUser: item.Editable,
					Recommended:        item.Recommended,
				}
				if item.Source == "platform" && item.TemplateID > 0 {
					entry.TemplateRepoPath = validatedTemplateRepoPath(item.Metadata.TemplateMetadata.RepoPath)
				}
				entries[index] = entry
			}
		}()
	}
	for index := range instances {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return entries, nil
}
func (m *Manager) Catalog(ctx context.Context) []CatalogEntry {
	return m.catalog(ctx, "", nil)
}

func (m *Manager) catalog(ctx context.Context, onlyID string, probeIDs []string) []CatalogEntry {
	m.mu.Lock()
	entries := make([]CatalogEntry, 0, len(m.manifest.Agents))
	connected := make(map[string]bool, len(m.running))
	for id, req := range m.manifest.Agents {
		if req.Visibility != "public" || (onlyID != "" && id != onlyID) {
			continue
		}
		if len(probeIDs) > 0 && !slices.Contains(probeIDs, id) {
			continue
		}
		entry := m.manifest.Catalog[id]
		if entry.Agent.ID == "" {
			entry.Agent = Agent{ID: id, Name: req.Name, Description: req.Description, SourceAgentID: req.SourceAgentID, Visibility: req.Visibility, Executor: req.Executor}
		}
		entry.RelayURL = m.manifest.Endpoint
		entry.ConnectionID = m.manifest.ConnectionID
		if entry.Type == "" {
			entry.Type = "csgclaw"
		}
		entries = append(entries, entry)
		connected[id] = m.running[id] != nil && m.running[id].connected
	}
	endpoint, token := m.manifest.Endpoint, m.manifest.Token
	m.mu.Unlock()
	client, clientErr := NewClient(endpoint, token, nil)
	for i := range entries {
		entry := &entries[i]
		entry.Agent.Status = "offline"
		entry.Agent.Busy = false
		if clientErr != nil || (probeIDs != nil && len(probeIDs) == 0) || ctx.Err() != nil {
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		var live Agent
		err := client.Request(probeCtx, http.MethodGet, "/v1/agents/"+url.PathEscape(entry.Agent.ID), "", nil, &live)
		cancel()
		if err == nil && live.ID == entry.Agent.ID {
			entry.Agent = live
			if live.Status != "online" || !connected[live.ID] {
				entry.Agent.Status = "offline"
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Agent.ID < entries[j].Agent.ID })
	return entries
}
