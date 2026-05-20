package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Store struct {
	mu    sync.RWMutex
	path  string
	items map[string]Bot
}

type persistedState struct {
	Bots []Bot `json:"bots"`
}

func NewStore(path string) (*Store, error) {
	s := &Store{
		path:  path,
		items: make(map[string]Bot),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func NewMemoryStore(bots []Bot) (*Store, error) {
	s := &Store{
		items: make(map[string]Bot),
	}
	for _, b := range bots {
		normalized, err := NormalizeBot(b)
		if err != nil {
			return nil, err
		}
		s.items[botStoreKey(normalized)] = normalized
	}
	return s, nil
}

func (s *Store) List() []Bot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sortedBotsFromMap(s.items)
}

func (s *Store) Get(id string) (Bot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if b, ok := s.items[id]; ok {
		return b, true
	}
	for _, b := range sortedBotsFromMap(s.items) {
		if b.ID == id {
			return b, true
		}
	}
	return Bot{}, false
}

func (s *Store) GetByChannelID(channel, id string) (Bot, bool, error) {
	normalizedChannel, err := NormalizeChannel(channel)
	if err != nil {
		return Bot{}, false, err
	}
	key := botStoreKeyParts(string(normalizedChannel), strings.TrimSpace(id))
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.items[key]
	return b, ok, nil
}

func (s *Store) Save(b Bot) error {
	normalized, err := NormalizeBot(b)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[botStoreKey(normalized)] = normalized
	return s.saveLocked()
}

func (s *Store) DeleteByChannelID(channel, id string) (Bot, bool, error) {
	normalizedChannel, err := NormalizeChannel(channel)
	if err != nil {
		return Bot{}, false, err
	}
	key := botStoreKeyParts(string(normalizedChannel), strings.TrimSpace(id))
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.items[key]
	if !ok {
		return Bot{}, false, nil
	}
	delete(s.items, key)
	if err := s.saveLocked(); err != nil {
		return Bot{}, false, err
	}
	return b, true, nil
}

func (s *Store) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return nil
	}
	items, err := s.readState()
	if err != nil {
		return err
	}
	s.items = items
	return nil
}

func (s *Store) load() error {
	items, err := s.readState()
	if err != nil {
		return err
	}
	for id, b := range items {
		s.items[id] = b
	}
	return nil
}

func (s *Store) readState() (map[string]Bot, error) {
	items := make(map[string]Bot)
	if s.path == "" {
		return items, nil
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return items, nil
		}
		return nil, fmt.Errorf("read bot state: %w", err)
	}

	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode bot state: %w", err)
	}
	for _, b := range state.Bots {
		normalized, err := NormalizeBot(b)
		if err != nil {
			return nil, fmt.Errorf("decode bot state: %w", err)
		}
		items[botStoreKey(normalized)] = normalized
	}
	return items, nil
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}

	data, err := json.MarshalIndent(persistedState{
		Bots: sortedBotsFromMap(s.items),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode bot state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create bot state dir: %w", err)
	}
	if err := os.WriteFile(s.path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write bot state: %w", err)
	}
	return nil
}

func botStoreKey(b Bot) string {
	return botStoreKeyParts(b.Channel, b.ID)
}

func botStoreKeyParts(channel, id string) string {
	return channel + "\x00" + id
}
