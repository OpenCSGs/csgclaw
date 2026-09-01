package codex

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	larkCLIConfigDirName = "lark-cli"
	larkCLISourceDirName = "lark-cli-source"
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
	if codexHomeDir := strings.TrimSpace(spec.CodexHomeDir); codexHomeDir != "" {
		if hasFeishuLarkCLIBinding(codexHomeDir, os.Stat) {
			envMap["LARKSUITE_CLI_CONFIG_DIR"] = larkCLIConfigDir(codexHomeDir)
			envMap["LARK_CHANNEL"] = "1"
			envMap["LARK_CHANNEL_HOME"] = codexHomeDir
			envMap["LARK_CHANNEL_PROFILE"] = strings.TrimSpace(spec.AgentID)
			envMap["LARK_CHANNEL_CONFIG"] = larkCLISourceConfigPath(codexHomeDir)
		}
	}
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

func larkCLIConfigDir(codexHomeDir string) string {
	return filepath.Join(codexHomeDir, larkCLIConfigDirName)
}

func larkCLISourceConfigPath(codexHomeDir string) string {
	return filepath.Join(codexHomeDir, larkCLISourceDirName, "config.json")
}

func larkCLIBindMarkerPath(codexHomeDir string) string {
	return filepath.Join(codexHomeDir, larkCLISourceDirName, "bound.json")
}

func hasFeishuLarkCLIBinding(codexHomeDir string, stat func(string) (os.FileInfo, error)) bool {
	codexHomeDir = strings.TrimSpace(codexHomeDir)
	if codexHomeDir == "" || stat == nil {
		return false
	}
	for _, path := range []string{larkCLISourceConfigPath(codexHomeDir), larkCLIBindMarkerPath(codexHomeDir)} {
		info, err := stat(path)
		if err != nil || info.IsDir() {
			return false
		}
	}
	return true
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
