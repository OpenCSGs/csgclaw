package agentmemory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// SeedTemplate saves only explicitly supplied template knowledge, never native
// runtime memory. A new user scope adopts it once, before scanning private chat.
func (s *Store) SeedTemplate(agentID, content string) error {
	if !safeID(agentID, "agent-") {
		return ErrInvalid
	}
	entries, err := parseMarkdown(content, nil)
	if err != nil {
		return err
	}
	entries, err = editEntries(nil, entries, time.Now())
	if err != nil {
		return err
	}
	if len(markdown(entries)) > maxDocumentBytes {
		return ErrCapacity
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	if len(data) > maxStateBytes {
		return ErrCapacity
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	home := filepath.Join(s.root, agentID)
	for _, dir := range []string{home, filepath.Join(home, "memory")} {
		if err := privateDir(dir); err != nil {
			return err
		}
	}
	return atomicWrite(filepath.Join(home, "memory", "template.json"), data)
}

func (s *Store) initialState(scope Scope) (state, error) {
	current := s.defaultState(scope)
	if !current.Enabled {
		return current, nil
	}
	err := readJSON(filepath.Join(s.root, scope.AgentID, "memory", "template.json"), &current.Entries)
	if errors.Is(err, os.ErrNotExist) {
		return current, nil
	}
	return current, err
}
