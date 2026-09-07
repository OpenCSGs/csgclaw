package agents

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RuntimeExtension returns one opaque Agent Engine extension resource.
func (s *Repository) RuntimeExtension(agentID, name string) (json.RawMessage, bool, error) {
	if s == nil {
		return nil, false, fmt.Errorf("agent repository is required")
	}
	agentID = canonicalAgentID(agentID)
	name = strings.TrimSpace(name)
	if agentID == "" || name == "" {
		return nil, false, fmt.Errorf("agent id and runtime extension name are required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	got, _, ok := s.agentByIDLocked(agentID)
	if !ok {
		return nil, false, fmt.Errorf("agent %q not found", agentID)
	}
	raw, ok := got.RuntimeExtensions[name]
	return append(json.RawMessage(nil), raw...), ok, nil
}

// RuntimeExtensionList returns all opaque extension resources for one Agent.
func (s *Repository) RuntimeExtensionList(agentID string) (map[string]json.RawMessage, error) {
	if s == nil {
		return nil, fmt.Errorf("agent repository is required")
	}
	agentID = canonicalAgentID(agentID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	got, _, ok := s.agentByIDLocked(agentID)
	if !ok {
		return nil, fmt.Errorf("agent %q not found", agentID)
	}
	return cloneRawMessages(got.RuntimeExtensions), nil
}

// PutRuntimeExtension atomically persists one opaque extension resource.
func (s *Repository) PutRuntimeExtension(agentID, name string, raw json.RawMessage) error {
	if s == nil {
		return fmt.Errorf("agent repository is required")
	}
	agentID = canonicalAgentID(agentID)
	name = strings.TrimSpace(name)
	if agentID == "" || name == "" || !json.Valid(raw) {
		return fmt.Errorf("valid agent id, runtime extension name, and JSON resource are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	got, key, ok := s.agentByIDLocked(agentID)
	if !ok {
		return fmt.Errorf("agent %q not found", agentID)
	}
	previous := got
	got = *cloneAgent(&got)
	if got.RuntimeExtensions == nil {
		got.RuntimeExtensions = make(map[string]json.RawMessage)
	}
	got.RuntimeExtensions[name] = append(json.RawMessage(nil), raw...)
	s.agents[key] = got
	if err := s.saveLocked(); err != nil {
		s.agents[key] = previous
		return err
	}
	return nil
}

// DeleteRuntimeExtension removes one opaque extension resource.
func (s *Repository) DeleteRuntimeExtension(agentID, name string) error {
	if s == nil {
		return fmt.Errorf("agent repository is required")
	}
	agentID = canonicalAgentID(agentID)
	name = strings.TrimSpace(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	got, key, ok := s.agentByIDLocked(agentID)
	if !ok {
		return fmt.Errorf("agent %q not found", agentID)
	}
	if _, ok := got.RuntimeExtensions[name]; !ok {
		return nil
	}
	previous := got
	got = *cloneAgent(&got)
	delete(got.RuntimeExtensions, name)
	s.agents[key] = got
	if err := s.saveLocked(); err != nil {
		s.agents[key] = previous
		return err
	}
	return nil
}
