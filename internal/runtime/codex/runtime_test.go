package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/codexcli"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
)

type fakeBinaryProvider struct {
	path string
	err  error
}

func (f fakeBinaryProvider) Ensure(context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.path, nil
}

type fakeManager struct {
	start  func(context.Context, SessionSpec) (*Session, error)
	stop   func(context.Context, SessionHandle) error
	live   func(SessionHandle) (*Session, error)
	get    func(SessionHandle) (*Session, error)
	prompt func(context.Context, SessionHandle, PromptRequest) (PromptResponse, error)
}

type fakeClosableManager struct {
	fakeManager
	close func() error
}

func (f *fakeClosableManager) Close() error {
	if f.close != nil {
		return f.close()
	}
	return nil
}

func (f fakeManager) LiveSession(handle SessionHandle) (*Session, error) {
	if f.live != nil {
		return f.live(handle)
	}
	return f.Session(handle)
}

func (f fakeManager) Start(ctx context.Context, spec SessionSpec) (*Session, error) {
	return f.start(ctx, spec)
}

func (f fakeManager) Stop(ctx context.Context, handle SessionHandle) error {
	if f.stop != nil {
		return f.stop(ctx, handle)
	}
	return nil
}

func (f fakeManager) Session(handle SessionHandle) (*Session, error) {
	if f.get != nil {
		return f.get(handle)
	}
	return nil, os.ErrNotExist
}

func (f fakeManager) Prompt(ctx context.Context, handle SessionHandle, req PromptRequest) (PromptResponse, error) {
	if f.prompt != nil {
		return f.prompt(ctx, handle, req)
	}
	return PromptResponse{}, os.ErrNotExist
}

func newTestCodexRuntime(root string, resolve func(agentruntime.Handle) (AgentRef, error)) *Runtime {
	return New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: resolve,
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-test",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					ProcessID:    os.Getpid(),
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
		},
	})
}

func TestRuntimeCreateStartAndInfo(t *testing.T) {
	root := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	if err := os.MkdirAll(filepath.Join(hostHome, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostHome, ".codex", "auth.json"), []byte(`{"tokens":{"access_token":"access","refresh_token":"refresh"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:           "u-alice",
				Name:         "alice",
				RuntimeID:    h.RuntimeID,
				Instructions: "Use concise Go comments.",
				Profile: agentruntime.Profile{
					ModelID: "gpt-5.5",
					BaseURL: "https://runtime.example/v1",
					APIKey:  "runtime-key",
				},
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				if spec.WorkspaceDir == "" || spec.HomeDir == "" || spec.CodexHomeDir == "" {
					t.Fatalf("expected runtime directories to be populated")
				}
				if want := hostHome; spec.HomeDir != want {
					t.Fatalf("HomeDir = %q, want host HOME %q", spec.HomeDir, want)
				}
				if want := filepath.Join(root, "agent-alice", ".codex", "home"); spec.CodexHomeDir != want {
					t.Fatalf("CodexHomeDir = %q, want %q", spec.CodexHomeDir, want)
				}
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-1",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					ProcessID:    os.Getpid(),
					CreatedAt:    time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC),
					StartedAt:    time.Date(2026, 5, 5, 8, 0, 1, 0, time.UTC),
				}, nil
			},
		},
	})

	handle, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
		Profile: agentruntime.Profile{
			ModelID: "gpt-5.5",
			BaseURL: "https://runtime.example/v1",
			APIKey:  "runtime-key",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if handle.HandleID != "sess-1" {
		t.Fatalf("Create() handle id = %q, want sess-1", handle.HandleID)
	}

	info, err := rt.Info(context.Background(), handle)
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.State != agentruntime.StateRunning {
		t.Fatalf("Info() state = %q, want %q", info.State, agentruntime.StateRunning)
	}
	if info.HandleID != "sess-1" {
		t.Fatalf("Info() handle id = %q, want sess-1", info.HandleID)
	}

	metaPath := filepath.Join(root, "agent-alice", ".codex", runtimeFileName)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read runtime metadata: %v", err)
	}
	var meta runtimeMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("unmarshal runtime metadata: %v", err)
	}
	if meta.BinaryPath != "/tmp/codex" {
		t.Fatalf("runtime metadata binary path = %q", meta.BinaryPath)
	}

	if _, err := os.Stat(filepath.Join(root, "agent-alice", ".codex", "home", "auth.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed proxy runtime auth stat error = %v, want missing", err)
	}

	assertRuntimeConfigContains(t, filepath.Join(root, "agent-alice", ".codex", "home", configFileName),
		`model = "gpt-5.5"`,
		`model_provider = "proxy"`,
		`model_catalog_json = "model_catalog.json"`,
		`wire_api = "responses"`,
		`supports_websockets = false`,
	)
	assertRuntimeModelCatalog(t, filepath.Join(root, "agent-alice", ".codex", "home", modelCatalogFileName), "gpt-5.5")
	agentsRaw, err := os.ReadFile(filepath.Join(root, "agent-alice", ".codex", "home", "AGENTS.md"))
	if err != nil {
		t.Fatalf("read codex home AGENTS.md: %v", err)
	}
	agentsText := string(agentsRaw)
	if !strings.Contains(agentsText, "BEGIN CSGCLAW-INSTRUCTIONS (auto-generated; do not edit)") {
		t.Fatalf("codex home AGENTS.md missing instructions block:\n%s", agentsText)
	}
	if !strings.Contains(agentsText, "Use concise Go comments.") {
		t.Fatalf("codex home AGENTS.md missing agent instructions:\n%s", agentsText)
	}
}

func TestRuntimeCloseClosesSessionManager(t *testing.T) {
	closeCalls := 0
	manager := &fakeClosableManager{
		fakeManager: fakeManager{
			start: func(context.Context, SessionSpec) (*Session, error) {
				return nil, nil
			},
		},
		close: func() error {
			closeCalls++
			return nil
		},
	}
	rt := New(Dependencies{Manager: manager})

	if err := rt.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("session manager Close() calls = %d, want 1", closeCalls)
	}
}

func TestRuntimeNewStopsUntrackedProcessesBeforeStartingSession(t *testing.T) {
	root := t.TempDir()
	var steps []string
	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(string) (string, error) {
			return filepath.Join(root, "agent-manager"), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        agent.ManagerUserID,
				Name:      agent.ManagerName,
				RuntimeID: h.RuntimeID,
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				steps = append(steps, "start")
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "manager-thread",
					RuntimeDir:   spec.RuntimeDir,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
				}, nil
			},
		},
		StopRuntimeProcesses: func(path string) ([]int, error) {
			steps = append(steps, "stop-untracked")
			want := filepath.Join(root, "agent-manager", ".codex")
			if path != want {
				t.Fatalf("StopRuntimeProcesses() path = %q, want %q", path, want)
			}
			return []int{123}, nil
		},
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-agent-manager",
		AgentID:   agent.ManagerUserID,
		AgentName: agent.ManagerName,
	}); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got, want := steps, []string{"stop-untracked", "start"}; !slices.Equal(got, want) {
		t.Fatalf("New() steps = %q, want %q", got, want)
	}
}

func TestRestartRequiredReturnsTrueWhenLocalWorkspaceDirChanges(t *testing.T) {
	rt := &Runtime{}
	got, err := rt.RestartRequired(agentruntime.RuntimeConfigChange{
		Previous: agentruntime.RuntimeConfigSnapshot{
			Options: map[string]any{"local_workspace_dir": "/tmp/old"},
		},
		Current: agentruntime.RuntimeConfigSnapshot{
			Options: map[string]any{"local_workspace_dir": "/tmp/new"},
		},
	})
	if err != nil {
		t.Fatalf("RestartRequired() error = %v", err)
	}
	if !got {
		t.Fatal("RestartRequired() = false, want true when local_workspace_dir changes")
	}
}

func TestRestartRequiredIgnoresProfileChanges(t *testing.T) {
	rt := &Runtime{}
	got, err := rt.RestartRequired(agentruntime.RuntimeConfigChange{
		Previous: agentruntime.RuntimeConfigSnapshot{
			Profile: agentruntime.RuntimeProfileConfig{
				ModelID: "gpt-5.5",
			},
		},
		Current: agentruntime.RuntimeConfigSnapshot{
			Profile: agentruntime.RuntimeProfileConfig{
				ModelID: "gpt-5.6",
			},
		},
	})
	if err != nil {
		t.Fatalf("RestartRequired() error = %v", err)
	}
	if got {
		t.Fatal("RestartRequired() = true, want false when only profile changes")
	}
}

func TestRuntimeCreateUsesLocalWorkspaceDir(t *testing.T) {
	root := t.TempDir()
	workspaceRoot := filepath.Join(root, "project")
	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:           "u-alice",
				Name:         "alice",
				RuntimeID:    h.RuntimeID,
				Instructions: "Use repo-local files.",
				RuntimeOptions: map[string]any{
					"local_workspace_dir": workspaceRoot,
				},
				Profile: agentruntime.Profile{
					ModelID: "gpt-5.5",
				},
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				if spec.WorkspaceDir != workspaceRoot {
					t.Fatalf("WorkspaceDir = %q, want %q", spec.WorkspaceDir, workspaceRoot)
				}
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-local",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
		},
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
		Profile:   agentruntime.Profile{ModelID: "gpt-5.5"},
	}); err != nil {
		t.Fatalf("New() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "agent-alice", ".codex", "home", "AGENTS.md"))
	if err != nil {
		t.Fatalf("read codex home AGENTS.md: %v", err)
	}
	if !strings.Contains(string(raw), "Use repo-local files.") {
		t.Fatalf("codex home AGENTS.md = %q, want instructions block", string(raw))
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "AGENTS.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace AGENTS.md stat error = %v, want not exist", err)
	}
}

func TestRefreshCodexHomeAgentsFileCreatesManagedFileWhenMissing(t *testing.T) {
	root := t.TempDir()
	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "u-alice", Name: "alice", RuntimeID: h.RuntimeID, Instructions: "Prefer targeted tests."}, nil
	})

	if err := rt.RefreshCodexHomeAgentsFile(context.Background(), agentruntime.Handle{RuntimeID: "rt-u-alice"}); err != nil {
		t.Fatalf("RefreshCodexHomeAgentsFile() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "agent-alice", ".codex", "home", "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "Prefer targeted tests.") {
		t.Fatalf("AGENTS.md = %q, want agent instructions", text)
	}
	if !strings.Contains(text, "END CSGCLAW-INSTRUCTIONS") {
		t.Fatalf("AGENTS.md = %q, want instructions block end marker", text)
	}
}

func TestRefreshCodexHomeAgentsFileAddsManagerConnectorRules(t *testing.T) {
	root := t.TempDir()
	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: agent.ManagerUserID, Name: agent.ManagerName, RuntimeID: h.RuntimeID, Instructions: "Stay focused."}, nil
	})

	if err := rt.RefreshCodexHomeAgentsFile(context.Background(), agentruntime.Handle{RuntimeID: "rt-agent-manager"}); err != nil {
		t.Fatalf("RefreshCodexHomeAgentsFile() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "agent-manager", ".codex", "home", "AGENTS.md"))
	if err != nil {
		t.Fatalf("read manager AGENTS.md: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"GitHub Connector Access",
		"/api/v1/agents/agent-manager/connectors/github/credential",
		"X-CSGClaw-Connector-Capability: $CSGCLAW_CONNECTOR_CAPABILITY",
		"`access_token`",
		"Do not rely on connector tokens from environment variables",
		"external Codex GitHub app connector",
		"reconnect the CSGClaw GitHub OAuth connector",
		"Historical Attachment Recovery",
		"csgclaw-cli message list --channel <current_channel> --room-id <target_room_id>",
		"jq '[.[] as $message | ($message.attachments // [])[]",
		"/api/v1/attachments/<attachment-id>",
		"curl -fsS -H \"Authorization: Bearer ${CSGCLAW_ACCESS_TOKEN:?}\"",
		"until durable CSGClaw history has been checked",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("manager AGENTS.md missing %q in %q", want, text)
		}
	}
}

func TestDecodeRuntimeOptionsNormalizesLocalWorkspaceDir(t *testing.T) {
	got, err := DecodeRuntimeOptions(map[string]any{
		"local_workspace_dir": "  /tmp/project  ",
	})
	if err != nil {
		t.Fatalf("DecodeRuntimeOptions() error = %v", err)
	}
	if got.LocalWorkspaceDir != "/tmp/project" {
		t.Fatalf("LocalWorkspaceDir = %q, want /tmp/project", got.LocalWorkspaceDir)
	}
}

func TestDecodeRuntimeOptionsRejectsNonStringLocalWorkspaceDir(t *testing.T) {
	_, err := DecodeRuntimeOptions(map[string]any{
		"local_workspace_dir": 42,
	})
	if err == nil {
		t.Fatal("DecodeRuntimeOptions() error = nil, want error")
	}
}

func TestRuntimeOptionsSchemaIncludesLocalWorkspaceDir(t *testing.T) {
	rt := newTestCodexRuntime(t.TempDir(), func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "u-alice", Name: "alice", RuntimeID: h.RuntimeID}, nil
	})

	got := rt.RuntimeOptionsSchema()
	if len(got) != 1 {
		t.Fatalf("RuntimeOptionsSchema() len = %d, want 1", len(got))
	}
	if got[0].Path != "local_workspace_dir" {
		t.Fatalf("RuntimeOptionsSchema()[0].Path = %q, want local_workspace_dir", got[0].Path)
	}
	if got[0].Type != "directory" {
		t.Fatalf("RuntimeOptionsSchema()[0].Type = %q, want directory", got[0].Type)
	}
	if got[0].LabelZh != "本地工作目录" {
		t.Fatalf("RuntimeOptionsSchema()[0].LabelZh = %q, want 本地工作目录", got[0].LabelZh)
	}
}

func TestRefreshCodexHomeAgentsFileAppendsManagedBlockToExistingUserFile(t *testing.T) {
	root := t.TempDir()
	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "u-alice", Name: "alice", RuntimeID: h.RuntimeID, Instructions: "Use Chinese in docs."}, nil
	})

	agentsPath := filepath.Join(root, "agent-alice", ".codex", "home", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(agentsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentsPath, []byte("# User AGENTS\n\nKeep custom notes here.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := rt.RefreshCodexHomeAgentsFile(context.Background(), agentruntime.Handle{RuntimeID: "rt-u-alice"}); err != nil {
		t.Fatalf("RefreshCodexHomeAgentsFile() error = %v", err)
	}

	raw, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "# User AGENTS\n\nKeep custom notes here.") {
		t.Fatalf("AGENTS.md = %q, want preserved user content", text)
	}
	if !strings.Contains(text, "Use Chinese in docs.") {
		t.Fatalf("AGENTS.md = %q, want appended instructions block", text)
	}
}

func TestProvisionTemplateInstructionsDefersManagedProfileBlockUntilUpdate(t *testing.T) {
	root := t.TempDir()
	instructions := ""
	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "agent-alice", Name: "alice", RuntimeID: h.RuntimeID, Instructions: instructions}, nil
	})

	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID:            "rt-agent-alice",
		AgentID:              "agent-alice",
		AgentName:            "alice",
		Instructions:         "request instructions must be ignored",
		TemplateInstructions: "# Template Base\n\nKeep this content.\n",
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	agentsPath := filepath.Join(root, "agent-alice", ".codex", "home", "AGENTS.md")
	raw, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read provisioned AGENTS.md: %v", err)
	}
	if got, want := string(raw), "# Template Base\n\nKeep this content.\n"; got != want {
		t.Fatalf("provisioned AGENTS.md = %q, want %q", got, want)
	}
	if strings.Contains(string(raw), "CSGCLAW-INSTRUCTIONS") || strings.Contains(string(raw), "request instructions") {
		t.Fatalf("provisioned AGENTS.md unexpectedly contains managed profile instructions: %q", string(raw))
	}
	workspaceAgentsPath := filepath.Join(root, "agent-alice", ".codex", "workspace", "AGENTS.md")
	if workspaceRaw, err := os.ReadFile(workspaceAgentsPath); err == nil {
		if strings.Contains(string(workspaceRaw), "# Template Base") {
			t.Fatalf("workspace AGENTS.md contains agent-global template instructions: %q", string(workspaceRaw))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read workspace AGENTS.md: %v", err)
	}

	instructions = "Prefer targeted tests."
	if err := rt.RefreshCodexHomeAgentsFile(context.Background(), agentruntime.Handle{RuntimeID: "rt-agent-alice"}); err != nil {
		t.Fatalf("RefreshCodexHomeAgentsFile() error = %v", err)
	}
	raw, err = os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read updated AGENTS.md: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "# Template Base\n\nKeep this content.") || !strings.Contains(text, "Prefer targeted tests.") {
		t.Fatalf("updated AGENTS.md did not preserve base and add managed instructions: %q", text)
	}
}

func TestRefreshCodexHomeAgentsFileReplacesExistingInstructionsBlock(t *testing.T) {
	root := t.TempDir()
	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "u-alice", Name: "alice", RuntimeID: h.RuntimeID, Instructions: "New instructions."}, nil
	})

	agentsPath := filepath.Join(root, "agent-alice", ".codex", "home", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(agentsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	oldBlock := `<!-- BEGIN CSGCLAW-INSTRUCTIONS (auto-generated; do not edit) -->

# CSGClaw Instructions

old block

<!-- END CSGCLAW-INSTRUCTIONS -->
`
	if err := os.WriteFile(agentsPath, []byte("# User AGENTS\n\n"+oldBlock+"\nProject footer.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := rt.RefreshCodexHomeAgentsFile(context.Background(), agentruntime.Handle{RuntimeID: "rt-u-alice"}); err != nil {
		t.Fatalf("RefreshCodexHomeAgentsFile() error = %v", err)
	}

	raw, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	text := string(raw)
	if strings.Contains(text, "old block") {
		t.Fatalf("AGENTS.md = %q, want old instructions block removed", text)
	}
	if !strings.Contains(text, "New instructions.") {
		t.Fatalf("AGENTS.md = %q, want new instructions block inserted", text)
	}
	if !strings.Contains(text, "Project footer.") {
		t.Fatalf("AGENTS.md = %q, want suffix preserved", text)
	}
	if !strings.Contains(text, "BEGIN CSGCLAW-INSTRUCTIONS (auto-generated; do not edit)") {
		t.Fatalf("AGENTS.md = %q, want new marker present", text)
	}
}

func TestRefreshCodexHomeAgentsFileIsIdempotent(t *testing.T) {
	root := t.TempDir()
	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "u-alice", Name: "alice", RuntimeID: h.RuntimeID, Instructions: "Stay focused."}, nil
	})

	handle := agentruntime.Handle{RuntimeID: "rt-u-alice"}
	if err := rt.RefreshCodexHomeAgentsFile(context.Background(), handle); err != nil {
		t.Fatalf("first RefreshCodexHomeAgentsFile() error = %v", err)
	}
	agentsPath := filepath.Join(root, "agent-alice", ".codex", "home", "AGENTS.md")
	first, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read first AGENTS.md: %v", err)
	}

	if err := rt.RefreshCodexHomeAgentsFile(context.Background(), handle); err != nil {
		t.Fatalf("second RefreshCodexHomeAgentsFile() error = %v", err)
	}
	second, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read second AGENTS.md: %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("AGENTS.md changed between refreshes:\nfirst:\n%s\nsecond:\n%s", string(first), string(second))
	}
	if got, want := strings.Count(string(second), "BEGIN CSGCLAW-INSTRUCTIONS (auto-generated; do not edit)"), 1; got != want {
		t.Fatalf("instructions block start count = %d, want %d", got, want)
	}
}

func TestSyncWorkspaceAgentsFileRefreshesCodexHomeAgentsFileWithoutTouchingWorkspace(t *testing.T) {
	root := t.TempDir()
	oldWorkspace := filepath.Join(root, "old")
	newWorkspace := filepath.Join(root, "new")
	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{
			ID:           "u-alice",
			Name:         "alice",
			RuntimeID:    h.RuntimeID,
			Instructions: "Stay focused.",
			RuntimeOptions: map[string]any{
				"local_workspace_dir": newWorkspace,
			},
		}, nil
	})

	homeAgentsPath := filepath.Join(root, "agent-alice", ".codex", "home", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(homeAgentsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(homeAgentsPath, []byte("# User AGENTS\n\nKeep this.\n\n"+agent.RenderAgentsInstructionsBlock("Old instructions.")), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := rt.SyncWorkspaceAgentsFile(context.Background(), agentruntime.Handle{RuntimeID: "rt-u-alice"}, map[string]any{
		"local_workspace_dir": oldWorkspace,
	}); err != nil {
		t.Fatalf("SyncWorkspaceAgentsFile() error = %v", err)
	}

	homeRaw, err := os.ReadFile(homeAgentsPath)
	if err != nil {
		t.Fatalf("read codex home AGENTS.md: %v", err)
	}
	homeText := string(homeRaw)
	if !strings.Contains(homeText, "# User AGENTS\n\nKeep this.") {
		t.Fatalf("codex home AGENTS.md = %q, want user content preserved", homeText)
	}
	if strings.Contains(homeText, "Old instructions.") {
		t.Fatalf("codex home AGENTS.md = %q, want old managed block replaced", homeText)
	}
	if !strings.Contains(homeText, "Stay focused.") {
		t.Fatalf("codex home AGENTS.md = %q, want new instructions", homeText)
	}
	for _, workspace := range []string{oldWorkspace, newWorkspace} {
		if _, err := os.Stat(filepath.Join(workspace, "AGENTS.md")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s AGENTS.md stat error = %v, want not exist", workspace, err)
		}
	}
}

func TestSyncWorkspaceAgentsFileCreatesCodexHomeAgentsFileWhenWorkspaceChanges(t *testing.T) {
	root := t.TempDir()
	oldWorkspace := filepath.Join(root, "old")
	newWorkspace := filepath.Join(root, "new")
	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{
			ID:           "u-alice",
			Name:         "alice",
			RuntimeID:    h.RuntimeID,
			Instructions: "Stay focused.",
			RuntimeOptions: map[string]any{
				"local_workspace_dir": newWorkspace,
			},
		}, nil
	})

	if err := rt.SyncWorkspaceAgentsFile(context.Background(), agentruntime.Handle{RuntimeID: "rt-u-alice"}, map[string]any{
		"local_workspace_dir": oldWorkspace,
	}); err != nil {
		t.Fatalf("SyncWorkspaceAgentsFile() error = %v", err)
	}

	homeRaw, err := os.ReadFile(filepath.Join(root, "agent-alice", ".codex", "home", "AGENTS.md"))
	if err != nil {
		t.Fatalf("read codex home AGENTS.md: %v", err)
	}
	if !strings.Contains(string(homeRaw), "Stay focused.") {
		t.Fatalf("codex home AGENTS.md = %q, want instructions", string(homeRaw))
	}
	for _, workspace := range []string{oldWorkspace, newWorkspace} {
		if _, err := os.Stat(filepath.Join(workspace, "AGENTS.md")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s AGENTS.md stat error = %v, want not exist", workspace, err)
		}
	}
}

func TestRuntimeStopAndDelete(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	calledStop := false
	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "u-alice", Name: "alice", RuntimeID: h.RuntimeID}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-2",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
			stop: func(_ context.Context, handle SessionHandle) error {
				calledStop = true
				return nil
			},
		},
	})

	handle, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	state, err := rt.Stop(context.Background(), handle)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !calledStop {
		t.Fatalf("Stop() did not call manager.Stop")
	}
	if state != agentruntime.StateStopped {
		t.Fatalf("Stop() state = %q, want %q", state, agentruntime.StateStopped)
	}

	if err := rt.Delete(context.Background(), handle); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "agent-alice", ".codex")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime dir still exists, stat err = %v", err)
	}
}

func TestBuildSessionEnvOnlyInjectsOpenAIAPIKey(t *testing.T) {
	t.Setenv("HOME", "/host-home")
	t.Setenv("OPENAI_BASE_URL", "https://host.example/v1")
	t.Setenv("OPENAI_API_KEY", "host-key")
	t.Setenv("OPENAI_MODEL", "host-model")
	t.Setenv("ZDOTDIR", "/host-zdotdir")
	t.Setenv("BASH_ENV", "/host-bashenv")
	t.Setenv("ENV", "/host-env")

	env := buildSessionEnv(SessionSpec{
		HomeDir:      "/host-home",
		CodexHomeDir: "/tmp/runtime-codex-home",
		Profile: agentruntime.Profile{
			ModelID: " gpt-5.5 ",
			BaseURL: "https://runtime.example/v1/",
			APIKey:  " runtime-key ",
			Env: map[string]string{
				"OPENAI_BASE_URL": "https://env.example/v1",
				"OPENAI_API_KEY":  "env-key",
				"OPENAI_MODEL":    "env-model",
				" EXTRA_FLAG ":    " 1 ",
			},
		},
	})

	envMap := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatalf("invalid env entry %q", entry)
		}
		envMap[key] = value
	}

	if got, want := envMap["HOME"], "/host-home"; got != want {
		t.Fatalf("HOME = %q, want %q", got, want)
	}
	if got, want := envMap["CODEX_HOME"], "/tmp/runtime-codex-home"; got != want {
		t.Fatalf("CODEX_HOME = %q, want %q", got, want)
	}
	if got, want := envMap["OPENAI_API_KEY"], "runtime-key"; got != want {
		t.Fatalf("OPENAI_API_KEY = %q, want %q", got, want)
	}
	if got := envMap["OPENAI_BASE_URL"]; got != "https://host.example/v1" {
		t.Fatalf("OPENAI_BASE_URL = %q, want host value preserved", got)
	}
	if got := envMap["OPENAI_MODEL"]; got != "host-model" {
		t.Fatalf("OPENAI_MODEL = %q, want host value preserved", got)
	}
	if got, want := envMap["EXTRA_FLAG"], "1"; got != want {
		t.Fatalf("EXTRA_FLAG = %q, want %q", got, want)
	}
	for _, key := range []string{"ZDOTDIR", "BASH_ENV", "ENV"} {
		if got, ok := envMap[key]; ok {
			t.Fatalf("%s = %q, want omitted from runtime env", key, got)
		}
	}
}

func TestDeleteRetriesTransientRuntimeDirRemovalError(t *testing.T) {
	root := t.TempDir()
	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-test",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
		},
	})

	handle, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	runtimeDir := filepath.Join(root, "agent-alice", ".codex")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "home", "tmp", "plugins-clone", "plugins"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(runtime tmp dir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "home", "tmp", "plugins-clone", "plugins", "cache.txt"), []byte("cache"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(cache.txt) error = %v", err)
	}

	var removeCalls int
	rt.deps.RemoveAll = func(path string) error {
		removeCalls++
		if path == runtimeDir && removeCalls == 1 {
			return &os.PathError{Op: "unlinkat", Path: filepath.Join(runtimeDir, "home", "stderr.log"), Err: errors.New("The process cannot access the file because it is being used by another process.")}
		}
		return os.RemoveAll(path)
	}

	err = rt.Delete(context.Background(), handle)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if removeCalls != 2 {
		t.Fatalf("RemoveAll() calls = %d, want 2", removeCalls)
	}
	if _, err := os.Stat(runtimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime dir stat error = %v, want not exist", err)
	}
}

func TestDeleteStopsLiveSessionWhenRuntimeMetadataIsMissing(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "agent-manager", ".codex")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(runtime dir) error = %v", err)
	}

	stopCalls := 0
	rt := New(Dependencies{
		AgentHome: func(string) (string, error) {
			return filepath.Join(root, "agent-manager"), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        agent.ManagerUserID,
				Name:      agent.ManagerName,
				RuntimeID: h.RuntimeID,
			}, nil
		},
		Manager: fakeManager{
			stop: func(_ context.Context, handle SessionHandle) error {
				stopCalls++
				if handle.RuntimeID != "rt-agent-manager" {
					t.Fatalf("Stop() runtime id = %q, want rt-agent-manager", handle.RuntimeID)
				}
				return nil
			},
		},
	})

	err := rt.Delete(context.Background(), agentruntime.Handle{RuntimeID: "rt-agent-manager"})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if stopCalls != 1 {
		t.Fatalf("session manager Stop() calls = %d, want 1", stopCalls)
	}
	if _, err := os.Stat(runtimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime dir stat error = %v, want not exist", err)
	}
}

func TestDeleteStopsUntrackedRuntimeProcessesBeforeRemoval(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "agent-manager", ".codex")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(runtime dir) error = %v", err)
	}

	var steps []string
	rt := New(Dependencies{
		AgentHome: func(string) (string, error) {
			return filepath.Join(root, "agent-manager"), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        agent.ManagerUserID,
				Name:      agent.ManagerName,
				RuntimeID: h.RuntimeID,
			}, nil
		},
		Manager: fakeManager{
			stop: func(context.Context, SessionHandle) error {
				steps = append(steps, "stop-session")
				return os.ErrNotExist
			},
		},
		StopRuntimeProcesses: func(path string) ([]int, error) {
			steps = append(steps, "stop-untracked")
			if path != runtimeDir {
				t.Fatalf("StopRuntimeProcesses() path = %q, want %q", path, runtimeDir)
			}
			return []int{123}, nil
		},
		RemoveAll: func(path string) error {
			steps = append(steps, "remove")
			return os.RemoveAll(path)
		},
	})

	if err := rt.Delete(context.Background(), agentruntime.Handle{RuntimeID: "rt-agent-manager"}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if got, want := steps, []string{"stop-session", "stop-untracked", "remove"}; !slices.Equal(got, want) {
		t.Fatalf("Delete() steps = %q, want %q", got, want)
	}
}

func TestRemoveRuntimeDirRetriesLockedPluginCloneFetchHead(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "agent-alice", ".codex")
	fetchHeadPath := filepath.Join(runtimeDir, "home", ".tmp", "plugins-clone-tHtJ6o", ".git", "FETCH_HEAD")
	if err := os.MkdirAll(filepath.Dir(fetchHeadPath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(plugin clone git dir) error = %v", err)
	}
	if err := os.WriteFile(fetchHeadPath, []byte("ref"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(FETCH_HEAD) error = %v", err)
	}

	origInitialDelay := runtimeDirRemoveInitialDelay
	origMaxDelay := runtimeDirRemoveMaxDelay
	origTries := runtimeDirRemoveTries
	runtimeDirRemoveInitialDelay = time.Millisecond
	runtimeDirRemoveMaxDelay = time.Millisecond
	runtimeDirRemoveTries = 6
	t.Cleanup(func() {
		runtimeDirRemoveInitialDelay = origInitialDelay
		runtimeDirRemoveMaxDelay = origMaxDelay
		runtimeDirRemoveTries = origTries
	})

	var removeCalls int
	rt := New(Dependencies{
		RemoveAll: func(path string) error {
			removeCalls++
			if path == runtimeDir && removeCalls < 4 {
				return &os.PathError{
					Op:   "unlinkat",
					Path: fetchHeadPath,
					Err:  syscall.EBUSY,
				}
			}
			return os.RemoveAll(path)
		},
	})

	if err := rt.removeRuntimeDir(context.Background(), "rt-u-alice", runtimeDir); err != nil {
		t.Fatalf("removeRuntimeDir() error = %v", err)
	}
	if removeCalls != 4 {
		t.Fatalf("RemoveAll() calls = %d, want 4", removeCalls)
	}
	if _, err := os.Stat(runtimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime dir stat error = %v, want not exist", err)
	}
}

func TestRemoveRuntimeDirRetriesNonEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "agent-manager", ".codex")
	blockedDir := filepath.Join(runtimeDir, "home", "tmp", "arg0", "codex-arg0zMf3j5")
	if err := os.MkdirAll(blockedDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(blocked runtime dir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(blockedDir, "pending"), []byte("pending"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(pending) error = %v", err)
	}

	origInitialDelay := runtimeDirRemoveInitialDelay
	origMaxDelay := runtimeDirRemoveMaxDelay
	origTries := runtimeDirRemoveTries
	runtimeDirRemoveInitialDelay = time.Millisecond
	runtimeDirRemoveMaxDelay = time.Millisecond
	runtimeDirRemoveTries = 4
	t.Cleanup(func() {
		runtimeDirRemoveInitialDelay = origInitialDelay
		runtimeDirRemoveMaxDelay = origMaxDelay
		runtimeDirRemoveTries = origTries
	})

	var removeCalls int
	rt := New(Dependencies{
		RemoveAll: func(path string) error {
			removeCalls++
			if path == runtimeDir && removeCalls < 3 {
				return &os.PathError{Op: "unlinkat", Path: blockedDir, Err: syscall.ENOTEMPTY}
			}
			return os.RemoveAll(path)
		},
	})

	if err := rt.removeRuntimeDir(context.Background(), "rt-agent-manager", runtimeDir); err != nil {
		t.Fatalf("removeRuntimeDir() error = %v", err)
	}
	if removeCalls != 3 {
		t.Fatalf("RemoveAll() calls = %d, want 3", removeCalls)
	}
}

func TestRemoveRuntimeDirFailureReportsDiagnostics(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "agent-manager", ".codex")
	blockedDir := filepath.Join(runtimeDir, "home", "tmp", "arg0", "codex-arg0zMf3j5")
	if err := os.MkdirAll(blockedDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(blocked runtime dir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(blockedDir, ".nfs0000000000000001"), []byte("pending"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(.nfs file) error = %v", err)
	}

	origInitialDelay := runtimeDirRemoveInitialDelay
	origMaxDelay := runtimeDirRemoveMaxDelay
	origTries := runtimeDirRemoveTries
	runtimeDirRemoveInitialDelay = time.Millisecond
	runtimeDirRemoveMaxDelay = time.Millisecond
	runtimeDirRemoveTries = 2
	t.Cleanup(func() {
		runtimeDirRemoveInitialDelay = origInitialDelay
		runtimeDirRemoveMaxDelay = origMaxDelay
		runtimeDirRemoveTries = origTries
	})

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	var removeCalls int
	rt := New(Dependencies{
		RemoveAll: func(string) error {
			removeCalls++
			return &os.PathError{Op: "unlinkat", Path: blockedDir, Err: syscall.ENOTEMPTY}
		},
	})

	err := rt.removeRuntimeDir(context.Background(), "rt-agent-manager", runtimeDir)
	if err == nil {
		t.Fatal("removeRuntimeDir() error = nil, want diagnostics")
	}
	for _, want := range []string{
		"after 2 attempts",
		`errno="ENOTEMPTY"`,
		`error_path="` + blockedDir + `"`,
		"home/tmp/arg0/codex-arg0zMf3j5/.nfs0000000000000001",
		"filesystem_type=",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("removeRuntimeDir() error = %q, want %q", err, want)
		}
	}
	if removeCalls != 2 {
		t.Fatalf("RemoveAll() calls = %d, want 2", removeCalls)
	}
	for _, want := range []string{
		"codex runtime directory removal blocked; retrying",
		"codex runtime directory removal failed",
		"runtime_id=rt-agent-manager",
		"errno=ENOTEMPTY",
		"remaining_entries=",
		".nfs0000000000000001",
	} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("runtime removal logs = %q, want %q", logs.String(), want)
		}
	}
}

func TestRuntimeInfoNotFound(t *testing.T) {
	t.Parallel()

	rt := New(Dependencies{
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(t.TempDir(), agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "u-missing", Name: "missing", RuntimeID: h.RuntimeID}, nil
		},
	})

	_, err := rt.Info(context.Background(), agentruntime.Handle{RuntimeID: "rt-missing"})
	if !sandbox.IsNotFound(err) {
		t.Fatalf("Info() error = %v, want sandbox not found", err)
	}
}

func TestRuntimeCreateKeepsExistingRuntimeAuth(t *testing.T) {
	root := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	if err := os.MkdirAll(filepath.Join(hostHome, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostHome, ".codex", "auth.json"), []byte(`{"tokens":{"access_token":"host","refresh_token":"host-refresh"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	runtimeAuthPath := filepath.Join(root, "agent-alice", ".codex", "home", "auth.json")
	if err := os.MkdirAll(filepath.Dir(runtimeAuthPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtimeAuthPath, []byte(`{"tokens":{"access_token":"runtime","refresh_token":"runtime-refresh"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-existing-auth",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					ProcessID:    os.Getpid(),
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
		},
	})

	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID: "rt-" + agent.ManagerUserID,
		AgentID:   agent.ManagerUserID,
		AgentName: agent.ManagerName,
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	authRaw, err := os.ReadFile(runtimeAuthPath)
	if err != nil {
		t.Fatalf("read runtime auth: %v", err)
	}
	if string(authRaw) != `{"tokens":{"access_token":"runtime","refresh_token":"runtime-refresh"}}` {
		t.Fatalf("runtime auth = %q, want existing runtime auth preserved", string(authRaw))
	}
	assertRuntimeConfigContains(t, filepath.Join(root, "agent-alice", ".codex", "home", configFileName),
		`sandbox_mode = "workspace-write"`,
		`sandbox_workspace_write.network_access = true`,
		`features.multi_agent = false`,
		`features.default_mode_request_user_input = true`,
		`features.memories = false`,
		`memories.generate_memories = false`,
		`memories.use_memories = false`,
	)
}

func TestRuntimeCreateRemovesStaleAuthForManagedProxy(t *testing.T) {
	root := t.TempDir()
	runtimeAuthPath := filepath.Join(root, "agent-alice", ".codex", "home", "auth.json")
	if err := os.MkdirAll(filepath.Dir(runtimeAuthPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtimeAuthPath, []byte(`{"tokens":{"refresh_token":"stale-refresh"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
				Profile: agentruntime.Profile{
					ModelID: "gpt-5.5",
					BaseURL: "https://runtime.example/v1",
					APIKey:  "runtime-key",
				},
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-managed-auth",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					ProcessID:    os.Getpid(),
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
		},
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
		Profile: agentruntime.Profile{
			ModelID: "gpt-5.5",
			BaseURL: "https://runtime.example/v1",
			APIKey:  "runtime-key",
		},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := os.Stat(runtimeAuthPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed proxy runtime auth stat error = %v, want missing", err)
	}
}

func TestRuntimeCreateWritesConfigWhenHostAuthIsSeeded(t *testing.T) {
	root := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	if err := os.MkdirAll(filepath.Join(hostHome, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostHome, ".codex", "auth.json"), []byte(`{"tokens":{"access_token":"access","refresh_token":"refresh"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
				Profile: agentruntime.Profile{
					ModelID: "gpt-5.5",
					BaseURL: "https://runtime.example/v1",
					APIKey:  "runtime-key",
				},
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-seeded-auth",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					ProcessID:    os.Getpid(),
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
		},
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
		Profile: agentruntime.Profile{
			ModelID: "gpt-5.5",
			BaseURL: "https://runtime.example/v1",
			APIKey:  "runtime-key",
		},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	assertRuntimeConfigContains(t, filepath.Join(root, "agent-alice", ".codex", "home", configFileName),
		`model = "gpt-5.5"`,
		`model_catalog_json = "model_catalog.json"`,
		`wire_api = "responses"`,
		`supports_websockets = false`,
	)
	assertRuntimeModelCatalog(t, filepath.Join(root, "agent-alice", ".codex", "home", modelCatalogFileName), "gpt-5.5")
}

func TestRuntimeSessionManagerHydratesPersistedSession(t *testing.T) {
	withAppServerHelperCommand(t, "resume-success")

	root := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	initialBinary := filepath.Join(t.TempDir(), "initial-codex")
	explicitBinary := filepath.Join(t.TempDir(), "explicit-codex")
	for _, path := range []string{initialBinary, explicitBinary} {
		if err := os.WriteFile(path, []byte("codex"), 0o755); err != nil {
			t.Fatalf("write test codex binary %s: %v", path, err)
		}
	}
	t.Setenv(codexcli.EnvBinaryPath, initialBinary)
	deps := Dependencies{
		BinaryProvider: codexcli.Provider{},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:           "u-alice",
				Name:         "alice",
				RuntimeID:    h.RuntimeID,
				Instructions: "Resume with repo-specific instructions.",
				Profile: agentruntime.Profile{
					ModelID:         "gpt-5",
					BaseURL:         "https://runtime.example/v1",
					APIKey:          "runtime-key",
					ReasoningEffort: "medium",
				},
			}, nil
		},
	}

	rt := New(deps)
	handle, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
		Profile: agentruntime.Profile{
			ModelID:         "gpt-5",
			BaseURL:         "https://runtime.example/v1",
			APIKey:          "runtime-key",
			ReasoningEffort: "medium",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if handle.HandleID != "main-thread" {
		t.Fatalf("initial handle id = %q, want main-thread", handle.HandleID)
	}

	legacyWorkspaceDir := filepath.Join(root, "alice", ".codex", "workspace")
	legacyCodexHomeDir := filepath.Join(root, "alice", ".codex", "home")
	if err := os.MkdirAll(legacyWorkspaceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(legacy workspace) error = %v", err)
	}
	if err := writeJSONFile(os.WriteFile, filepath.Join(root, "agent-alice", ".codex", sessionFileName), sessionMetadata{
		RuntimeID:    "rt-u-alice",
		SessionID:    "main-thread",
		WorkspaceDir: legacyWorkspaceDir,
		HomeDir:      hostHome,
		CodexHomeDir: legacyCodexHomeDir,
		StartedAt:    time.Date(2026, 6, 25, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("write legacy session metadata: %v", err)
	}

	t.Setenv(codexcli.EnvBinaryPath, explicitBinary)
	reloaded := New(deps)
	manager := reloaded.SessionManager()
	session, err := manager.Session(SessionHandle{RuntimeID: "rt-u-alice"})
	if err != nil {
		t.Fatalf("Session() after reload error = %v", err)
	}
	if session.SessionID != "resumed-thread" {
		t.Fatalf("hydrated session id = %q, want resumed-thread", session.SessionID)
	}
	if session.BinaryPath != explicitBinary {
		t.Fatalf("hydrated binary path = %q, want explicit override %q", session.BinaryPath, explicitBinary)
	}
	if want := filepath.Join(root, "agent-alice", ".codex", "workspace"); session.WorkspaceDir != want {
		t.Fatalf("hydrated workspace dir = %q, want %q", session.WorkspaceDir, want)
	}
	if want := filepath.Join(root, "agent-alice", ".codex", "home"); session.CodexHomeDir != want {
		t.Fatalf("hydrated codex home dir = %q, want %q", session.CodexHomeDir, want)
	}
	if _, err := os.Stat(filepath.Join(legacyCodexHomeDir, "AGENTS.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy codex home AGENTS.md stat error = %v, want not exist", err)
	}
	homeAgentsRaw, err := os.ReadFile(filepath.Join(root, "agent-alice", ".codex", "home", "AGENTS.md"))
	if err != nil {
		t.Fatalf("read hydrated codex home AGENTS.md: %v", err)
	}
	if !strings.Contains(string(homeAgentsRaw), "Resume with repo-specific instructions.") {
		t.Fatalf("hydrated codex home AGENTS.md = %q, want refreshed instructions", string(homeAgentsRaw))
	}
	var persisted sessionMetadata
	if err := readJSONFile(os.ReadFile, filepath.Join(root, "agent-alice", ".codex", sessionFileName), &persisted); err != nil {
		t.Fatalf("read hydrated session metadata: %v", err)
	}
	if want := filepath.Join(root, "agent-alice", ".codex", "home"); persisted.CodexHomeDir != want {
		t.Fatalf("persisted codex home dir = %q, want %q", persisted.CodexHomeDir, want)
	}
	if _, err := os.Stat(filepath.Join(session.WorkspaceDir, "AGENTS.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace AGENTS.md stat error = %v, want not exist", err)
	}

	resp, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: "rt-u-alice"}, PromptRequest{
		SessionID: session.SessionID,
		Prompt:    []PromptContentBlock{TextBlock("hello again")},
	})
	if err != nil {
		t.Fatalf("Prompt() after reload error = %v", err)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Fatalf("StopReason = %q, want %q", resp.StopReason, StopReasonEndTurn)
	}

	if _, err := reloaded.Stop(context.Background(), agentruntime.Handle{RuntimeID: "rt-u-alice"}); err != nil {
		t.Fatalf("Stop() after reload error = %v", err)
	}
}

func TestRuntimeCreateWritesConfigWithoutAuth(t *testing.T) {
	root := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	t.Setenv("CODEX_HOME", "")

	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
				Profile: agentruntime.Profile{
					ModelID: "gpt-5.5",
					BaseURL: "https://runtime.example/v1",
					APIKey:  "runtime-key",
				},
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-write-config",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					ProcessID:    os.Getpid(),
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
		},
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
		Profile: agentruntime.Profile{
			ModelID: "gpt-5.5",
			BaseURL: "https://runtime.example/v1",
			APIKey:  "runtime-key",
		},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	configRaw, err := os.ReadFile(filepath.Join(root, "agent-alice", ".codex", "home", configFileName))
	if err != nil {
		t.Fatalf("read seeded runtime config: %v", err)
	}
	configText := string(configRaw)
	for _, want := range []string{
		`model = "gpt-5.5"`,
		`model_provider = "proxy"`,
		`model_catalog_json = "model_catalog.json"`,
		`[model_providers.proxy]`,
		`name = "OpenAI using LLM proxy"`,
		`base_url = "https://runtime.example/v1"`,
		`env_key = "OPENAI_API_KEY"`,
		`wire_api = "responses"`,
		`supports_websockets = false`,
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf("runtime config missing %q:\n%s", want, configText)
		}
	}
	for _, unwanted := range []string{
		`wire_api = "chat"`,
		`runtime-key`,
	} {
		if strings.Contains(configText, unwanted) {
			t.Fatalf("runtime config unexpectedly contains %q:\n%s", unwanted, configText)
		}
	}
}

func TestRuntimeCreateCopiesHostCodexSkills(t *testing.T) {
	root := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	hostSkillsRoot := filepath.Join(hostHome, ".codex", "skills")
	if err := os.MkdirAll(filepath.Join(hostSkillsRoot, "demo", "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostSkillsRoot, "demo", "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostSkillsRoot, "demo", "scripts", "run.sh"), []byte("#!/bin/sh\necho ready\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-copy-skills",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					ProcessID:    os.Getpid(),
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
		},
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	assertRuntimeSkillFile(t, filepath.Join(root, "agent-alice", ".codex", "home", "skills", "demo", "SKILL.md"), "# Demo\n", 0o644)
	assertRuntimeSkillFile(t, filepath.Join(root, "agent-alice", ".codex", "home", "skills", "demo", "scripts", "run.sh"), "#!/bin/sh\necho ready\n", 0o755)
}

func TestRuntimeCreateRefreshesCodexSkillsFromHost(t *testing.T) {
	root := t.TempDir()
	hostCodexHome := filepath.Join(t.TempDir(), "shared-codex-home")
	t.Setenv("CODEX_HOME", hostCodexHome)
	hostSkillsRoot := filepath.Join(hostCodexHome, "skills")
	if err := os.MkdirAll(filepath.Join(hostSkillsRoot, "fresh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostSkillsRoot, "fresh", "SKILL.md"), []byte("# Fresh\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runtimeSkillsRoot := filepath.Join(root, "agent-alice", ".codex", "home", "skills")
	if err := os.MkdirAll(filepath.Join(runtimeSkillsRoot, "stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeSkillsRoot, "stale", "SKILL.md"), []byte("# Stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(runtimeSkillsRoot, "custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeSkillsRoot, "custom", "SKILL.md"), []byte("# Custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(filepath.Dir(runtimeSkillsRoot), ".csgclaw-host-skills.json")
	if err := os.WriteFile(manifestPath, []byte("{\"names\":[\"stale\"]}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-refresh-skills",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					ProcessID:    os.Getpid(),
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
		},
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	assertRuntimeSkillFile(t, filepath.Join(runtimeSkillsRoot, "fresh", "SKILL.md"), "# Fresh\n", 0o644)
	if _, err := os.Stat(filepath.Join(runtimeSkillsRoot, "stale", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale runtime skill should be removed, stat err = %v", err)
	}
	assertRuntimeSkillFile(t, filepath.Join(runtimeSkillsRoot, "custom", "SKILL.md"), "# Custom\n", 0o644)
}

func TestSeedCodexHomeSkillsPreservesUnmanagedSkillsWithoutManifest(t *testing.T) {
	hostCodexHome := filepath.Join(t.TempDir(), "shared-codex-home")
	t.Setenv("CODEX_HOME", hostCodexHome)
	hostSkillsRoot := filepath.Join(hostCodexHome, "skills")
	if err := os.MkdirAll(filepath.Join(hostSkillsRoot, "host-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostSkillsRoot, "host-skill", "SKILL.md"), []byte("# Host\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runtimeCodexHome := filepath.Join(t.TempDir(), "runtime-codex-home")
	runtimeSkillsRoot := filepath.Join(runtimeCodexHome, "skills")
	if err := os.MkdirAll(filepath.Join(runtimeSkillsRoot, "user-added"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeSkillsRoot, "user-added", "SKILL.md"), []byte("# User Added\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := New(Dependencies{})
	if err := rt.seedCodexHomeSkills(runtimeCodexHome); err != nil {
		t.Fatalf("seedCodexHomeSkills() error = %v", err)
	}

	assertRuntimeSkillFile(t, filepath.Join(runtimeSkillsRoot, "host-skill", "SKILL.md"), "# Host\n", 0o644)
	assertRuntimeSkillFile(t, filepath.Join(runtimeSkillsRoot, "user-added", "SKILL.md"), "# User Added\n", 0o644)

	var manifest hostSkillsManifest
	if err := readJSONFile(os.ReadFile, filepath.Join(runtimeCodexHome, hostSkillsManifestName), &manifest); err != nil {
		t.Fatalf("read host skills manifest: %v", err)
	}
	if !slices.Equal(manifest.Names, []string{"host-skill"}) {
		t.Fatalf("host skills manifest = %#v, want host skill only", manifest.Names)
	}
}

func TestRuntimeCreateInstallsManagerTemplate(t *testing.T) {
	root := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)

	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			RuntimeID: h.RuntimeID,
		}, nil
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-" + agent.ManagerUserID,
		AgentID:   agent.ManagerUserID,
		AgentName: agent.ManagerName,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	skillsRoot := filepath.Join(root, agent.ManagerUserID, ".codex", "home", "skills")
	for _, name := range []string{"agent-creator", "agent-teams", "csgclaw-interactive-output-demo", "feishu"} {
		if _, err := os.Stat(filepath.Join(skillsRoot, name, "SKILL.md")); err != nil {
			t.Fatalf("manager template skill %q missing: %v", name, err)
		}
	}
	for _, name := range []string{"agents/openai.yaml", "scripts/emit_demo.py"} {
		if _, err := os.Stat(filepath.Join(skillsRoot, "csgclaw-interactive-output-demo", filepath.FromSlash(name))); err != nil {
			t.Fatalf("manager interactive output demo file %q missing: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(skillsRoot, "basics", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("basics skill stat error = %v, want not installed by manager template", err)
	}

	agentsRaw, err := os.ReadFile(filepath.Join(root, agent.ManagerUserID, ".codex", "home", "AGENTS.md"))
	if err != nil {
		t.Fatalf("read manager AGENTS.md: %v", err)
	}
	if !strings.Contains(string(agentsRaw), "CSGClaw Codex Manager") {
		t.Fatalf("manager AGENTS.md missing codex manager template content:\n%s", string(agentsRaw))
	}

	feishuRaw, err := os.ReadFile(filepath.Join(skillsRoot, "feishu", "SKILL.md"))
	if err != nil {
		t.Fatalf("read feishu manager skill: %v", err)
	}
	if strings.Contains(string(feishuRaw), "/home/picoclaw") {
		t.Fatalf("feishu manager skill contains PicoClaw absolute path:\n%s", string(feishuRaw))
	}

	teamsRaw, err := os.ReadFile(filepath.Join(skillsRoot, "agent-teams", "SKILL.md"))
	if err != nil {
		t.Fatalf("read agent-teams manager skill: %v", err)
	}
	if strings.Contains(string(teamsRaw), "~/.picoclaw") {
		t.Fatalf("agent-teams manager skill contains PicoClaw workspace path:\n%s", string(teamsRaw))
	}
}

func TestRuntimeProvisionInstallsMissingEmbeddedManagerSkillBeforeSessionStart(t *testing.T) {
	root := t.TempDir()
	agentHome := filepath.Join(root, agent.ManagerUserID)
	skillRoot := filepath.Join(agentHome, hostStateDirName, homeDirName, "skills", "csgclaw-interactive-output-demo")

	rt := New(Dependencies{
		AgentHome: func(id string) (string, error) {
			if id != agent.ManagerUserID {
				t.Fatalf("AgentHome() id = %q, want manager", id)
			}
			return agentHome, nil
		},
	})
	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID: "rt-" + agent.ManagerUserID,
		AgentID:   agent.ManagerUserID,
		AgentName: agent.ManagerName,
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(skillRoot, "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed demo skill: %v", err)
	}
	if !strings.Contains(string(raw), "three-stage CSGClaw structured-output demo") {
		t.Fatalf("installed SKILL.md does not contain the embedded Manager skill:\n%s", raw)
	}
}

func TestRuntimeProvisionUpgradesExistingManagerSkill(t *testing.T) {
	root := t.TempDir()
	agentHome := filepath.Join(root, agent.ManagerUserID)
	skillRoot := filepath.Join(agentHome, hostStateDirName, homeDirName, "skills", "csgclaw-interactive-output-demo")
	customSkillRoot := filepath.Join(filepath.Dir(skillRoot), "custom")
	if err := os.MkdirAll(filepath.Join(skillRoot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(customSkillRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	const existingSkill = "# Existing customized demo\n"
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(existingSkill), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "obsolete.txt"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const customSkill = "# Custom skill\n"
	if err := os.WriteFile(filepath.Join(customSkillRoot, "SKILL.md"), []byte(customSkill), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := New(Dependencies{
		AgentHome: func(id string) (string, error) {
			if id != agent.ManagerUserID {
				t.Fatalf("AgentHome() id = %q, want manager", id)
			}
			return agentHome, nil
		},
	})
	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID: "rt-" + agent.ManagerUserID,
		AgentID:   agent.ManagerUserID,
		AgentName: agent.ManagerName,
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(skillRoot, "SKILL.md"))
	if err != nil {
		t.Fatalf("read existing demo skill: %v", err)
	}
	if strings.Contains(string(raw), existingSkill) || !strings.Contains(string(raw), "three-stage CSGClaw structured-output demo") {
		t.Fatalf("upgraded SKILL.md does not contain the embedded Manager skill:\n%s", raw)
	}
	for _, stalePath := range []string{".git", "obsolete.txt"} {
		if _, err := os.Stat(filepath.Join(skillRoot, stalePath)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stale manager skill path %q still exists, err=%v", stalePath, err)
		}
	}
	assertRuntimeSkillFile(t, filepath.Join(customSkillRoot, "SKILL.md"), customSkill, 0o644)
}

func TestRuntimeCreateDoesNotInstallManagerTemplateForWorker(t *testing.T) {
	root := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)

	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{
			ID:        "agent-alice",
			Name:      "alice",
			RuntimeID: h.RuntimeID,
		}, nil
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-agent-alice",
		AgentID:   "agent-alice",
		AgentName: "alice",
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	skillsRoot := filepath.Join(root, "agent-alice", ".codex", "home", "skills")
	for _, name := range []string{"agent-creator", "agent-teams", "feishu"} {
		if _, err := os.Stat(filepath.Join(skillsRoot, name, "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("worker manager-only skill %q stat error = %v, want not installed", name, err)
		}
	}

	if raw, err := os.ReadFile(filepath.Join(root, "agent-alice", ".codex", "home", "AGENTS.md")); err == nil {
		if strings.Contains(string(raw), "CSGClaw Codex Manager") {
			t.Fatalf("worker AGENTS.md contains manager template content:\n%s", string(raw))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read worker AGENTS.md: %v", err)
	}
}

func TestRuntimeProvisionSeedsCodexWorkerTemplateSkills(t *testing.T) {
	root := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)

	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{
			ID:        "agent-alice",
			Name:      "alice",
			RuntimeID: h.RuntimeID,
		}, nil
	})

	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID: "rt-agent-alice",
		AgentID:   "agent-alice",
		AgentName: "alice",
		Profile:   agentruntime.Profile{ModelID: "gpt-5.5"},
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-agent-alice",
		AgentID:   "agent-alice",
		AgentName: "alice",
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	workspaceRoot := filepath.Join(root, "agent-alice", ".codex", "workspace")
	if _, err := os.Stat(filepath.Join(workspaceRoot, "AGENTS.md")); err != nil {
		t.Fatalf("worker workspace AGENTS.md missing: %v", err)
	}
	skillPath := filepath.Join(root, "agent-alice", ".codex", "home", "skills", "agent-teams", "SKILL.md")
	raw, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read worker template skill: %v", err)
	}
	if !strings.Contains(string(raw), "agent-teams") {
		t.Fatalf("worker template skill missing expected content:\n%s", string(raw))
	}
}

func TestRuntimeCreateWritesManagerMCPServers(t *testing.T) {
	root := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)

	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			RuntimeID: h.RuntimeID,
			Profile: agentruntime.Profile{
				ModelID: "gpt-5.5",
			},
			MCPServers: map[string]any{
				"context7": map[string]any{
					"command": "npx",
					"args":    []any{"-y", "@upstash/context7-mcp"},
				},
			},
		}, nil
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-" + agent.ManagerUserID,
		AgentID:   agent.ManagerUserID,
		AgentName: agent.ManagerName,
		Profile:   agentruntime.Profile{ModelID: "gpt-5.5"},
	}); err != nil {
		t.Fatalf("New() error = %v", err)
	}

	configPath := filepath.Join(root, agent.ManagerUserID, ".codex", "home", "config.toml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read manager codex config: %v", err)
	}
	config := string(raw)
	for _, want := range []string{
		`[mcp_servers."context7"]`,
		`command = "npx"`,
		`args = ["-y", "@upstash/context7-mcp"]`,
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("manager codex config missing %q:\n%s", want, config)
		}
	}
}

func TestRuntimeProvisionSyncsCodexOverlaySkills(t *testing.T) {
	root := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	overlayRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(overlayRoot, "skills", "custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(overlayRoot, "skills", "custom", "SKILL.md"), []byte("# Custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{
			ID:        "agent-alice",
			Name:      "alice",
			RuntimeID: h.RuntimeID,
		}, nil
	})

	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID:        "rt-agent-alice",
		AgentID:          "agent-alice",
		AgentName:        "alice",
		WorkspaceOverlay: overlayRoot,
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-agent-alice",
		AgentID:   "agent-alice",
		AgentName: "alice",
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	assertRuntimeSkillFile(t, filepath.Join(root, "agent-alice", ".codex", "workspace", "skills", "custom", "SKILL.md"), "# Custom\n", 0o644)
	assertRuntimeSkillFile(t, filepath.Join(root, "agent-alice", ".codex", "home", "skills", "custom", "SKILL.md"), "# Custom\n", 0o644)
}

func TestRuntimeCreateOverlaysManagerTemplateAfterHostSkills(t *testing.T) {
	root := t.TempDir()
	hostCodexHome := filepath.Join(t.TempDir(), "shared-codex-home")
	t.Setenv("CODEX_HOME", hostCodexHome)
	hostSkillsRoot := filepath.Join(hostCodexHome, "skills")
	if err := os.MkdirAll(filepath.Join(hostSkillsRoot, "agent-creator"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostSkillsRoot, "agent-creator", "SKILL.md"), []byte("# Host Agent Creator\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			RuntimeID: h.RuntimeID,
		}, nil
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-" + agent.ManagerUserID,
		AgentID:   agent.ManagerUserID,
		AgentName: agent.ManagerName,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, agent.ManagerUserID, ".codex", "home", "skills", "agent-creator", "SKILL.md"))
	if err != nil {
		t.Fatalf("read agent-creator manager skill: %v", err)
	}
	text := string(raw)
	if strings.Contains(text, "# Host Agent Creator") {
		t.Fatalf("host agent-creator skill was not overwritten:\n%s", text)
	}
	if !strings.Contains(text, "Mandatory skill for provisioning any new CSGClaw agent-backed participant or worker") {
		t.Fatalf("agent-creator manager skill missing template content:\n%s", text)
	}
}

func TestRuntimeCreateUpgradesExistingManagerSkillNotSyncedFromHost(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "shared-codex-home"))
	skillRoot := filepath.Join(root, agent.ManagerUserID, hostStateDirName, homeDirName, "skills", "csgclaw-interactive-output-demo")
	if err := os.MkdirAll(filepath.Join(skillRoot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	const existingSkill = "# Existing customized demo\n"
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(existingSkill), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "obsolete.txt"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			RuntimeID: h.RuntimeID,
		}, nil
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-" + agent.ManagerUserID,
		AgentID:   agent.ManagerUserID,
		AgentName: agent.ManagerName,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(skillRoot, "SKILL.md"))
	if err != nil {
		t.Fatalf("read existing demo skill: %v", err)
	}
	if strings.Contains(string(raw), existingSkill) || !strings.Contains(string(raw), "three-stage CSGClaw structured-output demo") {
		t.Fatalf("upgraded SKILL.md does not contain the embedded Manager skill:\n%s", raw)
	}
	for _, stalePath := range []string{".git", "obsolete.txt"} {
		if _, err := os.Stat(filepath.Join(skillRoot, stalePath)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stale manager skill path %q still exists, err=%v", stalePath, err)
		}
	}
}

func TestRuntimeCreateCopiesAndSanitizesHostConfig(t *testing.T) {
	root := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	if err := os.MkdirAll(filepath.Join(hostHome, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	hostConfig := strings.Join([]string{
		`approval_policy = "manual"`,
		`[[skills.config]]`,
		`name = "superpowers:brainstorming"`,
		``,
		`[features]`,
		`multi_agent = true`,
		`default_mode_request_user_input = false`,
		`memories = true`,
		``,
		`[memories]`,
		`generate_memories = true`,
		`use_memories = true`,
		``,
	}, "\n")
	if err := os.WriteFile(filepath.Join(hostHome, ".codex", configFileName), []byte(hostConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
				Profile: agentruntime.Profile{
					ModelID: "gpt-5.5",
					BaseURL: "https://runtime.example/v1",
					APIKey:  "runtime-key",
				},
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-copy-host-config",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					ProcessID:    os.Getpid(),
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
		},
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
		Profile: agentruntime.Profile{
			ModelID: "gpt-5.5",
			BaseURL: "https://runtime.example/v1",
			APIKey:  "runtime-key",
		},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	configRaw, err := os.ReadFile(filepath.Join(root, "agent-alice", ".codex", "home", configFileName))
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	configText := string(configRaw)
	if strings.Contains(configText, "[[skills.config]]") {
		t.Fatalf("runtime config should strip inherited skills.config blocks:\n%s", configText)
	}
	for _, want := range []string{
		`approval_policy = "manual"`,
		csgclawProviderBeginMarker,
		csgclawSandboxBeginMarker,
		csgclawMultiAgentBeginMarker,
		csgclawUserInputBeginMarker,
		csgclawMemoryFeatureBeginMarker,
		csgclawMemoryConfigBeginMarker,
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf("runtime config missing %q:\n%s", want, configText)
		}
	}
	for _, unwanted := range []string{
		`multi_agent = true`,
		`default_mode_request_user_input = false`,
		`memories = true`,
		`generate_memories = true`,
		`use_memories = true`,
	} {
		if strings.Contains(configText, unwanted) {
			t.Fatalf("runtime config still contains stale host directive %q:\n%s", unwanted, configText)
		}
	}
}

func TestConfigureCodexHomeConfigReplacesManagedBlocksIdempotently(t *testing.T) {
	initial := strings.Join([]string{
		csgclawProviderBeginMarker,
		`model = "old-model"`,
		csgclawProviderEndMarker,
		``,
		csgclawSandboxBeginMarker,
		`sandbox_mode = "danger-full-access"`,
		csgclawSandboxEndMarker,
		``,
		`[features]`,
		csgclawMultiAgentBeginMarker,
		`multi_agent = true`,
		csgclawMultiAgentEndMarker,
		csgclawUserInputBeginMarker,
		`default_mode_request_user_input = false`,
		csgclawUserInputEndMarker,
		`memories = true`,
		``,
		`[memories]`,
		csgclawMemoryConfigBeginMarker,
		`generate_memories = true`,
		csgclawMemoryConfigEndMarker,
		`use_memories = true`,
		``,
	}, "\n")

	profile := agentruntime.Profile{
		ModelID: "gpt-5.5",
		BaseURL: "https://runtime.example/v1",
		APIKey:  "runtime-key",
	}
	first := configureCodexHomeConfig(initial, profile, nil)
	second := configureCodexHomeConfig(first, profile, nil)
	if first != second {
		t.Fatalf("configureCodexHomeConfig should be idempotent\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	for _, marker := range []string{
		csgclawProviderBeginMarker,
		csgclawSandboxBeginMarker,
		csgclawMultiAgentBeginMarker,
		csgclawUserInputBeginMarker,
		csgclawMemoryFeatureBeginMarker,
		csgclawMemoryConfigBeginMarker,
	} {
		if got := strings.Count(first, marker); got != 1 {
			t.Fatalf("marker %q count = %d, want 1\n%s", marker, got, first)
		}
	}
	for _, unwanted := range []string{
		`model = "old-model"`,
		`multi_agent = true`,
		`default_mode_request_user_input = false`,
		`generate_memories = true`,
		`use_memories = true`,
	} {
		if strings.Contains(first, unwanted) {
			t.Fatalf("managed config should replace stale directive %q:\n%s", unwanted, first)
		}
	}
	for _, expected := range []string{
		`sandbox_mode = "workspace-write"`,
		`sandbox_workspace_write.network_access = true`,
		`default_mode_request_user_input = true`,
	} {
		if !strings.Contains(first, expected) {
			t.Fatalf("managed config should contain sandbox directive %q:\n%s", expected, first)
		}
	}
}

func TestConfigureCodexHomeConfigIncompleteProfileSkipsProvider(t *testing.T) {
	config := configureCodexHomeConfig("approval_policy = \"manual\"\n", agentruntime.Profile{
		BaseURL: "https://runtime.example/v1",
	}, nil)
	if strings.Contains(config, csgclawProviderBeginMarker) {
		t.Fatalf("config should skip provider block for incomplete profile:\n%s", config)
	}
	for _, want := range []string{
		`approval_policy = "manual"`,
		csgclawSandboxBeginMarker,
		csgclawMultiAgentBeginMarker,
		csgclawMemoryFeatureBeginMarker,
		csgclawMemoryConfigBeginMarker,
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("config missing %q:\n%s", want, config)
		}
	}
}

func TestConfigureCodexHomeConfigRendersMCPServers(t *testing.T) {
	config := configureCodexHomeConfig("approval_policy = \"manual\"\n", agentruntime.Profile{}, map[string]any{
		"context7": map[string]any{
			"command":             "uvx",
			"args":                []any{"context7-mcp"},
			"startup_timeout_sec": float64(90),
			"tool_timeout_sec":    120,
			"enabled":             true,
			"required":            false,
			"enabled_tools":       []any{"search"},
			"disabled_tools":      []any{"delete"},
			"env": map[string]any{
				"CONTEXT7_API_KEY": "secret",
			},
		},
		"remote": map[string]any{
			"url":                  "https://mcp.example.com/mcp",
			"bearer_token_env_var": "MCP_TOKEN",
			"headers": map[string]any{
				"X-MCP-Trace": "trace-id",
			},
			"transport": "streamable-http",
		},
	})

	for _, want := range []string{
		csgclawMCPBeginMarker,
		`[mcp_servers."context7"]`,
		`command = "uvx"`,
		`args = ["context7-mcp"]`,
		`env = { "CONTEXT7_API_KEY" = "secret" }`,
		`startup_timeout_sec = 90`,
		`tool_timeout_sec = 120`,
		`enabled = true`,
		`required = false`,
		`enabled_tools = ["search"]`,
		`disabled_tools = ["delete"]`,
		`[mcp_servers."remote"]`,
		`url = "https://mcp.example.com/mcp"`,
		`bearer_token_env_var = "MCP_TOKEN"`,
		`http_headers = { "X-MCP-Trace" = "trace-id" }`,
		csgclawMCPEndMarker,
		`approval_policy = "manual"`,
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("config missing %q:\n%s", want, config)
		}
	}

	parsed, err := parseCodexMCPServers(config)
	if err != nil {
		t.Fatalf("parseCodexMCPServers() error = %v", err)
	}
	context7 := parsed["context7"].(map[string]any)
	if got, want := context7["startup_timeout_sec"], int64(90); got != want {
		t.Fatalf("context7 startup_timeout_sec = %#v, want %#v", got, want)
	}
	remote := parsed["remote"].(map[string]any)
	headers := remote["headers"].(map[string]any)
	if got, want := headers["X-MCP-Trace"], "trace-id"; got != want {
		t.Fatalf("remote headers X-MCP-Trace = %#v, want %q", got, want)
	}
	if _, ok := remote["http_headers"]; ok {
		t.Fatalf("remote retained codex-specific http_headers = %#v, want generic headers", remote)
	}
	if got, want := remote["transport"], "streamable-http"; got != want {
		t.Fatalf("remote transport = %#v, want %q", got, want)
	}
	if _, ok := remote["approval_policy"]; ok {
		t.Fatalf("remote captured root config fields = %#v", remote)
	}
}

func TestConfigureCodexHomeConfigResolvesWorkspaceMCPArgs(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "project")
	config := configureCodexHomeConfigWithWorkspace("approval_policy = \"manual\"\n", agentruntime.Profile{}, map[string]any{
		"filesystem": map[string]any{
			"command": "npx",
			"args": []any{
				"-y",
				"@modelcontextprotocol/server-filesystem",
				"/home/user/workspace",
				"/home/user/workspace/nested",
				"${workspace}",
				"${workspace}/from-placeholder",
				"--root=/home/user/workspace",
			},
		},
	}, workspace)

	parsed, err := parseCodexMCPServers(config)
	if err != nil {
		t.Fatalf("parseCodexMCPServers() error = %v", err)
	}
	filesystem := parsed["filesystem"].(map[string]any)
	got := filesystem["args"]
	want := []any{
		"-y",
		"@modelcontextprotocol/server-filesystem",
		"/home/user/workspace",
		"/home/user/workspace/nested",
		workspace,
		filepath.Join(workspace, "from-placeholder"),
		"--root=/home/user/workspace",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filesystem.args = %#v, want %#v\nconfig:\n%s", got, want, config)
	}
}

func TestConfigureCodexHomeConfigReplacesExistingMCPServerTablesWhenManaged(t *testing.T) {
	existing := strings.Join([]string{
		`approval_policy = "manual"`,
		``,
		`[mcp_servers."manual"]`,
		`command = "uvx"`,
		`args = ["manual-mcp"]`,
		``,
		`[tools]`,
		`enabled = true`,
		``,
	}, "\n")

	config := configureCodexHomeConfig(existing, agentruntime.Profile{}, map[string]any{
		"context7": map[string]any{
			"command": "npx",
			"args":    []any{"context7-mcp"},
		},
	})

	for _, unwanted := range []string{
		`[mcp_servers."manual"]`,
		`manual-mcp`,
	} {
		if strings.Contains(config, unwanted) {
			t.Fatalf("config should replace imported MCP server %q:\n%s", unwanted, config)
		}
	}
	for _, want := range []string{
		`[mcp_servers."context7"]`,
		`command = "npx"`,
		`args = ["context7-mcp"]`,
		`[tools]`,
		`enabled = true`,
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("config missing %q:\n%s", want, config)
		}
	}
}

func TestConfigureCodexHomeConfigClearsExistingMCPServerTablesWhenManagedEmpty(t *testing.T) {
	existing := strings.Join([]string{
		`approval_policy = "manual"`,
		``,
		`[mcp_servers."manual"]`,
		`command = "uvx"`,
		`args = ["manual-mcp"]`,
		``,
		`[tools]`,
		`enabled = true`,
		``,
	}, "\n")

	config := configureCodexHomeConfig(existing, agentruntime.Profile{}, map[string]any{})

	for _, unwanted := range []string{
		`[mcp_servers."manual"]`,
		`manual-mcp`,
	} {
		if strings.Contains(config, unwanted) {
			t.Fatalf("config should clear imported MCP server %q:\n%s", unwanted, config)
		}
	}
	for _, want := range []string{
		csgclawMCPBeginMarker,
		csgclawMCPEndMarker,
		`[tools]`,
		`enabled = true`,
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("config missing %q:\n%s", want, config)
		}
	}
	if strings.Contains(config, `[mcp_servers.`) {
		t.Fatalf("empty managed MCP config should not render MCP server tables:\n%s", config)
	}
}

func TestConfigureCodexHomeConfigKeepsUserMCPServerTablesWhenUnmanaged(t *testing.T) {
	existing := strings.Join([]string{
		`approval_policy = "manual"`,
		``,
		`[mcp_servers."manual"]`,
		`command = "uvx"`,
		`args = ["manual-mcp"]`,
		``,
		`[tools]`,
		`enabled = true`,
		``,
	}, "\n")

	config := configureCodexHomeConfig(existing, agentruntime.Profile{}, nil)

	for _, want := range []string{
		`[mcp_servers."manual"]`,
		`manual-mcp`,
		`[tools]`,
		`enabled = true`,
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("config missing %q:\n%s", want, config)
		}
	}
	if strings.Contains(config, csgclawMCPBeginMarker) {
		t.Fatalf("unmanaged MCP config should not add a managed MCP block:\n%s", config)
	}
}

func TestRuntimeCreateAlwaysWritesResponsesConfig(t *testing.T) {
	root := t.TempDir()
	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-chat",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					ProcessID:    os.Getpid(),
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
		},
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
		Profile: agentruntime.Profile{
			ModelID: "deepseek-v4-pro",
			BaseURL: "https://runtime.example/v1",
			APIKey:  "runtime-key",
		},
	}); err != nil {
		t.Fatalf("New() error = %v", err)
	}

	assertRuntimeConfigContains(t, filepath.Join(root, "agent-alice", ".codex", "home", configFileName),
		`model = "deepseek-v4-pro"`,
		`model_catalog_json = "model_catalog.json"`,
		`wire_api = "responses"`,
		`supports_websockets = false`,
	)
	assertRuntimeModelCatalog(t, filepath.Join(root, "agent-alice", ".codex", "home", modelCatalogFileName), "deepseek-v4-pro")
	configText, err := os.ReadFile(filepath.Join(root, "agent-alice", ".codex", "home", configFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configText), `wire_api = "chat"`) {
		t.Fatalf("runtime config must not contain chat wire_api:\n%s", configText)
	}
}

func TestRuntimeCreateRemovesStaleConfigWhenAuthExists(t *testing.T) {
	root := t.TempDir()
	runtimeRoot := filepath.Join(root, "agent-alice", ".codex", "home")
	if err := os.MkdirAll(runtimeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeRoot, "auth.json"), []byte(`{"tokens":{"access_token":"runtime","refresh_token":"runtime-refresh"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeRoot, configFileName), []byte("stale = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
				Profile: agentruntime.Profile{
					ModelID: "gpt-5.5",
					BaseURL: "https://runtime.example/v1",
					APIKey:  "runtime-key",
				},
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-clean-config",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					ProcessID:    os.Getpid(),
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
		},
	})

	if _, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
		Profile: agentruntime.Profile{
			ModelID: "gpt-5.5",
			BaseURL: "https://runtime.example/v1",
			APIKey:  "runtime-key",
		},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	configRaw, err := os.ReadFile(filepath.Join(runtimeRoot, configFileName))
	if err != nil {
		t.Fatalf("read rewritten config: %v", err)
	}
	configText := string(configRaw)
	for _, want := range []string{`model = "gpt-5.5"`, `wire_api = "responses"`, `stale = true`} {
		if !strings.Contains(configText, want) {
			t.Fatalf("runtime config missing %q:\n%s", want, configText)
		}
	}
	assertRuntimeModelCatalog(t, filepath.Join(runtimeRoot, modelCatalogFileName), "gpt-5.5")
}

func assertRuntimeConfigContains(t *testing.T, path string, wants ...string) {
	t.Helper()

	configRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	configText := string(configRaw)
	for _, want := range wants {
		if !strings.Contains(configText, want) {
			t.Fatalf("runtime config missing %q:\n%s", want, configText)
		}
	}
}

func assertRuntimeModelCatalog(t *testing.T, path, wantModel string) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime model catalog: %v", err)
	}
	if strings.Contains(string(raw), "runtime-key") {
		t.Fatalf("runtime model catalog contains raw API key:\n%s", raw)
	}
	var payload struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode runtime model catalog: %v", err)
	}
	if len(payload.Models) != 1 {
		t.Fatalf("runtime model catalog models = %#v, want one model", payload.Models)
	}
	if got := payload.Models[0]["slug"]; got != wantModel {
		t.Fatalf("runtime model catalog slug = %#v, want %q", got, wantModel)
	}
	if _, ok := payload.Models[0]["model_messages"]; !ok {
		t.Fatalf("runtime model catalog missing model_messages: %#v", payload.Models[0])
	}
}

func assertRuntimeSkillFile(t *testing.T, path, want string, wantPerm os.FileMode) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime skill file %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("runtime skill file %s = %q, want %q", path, string(data), want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat runtime skill file %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != wantPerm {
		t.Fatalf("runtime skill file %s perm = %o, want %o", path, got, wantPerm)
	}
}

func TestRuntimeStartKeepsExistingRunningSession(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	startCalls := 0
	sessionCalls := 0
	var liveSession *Session
	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
			}, nil
		},
		Manager: fakeManager{
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				startCalls++
				liveSession = &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-keep",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					ProcessID:    os.Getpid(),
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}
				return liveSession, nil
			},
			get: func(SessionHandle) (*Session, error) {
				sessionCalls++
				if liveSession == nil {
					return nil, os.ErrNotExist
				}
				cloned := *liveSession
				return &cloned, nil
			},
		},
	})

	handle, err := rt.New(context.Background(), agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if startCalls != 1 {
		t.Fatalf("Create() manager start calls = %d, want 1", startCalls)
	}

	state, err := rt.Start(context.Background(), handle)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if state != agentruntime.StateRunning {
		t.Fatalf("Start() state = %q, want %q", state, agentruntime.StateRunning)
	}
	if startCalls != 1 {
		t.Fatalf("Start() manager start calls = %d, want still 1", startCalls)
	}
	if sessionCalls != 1 {
		t.Fatalf("Start() manager session restore calls = %d, want 1", sessionCalls)
	}
}

func TestRuntimeStartRepairsFailedPersistedSession(t *testing.T) {
	root := t.TempDir()
	oldProcess := exec.Command("sleep", "30")
	if err := oldProcess.Start(); err != nil {
		t.Fatalf("start old process: %v", err)
	}
	processDone := make(chan error, 1)
	go func() { processDone <- oldProcess.Wait() }()
	processExited := false
	t.Cleanup(func() {
		if processExited {
			return
		}
		_ = oldProcess.Process.Kill()
		<-processDone
	})

	restoreErr := errors.New("persisted session cannot be hydrated")
	sessionCalls := 0
	stopCalls := 0
	startCalls := 0
	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
				Profile: agentruntime.Profile{
					BaseURL: "https://api.example/v1",
					APIKey:  "test-key",
					ModelID: "gpt-5.5",
				},
			}, nil
		},
		Manager: fakeManager{
			get: func(SessionHandle) (*Session, error) {
				sessionCalls++
				return nil, restoreErr
			},
			stop: func(context.Context, SessionHandle) error {
				stopCalls++
				return nil
			},
			start: func(_ context.Context, spec SessionSpec) (*Session, error) {
				startCalls++
				now := time.Now().UTC()
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-repaired",
					BinaryPath:   spec.BinaryPath,
					RuntimeDir:   spec.RuntimeDir,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					CreatedAt:    now,
					StartedAt:    now,
				}, nil
			},
		},
	})
	dirs, err := rt.ensureRuntimeDirs("u-alice")
	if err != nil {
		t.Fatalf("ensureRuntimeDirs() error = %v", err)
	}
	markerPath := filepath.Join(dirs.Root, "preserve-me.txt")
	if err := os.WriteFile(markerPath, []byte("runtime data"), 0o600); err != nil {
		t.Fatalf("write runtime marker: %v", err)
	}
	if err := rt.writeMetadata(runtimeMetadata{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
		SessionID: "sess-old",
		ProcessID: oldProcess.Process.Pid,
		State:     agentruntime.StateRunning,
		CreatedAt: time.Now().Add(-time.Hour).UTC(),
		StartedAt: time.Now().Add(-time.Hour).UTC(),
	}); err != nil {
		t.Fatalf("writeMetadata() error = %v", err)
	}

	state, err := rt.Start(context.Background(), agentruntime.Handle{RuntimeID: "rt-u-alice"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if state != agentruntime.StateRunning {
		t.Fatalf("Start() state = %q, want %q", state, agentruntime.StateRunning)
	}
	if sessionCalls != 1 || stopCalls != 1 || startCalls != 1 {
		t.Fatalf("lifecycle calls = session %d/stop %d/start %d, want 1/1/1", sessionCalls, stopCalls, startCalls)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("runtime marker was not preserved during repair: %v", err)
	}
	select {
	case <-processDone:
		processExited = true
	case <-time.After(3 * time.Second):
		t.Fatal("old persisted process was not stopped during repair")
	}
}

func TestRuntimeCreateDetachesManagerStartContext(t *testing.T) {
	root := t.TempDir()
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	if err := os.MkdirAll(filepath.Join(hostHome, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostHome, ".codex", "auth.json"), []byte(`{"tokens":{"access_token":"access","refresh_token":"refresh"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var startCtx context.Context
	rt := New(Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome: func(agentName string) (string, error) {
			return filepath.Join(root, agentName), nil
		},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
				Profile:   agentruntime.Profile{ModelID: "gpt-5.5"},
			}, nil
		},
		Manager: fakeManager{
			start: func(ctx context.Context, spec SessionSpec) (*Session, error) {
				startCtx = ctx
				return &Session{
					RuntimeID:    spec.RuntimeID,
					AgentID:      spec.AgentID,
					AgentName:    spec.AgentName,
					SessionID:    "sess-ctx",
					BinaryPath:   spec.BinaryPath,
					WorkspaceDir: spec.WorkspaceDir,
					HomeDir:      spec.HomeDir,
					CodexHomeDir: spec.CodexHomeDir,
					StderrPath:   spec.StderrPath,
					ProcessID:    os.Getpid(),
					CreatedAt:    time.Now().UTC(),
					StartedAt:    time.Now().UTC(),
				}, nil
			},
		},
	})

	parentCtx, cancel := context.WithCancel(context.Background())
	if _, err := rt.New(parentCtx, agentruntime.Spec{
		RuntimeID: "rt-u-alice",
		AgentID:   "u-alice",
		AgentName: "alice",
		Profile:   agentruntime.Profile{ModelID: "gpt-5.5"},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	cancel()

	if startCtx == nil {
		t.Fatal("manager start context was not captured")
	}
	select {
	case <-startCtx.Done():
		t.Fatal("manager start context was canceled with parent request context")
	default:
	}
}

func TestRuntimeInfoMarksExitedAndFailedWhenProcessIsGone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		exitCode int
		want     agentruntime.State
	}{
		{name: "exited", exitCode: 0, want: agentruntime.StateExited},
		{name: "failed", exitCode: 7, want: agentruntime.StateFailed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			rt := New(Dependencies{
				AgentHome: func(agentName string) (string, error) {
					return filepath.Join(root, agentName), nil
				},
				ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
					return AgentRef{
						ID:        "u-alice",
						Name:      "alice",
						RuntimeID: h.RuntimeID,
					}, nil
				},
			})

			meta := runtimeMetadata{
				RuntimeID: "rt-u-alice",
				AgentID:   "u-alice",
				AgentName: "alice",
				SessionID: "sess-1",
				ProcessID: 999999,
				State:     agentruntime.StateRunning,
				ExitCode:  tc.exitCode,
				CreatedAt: time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC),
				StartedAt: time.Date(2026, 5, 5, 8, 0, 1, 0, time.UTC),
			}
			if err := rt.ensureRuntimeHome("alice"); err != nil {
				t.Fatalf("ensureRuntimeHome() error = %v", err)
			}
			if err := rt.writeMetadata(meta); err != nil {
				t.Fatalf("writeMetadata() error = %v", err)
			}

			info, err := rt.Info(context.Background(), agentruntime.Handle{RuntimeID: "rt-u-alice"})
			if err != nil {
				t.Fatalf("Info() error = %v", err)
			}
			if info.State != tc.want {
				t.Fatalf("Info() state = %q, want %q", info.State, tc.want)
			}

			saved, err := rt.readRuntimeMetadata("rt-u-alice")
			if err != nil {
				t.Fatalf("readRuntimeMetadata() error = %v", err)
			}
			if saved.State != tc.want {
				t.Fatalf("saved state = %q, want %q", saved.State, tc.want)
			}
			if saved.ProcessID != 0 {
				t.Fatalf("saved pid = %d, want 0", saved.ProcessID)
			}
		})
	}
}
