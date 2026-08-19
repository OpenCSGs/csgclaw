package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentruntime "csgclaw/internal/runtime"
)

func TestProvisionMaterializesCredentialsBeforeInitShell(t *testing.T) {
	root := t.TempDir()
	runtimeImpl := newTestCodexRuntime(root, func(agentruntime.Handle) (AgentRef, error) { return AgentRef{}, nil })
	err := runtimeImpl.Provision(context.Background(), agentruntime.ProvisionRequest{
		AgentID: "agent-alice", AgentName: "alice",
		Profile: agentruntime.Profile{APIKey: "init-api-key", Env: map[string]string{"CONTRACT_INIT": "ready"}},
		Credentials: map[string]string{
			"secrets/token.txt":  "token-value",
			"config/service.ini": "enabled=true\n",
		},
		InitShell: `test "$(cat secrets/token.txt)" = "token-value"
test "$(cat config/service.ini)" = "enabled=true"
test "$CONTRACT_INIT" = "ready"
test "$OPENAI_API_KEY" = "init-api-key"
test -n "$HOME"
test -n "$CODEX_HOME"
printf initialized > init-complete
printf %s "$HOME" > init-home
printf %s "$CODEX_HOME" > init-codex-home`,
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	workspace := filepath.Join(root, "agent-alice", ".codex", "workspace")
	for name, want := range map[string]string{
		"secrets/token.txt":  "token-value",
		"config/service.ini": "enabled=true\n",
		"init-complete":      "initialized",
	} {
		path := filepath.Join(workspace, filepath.FromSlash(name))
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("credential %q = %q, %v", name, data, err)
		}
		if name != "init-complete" {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat credential %q: %v", name, err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("credential %q mode = %v", name, info.Mode().Perm())
			}
		}
	}
	homeRaw, err := os.ReadFile(filepath.Join(workspace, "init-home"))
	if err != nil || string(homeRaw) != runtimeImpl.hostSessionHomeDir(filepath.Join(root, "agent-alice", ".codex", "home")) {
		t.Fatalf("initShell HOME = %q, %v", homeRaw, err)
	}
	codexHomeRaw, err := os.ReadFile(filepath.Join(workspace, "init-codex-home"))
	if err != nil || string(codexHomeRaw) != filepath.Join(root, "agent-alice", ".codex", "home") {
		t.Fatalf("initShell CODEX_HOME = %q, %v", codexHomeRaw, err)
	}
}

func TestProvisionReplacesManagedCredentialsAndRollsBackInitFailure(t *testing.T) {
	root := t.TempDir()
	runtimeImpl := newTestCodexRuntime(root, func(agentruntime.Handle) (AgentRef, error) { return AgentRef{}, nil })
	initial := agentruntime.ProvisionRequest{
		AgentID: "agent-alice", AgentName: "alice",
		Credentials: map[string]string{"secrets/keep": "old", "secrets/remove": "remove"},
	}
	if err := runtimeImpl.Provision(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	runtimeImpl.deps.RunInitShell = func(_ context.Context, workspace, _ string, environment []string) error {
		env := testEnvironmentMap(environment)
		if env["HOME"] != runtimeImpl.hostSessionHomeDir(filepath.Join(root, "agent-alice", ".codex", "home")) || env["CODEX_HOME"] != filepath.Join(root, "agent-alice", ".codex", "home") {
			t.Fatalf("initShell environment HOME=%q CODEX_HOME=%q", env["HOME"], env["CODEX_HOME"])
		}
		if data, err := os.ReadFile(filepath.Join(workspace, "secrets", "keep")); err != nil || string(data) != "new" {
			t.Fatalf("initShell keep credential = %q, %v", data, err)
		}
		if _, err := os.Stat(filepath.Join(workspace, "secrets", "remove")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("removed credential still exists: %v", err)
		}
		if data, err := os.ReadFile(filepath.Join(workspace, "secrets", "added")); err != nil || string(data) != "added" {
			t.Fatalf("initShell added credential = %q, %v", data, err)
		}
		return errors.New("initialization failed")
	}
	err := runtimeImpl.Provision(context.Background(), agentruntime.ProvisionRequest{
		AgentID: "agent-alice", AgentName: "alice",
		PreviousCredentials: []string{"secrets/keep", "secrets/remove"},
		Credentials:         map[string]string{"secrets/keep": "new", "secrets/added": "added"},
		InitShell:           "initialize",
	})
	if err == nil || !strings.Contains(err.Error(), "initShell failed") {
		t.Fatalf("Provision() error = %v", err)
	}
	workspace := filepath.Join(root, "agent-alice", ".codex", "workspace", "secrets")
	for name, want := range map[string]string{"keep": "old", "remove": "remove"} {
		data, err := os.ReadFile(filepath.Join(workspace, name))
		if err != nil || string(data) != want {
			t.Fatalf("rolled back credential %q = %q, %v", name, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "added")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new credential survived rollback: %v", err)
	}
}

func testEnvironmentMap(environment []string) map[string]string {
	out := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}

func TestProvisionRejectsCredentialPathsOutsideWorkspaceAndSymlinks(t *testing.T) {
	root := t.TempDir()
	runtimeImpl := newTestCodexRuntime(root, func(agentruntime.Handle) (AgentRef, error) { return AgentRef{}, nil })
	for _, name := range []string{"", ".", "../secret", "/absolute", " leading", `windows\\path`} {
		err := runtimeImpl.Provision(context.Background(), agentruntime.ProvisionRequest{
			AgentID: "agent-alice", AgentName: "alice", Credentials: map[string]string{name: "value"},
		})
		if err == nil {
			t.Fatalf("credential path %q was accepted", name)
		}
	}

	workspace := filepath.Join(root, "agent-alice", ".codex", "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "credential")); err != nil {
		t.Fatal(err)
	}
	err := runtimeImpl.Provision(context.Background(), agentruntime.ProvisionRequest{
		AgentID: "agent-alice", AgentName: "alice", Credentials: map[string]string{"credential": "replacement"},
	})
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("symlink Provision() error = %v", err)
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "outside" {
		t.Fatalf("outside file changed: %q, %v", data, err)
	}
}

func TestProvisionWithoutCredentialsOrInitShellSkipsCredentialWorkspace(t *testing.T) {
	root := t.TempDir()
	runtimeImpl := newTestCodexRuntime(root, func(agentruntime.Handle) (AgentRef, error) { return AgentRef{}, nil })
	err := runtimeImpl.Provision(context.Background(), agentruntime.ProvisionRequest{
		AgentID: "agent-alice", AgentName: "alice",
		RuntimeOptions: map[string]any{localWorkspaceDirOptionKey: "invalid\x00workspace"},
	})
	if err != nil {
		t.Fatalf("empty Runtime provisioning touched credential workspace: %v", err)
	}
}
