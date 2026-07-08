package codex

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
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
	get    func(SessionHandle) (*Session, error)
	prompt func(context.Context, SessionHandle, PromptRequest) (PromptResponse, error)
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

	authRaw, err := os.ReadFile(filepath.Join(root, "agent-alice", ".codex", "home", "auth.json"))
	if err != nil {
		t.Fatalf("read seeded runtime auth: %v", err)
	}
	if string(authRaw) != `{"tokens":{"access_token":"access","refresh_token":"refresh"}}` {
		t.Fatalf("runtime auth = %q, want copied host auth", string(authRaw))
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
		"`access_token`",
		"Do not rely on connector tokens from environment variables",
		"external Codex GitHub app connector",
		"reconnect the CSGClaw GitHub OAuth connector",
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

func TestDeleteReturnsRuntimeDirRemovalError(t *testing.T) {
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
			return &os.PathError{Op: "unlinkat", Path: filepath.Join(runtimeDir, "home", "logs_2.sqlite"), Err: errors.New("The process cannot access the file because it is being used by another process.")}
		}
		return os.RemoveAll(path)
	}

	err = rt.Delete(context.Background(), handle)
	if err == nil || !strings.Contains(err.Error(), "being used by another process") {
		t.Fatalf("Delete() error = %v, want locked file error", err)
	}
	if removeCalls != 1 {
		t.Fatalf("RemoveAll() calls = %d, want 1", removeCalls)
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
		`features.memories = false`,
		`memories.generate_memories = false`,
		`memories.use_memories = false`,
	)
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
	deps := Dependencies{
		BinaryProvider: fakeBinaryProvider{path: "codex"},
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

	reloaded := New(deps)
	manager := reloaded.SessionManager()
	session, err := manager.Session(SessionHandle{RuntimeID: "rt-u-alice"})
	if err != nil {
		t.Fatalf("Session() after reload error = %v", err)
	}
	if session.SessionID != "resumed-thread" {
		t.Fatalf("hydrated session id = %q, want resumed-thread", session.SessionID)
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
	for _, name := range []string{"agent-creator", "agent-teams", "feishu"} {
		if _, err := os.Stat(filepath.Join(skillsRoot, name, "SKILL.md")); err != nil {
			t.Fatalf("manager template skill %q missing: %v", name, err)
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
		csgclawMemoryFeatureBeginMarker,
		csgclawMemoryConfigBeginMarker,
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf("runtime config missing %q:\n%s", want, configText)
		}
	}
	for _, unwanted := range []string{
		`multi_agent = true`,
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
	first := configureCodexHomeConfig(initial, profile)
	second := configureCodexHomeConfig(first, profile)
	if first != second {
		t.Fatalf("configureCodexHomeConfig should be idempotent\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	for _, marker := range []string{
		csgclawProviderBeginMarker,
		csgclawSandboxBeginMarker,
		csgclawMultiAgentBeginMarker,
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
	} {
		if !strings.Contains(first, expected) {
			t.Fatalf("managed config should contain sandbox directive %q:\n%s", expected, first)
		}
	}
}

func TestConfigureCodexHomeConfigIncompleteProfileSkipsProvider(t *testing.T) {
	config := configureCodexHomeConfig("approval_policy = \"manual\"\n", agentruntime.Profile{
		BaseURL: "https://runtime.example/v1",
	})
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
				return &Session{
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
