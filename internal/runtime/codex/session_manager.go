package codex

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

type liveSession struct {
	mu                    sync.Mutex
	conversationResumeMu  sync.Mutex
	conversationPersistMu sync.Mutex
	memoryCheckpointMu    sync.Mutex
	memoryMaintenanceMu   sync.Mutex
	session               *Session
	appClient             *appServerClient
	cmd                   *exec.Cmd
	stdin                 io.Closer
	stderr                *os.File
	done                  chan struct{}
	spec                  SessionSpec
	conversationSessions  map[string]string
	memoryCheckpointLast  map[string]time.Time
	memoryCheckpointBusy  map[string]bool
	memoryLastMaintenance time.Time
	memoryMaintenanceID   string
	loadedConversations   map[string]bool
	filePublishingThreads map[string]bool
	turnWaiters           map[string]*appServerTurnWaiter
	turnThreads           map[string]string
	turnThreadOrder       []string
	commandOutputs        map[string]*appServerCommandOutputState
	replayedExecCommands  map[string]struct{}
	replayedAgentMessages map[string]struct{}
	streamedAgentMessages map[string]struct{}
	streamedAgentThreads  map[string]struct{}
	agentMessagePhases    map[string]string
	inferredAgentPhases   map[string]struct{}
	agentMessageStreams   map[string]*assistantStructuredOutputStream
	appProtocol           string
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
	if spec.ExecutionMode == ExecutionModeReadOnly {
		for _, key := range []string{"LANG", "LC_ALL", "PATH", "SSL_CERT_DIR", "SSL_CERT_FILE", "TMPDIR"} {
			if value := os.Getenv(key); value != "" {
				envMap[key] = value
			}
		}
	} else {
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
	}
	if homeDir := strings.TrimSpace(spec.HomeDir); homeDir != "" {
		envMap["HOME"] = homeDir
	}
	envMap["CODEX_HOME"] = spec.CodexHomeDir
	if apiKey := spec.Profile.APIKey; apiKey != "" {
		envMap["OPENAI_API_KEY"] = apiKey
	}
	if spec.ExecutionMode != ExecutionModeReadOnly {
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
	case "ZDOTDIR", "BASH_ENV", "ENV", "LARKSUITE_CLI_CONFIG_DIR", "LARK_CHANNEL", "LARK_CHANNEL_HOME", "LARK_CHANNEL_PROFILE", "LARK_CHANNEL_CONFIG":
		return true
	default:
		return false
	}
}

func isReservedSessionEnvKey(key string) bool {
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "HOME", "CODEX_HOME", "OPENAI_BASE_URL", "OPENAI_API_KEY", "OPENAI_MODEL", "LARKSUITE_CLI_CONFIG_DIR", "LARK_CHANNEL", "LARK_CHANNEL_HOME", "LARK_CHANNEL_PROFILE", "LARK_CHANNEL_CONFIG":
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

func buildSessionEnvironment(spec SessionSpec) ([]string, map[string]string, error) {
	base := buildSessionEnv(spec)
	if strings.TrimSpace(spec.CodexHomeDir) == "" {
		return base, nil, nil
	}
	store, err := extensionStore(spec.CodexHomeDir)
	if err != nil {
		return nil, nil, err
	}
	projections, err := store.List()
	if err != nil {
		return nil, nil, err
	}
	values := make(map[string]string, len(base))
	for _, entry := range base {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
	}
	contributed := make(map[string]string)
	digests := make(map[string]string, len(projections))
	for _, projection := range projections {
		for key, value := range projection.Environment {
			if previous, ok := contributed[key]; ok && previous != value {
				return nil, nil, fmt.Errorf("conflicting extension environment key %q", key)
			}
			if previous, ok := spec.Profile.Env[key]; ok && previous != value {
				return nil, nil, fmt.Errorf("extension environment key %q conflicts with the Agent profile", key)
			}
			contributed[key] = value
			values[key] = value
		}
		digests[projection.Name] = projection.Digest
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out, digests, nil
}
