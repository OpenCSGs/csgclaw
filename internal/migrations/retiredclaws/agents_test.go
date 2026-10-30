package retiredclaws

import (
	"bytes"
	"csgclaw/internal/localstore"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRetiredRuntimeAgentsPreserveDataAndRequestAutomaticStart(t *testing.T) {
	for _, kind := range []string{"openclaw", "openclaw_sandbox", "picoclaw", "picoclaw_sandbox"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, ".csgclaw")
			mustMkdir(t, root)
			runtimeDir := ".openclaw"
			instructionFile := "AGENTS.md"
			if kind == "picoclaw" || kind == "picoclaw_sandbox" {
				runtimeDir = ".picoclaw"
				instructionFile = "AGENT.md"
			}
			workspace := filepath.Join(root, "agents", "agent-old", runtimeDir, "workspace")
			writeFile(t, filepath.Join(workspace, "notes.txt"), "preserve working file")
			writeFile(t, filepath.Join(workspace, instructionFile), "Custom instructions")
			writeFile(t, filepath.Join(workspace, "skills", "custom", "SKILL.md"), "# Custom skill")
			original := `{"version":1,"unrelated":{"large":9007199254740993},"agents":{"extra":true,"items":[{"id":"agent-old","name":"Old","role":"worker","desired_state":"running","image":"retired-image","runtime":{"kind":"` + kind + `","state":"running","sandbox_id":"old-box","options":{"auto_compact":"disabled","native":"old"}},"model_config":{"model_provider_id":"fixture","model_id":"fixture-model","image_upgrade_required":true,"env_restart_required":true,"env":{"OPENCLAW_HOME":"old","PICOCLAW_MODE":"old","USER_SETTING":"keep"}},"mcpServers":{"test":{"command":"test"}},"skill_states":{"custom":{"enabled":false}}},{"id":"agent-current","runtime":{"kind":"dsh","state":"running","options":{"permission_mode":"bypass"}}}]},"participants":{"items":[{"id":"pt-old","agent_id":"agent-old"}]}}`
			path := filepath.Join(root, localstore.RootStateFileName)
			writeFile(t, path, original)
			if err := migrateAgentsWithMissingSandbox(t, root); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(data, []byte("9007199254740993")) {
				t.Fatal("unrelated integer lost precision")
			}
			var state struct {
				Agents struct {
					Extra bool
					Items []map[string]json.RawMessage
				}
				Participants json.RawMessage
			}
			if err := json.Unmarshal(data, &state); err != nil {
				t.Fatal(err)
			}
			old := state.Agents.Items[0]
			var rt struct {
				Kind, State string
				SandboxID   string `json:"sandbox_id"`
				Options     map[string]any
			}
			if err := json.Unmarshal(old["runtime"], &rt); err != nil {
				t.Fatal(err)
			}
			if rt.Kind != "codex" || rt.State != "stopped" || rt.SandboxID != "" || rt.Options["local_workspace_dir"] != workspace || len(rt.Options) != 1 {
				t.Fatalf("migrated runtime: %+v", rt)
			}
			if migrationString(old["desired_state"]) != "running" || migrationString(old["instructions"]) != "Custom instructions" {
				t.Fatalf("lifecycle/instructions not migrated: %s", data)
			}
			if _, ok := old["image"]; ok {
				t.Fatal("retired image retained")
			}
			var profile map[string]json.RawMessage
			if err := json.Unmarshal(old["model_config"], &profile); err != nil {
				t.Fatal(err)
			}
			if migrationString(profile["model_provider_id"]) != "fixture" || migrationString(profile["model_id"]) != "fixture-model" {
				t.Fatal("model changed")
			}
			if bytes.Contains(old["model_config"], []byte("OPENCLAW_")) || bytes.Contains(old["model_config"], []byte("PICOCLAW_")) {
				t.Fatal("retired environment retained")
			}
			if !bytes.Contains(old["model_config"], []byte("USER_SETTING")) || !bytes.Contains(old["mcpServers"], []byte("test")) || !bytes.Contains(old["skill_states"], []byte("false")) || !state.Agents.Extra {
				t.Fatal("resources or agent metadata lost")
			}
			if !bytes.Contains(state.Agents.Items[1]["runtime"], []byte("dsh")) || !bytes.Contains(state.Agents.Items[1]["runtime"], []byte("bypass")) {
				t.Fatal("active runtime changed")
			}
			if got, err := os.ReadFile(filepath.Join(workspace, "notes.txt")); err != nil || string(got) != "preserve working file" {
				t.Fatal("working files changed")
			}
			backups, err := filepath.Glob(filepath.Join(home, ".csgclaw_backup_*"))
			if err != nil {
				t.Fatal(err)
			}
			if len(backups) != 1 {
				t.Fatalf("backups: %v", backups)
			}
			backup, err := os.ReadFile(filepath.Join(backups[0], localstore.RootStateFileName))
			if err != nil || string(backup) != original {
				t.Fatal("backup did not preserve original state")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := migrateAgentsWithMissingSandbox(t, root); err != nil {
				t.Fatal(err)
			}
			again, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !again.ModTime().Equal(info.ModTime()) {
				t.Fatal("migration repeats")
			}
		})
	}
}

func TestRetiredRuntimeMigrationSupportsSplitSelectionAndNoWorkspace(t *testing.T) {
	for _, selection := range []string{`"runtime":{"name":"openclaw","sandbox_enabled":true}`, `"runtime_kind":"picoclaw_sandbox"`, `"runtime_name":"picoclaw"`} {
		root := t.TempDir()
		path := filepath.Join(root, localstore.RootStateFileName)
		writeFile(t, path, `{"version":1,"agents":{"items":[{"id":"agent-old",`+selection+`}]}}`)
		if err := migrateAgentsWithMissingSandbox(t, root); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("picoclaw")) || bytes.Contains(data, []byte("openclaw")) || !bytes.Contains(data, []byte(`"codex"`)) {
			t.Fatalf("selection not migrated: %s", data)
		}
	}
}

func TestRetiredRuntimeMigrationRejectsCorruptStateWithoutWriting(t *testing.T) {
	for _, data := range []string{`{`, `{"agents":{"items":[{"id":"../escape","runtime":{"kind":"openclaw"}}]}}`, `{"agents":{"items":[{"id":"agent-old","runtime":"invalid"}]}}`} {
		root := t.TempDir()
		path := filepath.Join(root, localstore.RootStateFileName)
		writeFile(t, path, data)
		if err := migrateAgentsWithMissingSandbox(t, root); err == nil {
			t.Fatal("corrupt state accepted")
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != data {
			t.Fatal("corrupt state was rewritten")
		}
	}
}

func TestLegacyRuntimeRecordMigrationPreservesOtherRootSections(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, localstore.RootStateFileName), `{"version":1,"mcpServers":{"keep":{"command":"test"}},"extension":{"large":9007199254740993},"agents":{"items":[{"id":"agent-old","name":"Old","role":"worker","runtime_id":"rt-agent-old"}],"runtimes":[{"id":"rt-agent-old","kind":"openclaw_sandbox","state":"running","sandbox_id":"old-box"}]},"participants":{"items":[]}}`)
	if _, err := localstore.MigrateTypedIDs(localstore.MigrateOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if err := migrateAgentsWithMissingSandbox(t, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, localstore.RootStateFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"codex"`, `"keep"`, `9007199254740993`} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("lost %s: %s", want, data)
		}
	}
	if bytes.Contains(data, []byte("openclaw")) {
		t.Fatalf("legacy runtime did not migrate: %s", data)
	}
}

func TestRuntimeMigrationBackupSkipsOnlyUnreadableGeneratedPluginCopies(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".csgclaw")
	writeFile(t, filepath.Join(root, localstore.RootStateFileName), `{"version":1,"agents":{"items":[{"id":"agent-old","runtime":{"kind":"openclaw"}}]}}`)
	path := filepath.Join(root, "agents", "agent-old", ".openclaw", "plugin-skills", "generated")
	writeFile(t, path, "container-generated plugin copy")
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0600) })
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("process can read mode-000 files")
	}
	if err := migrateAgentsWithMissingSandbox(t, root); err != nil {
		t.Fatalf("unreadable generated copy blocked migration: %v", err)
	}
	backups, err := filepath.Glob(filepath.Join(home, ".csgclaw_backup_*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups %v %v", backups, err)
	}
	if _, err := os.Stat(filepath.Join(backups[0], "agents", "agent-old", ".openclaw", "plugin-skills", "generated")); !os.IsNotExist(err) {
		t.Fatalf("unreadable generated copy was retained: %v", err)
	}
}

func TestRetiredNativeMCPAndMemoryAreImportedWithoutReplacingExplicitSettings(t *testing.T) {
	for _, kind := range []string{"openclaw", "picoclaw"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			agentHome := filepath.Join(root, "agents", "agent-old")
			workspace := filepath.Join(agentHome, "."+kind, "workspace")
			writeFile(t, filepath.Join(workspace, "notes.txt"), "preserved")
			memory := filepath.Join(workspace, "MEMORY.md")
			configName := "openclaw.json"
			native := `{"mcp":{"servers":{"legacy docs":{"command":"test","args":["/home/node/.openclaw/workspace"],"connectionTimeoutMs":1250,"requestTimeoutMs":90000}}}}`
			if kind == "picoclaw" {
				configName = "config.json"
				memory = filepath.Join(workspace, "memory", "MEMORY.md")
				native = `{"tools":{"mcp":{"enabled":true,"servers":{"legacy docs":{"command":"test","args":["/home/picoclaw/.picoclaw/workspace"],"startup_timeout_sec":2}}}}}`
			}
			writeFile(t, memory, "A retained personal memory.")
			writeFile(t, filepath.Join(agentHome, "."+kind, configName), native)
			path := filepath.Join(root, localstore.RootStateFileName)
			original := `{"version":1,"agents":{"items":[{"id":"agent-old","runtime":{"kind":"` + kind + `"}}]}}`
			writeFile(t, path, original)
			if err := migrateAgentsWithMissingSandbox(t, root); err != nil {
				t.Fatal(err)
			}
			var state struct {
				Agents struct{ Items []map[string]json.RawMessage }
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &state); err != nil {
				t.Fatal(err)
			}
			var servers map[string]struct {
				Args    []string
				Startup int `json:"startup_timeout_sec"`
				Tool    int `json:"tool_timeout_sec"`
			}
			if err := json.Unmarshal(state.Agents.Items[0]["mcpServers"], &servers); err != nil {
				t.Fatal(err)
			}
			entry := servers["legacy docs"]
			if len(entry.Args) != 1 || entry.Args[0] != workspace || entry.Startup != 2 {
				t.Fatalf("MCP migration: %+v", servers)
			}
			if kind == "openclaw" && entry.Tool != 90 {
				t.Fatalf("MCP timeout lost: %+v", entry)
			}
			target := filepath.Join(agentHome, ".codex", "home", "memories", "memory_summary.md")
			if data, err := os.ReadFile(target); err != nil || string(data) != "A retained personal memory." {
				t.Fatal("personal memory not imported")
			}
			writeFile(t, target, "Existing current memory.")
			writeFile(t, path, `{"version":1,"agents":{"items":[{"id":"agent-old","runtime":{"kind":"`+kind+`"},"mcpServers":{}}]}}`)
			if err := migrateAgentsWithMissingSandbox(t, root); err != nil {
				t.Fatal(err)
			}
			data, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &state); err != nil {
				t.Fatal(err)
			}
			if string(state.Agents.Items[0]["mcpServers"]) != "{}" {
				t.Fatal("explicit empty MCP snapshot overwritten")
			}
			if data, err := os.ReadFile(target); err != nil || string(data) != "Existing current memory." {
				t.Fatal("existing current memory overwritten")
			}
		})
	}
}

func TestMigrationStartsPreviouslyStoppedRetiredAgentsAndHonorsLaterStops(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, localstore.RootStateFileName)
	writeFile(t, path, `{"version":1,"agents":{"items":[{"id":"agent-old","runtime":{"kind":"openclaw_sandbox","state":"stopped"},"desired_state":"stopped"},{"id":"agent-pico","runtime":{"kind":"picoclaw_sandbox","state":"stopped"},"desired_state":"stopped"},{"id":"agent-current","runtime":{"kind":"codex","state":"stopped"},"desired_state":"stopped"}]}}`)
	if err := migrateAgentsWithMissingSandbox(t, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Version int `json:"version"`
		Agents  struct {
			Items []map[string]json.RawMessage `json:"items"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	for _, item := range state.Agents.Items {
		want := "running"
		if migrationString(item["id"]) == "agent-current" {
			want = "stopped"
		}
		if migrationString(item["desired_state"]) != want {
			t.Fatalf("Agent %s desired state = %s, want %s", item["id"], item["desired_state"], want)
		}
	}
	state.Agents.Items[0]["desired_state"] = json.RawMessage(`"stopped"`)
	stopped, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(stopped))
	if err := migrateAgentsWithMissingSandbox(t, root); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(stopped, after) {
		t.Fatalf("completed migration overrode a later user stop: %s, %v", after, err)
	}
}
