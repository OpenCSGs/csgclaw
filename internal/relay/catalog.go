package relay

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"
)

// CatalogEntry is a browser-safe discovery record. ConnectionID is an opaque
// server/login reference; it never contains credentials.
type CatalogEntry struct {
	RelayURL     string `json:"relay_url"`
	ConnectionID string `json:"connection_id"`
	Agent        Agent  `json:"agent"`
	Avatar       string `json:"avatar,omitempty"`
	Source       string `json:"source,omitempty"`
	Type         string `json:"type,omitempty"`
}

// Discover reads community instances and resolves their subscription IDs through
// Relay. The local catalog is retained for standalone protocol diagnostics.
func (m *Manager) Discover(ctx context.Context) ([]CatalogEntry, error) {
	return m.discover(ctx, "")
}

// DiscoverAgent probes only the requested registration when starting a task.
func (m *Manager) DiscoverAgent(ctx context.Context, id, connectionID string) (CatalogEntry, error) {
	entries, err := m.discover(ctx, id)
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

func (m *Manager) discover(ctx context.Context, onlyID string) ([]CatalogEntry, error) {
	if m.community == nil {
		return m.Catalog(ctx), nil
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
	entries := make([]CatalogEntry, len(instances))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var probeFailure error
	var failureMu sync.Mutex
	// Bound fanout while avoiding one slow registration blocking every later one.
	for range min(8, len(instances)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				item := instances[index]
				id := item.relayID()
				a := Agent{ID: id, Status: "offline"}
				probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				var live Agent
				probeErr := c.Request(probeCtx, http.MethodGet, "/v1/agents/"+url.PathEscape(id), "", nil, &live)
				var availabilityErr error
				if probeCtx.Err() != nil {
					availabilityErr = fmt.Errorf("relay availability query timed out or was cancelled: %w", probeCtx.Err())
				} else if probeErr != nil && !isNotFound(probeErr) {
					availabilityErr = fmt.Errorf("relay availability query failed: %w", probeErr)
				} else if probeErr == nil {
					if live.ID != id || (live.Status != "online" && live.Status != "offline" && live.Status != "unhealthy") {
						availabilityErr = fmt.Errorf("relay returned an invalid availability response")
					} else {
						a = live
					}
				}
				cancel()
				if availabilityErr != nil {
					failureMu.Lock()
					probeFailure = availabilityErr
					failureMu.Unlock()
				}
				// Community controls discovery visibility, even if Relay's copy differs.
				a.Visibility = "private"
				if item.Public {
					a.Visibility = "public"
				}
				a.Name, a.Description = item.Name, item.Description
				entries[index] = CatalogEntry{RelayURL: c.base.String(), ConnectionID: connection, Agent: a, Avatar: item.Metadata.Avatar, Source: item.Source, Type: item.Type}
			}
		}()
	}
	for index := range instances {
		select {
		case jobs <- index:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if probeFailure != nil {
		return nil, probeFailure
	}
	return entries, nil
}
func (m *Manager) Catalog(ctx context.Context) []CatalogEntry {
	m.mu.Lock()
	entries := make([]CatalogEntry, 0, len(m.manifest.Agents))
	connected := make(map[string]bool, len(m.running))
	for id, req := range m.manifest.Agents {
		if req.Visibility != "public" {
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
		if clientErr != nil {
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
