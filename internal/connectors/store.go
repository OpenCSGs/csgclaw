package connectors

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"csgclaw/internal/config"
	"csgclaw/internal/localstore"
)

const (
	rootAuthSectionName       = "auth"
	gitLabAgentsAuthStateName = "gitlab_agents"
)

type Store struct {
	path string
}

func NewStore(path string) Store {
	return Store{path: strings.TrimSpace(path)}
}

func DefaultStore() (Store, error) {
	path, err := config.DefaultStatePath()
	if err != nil {
		return Store{}, fmt.Errorf("resolve connector state path: %w", err)
	}
	return NewStore(path), nil
}

func (s Store) Path() (string, error) {
	path := strings.TrimSpace(s.path)
	if path != "" {
		return path, nil
	}
	return config.DefaultStatePath()
}

func (s Store) LoadGitHub() (State, bool, error) {
	return s.load(ProviderGitHub)
}

func (s Store) LoadGitLab() (State, bool, error) {
	state, ok, err := s.load(ProviderGitLab)
	if err != nil || ok {
		return state, ok, err
	}
	// Migrate the old agent-scoped layout on first use. Prefer the manager's
	// connector because it was already exposed as the workspace-wide status.
	states, err := s.loadGitLabAgents()
	if err != nil || len(states) == 0 {
		return State{}, false, err
	}
	key := ""
	if manager, exists := states["agent-manager"]; exists && hasGitLabState(manager) {
		key = "agent-manager"
	} else {
		keys := make([]string, 0, len(states))
		for candidate, candidateState := range states {
			if hasGitLabState(candidateState) {
				keys = append(keys, candidate)
			}
		}
		if len(keys) == 0 {
			return State{}, false, nil
		}
		sort.Strings(keys)
		key = keys[0]
	}
	state = normalizeGitLabState(states[key])
	if err := s.SaveGitLab(state); err != nil {
		return State{}, false, err
	}
	return state, true, nil
}

func (s Store) load(provider string) (State, bool, error) {
	path, err := s.Path()
	if err != nil {
		return State{}, false, err
	}
	authState, found, err := readRootAuthState(path)
	if err != nil || !found {
		return State{}, false, err
	}
	raw, ok := authState[provider]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return State{}, false, nil
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, false, fmt.Errorf("decode root %s auth: %w", provider, err)
	}
	if provider == ProviderGitLab {
		state = normalizeGitLabState(state)
		return state, hasGitLabState(state), nil
	}
	return normalizeState(state), hasState(state), nil
}

func (s Store) SaveGitHub(state State) error {
	return s.save(ProviderGitHub, normalizeState(state))
}

func (s Store) SaveGitLab(state State) error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(normalizeGitLabState(state))
	if err != nil {
		return fmt.Errorf("encode root gitlab auth: %w", err)
	}
	return localstore.UpdateObjectSection(path, rootAuthSectionName, func(authState map[string]json.RawMessage) error {
		authState[ProviderGitLab] = raw
		delete(authState, gitLabAgentsAuthStateName)
		return nil
	})
}

func (s Store) DeleteGitLab() error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	return localstore.UpdateObjectSection(path, rootAuthSectionName, func(authState map[string]json.RawMessage) error {
		delete(authState, ProviderGitLab)
		delete(authState, gitLabAgentsAuthStateName)
		return nil
	})
}

// Deprecated agent-scoped helpers retain source compatibility while all
// callers transition to the workspace-wide GitLab connector.
func (s Store) LoadGitLabForAgent(string) (State, bool, error) { return s.LoadGitLab() }
func (s Store) SaveGitLabForAgent(_ string, state State) error { return s.SaveGitLab(state) }
func (s Store) DeleteGitLabForAgent(string) error              { return s.DeleteGitLab() }

func (s Store) loadGitLabAgents() (map[string]State, error) {
	path, err := s.Path()
	if err != nil {
		return nil, err
	}
	authState, _, err := readRootAuthState(path)
	if err != nil {
		return nil, err
	}
	states := make(map[string]State)
	if raw := authState[gitLabAgentsAuthStateName]; len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &states); err != nil {
			return nil, fmt.Errorf("decode agent GitLab connector states: %w", err)
		}
	}
	return states, nil
}

func (s Store) save(provider string, state State) error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode root %s auth: %w", provider, err)
	}
	if err := localstore.UpdateObjectSection(path, rootAuthSectionName, func(authState map[string]json.RawMessage) error {
		authState[provider] = raw
		return nil
	}); err != nil {
		return fmt.Errorf("write %s connector store: %w", provider, err)
	}
	return nil
}

func hasGitLabState(state State) bool {
	state = normalizeGitLabState(state)
	config := state.Config
	return config.BaseURL != "" || config.AccessToken != "" || config.ClientID != "" || state.Pending != nil || state.Token != nil || state.Account != nil
}

func (s Store) DeleteGitHub() error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	if err := localstore.UpdateObjectSection(path, rootAuthSectionName, func(authState map[string]json.RawMessage) error {
		delete(authState, ProviderGitHub)
		return nil
	}); err != nil {
		return fmt.Errorf("delete github connector store: %w", err)
	}
	return nil
}

func readRootAuthState(path string) (map[string]json.RawMessage, bool, error) {
	authState := make(map[string]json.RawMessage)
	found, err := localstore.ReadSection(path, rootAuthSectionName, &authState)
	if err != nil {
		return nil, false, err
	}
	if authState == nil {
		authState = make(map[string]json.RawMessage)
	}
	return authState, found, nil
}

func hasState(state State) bool {
	state = normalizeState(state)
	return state.Config.ClientID != "" ||
		state.Config.ClientSecret != "" ||
		state.Pending != nil ||
		state.Token != nil ||
		state.Account != nil
}
