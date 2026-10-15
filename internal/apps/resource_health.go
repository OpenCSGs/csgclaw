package apps

import (
	"context"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const resourceHealthTTL = 30 * time.Second

type resourceHealth struct {
	version   time.Time
	checkedAt time.Time
	checking  bool
	status    Installation
	tools     []*mcp.Tool
}

// CheckResource validates the resource's own credentials without an Agent binding.
// Results are cached briefly; no persistent MCP session is retained by this check.
func (s *Service) CheckResource(ctx context.Context, id string, force bool) (Installation, error) {
	s.mu.Lock()
	e, err := s.findLocked("", id)
	if err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	if !e.record.Enabled {
		out := s.viewLocked(e)
		s.mu.Unlock()
		return out, nil
	}
	previous := e.resourceHealth
	if previous != nil && previous.version.Equal(e.record.UpdatedAt) && (previous.checking || (!force && time.Since(previous.checkedAt) < resourceHealthTTL)) {
		out := s.viewLocked(e)
		s.mu.Unlock()
		return out, nil
	}
	r := copyJSON(e.record)
	health := &resourceHealth{version: r.UpdatedAt, checking: true, status: Installation{Status: "connecting"}}
	e.resourceHealth = health
	s.mu.Unlock()
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, probeErr := s.Probe(probeCtx, "", ProbeRequest{AppID: r.AppID, Config: r.Config, Credentials: r.Credentials})
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.findLocked("", id)
	if err != nil {
		return Installation{}, err
	}
	if current != e || current.resourceHealth != health || !current.record.UpdatedAt.Equal(r.UpdatedAt) {
		return s.viewLocked(current), nil
	}
	if ctx.Err() != nil {
		current.resourceHealth = previous
		return s.viewLocked(current), ctx.Err()
	}
	health.checking = false
	health.checkedAt = time.Now()
	health.status.Status = "connected"
	if probeErr != nil {
		setConnectionError(&health.status, probeErr)
	} else {
		health.tools = result.Tools
	}
	return s.viewLocked(current), nil
}

// CheckResources probes independent resources concurrently, sharing the per-resource cache.
func (s *Service) CheckResources(ctx context.Context) ([]Installation, error) {
	items, err := s.List(ctx, "")
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Add(1)
		go func(id string) { defer wg.Done(); _, _ = s.CheckResource(ctx, id, false) }(item.InstallationID)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.List(ctx, "")
}

// RecordResourceProbe reuses a successful save-time validation for that exact revision.
func (s *Service) RecordResourceProbe(resource Installation, probe ProbeResult) (Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.findLocked("", resource.InstallationID)
	if err != nil {
		return Installation{}, err
	}
	if e.record.UpdatedAt.Equal(resource.UpdatedAt) && probe.Connected &&
		probe.credentialService == s && probe.credentialGeneration == s.credentialGeneration {
		e.resourceHealth = &resourceHealth{version: resource.UpdatedAt, checkedAt: time.Now(), status: Installation{Status: "connected"}, tools: copyJSON(probe.Tools)}
	}
	return s.viewLocked(e), nil
}
