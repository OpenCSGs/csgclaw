package retiredclaws

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cleanupFixture(t *testing.T) (string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".csgclaw")
	state := filepath.Join(root, "state.json")
	writeFile(t, state, `{"version":1,"agents":{"items":[{"id":"agent-a","runtime":{"kind":"codex","state":"running","sandbox_id":"native-handle"}},{"id":"agent-b","runtime":{"kind":"dsh","state":"stopped"}}]}}`)
	return root, state
}

func TestCleanupDeletesOnlyManagedAgentHomesWithoutBackupAndIsIdempotent(t *testing.T) {
	root, state := cleanupFixture(t)
	original, _ := os.ReadFile(state)
	a := filepath.Join(root, "agents", "agent-a", ".boxlite")
	b := filepath.Join(root, "agents", "agent-b", "boxlite")
	writeFile(t, filepath.Join(a, "db", "original"), "old VM data")
	mustMkdir(t, b)
	protected := []string{filepath.Join(root, "boxlite", "keep"), filepath.Join(root, "agents", "agent-a", ".codex", "workspace", "keep"), filepath.Join(root, "agents", "agent-a", ".openclaw", "workspace", "keep"), filepath.Join(root, "agents", "orphan", "boxlite", "keep")}
	for _, p := range protected {
		writeFile(t, p, "keep")
	}
	calls := 0
	check := func(_ context.Context, home string) error {
		calls++
		if home != a {
			t.Fatalf("unexpected inventory %s", home)
		}
		return nil
	}
	if err := CleanupAgentSandboxHomes(context.Background(), root, check); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{a, b} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("retired home remains: %s, %v", p, err)
		}
	}
	archives, _ := filepath.Glob(root + "_boxlite_backup_*")
	if len(archives) != 0 {
		t.Fatalf("cleanup created a backup: %v", archives)
	}
	for _, p := range protected {
		data, err := os.ReadFile(p)
		if err != nil || string(data) != "keep" {
			t.Fatalf("unrelated data changed: %s", p)
		}
	}
	after, _ := os.ReadFile(state)
	if string(after) != string(original) {
		t.Fatal("native Agent state/handle changed")
	}
	if err := CleanupAgentSandboxHomes(context.Background(), root, check); err != nil {
		t.Fatal(err)
	}
	again, _ := filepath.Glob(root + "_boxlite_backup_*")
	if calls != 1 || len(again) != 0 {
		t.Fatalf("cleanup repeated: calls=%d archives=%v", calls, again)
	}
}

func TestCleanupRetainsActiveUnverifiableAndUnmigratedHomes(t *testing.T) {
	for _, failure := range []string{"active", "unavailable", "unknown", "no checker", "unmigrated"} {
		t.Run(failure, func(t *testing.T) {
			root, state := cleanupFixture(t)
			home := filepath.Join(root, "agents", "agent-a", "boxlite")
			writeFile(t, filepath.Join(home, "vm"), "keep")
			if failure == "unmigrated" {
				data, _ := os.ReadFile(state)
				writeFile(t, state, strings.Replace(string(data), `"codex"`, `"openclaw_sandbox"`, 1))
			}
			var checker func(context.Context, string) error
			if failure != "no checker" {
				checker = func(context.Context, string) error { return errors.New(failure) }
			}
			if err := CleanupAgentSandboxHomes(context.Background(), root, checker); err == nil {
				t.Fatal("missing cleanup warning")
			}
			data, err := os.ReadFile(filepath.Join(home, "vm"))
			if err != nil || string(data) != "keep" {
				t.Fatal("unsafe deletion")
			}
			archives, _ := filepath.Glob(root + "_boxlite_backup_*")
			if len(archives) != 0 {
				t.Fatalf("ineligible home created an archive: %v", archives)
			}
		})
	}
}

func TestCleanupProtectsPersistedReferencesAndSymlinks(t *testing.T) {
	for _, reference := range []string{"workspace", "ancestor workspace", "filesystem root workspace", "MCP", "room", "config", "link"} {
		t.Run(reference, func(t *testing.T) {
			root, state := cleanupFixture(t)
			home := filepath.Join(root, "agents", "agent-a", ".boxlite")
			writeFile(t, filepath.Join(home, "file"), "keep")
			var configPaths []string
			switch reference {
			case "workspace", "ancestor workspace", "filesystem root workspace", "MCP":
				value := filepath.Join(home, "project")
				if reference == "ancestor workspace" {
					value = filepath.Dir(home)
				}
				if reference == "filesystem root workspace" {
					value = filepath.VolumeName(home) + string(filepath.Separator)
				}
				var record map[string]any
				data, _ := os.ReadFile(state)
				json.Unmarshal(data, &record)
				agents := record["agents"].(map[string]any)["items"].([]any)
				if reference == "MCP" {
					agents[1].(map[string]any)["mcpServers"] = map[string]any{"local": map[string]any{"command": "python", "args": []string{filepath.Join(home, "mcp.py")}}}
				} else {
					agents[1].(map[string]any)["runtime"].(map[string]any)["options"] = map[string]any{"local_workspace_dir": value}
				}
				data, _ = json.Marshal(record)
				writeFile(t, state, string(data))
			case "room":
				data, _ := json.Marshal(map[string]any{"rooms": []any{map[string]any{"creation_workspace_dir": home}}})
				writeFile(t, filepath.Join(root, "im", "state.json"), string(data))
			case "config":
				path := filepath.Join(filepath.Dir(root), "custom.toml")
				data, _ := json.Marshal(home)
				writeFile(t, path, "[custom]\npath = "+string(data)+"\n")
				configPaths = append(configPaths, path)
			case "link":
				link := filepath.Join(root, "agents", "agent-b", ".codex", "workspace", "old-data")
				mustMkdir(t, filepath.Dir(link))
				if err := os.Symlink(home, link); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			if err := CleanupAgentSandboxHomes(context.Background(), root, func(context.Context, string) error { return nil }, configPaths...); err == nil {
				t.Fatal("missing reference warning")
			}
			if data, err := os.ReadFile(filepath.Join(home, "file")); err != nil || string(data) != "keep" {
				t.Fatal("referenced data removed")
			}
		})
	}
}

func TestCleanupDoesNotFollowLinkedAgentOrSandboxHomes(t *testing.T) {
	for _, parent := range []bool{false, true} {
		t.Run(map[bool]string{false: "home", true: "Agent"}[parent], func(t *testing.T) {
			root, _ := cleanupFixture(t)
			external := t.TempDir()
			writeFile(t, filepath.Join(external, "boxlite", "keep"), "external")
			target := filepath.Join(root, "agents", "agent-a", "boxlite")
			source := filepath.Join(external, "boxlite")
			if parent {
				target = filepath.Dir(target)
				source = external
			}
			mustMkdir(t, filepath.Dir(target))
			if err := os.Symlink(source, target); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if err := CleanupAgentSandboxHomes(context.Background(), root, func(context.Context, string) error { return nil }); err == nil {
				t.Fatal("linked home did not warn")
			}
			if data, err := os.ReadFile(filepath.Join(external, "boxlite", "keep")); err != nil || string(data) != "external" {
				t.Fatal("external data changed")
			}
		})
	}
}

func TestCleanupPermissionFailureRetainsSourceAndRetries(t *testing.T) {
	root, _ := cleanupFixture(t)
	home := filepath.Join(root, "agents", "agent-a", "boxlite")
	writeFile(t, filepath.Join(home, "vm"), "keep")
	parent := filepath.Dir(home)
	if err := os.Chmod(parent, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0755) })
	if err := os.WriteFile(filepath.Join(parent, "probe"), nil, 0600); err == nil {
		t.Skip("filesystem does not enforce permissions")
	}
	checker := func(context.Context, string) error { return nil }
	if err := CleanupAgentSandboxHomes(context.Background(), root, checker); err == nil {
		t.Fatal("missing cleanup warning")
	}
	if _, err := os.Stat(home); err != nil {
		t.Fatal("failed deletion did not leave the directory for retry")
	}
	archives, _ := filepath.Glob(root + "_boxlite_backup_*")
	if len(archives) != 0 {
		t.Fatalf("cleanup created a backup: %v", archives)
	}
	if err := os.Chmod(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := CleanupAgentSandboxHomes(context.Background(), root, checker); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("retry did not clean home")
	}
}

func TestCleanupRechecksStateAfterInventory(t *testing.T) {
	root, state := cleanupFixture(t)
	home := filepath.Join(root, "agents", "agent-a", "boxlite")
	writeFile(t, filepath.Join(home, "vm"), "keep")
	check := func(context.Context, string) error { writeFile(t, state, `{"version":1,"changed":true}`); return nil }
	if err := CleanupAgentSandboxHomes(context.Background(), root, check); err == nil {
		t.Fatal("state change ignored")
	}
	if _, err := os.Stat(home); err != nil {
		t.Fatal("home deleted despite concurrent configuration edit")
	}
}

func TestCleanupProtectsEnvironmentAndShellWorkspacePaths(t *testing.T) {
	root, _ := cleanupFixture(t)
	home := filepath.Join(root, "agents", "agent-a", "boxlite")
	mustMkdir(t, home)
	t.Setenv("RETIRED_VM_HOME", home)
	if !cleanupReferenced("python ${RETIRED_VM_HOME}/mcp.py", home) {
		t.Fatal("shell environment reference ignored")
	}
	parent := filepath.Dir(root)
	t.Setenv("HOME", parent)
	t.Setenv("USERPROFILE", parent)
	if !cleanupReferenced("python ~/.csgclaw/agents/agent-a/boxlite/mcp.py", home) {
		t.Fatal("home shorthand reference ignored")
	}
}

func TestCleanupRejectsNonPortableAgentIdentities(t *testing.T) {
	for _, id := range []string{".. ", "agent-a.", "agent-a ", "C:relative", "agent-a/../.."} {
		root, state := cleanupFixture(t)
		mustMkdir(t, filepath.Join(root, "agents"))
		global := filepath.Join(root, "boxlite", "keep")
		writeFile(t, global, "keep")
		data, _ := json.Marshal(map[string]any{"version": 1, "agents": map[string]any{"items": []any{map[string]any{"id": id, "runtime": map[string]any{"kind": "codex"}}}}})
		writeFile(t, state, string(data))
		if err := CleanupAgentSandboxHomes(context.Background(), root, nil); err == nil {
			t.Fatalf("unsafe identity accepted: %q", id)
		}
		if data, err := os.ReadFile(global); err != nil || string(data) != "keep" {
			t.Fatal("global directory changed")
		}
	}
}

func TestDirectCleanupDoesNotFollowSymlinksInsideRetiredHome(t *testing.T) {
	root, _ := cleanupFixture(t)
	home := filepath.Join(root, "agents", "agent-a", ".boxlite")
	external := t.TempDir()
	writeFile(t, filepath.Join(external, "keep"), "outside")
	mustMkdir(t, home)
	if err := os.Symlink(external, filepath.Join(home, "external")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := CleanupAgentSandboxHomes(context.Background(), root, func(context.Context, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("retired directory was not removed")
	}
	data, err := os.ReadFile(filepath.Join(external, "keep"))
	if err != nil || string(data) != "outside" {
		t.Fatal("cleanup followed a nested symlink")
	}
}
