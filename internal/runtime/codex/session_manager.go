package codex

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type liveSession struct {
	mu                   sync.Mutex
	session              *Session
	appClient            *appServerClient
	cmd                  *exec.Cmd
	stdin                io.Closer
	stderr               *os.File
	done                 chan struct{}
	spec                 SessionSpec
	conversationSessions map[string]string
	turnWaiters          map[string]*appServerTurnWaiter
	appProtocol          string
}

func (s *liveSession) sessionIDs() []string {
	if s == nil || s.session == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var ids []string
	add := func(sessionID string) {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			return
		}
		if _, ok := seen[sessionID]; ok {
			return
		}
		seen[sessionID] = struct{}{}
		ids = append(ids, sessionID)
	}
	add(s.session.SessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sessionID := range s.conversationSessions {
		add(sessionID)
	}
	return ids
}

func buildSessionEnv(spec SessionSpec) []string {
	spec.Profile = spec.Profile.Normalized()
	envMap := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if shouldOmitInheritedSessionEnvKey(key) {
			continue
		}
		envMap[key] = value
	}
	if homeDir := strings.TrimSpace(spec.HomeDir); homeDir != "" {
		envMap["HOME"] = homeDir
	}
	envMap["CODEX_HOME"] = spec.CodexHomeDir
	if apiKey := spec.Profile.APIKey; apiKey != "" {
		envMap["OPENAI_API_KEY"] = apiKey
	}
	for key, value := range spec.Profile.Env {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if isReservedSessionEnvKey(key) {
			continue
		}
		envMap[key] = value
	}
	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+envMap[key])
	}
	return out
}

func shouldOmitInheritedSessionEnvKey(key string) bool {
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "ZDOTDIR", "BASH_ENV", "ENV":
		return true
	default:
		return false
	}
}

func isReservedSessionEnvKey(key string) bool {
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "HOME", "CODEX_HOME", "OPENAI_BASE_URL", "OPENAI_API_KEY", "OPENAI_MODEL":
		return true
	default:
		return false
	}
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
