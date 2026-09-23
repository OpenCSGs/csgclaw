package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"csgclaw/internal/knowledgebase"
)

const (
	availabilitySuccessTTL = 30 * time.Second
	availabilityFailureTTL = 10 * time.Second
	availabilityListWait   = 750 * time.Millisecond
	availabilityWorkers    = 10
)

var ErrServerUnavailable = errors.New("mcp server is unavailable")

type SkippedTemplateResource struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

const (
	TemplateResourceMCP           = "mcp"
	TemplateResourceKnowledgeBase = "knowledge_base"
)

type availabilityEntry struct {
	available bool
	checkedAt time.Time
	hash      string
}

// availabilityProbe represents one shared in-flight probe so concurrent page
// loads do not open duplicate MCP sessions for the same configuration.
type availabilityProbe struct {
	done      chan struct{}
	available bool
}

func (s *Service) initAvailability() {
	s.availabilityOnce.Do(func() {
		s.availability = map[string]availabilityEntry{}
		s.availabilityInflight = map[string]*availabilityProbe{}
		s.availabilitySem = make(chan struct{}, availabilityWorkers)
	})
}

// ListAvailableServers keeps manually configured MCP entries visible and only
// exposes remotely installed entries that recently passed an MCP handshake.
// Missing or stale probes are started concurrently and this call waits for at
// most a sub-second UI budget before failing closed for unfinished entries.
func (s *Service) ListAvailableServers(ctx context.Context) (map[string]any, error) {
	servers, _, err := s.ListAvailableServersWithStatus(ctx)
	return servers, err
}

// ListAvailableServersWithStatus also reports whether managed remote servers
// still have probes in flight so clients can refresh after the UI wait budget.
func (s *Service) ListAvailableServersWithStatus(ctx context.Context) (map[string]any, bool, error) {
	servers, err := s.ListServers(ctx)
	if err != nil {
		return nil, false, err
	}
	s.initAvailability()

	probes := make([]*availabilityProbe, 0)
	for name, raw := range servers {
		config, ok := raw.(map[string]any)
		if !ok || !IsRemoteHubServer(config) {
			continue
		}
		hash, err := availabilityConfigHash(config)
		if err != nil {
			continue
		}
		if !s.availabilityFresh(name, hash) {
			probes = append(probes, s.startAvailabilityProbe(name, config, hash))
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, availabilityListWait)
	defer cancel()
	for _, probe := range probes {
		select {
		case <-probe.done:
		case <-waitCtx.Done():
			return s.filterAvailableServers(servers), availabilityProbesPending(probes), nil
		}
	}
	return s.filterAvailableServers(servers), false, nil
}

func availabilityProbesPending(probes []*availabilityProbe) bool {
	for _, probe := range probes {
		select {
		case <-probe.done:
		default:
			return true
		}
	}
	return false
}

// RequireServerAvailable performs a bounded fresh check for a remotely
// installed MCP immediately before it is added to an Agent.
func (s *Service) RequireServerAvailable(ctx context.Context, name string, config map[string]any) error {
	if !IsRemoteHubServer(config) {
		return nil
	}
	s.initAvailability()
	hash, err := availabilityConfigHash(config)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrServerUnavailable, name)
	}
	probe := s.startAvailabilityProbe(name, config, hash)
	select {
	case <-probe.done:
		if probe.available {
			return nil
		}
	case <-ctx.Done():
	}
	return fmt.Errorf("%w: %s", ErrServerUnavailable, name)
}

// FilterAvailableTemplateServers probes managed remote resources concurrently
// before Agent creation. Ordinary third-party template MCP entries are kept:
// CSGClaw cannot infer their authorization model from connectivity alone.
func (s *Service) FilterAvailableTemplateServers(ctx context.Context, servers map[string]any) (map[string]any, []SkippedTemplateResource) {
	filtered := cloneMap(servers)
	if len(filtered) == 0 {
		return filtered, nil
	}
	s.initAvailability()
	type candidate struct {
		name         string
		displayName  string
		resourceType string
		config       map[string]any
		hash         string
		probe        *availabilityProbe
		available    bool
	}
	candidates := make([]candidate, 0)
	for name, raw := range filtered {
		config, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		resourceType, displayName, managed := templateManagedResource(name, config)
		if !managed {
			continue
		}
		hash, err := availabilityConfigHash(config)
		if err != nil {
			delete(filtered, name)
			candidates = append(candidates, candidate{name: name, displayName: displayName, resourceType: resourceType})
			continue
		}
		candidates = append(candidates, candidate{name: name, displayName: displayName, resourceType: resourceType, config: config, hash: hash})
	}
	if len(candidates) == 0 {
		return filtered, nil
	}

	for i := range candidates {
		if candidates[i].hash == "" {
			continue
		}
		candidates[i].probe = s.startAvailabilityProbe(candidates[i].name, candidates[i].config, candidates[i].hash)
	}
	for i := range candidates {
		if candidates[i].probe == nil {
			continue
		}
		select {
		case <-candidates[i].probe.done:
			candidates[i].available = candidates[i].probe.available
		case <-ctx.Done():
		}
	}

	skipped := make([]SkippedTemplateResource, 0)
	for _, item := range candidates {
		if item.available {
			continue
		}
		delete(filtered, item.name)
		skipped = append(skipped, SkippedTemplateResource{Type: item.resourceType, Name: item.displayName})
	}
	sort.Slice(skipped, func(i, j int) bool {
		if skipped[i].Type == skipped[j].Type {
			return skipped[i].Name < skipped[j].Name
		}
		return skipped[i].Type < skipped[j].Type
	})
	return filtered, skipped
}

func templateManagedResource(name string, config map[string]any) (resourceType, displayName string, ok bool) {
	if _, remoteName, managed := RemoteHubServerMetadata(config); managed {
		if remoteName == "" {
			remoteName = name
		}
		return TemplateResourceMCP, remoteName, true
	}
	if _, managed := knowledgebase.ManagedMetadataFromServer(config); managed {
		displayName := strings.TrimSpace(configString(config, "description"))
		if before, _, found := strings.Cut(displayName, " | "); found {
			displayName = strings.TrimSpace(before)
		}
		if displayName == "" {
			displayName = name
		}
		return TemplateResourceKnowledgeBase, displayName, true
	}
	return "", "", false
}

func configString(config map[string]any, key string) string {
	value, _ := config[key].(string)
	return value
}

func (s *Service) availabilityFresh(name, hash string) bool {
	s.availabilityMu.Lock()
	defer s.availabilityMu.Unlock()
	entry, ok := s.availability[name]
	if !ok || entry.hash != hash {
		return false
	}
	ttl := availabilityFailureTTL
	if entry.available {
		ttl = availabilitySuccessTTL
	}
	return time.Since(entry.checkedAt) < ttl
}

func (s *Service) availabilityAvailable(name, hash string) bool {
	s.availabilityMu.Lock()
	defer s.availabilityMu.Unlock()
	entry, ok := s.availability[name]
	return ok && entry.hash == hash && entry.available && time.Since(entry.checkedAt) < availabilitySuccessTTL
}

func (s *Service) startAvailabilityProbe(name string, config map[string]any, hash string) *availabilityProbe {
	key := name + "\x00" + hash
	s.availabilityMu.Lock()
	if existing := s.availabilityInflight[key]; existing != nil {
		s.availabilityMu.Unlock()
		return existing
	}
	probe := &availabilityProbe{done: make(chan struct{})}
	s.availabilityInflight[key] = probe
	s.availabilityMu.Unlock()

	cloned := cloneMap(config)
	go func() {
		probeCtx, cancel := context.WithTimeout(context.Background(), availabilityProbeTimeout(cloned))
		defer cancel()
		var probeErr error
		select {
		case s.availabilitySem <- struct{}{}:
			_, probeErr = s.ProbeServer(probeCtx, name, cloned)
			<-s.availabilitySem
		case <-probeCtx.Done():
			probeErr = probeCtx.Err()
		}

		s.availabilityMu.Lock()
		probe.available = probeErr == nil
		s.availability[name] = availabilityEntry{available: probeErr == nil, checkedAt: time.Now(), hash: hash}
		delete(s.availabilityInflight, key)
		close(probe.done)
		s.availabilityMu.Unlock()
	}()
	return probe
}

func availabilityProbeTimeout(config map[string]any) time.Duration {
	return probeTimeout(config, "startup_timeout_sec", defaultProbeStartupTimeout) +
		probeTimeout(config, "tool_timeout_sec", defaultProbeToolTimeout)
}

func (s *Service) filterAvailableServers(servers map[string]any) map[string]any {
	filtered := make(map[string]any, len(servers))
	for name, raw := range servers {
		config, ok := raw.(map[string]any)
		if !ok || !IsRemoteHubServer(config) {
			filtered[name] = raw
			continue
		}
		hash, err := availabilityConfigHash(config)
		if err == nil && s.availabilityAvailable(name, hash) {
			filtered[name] = raw
		}
	}
	return filtered
}

func availabilityConfigHash(config map[string]any) (string, error) {
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
