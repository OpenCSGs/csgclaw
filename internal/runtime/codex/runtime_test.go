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
				if spec.WorkspaceDir == "" || spec.HomeDir == "" || spec.CodexHomeDir == "" {
					t.Fatalf("expected runtime directories to be populated")
				}
				if want := hostHome; spec.HomeDir != want {
					t.Fatalf("HomeDir = %q, want host HOME %q", spec.HomeDir, want)
				}
				if want := filepath.Join(root, "alice", ".codex", "home"); spec.CodexHomeDir != want {
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

	metaPath := filepath.Join(root, "alice", ".codex", runtimeFileName)
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

	authRaw, err := os.ReadFile(filepath.Join(root, "alice", ".codex", "home", "auth.json"))
	if err != nil {
		t.Fatalf("read seeded runtime auth: %v", err)
	}
	if string(authRaw) != `{"tokens":{"access_token":"access","refresh_token":"refresh"}}` {
		t.Fatalf("runtime auth = %q, want copied host auth", string(authRaw))
	}

	assertRuntimeConfigContains(t, filepath.Join(root, "alice", ".codex", "home", configFileName),
		`model = "gpt-5.5"`,
		`model_provider = "proxy"`,
		`model_catalog_json = "model_catalog.json"`,
		`wire_api = "responses"`,
		`supports_websockets = false`,
	)
	assertRuntimeModelCatalog(t, filepath.Join(root, "alice", ".codex", "home", modelCatalogFileName), "gpt-5.5")
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
	if _, err := os.Stat(filepath.Join(root, "alice", ".codex")); !errors.Is(err, os.ErrNotExist) {
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

	runtimeAuthPath := filepath.Join(root, "alice", ".codex", "home", "auth.json")
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
	assertRuntimeConfigContains(t, filepath.Join(root, "alice", ".codex", "home", configFileName),
		`sandbox_mode = "workspace-write"`,
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

	assertRuntimeConfigContains(t, filepath.Join(root, "alice", ".codex", "home", configFileName),
		`model = "gpt-5.5"`,
		`model_catalog_json = "model_catalog.json"`,
		`wire_api = "responses"`,
		`supports_websockets = false`,
	)
	assertRuntimeModelCatalog(t, filepath.Join(root, "alice", ".codex", "home", modelCatalogFileName), "gpt-5.5")
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
				ID:        "u-alice",
				Name:      "alice",
				RuntimeID: h.RuntimeID,
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

	reloaded := New(deps)
	manager := reloaded.SessionManager()
	session, err := manager.Session(SessionHandle{RuntimeID: "rt-u-alice"})
	if err != nil {
		t.Fatalf("Session() after reload error = %v", err)
	}
	if session.SessionID != "resumed-thread" {
		t.Fatalf("hydrated session id = %q, want resumed-thread", session.SessionID)
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

	configRaw, err := os.ReadFile(filepath.Join(root, "alice", ".codex", "home", configFileName))
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

	assertRuntimeSkillFile(t, filepath.Join(root, "alice", ".codex", "home", "skills", "demo", "SKILL.md"), "# Demo\n", 0o644)
	assertRuntimeSkillFile(t, filepath.Join(root, "alice", ".codex", "home", "skills", "demo", "scripts", "run.sh"), "#!/bin/sh\necho ready\n", 0o755)
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

	runtimeSkillsRoot := filepath.Join(root, "alice", ".codex", "home", "skills")
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

	configRaw, err := os.ReadFile(filepath.Join(root, "alice", ".codex", "home", configFileName))
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
		`sandbox_mode = "danger-full-access"`,
		`multi_agent = true`,
		`generate_memories = true`,
		`use_memories = true`,
	} {
		if strings.Contains(first, unwanted) {
			t.Fatalf("managed config should replace stale directive %q:\n%s", unwanted, first)
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

	assertRuntimeConfigContains(t, filepath.Join(root, "alice", ".codex", "home", configFileName),
		`model = "deepseek-v4-pro"`,
		`model_catalog_json = "model_catalog.json"`,
		`wire_api = "responses"`,
		`supports_websockets = false`,
	)
	assertRuntimeModelCatalog(t, filepath.Join(root, "alice", ".codex", "home", modelCatalogFileName), "deepseek-v4-pro")
	configText, err := os.ReadFile(filepath.Join(root, "alice", ".codex", "home", configFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configText), `wire_api = "chat"`) {
		t.Fatalf("runtime config must not contain chat wire_api:\n%s", configText)
	}
}

func TestRuntimeCreateRemovesStaleConfigWhenAuthExists(t *testing.T) {
	root := t.TempDir()
	runtimeRoot := filepath.Join(root, "alice", ".codex", "home")
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
