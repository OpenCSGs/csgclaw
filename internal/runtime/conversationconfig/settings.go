package conversationconfig

import (
	"reflect"
	"sync"

	agentruntime "csgclaw/internal/runtime"
)

type Settings struct {
	Profile agentruntime.Profile
	Options map[string]any
}

type State struct {
	Coordinator
	mu       sync.RWMutex
	settings Settings
	applied  bool
}

func (s *State) Current() (Settings, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings, s.applied
}

func (s *State) Matches(settings Settings) bool {
	current, applied := s.Current()
	return applied && reflect.DeepEqual(current, settings)
}

func (s *State) Set(settings Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings, s.applied = settings, true
}

func (s *State) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings, s.applied = Settings{}, false
}
