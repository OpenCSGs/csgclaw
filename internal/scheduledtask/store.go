package scheduledtask

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const stateFileName = "state.json"

type Store struct {
	mu   sync.Mutex
	path string
}

func NewStore(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("scheduled task store root is required")
	}
	return &Store{path: filepath.Join(root, stateFileName)}, nil
}

func (s *Store) Load() (state, error) {
	if s == nil {
		return state{}, fmt.Errorf("scheduled task store is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) Save(next state) error {
	if s == nil {
		return fmt.Errorf("scheduled task store is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(next)
}

func (s *Store) loadLocked() (state, error) {
	var current state
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			current.NextTaskID = 1
			current.NextRunID = 1
			return current, nil
		}
		return state{}, err
	}
	if len(data) == 0 {
		current.NextTaskID = 1
		current.NextRunID = 1
		return current, nil
	}
	if err := json.Unmarshal(data, &current); err != nil {
		return state{}, err
	}
	if current.NextTaskID <= 0 {
		current.NextTaskID = 1
	}
	if current.NextRunID <= 0 {
		current.NextRunID = 1
	}
	return current, nil
}

func (s *Store) saveLocked(next state) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
