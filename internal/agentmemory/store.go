package agentmemory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/localstore"
)

const (
	maxJobs        = 8
	maxMessageSize = 2 << 10
	jobLifetime    = 24 * time.Hour
)

type instance struct {
	Mode         string    `json:"mode"`
	Epoch        string    `json:"epoch"`
	HistoryAfter time.Time `json:"history_after,omitzero"`
}

type job struct {
	ID             string        `json:"id"`
	Message        *Message      `json:"message,omitempty"`
	HistoryThrough time.Time     `json:"history_through,omitzero"`
	HistoryCursor  HistoryCursor `json:"history_cursor,omitzero"`
	CreatedAt      time.Time     `json:"created_at"`
	NextAttempt    time.Time     `json:"next_attempt"`
	Attempts       int           `json:"attempts"`
}

type receipt struct {
	ID string    `json:"id"`
	At time.Time `json:"at"`
}

type state struct {
	Lifetime       string    `json:"lifetime"`
	Scope          Scope     `json:"scope"`
	Policy         string    `json:"policy"`
	Enabled        bool      `json:"enabled"`
	Revision       uint64    `json:"revision"`
	WriteEpoch     uint64    `json:"write_epoch"`
	ContextEpoch   uint64    `json:"context_epoch"`
	Entries        []Entry   `json:"entries"`
	Jobs           []job     `json:"jobs,omitempty"`
	Receipts       []receipt `json:"receipts,omitempty"`
	Initialization string    `json:"initialization,omitempty"`
	HistoryThrough time.Time `json:"history_through,omitzero"`
	Failure        bool      `json:"failure,omitempty"`
	LastError      *Failure  `json:"last_error,omitempty"`
}

type Store struct {
	root     string
	instance instance
	mu       sync.Mutex
	pending  map[Scope]struct{}
	project  map[Scope]struct{}
	attempts map[Scope]context.CancelFunc
	wake     chan struct{}
}

// Open stores per-user memories under root and instance metadata in the shared
// application state. Instance metadata is only written on initialization or a
// deployment mode change, never when a memory or its queue changes.
func Open(root, statePath, mode string) (*Store, error) {
	if strings.TrimSpace(statePath) == "" {
		return nil, ErrInvalid
	}
	if err := privateDir(root); err != nil {
		return nil, err
	}
	s := &Store{root: root, pending: make(map[Scope]struct{}), project: make(map[Scope]struct{}), attempts: make(map[Scope]context.CancelFunc), wake: make(chan struct{}, 1)}
	if _, err := localstore.ReadSection(statePath, "agent_memory", &s.instance); err != nil {
		return nil, err
	}
	if s.instance.Mode != mode || s.instance.Epoch == "" {
		if s.instance.Mode == "disabled" && mode != "disabled" {
			s.instance.HistoryAfter = time.Now()
		}
		s.instance.Mode, s.instance.Epoch = mode, newID()
		if err := localstore.WriteSection(statePath, "agent_memory", s.instance); err != nil {
			return nil, err
		}
	}
	if err := s.recover(); err != nil {
		return nil, err
	}
	return s, nil
}

func digest(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

func (s *Store) scopeDir(scope Scope) string {
	return filepath.Join(s.root, scope.AgentID, "memory", scope.UserID)
}

func (current state) contextVersion() string { return strconv.FormatUint(current.ContextEpoch, 10) }

func (s *Store) defaultState(scope Scope) state {
	return state{Scope: scope, Policy: s.instance.Epoch, Enabled: s.instance.Mode == "local" || s.instance.Mode == "desktop"}
}

func safeID(id, prefix string) bool {
	if !strings.HasPrefix(id, prefix) || len(id) <= len(prefix) || len(id) > 128 {
		return false
	}
	for _, char := range id {
		if char != '-' && char != '_' && !(char >= 'a' && char <= 'z') &&
			!(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}

func validScope(scope Scope) bool {
	return safeID(scope.AgentID, "agent-") && safeID(scope.UserID, "user-")
}

func (s *Store) load(scope Scope) (state, error) {
	if !validScope(scope) {
		return state{}, ErrInvalid
	}
	dir := s.scopeDir(scope)
	for _, path := range []string{filepath.Join(s.root, scope.AgentID), filepath.Dir(dir), dir} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return s.defaultState(scope), nil
		}
		if err != nil {
			return state{}, err
		}
		if !info.IsDir() {
			return state{}, ErrInvalid
		}
	}
	current := s.defaultState(scope)
	err := readJSON(filepath.Join(dir, "state.json"), &current)
	if errors.Is(err, os.ErrNotExist) {
		return current, nil
	}
	if err != nil {
		return state{}, err
	}
	if current.Scope != scope {
		return state{}, ErrInvalid
	}
	if current.Policy != s.instance.Epoch {
		current.Policy = s.instance.Epoch
		current.invalidate()
		current.ContextEpoch++
		current.Revision++
	}
	return current, nil
}

func (s *Store) commit(current state) error {
	if !validScope(current.Scope) {
		return ErrInvalid
	}
	content := markdown(current.Entries)
	encoded, err := json.Marshal(current)
	if err != nil {
		return err
	}
	if len(content) > maxDocumentBytes || len(encoded) > maxStateBytes {
		return ErrCapacity
	}
	dir := s.scopeDir(current.Scope)
	for _, path := range []string{filepath.Join(s.root, current.Scope.AgentID), filepath.Dir(dir), dir} {
		if err := privateDir(path); err != nil {
			return err
		}
	}
	if err := atomicWrite(filepath.Join(dir, "state.json"), encoded); err != nil {
		return err
	}
	if len(current.Jobs) > 0 {
		s.pending[current.Scope] = struct{}{}
	} else {
		delete(s.pending, current.Scope)
	}
	if err := writeProjection(dir, content); err != nil {
		s.project[current.Scope] = struct{}{}
	} else {
		delete(s.project, current.Scope)
	}
	s.signal()
	return nil
}

func (s *Store) recover() error {
	agents, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		if !agent.IsDir() && strings.HasPrefix(agent.Name(), ".memory-") {
			_ = os.Remove(filepath.Join(s.root, agent.Name()))
			continue
		}
		if !agent.IsDir() || !safeID(agent.Name(), "agent-") {
			continue
		}
		memoryDir := filepath.Join(s.root, agent.Name(), "memory")
		info, err := os.Lstat(memoryDir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return ErrInvalid
		}
		users, err := os.ReadDir(memoryDir)
		if err != nil {
			return err
		}
		for _, user := range users {
			if !user.IsDir() || !safeID(user.Name(), "user-") {
				continue
			}
			s.recoverScope(Scope{AgentID: agent.Name(), UserID: user.Name()})
		}
	}
	return nil
}

func (s *Store) recoverScope(scope Scope) {
	dir := s.scopeDir(scope)
	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".memory-") && !file.IsDir() {
			_ = os.Remove(filepath.Join(dir, file.Name()))
		}
	}
	var raw state
	if readJSON(filepath.Join(dir, "state.json"), &raw) != nil || raw.Scope != scope {
		return // A damaged scope cannot prevent other scopes from starting.
	}
	if raw.Policy != s.instance.Epoch {
		if current, err := s.load(scope); err == nil {
			if s.commit(current) == nil {
				return
			}
		}
	}
	if raw.Policy == s.instance.Epoch && len(raw.Jobs) > 0 {
		s.pending[scope] = struct{}{}
	}
	if err := writeProjection(dir, markdown(raw.Entries)); err != nil {
		s.project[scope] = struct{}{}
	}
}

func (s *Store) DeleteAgent(agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !safeID(agentID, "agent-") {
		return ErrInvalid
	}
	home := filepath.Join(s.root, agentID)
	if info, err := os.Lstat(home); err == nil {
		if !info.IsDir() {
			return ErrInvalid
		}
		if err := os.RemoveAll(filepath.Join(home, "memory")); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for scope, cancel := range s.attempts {
		if scope.AgentID == agentID {
			cancel()
		}
	}
	for scope := range s.pending {
		if scope.AgentID == agentID {
			delete(s.pending, scope)
		}
	}
	for scope := range s.project {
		if scope.AgentID == agentID {
			delete(s.project, scope)
		}
	}
	return nil
}

func (s *Store) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func privateDir(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("memory path must be a directory: %s", path)
	}
	return nil
}

func readJSON(path string, value any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxStateBytes {
		return ErrInvalid
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxStateBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxStateBytes {
		return ErrInvalid
	}
	return json.Unmarshal(data, value)
}

func writeProjection(dir, text string) error {
	path := filepath.Join(dir, "MEMORY.md")
	info, err := os.Lstat(path)
	if err == nil && info.Mode().IsRegular() && info.Size() == int64(len(text)) {
		data, readErr := os.ReadFile(path)
		if readErr == nil && bytes.Equal(data, []byte(text)) {
			return nil
		}
	}
	return atomicWrite(path, []byte(text))
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".memory-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
