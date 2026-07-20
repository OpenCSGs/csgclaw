package connectors

import (
	"encoding/json"
	"fmt"
	"strings"

	"csgclaw/internal/config"
	"csgclaw/internal/localstore"
)

const rootAuthSectionName = "auth"

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
	return s.load(ProviderGitLab)
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
		state.Config = NormalizeGitLabConfig(state.Config)
		return state, hasGitLabState(state), nil
	}
	return normalizeState(state), hasState(state), nil
}

func (s Store) SaveGitHub(state State) error {
	return s.save(ProviderGitHub, normalizeState(state))
}

func (s Store) SaveGitLab(state State) error {
	state.Config = NormalizeGitLabConfig(state.Config)
	return s.save(ProviderGitLab, state)
}

func (s Store) save(provider string, state State) error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	authState, _, err := readRootAuthState(path)
	if err != nil {
		return err
	}
	if authState == nil {
		authState = make(map[string]json.RawMessage)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode root %s auth: %w", provider, err)
	}
	authState[provider] = raw
	if err := localstore.WriteSection(path, rootAuthSectionName, authState); err != nil {
		return fmt.Errorf("write %s connector store: %w", provider, err)
	}
	return nil
}

func hasGitLabState(state State) bool {
	config := NormalizeGitLabConfig(state.Config)
	return config.BaseURL != "" || config.AccessToken != "" || state.Account != nil
}

func (s Store) DeleteGitHub() error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	authState, found, err := readRootAuthState(path)
	if err != nil || !found {
		return err
	}
	delete(authState, ProviderGitHub)
	if err := localstore.WriteSection(path, rootAuthSectionName, authState); err != nil {
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
