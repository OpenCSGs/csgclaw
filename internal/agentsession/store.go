// Package agentsession owns the external Session ID to Engine Conversation Key
// binding used by the anonymous Session API.
package agentsession

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"csgclaw/internal/localstore"
)

const (
	bindingFileSuffix      = ".jsonl"
	legacyRootStateSection = "agent_session_bindings"
)

type Binding struct {
	AgentID         string `json:"agent_id"`
	ExternalSession string `json:"external_session_id"`
	ConversationKey string `json:"conversation_key"`
}

type pendingBinding struct {
	done    chan struct{}
	binding Binding
	err     error
}

type legacyPersistedState struct {
	Items []Binding `json:"items"`
}

// Store keeps immutable bindings in memory and appends each new binding to an
// agent-specific JSONL file. There is deliberately no lock around persistence:
// unrelated sessions never wait on each other's disk writes.
type Store struct {
	dir       string
	items     sync.Map // map[string]Binding
	pendingMu sync.Mutex
	pending   map[string]*pendingBinding
}

func NewStore(dir string) (*Store, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("session binding directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create session binding directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure session binding directory: %w", err)
	}
	store := &Store{dir: dir, pending: make(map[string]*pendingBinding)}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read session binding directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), bindingFileSuffix) {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("session binding file %q must not be a symbolic link", filepath.Join(dir, entry.Name()))
		}
		if err := store.loadFile(filepath.Join(dir, entry.Name())); err != nil {
			return nil, err
		}
	}
	return store, nil
}

// ImportLegacyState performs a startup-only, idempotent import from the former
// root state section. Runtime session requests never read or write root state.
func ImportLegacyState(path string, store *Store) error {
	if store == nil {
		return fmt.Errorf("session binding store is required")
	}
	var state legacyPersistedState
	found, err := localstore.ReadSection(path, legacyRootStateSection, &state)
	if err != nil {
		return fmt.Errorf("load legacy session bindings: %w", err)
	}
	if !found {
		return nil
	}
	for _, binding := range state.Items {
		if err := store.importBinding(binding); err != nil {
			return fmt.Errorf("import legacy session binding: %w", err)
		}
	}
	return nil
}

func (s *Store) GetOrCreate(agentID, externalSession string) (Binding, error) {
	if s == nil {
		return Binding{}, fmt.Errorf("session binding store is required")
	}
	agentID = strings.TrimSpace(agentID)
	externalSession = strings.TrimSpace(externalSession)
	if agentID == "" || externalSession == "" {
		return Binding{}, fmt.Errorf("agent ID and external session ID are required")
	}

	key := bindingKey(agentID, externalSession)
	if value, ok := s.items.Load(key); ok {
		return value.(Binding), nil
	}

	s.pendingMu.Lock()
	if value, ok := s.items.Load(key); ok {
		s.pendingMu.Unlock()
		return value.(Binding), nil
	}
	if pending := s.pending[key]; pending != nil {
		s.pendingMu.Unlock()
		<-pending.done
		return pending.binding, pending.err
	}
	pending := &pendingBinding{done: make(chan struct{})}
	s.pending[key] = pending
	s.pendingMu.Unlock()

	conversationKey, err := newConversationKey()
	if err != nil {
		s.finishPending(key, pending, Binding{}, err)
		return Binding{}, err
	}
	binding := Binding{
		AgentID:         agentID,
		ExternalSession: externalSession,
		ConversationKey: conversationKey,
	}
	if err := appendBinding(filepath.Join(s.dir, bindingFileName(agentID)), binding); err != nil {
		err = fmt.Errorf("save session binding: %w", err)
		s.finishPending(key, pending, Binding{}, err)
		return Binding{}, err
	}
	s.items.Store(key, binding)
	s.finishPending(key, pending, binding, nil)
	return binding, nil
}

func (s *Store) Bindings() []Binding {
	if s == nil {
		return nil
	}
	items := make([]Binding, 0)
	s.items.Range(func(_, value any) bool {
		items = append(items, value.(Binding))
		return true
	})
	sortBindings(items)
	return items
}

func (s *Store) finishPending(key string, pending *pendingBinding, binding Binding, err error) {
	s.pendingMu.Lock()
	pending.binding = binding
	pending.err = err
	delete(s.pending, key)
	close(pending.done)
	s.pendingMu.Unlock()
}

func (s *Store) importBinding(binding Binding) error {
	binding = normalizeBinding(binding)
	if err := validateBinding(binding); err != nil {
		return err
	}
	key := bindingKey(binding.AgentID, binding.ExternalSession)
	if existing, ok := s.items.Load(key); ok {
		if existing.(Binding).ConversationKey != binding.ConversationKey {
			return fmt.Errorf("conflicting binding for agent %q and session %q", binding.AgentID, binding.ExternalSession)
		}
		return nil
	}
	if err := appendBinding(filepath.Join(s.dir, bindingFileName(binding.AgentID)), binding); err != nil {
		return err
	}
	s.items.Store(key, binding)
	return nil
}

func (s *Store) loadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read session bindings %q: %w", path, err)
	}
	data, err = repairTrailingRecord(path, data)
	if err != nil {
		return err
	}
	for lineNumber, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var binding Binding
		if err := json.Unmarshal(line, &binding); err != nil {
			return fmt.Errorf("decode session binding %q line %d: %w", path, lineNumber+1, err)
		}
		binding = normalizeBinding(binding)
		if err := validateBinding(binding); err != nil {
			return fmt.Errorf("load session binding %q line %d: %w", path, lineNumber+1, err)
		}
		if got, want := filepath.Base(path), bindingFileName(binding.AgentID); got != want {
			return fmt.Errorf("load session binding %q line %d: agent %q belongs in %q", path, lineNumber+1, binding.AgentID, want)
		}
		key := bindingKey(binding.AgentID, binding.ExternalSession)
		if existing, loaded := s.items.LoadOrStore(key, binding); loaded && existing.(Binding).ConversationKey != binding.ConversationKey {
			return fmt.Errorf("load session bindings: conflicting binding for agent %q and session %q", binding.AgentID, binding.ExternalSession)
		}
	}
	return nil
}

func repairTrailingRecord(path string, data []byte) ([]byte, error) {
	if len(data) == 0 || data[len(data)-1] == '\n' {
		return data, nil
	}
	lastNewline := bytes.LastIndexByte(data, '\n')
	tail := bytes.TrimSpace(data[lastNewline+1:])
	var binding Binding
	if json.Unmarshal(tail, &binding) == nil && validateBinding(normalizeBinding(binding)) == nil {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return nil, fmt.Errorf("repair session binding newline %q: %w", path, err)
		}
		_, writeErr := file.Write([]byte{'\n'})
		closeErr := file.Close()
		if writeErr != nil {
			return nil, fmt.Errorf("repair session binding newline %q: %w", path, writeErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close repaired session binding file %q: %w", path, closeErr)
		}
		return append(data, '\n'), nil
	}

	keep := lastNewline + 1
	if err := os.Truncate(path, int64(keep)); err != nil {
		return nil, fmt.Errorf("truncate incomplete session binding record %q: %w", path, err)
	}
	return data[:keep], nil
}

func appendBinding(path string, binding Binding) error {
	data, err := json.Marshal(binding)
	if err != nil {
		return fmt.Errorf("encode binding: %w", err)
	}
	data = append(data, '\n')
	// O_APPEND reserves the end position for this single record write. Keeping
	// the complete JSONL record in one Write call prevents concurrent sessions
	// from sharing a partially advanced file offset.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open binding file: %w", err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("append binding: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close binding file: %w", closeErr)
	}
	return nil
}

func bindingFileName(agentID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSpace(agentID))) + bindingFileSuffix
}

func validateBinding(binding Binding) error {
	if binding.AgentID == "" || binding.ExternalSession == "" || binding.ConversationKey == "" {
		return fmt.Errorf("binding fields are required")
	}
	return nil
}

func normalizeBinding(binding Binding) Binding {
	binding.AgentID = strings.TrimSpace(binding.AgentID)
	binding.ExternalSession = strings.TrimSpace(binding.ExternalSession)
	binding.ConversationKey = strings.TrimSpace(binding.ConversationKey)
	return binding
}

func bindingKey(agentID, externalSession string) string {
	return strings.TrimSpace(agentID) + "\x00" + strings.TrimSpace(externalSession)
}

func sortBindings(items []Binding) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].AgentID != items[j].AgentID {
			return items[i].AgentID < items[j].AgentID
		}
		return items[i].ExternalSession < items[j].ExternalSession
	})
}

func newConversationKey() (string, error) {
	var value [16]byte
	if _, err := io.ReadFull(rand.Reader, value[:]); err != nil {
		return "", fmt.Errorf("generate conversation key: %w", err)
	}
	return "conv_" + hex.EncodeToString(value[:]), nil
}
