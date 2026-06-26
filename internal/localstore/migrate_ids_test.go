package localstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigrateStoreCreatesSiblingBackupAndRootState(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, ".csgclaw")
	mustMkdir(t, filepath.Join(root, "agents", "manager"))
	mustMkdir(t, filepath.Join(root, "agents", "张三"))
	mustMkdir(t, filepath.Join(root, "im", "sessions"))
	mustMkdir(t, filepath.Join(root, "im", "threads", "ops"))
	mustMkdir(t, filepath.Join(root, "teams", "room-ops"))

	writeFile(t, filepath.Join(root, "agents", "manager", "runtime.json"), `{"agent_id":"u-manager","participant_id":"manager","runtime_id":"rt-u-manager"}`)
	writeJSON(t, filepath.Join(root, "agents", "state.json"), map[string]any{
		"profile_defaults": map[string]any{
			"provider":         "openai",
			"model_id":         "gpt-4.1",
			"reasoning_effort": "medium",
		},
		"agents": []map[string]any{
			{
				"id":           "u-manager",
				"name":         "manager",
				"role":         "manager",
				"runtime_id":   "rt-u-manager",
				"runtime_kind": "picoclaw_sandbox",
				"created_at":   "2026-06-24T01:02:03Z",
				"agent_profile": map[string]any{
					"provider":         "openai",
					"model_id":         "gpt-4.1",
					"reasoning_effort": "medium",
				},
			},
			{
				"id":           "张三",
				"name":         "张三",
				"role":         "worker",
				"runtime_id":   "rt-zhang",
				"runtime_kind": "codex",
				"created_at":   "2026-06-24T01:03:03Z",
			},
		},
		"runtimes": []map[string]any{
			{"id": "rt-u-manager", "kind": "picoclaw_sandbox"},
			{"id": "rt-zhang", "kind": "codex"},
		},
	})
	writeJSON(t, filepath.Join(root, "models.json"), map[string]any{
		"version": 1,
		"default": "openai/gpt-4.1",
		"providers": map[string]any{
			"openai": map[string]any{
				"display_name": "OpenAI",
				"models":       []string{"gpt-4.1"},
			},
		},
	})
	writeJSON(t, filepath.Join(root, "im", "participants.json"), map[string]any{
		"participants": []map[string]any{
			{
				"id":               "manager",
				"channel":          "csgclaw",
				"type":             "agent",
				"name":             "manager",
				"agent_id":         "u-manager",
				"channel_user_ref": "manager",
				"lifecycle_status": "active",
				"mentionable":      true,
				"created_at":       "2026-06-24T01:02:03Z",
				"updated_at":       "2026-06-24T01:02:03Z",
			},
			{
				"id":               "admin",
				"channel":          "csgclaw",
				"type":             "human",
				"name":             "Admin",
				"channel_user_ref": "u-admin",
				"lifecycle_status": "active",
				"mentionable":      true,
				"created_at":       "2026-06-24T01:02:03Z",
				"updated_at":       "2026-06-24T01:02:03Z",
			},
			{
				"id":               "admin",
				"channel":          "feishu",
				"type":             "human",
				"name":             "Admin",
				"channel_user_ref": "ou_admin",
				"lifecycle_status": "active",
				"mentionable":      true,
				"created_at":       "2026-06-24T01:02:03Z",
				"updated_at":       "2026-06-24T01:02:03Z",
			},
			{
				"id":               "manager",
				"channel":          "feishu",
				"type":             "agent",
				"name":             "manager",
				"agent_id":         "u-manager",
				"channel_user_ref": "ou_manager",
				"lifecycle_status": "active",
				"mentionable":      true,
				"created_at":       "2026-06-24T01:02:03Z",
				"updated_at":       "2026-06-24T01:02:03Z",
			},
		},
	})
	writeJSON(t, filepath.Join(root, "im", "state.json"), map[string]any{
		"current_user_id": "u-admin",
		"users": []map[string]any{
			{"id": "u-admin", "name": "Admin", "handle": "admin", "role": "admin"},
			{"id": "manager", "name": "manager", "handle": "manager", "role": "manager"},
		},
		"rooms": []map[string]any{
			{
				"id":       "ops",
				"title":    "Ops",
				"members":  []string{"u-admin", "manager"},
				"messages": "sessions/ops.jsonl",
				"threads": []map[string]any{{
					"root_message_id": "root",
					"created_at":      "2026-06-24T01:05:03Z",
					"context": []map[string]any{{
						"id":         "ctx-1",
						"sender_id":  "u-admin",
						"content":    "context",
						"created_at": "2026-06-24T01:04:03Z",
					}},
					"summary": map[string]any{"root_excerpt": "hello", "message_count": 1},
				}},
			},
		},
	})
	writeFile(t, filepath.Join(root, "im", "sessions", "ops.jsonl"), `{"id":"root","sender_id":"u-admin","content":"hello","created_at":"2026-06-24T01:05:03Z","mentions":[{"id":"manager","name":"manager"}],"relates_to":{"rel_type":"m.thread","event_id":"root"},"event":{"actor_id":"u-admin","target_ids":["manager"]}}`+"\n")
	writeJSON(t, filepath.Join(root, "im", "threads", "ops", "existing-root.json"), map[string]any{
		"root_message_id": "existing-root",
		"created_at":      "2026-06-24T01:06:03Z",
		"context": []map[string]any{{
			"id":         "existing-root",
			"sender_id":  "manager",
			"content":    "existing thread",
			"created_at": "2026-06-24T01:06:03Z",
		}},
		"summary": map[string]any{"root_excerpt": "existing thread", "message_count": 1},
	})
	writeJSON(t, filepath.Join(root, "teams", "room-ops", "team.json"), map[string]any{
		"id":            "room-ops",
		"room_id":       "ops",
		"channel":       "csgclaw",
		"title":         "Ops team",
		"lead_agent_id": "u-manager",
		"status":        "active",
	})
	writeJSON(t, filepath.Join(root, "teams", "room-ops", "tasks.json"), []map[string]any{{
		"id":          "1",
		"team_id":     "room-ops",
		"room_id":     "ops",
		"created_by":  "manager",
		"assigned_to": "manager",
		"depends_on":  []string{"2"},
		"title":       "Plan",
		"status":      "pending",
	}})
	writeJSON(t, filepath.Join(root, "teams", "room-ops", "approvals.json"), []map[string]any{{
		"id":           "1",
		"team_id":      "room-ops",
		"room_id":      "ops",
		"task_id":      "1",
		"requested_by": "manager",
		"approver_id":  "u-admin",
		"kind":         "plan",
		"summary":      "Approve",
		"status":       "pending",
	}})
	writeJSON(t, filepath.Join(root, "teams", "room-ops", "presence.json"), []map[string]any{{
		"team_id":         "room-ops",
		"participant_id":  "manager",
		"user_id":         "manager",
		"agent_id":        "u-manager",
		"role":            "lead",
		"state":           "idle",
		"current_task_id": "1",
	}})
	writeFile(t, filepath.Join(root, "teams", "room-ops", "events.jsonl"), `{"seq":1,"team_id":"room-ops","room_id":"ops","type":"task.created","actor_id":"manager","task_id":"1","target_id":"u-admin","created_at":"2026-06-24T01:05:03Z"}`+"\n")

	result, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 24, 12, 0, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("MigrateTypedIDs() error = %v", err)
	}

	wantBackup := filepath.Join(parent, ".csgclaw_backup_20260624_001")
	if result.BackupPath != wantBackup {
		t.Fatalf("BackupPath = %q, want %q", result.BackupPath, wantBackup)
	}
	if _, err := os.Stat(filepath.Join(wantBackup, "agents", "state.json")); err != nil {
		t.Fatalf("backup missing legacy agents state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "backup")); !os.IsNotExist(err) {
		t.Fatalf("unexpected nested backup under root: %v", err)
	}
	assertMissing(t, filepath.Join(root, "models.json"))
	assertMissing(t, filepath.Join(root, "im", "participants.json"))
	assertMissing(t, filepath.Join(root, "agents", "state.json"))

	var rootState map[string]any
	readJSON(t, filepath.Join(root, "state.json"), &rootState)
	if _, ok := rootState["model_providers"].(map[string]any); !ok {
		t.Fatalf("root state missing model_providers: %#v", rootState)
	}
	modelProviders := rootState["model_providers"].(map[string]any)
	if _, ok := modelProviders["default_model"]; ok {
		t.Fatalf("model_providers.default_model persisted: %#v", modelProviders)
	}
	agents := rootState["agents"].(map[string]any)
	modelDefaults := agents["model_defaults"].(map[string]any)
	if modelDefaults["model_provider_id"] != "openai" || modelDefaults["model_id"] != "gpt-4.1" {
		t.Fatalf("model_defaults = %#v, want openai/gpt-4.1", modelDefaults)
	}
	if _, ok := agents["profile_defaults"]; ok {
		t.Fatalf("legacy profile_defaults persisted: %#v", agents)
	}
	agentItems := agents["items"].([]any)
	managerAgent := jsonObjectWithID(t, agentItems, "agent-manager")
	if _, ok := managerAgent["profile"]; ok {
		t.Fatalf("legacy agent profile persisted: %#v", managerAgent)
	}
	modelConfig := managerAgent["model_config"].(map[string]any)
	if modelConfig["model_provider_id"] != "openai" || modelConfig["model_id"] != "gpt-4.1" {
		t.Fatalf("model_config = %#v, want openai/gpt-4.1", modelConfig)
	}
	assertJSONContainsID(t, agentItems, "agent-manager")
	assertJSONContainsIDPrefix(t, agentItems, "agent-")
	participants := rootState["participants"].(map[string]any)
	participantItems := participants["items"].([]any)
	assertJSONContainsID(t, participantItems, "pt-manager")
	assertJSONContainsID(t, participantItems, "pt-admin")
	assertUniqueJSONIDs(t, participantItems)
	assertJSONContainsIDPrefix(t, participantItems, "pt-admin-")
	assertJSONContainsIDPrefix(t, participantItems, "pt-manager-")

	if _, err := os.Stat(filepath.Join(root, "agents", "agent-manager")); err != nil {
		t.Fatalf("agent-manager dir missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "agents", "张三")); !os.IsNotExist(err) {
		t.Fatalf("UTF-8 name directory still exists after migration: %v", err)
	}

	var imState map[string]any
	readJSON(t, filepath.Join(root, "im", "state.json"), &imState)
	if imState["current_user_id"] != "user-admin" {
		t.Fatalf("current_user_id = %v, want user-admin", imState["current_user_id"])
	}
	for _, item := range imState["users"].([]any) {
		user := item.(map[string]any)
		if _, ok := user["handle"]; ok {
			t.Fatalf("migrated user still has handle: %#v", user)
		}
		if user["name"] == "" {
			t.Fatalf("migrated user missing name: %#v", user)
		}
	}
	room := imState["rooms"].([]any)[0].(map[string]any)
	if room["id"] != "room-ops" {
		t.Fatalf("room id = %v, want room-ops", room["id"])
	}
	assertStringArray(t, room["members"], []string{"user-admin", "user-manager"})
	threads := room["threads"].([]any)
	if len(threads) != 2 {
		t.Fatalf("room threads = %#v, want two lightweight refs", room["threads"])
	}
	for _, item := range threads {
		thread := item.(map[string]any)
		if _, ok := thread["context"]; ok {
			t.Fatalf("im/state room thread still contains heavy context: %#v", thread)
		}
	}
	threadPath := filepath.Join(root, "im", "threads", "room-ops", "msg-root.json")
	var threadState map[string]any
	readJSON(t, threadPath, &threadState)
	if threadState["root_message_id"] != "msg-root" {
		t.Fatalf("thread root = %v, want msg-root", threadState["root_message_id"])
	}
	threadContext := threadState["context"].([]any)
	threadMessage := threadContext[0].(map[string]any)
	if threadMessage["sender_id"] != "user-admin" {
		t.Fatalf("thread context sender = %v, want user-admin", threadMessage["sender_id"])
	}
	existingThreadPath := filepath.Join(root, "im", "threads", "room-ops", "msg-existing-root.json")
	var existingThreadState map[string]any
	readJSON(t, existingThreadPath, &existingThreadState)
	if existingThreadState["root_message_id"] != "msg-existing-root" {
		t.Fatalf("existing thread root = %v, want msg-existing-root", existingThreadState["root_message_id"])
	}
	existingContext := existingThreadState["context"].([]any)
	existingMessage := existingContext[0].(map[string]any)
	if existingMessage["sender_id"] != "user-manager" {
		t.Fatalf("existing thread context sender = %v, want user-manager", existingMessage["sender_id"])
	}
	assertMissing(t, filepath.Join(root, "im", "threads", "ops"))

	sessionRaw, err := os.ReadFile(filepath.Join(root, "im", "sessions", "room-ops.jsonl"))
	if err != nil {
		t.Fatalf("read migrated session: %v", err)
	}
	sessionText := string(sessionRaw)
	for _, want := range []string{`"id":"msg-root"`, `"sender_id":"user-admin"`, `"id":"user-manager"`, `"event_id":"msg-root"`} {
		if !strings.Contains(sessionText, want) {
			t.Fatalf("migrated session missing %s in %s", want, sessionText)
		}
	}

	var teamMeta map[string]any
	readJSON(t, filepath.Join(root, "teams", "team-room-ops", "team.json"), &teamMeta)
	if teamMeta["id"] != "team-room-ops" || teamMeta["room_id"] != "room-ops" || teamMeta["lead_agent_id"] != "agent-manager" {
		t.Fatalf("team meta not migrated: %#v", teamMeta)
	}
	var tasks []map[string]any
	readJSON(t, filepath.Join(root, "teams", "team-room-ops", "tasks.json"), &tasks)
	if tasks[0]["id"] != "task-1" || tasks[0]["created_by"] != "pt-manager" || tasks[0]["assigned_to"] != "pt-manager" {
		t.Fatalf("task not migrated: %#v", tasks[0])
	}
	var approvals []map[string]any
	readJSON(t, filepath.Join(root, "teams", "team-room-ops", "approvals.json"), &approvals)
	if approvals[0]["id"] != "approval-1" || approvals[0]["task_id"] != "task-1" || approvals[0]["approver_id"] != "pt-admin" {
		t.Fatalf("approval not migrated: %#v", approvals[0])
	}
}

func TestMigrateTypedIDsBacksUpBrokenSymlinks(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, ".csgclaw")
	mustMkdir(t, filepath.Join(root, "agents", "gitlab-assistant", ".openclaw", "plugin-skills"))
	if err := os.Symlink("/app/dist/extensions/browser/skills/browser-automation", filepath.Join(root, "agents", "gitlab-assistant", ".openclaw", "plugin-skills", "browser-automation")); err != nil {
		t.Skipf("Symlink() unsupported: %v", err)
	}
	writeJSON(t, filepath.Join(root, "state.json"), map[string]any{
		"version":         1,
		"model_providers": map[string]any{"default_model": map[string]any{"model_provider_id": "openai", "model_id": "gpt-4.1"}, "items": map[string]any{}},
		"agents":          map[string]any{"items": []any{}},
		"participants":    map[string]any{"items": []any{}},
	})

	result, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 24, 12, 0, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("MigrateTypedIDs() error = %v", err)
	}
	linkPath := filepath.Join(result.BackupPath, "agents", "gitlab-assistant", ".openclaw", "plugin-skills", "browser-automation")
	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink(backup browser-automation) error = %v", err)
	}
	if got != "/app/dist/extensions/browser/skills/browser-automation" {
		t.Fatalf("backup symlink target = %q, want container plugin target", got)
	}
}

func TestMigrateTypedIDsSkipsUnreadableOpenClawPluginSkillBackupEntry(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, ".csgclaw")
	pluginPath := filepath.Join(root, "agents", "feishu-assi", ".openclaw", "plugin-skills", "browser-automation")
	mustMkdir(t, filepath.Dir(pluginPath))
	writeFile(t, pluginPath, "container plugin placeholder")
	if err := os.Chmod(pluginPath, 0); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(pluginPath, 0o600) })
	writeJSON(t, filepath.Join(root, "state.json"), map[string]any{
		"version":         1,
		"model_providers": map[string]any{"default_model": map[string]any{"model_provider_id": "openai", "model_id": "gpt-4.1"}, "items": map[string]any{}},
		"agents":          map[string]any{"items": []any{}},
		"participants":    map[string]any{"items": []any{}},
	})

	result, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 24, 12, 0, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("MigrateTypedIDs() error = %v", err)
	}
	if result.BackupPath == "" {
		t.Fatal("BackupPath is empty, want backup despite unreadable volatile plugin entry")
	}
	assertMissing(t, filepath.Join(result.BackupPath, "agents", "feishu-assi", ".openclaw", "plugin-skills", "browser-automation"))
}

func TestMigrateTypedIDsCollapsesLegacyLocalIdentityAliases(t *testing.T) {
	m := newTypedIDMigrator(t.TempDir())

	if got := m.participantID("admin"); got != "pt-admin" {
		t.Fatalf("participantID(admin) = %q, want pt-admin", got)
	}
	for _, old := range []string{"u-admin", "user-admin", "pt-admin"} {
		if got := m.participantID(old); got != "pt-admin" {
			t.Fatalf("participantID(%s) = %q, want pt-admin", old, got)
		}
	}

	if got := m.participantID("agent-alice"); got != "pt-alice" {
		t.Fatalf("participantID(agent-alice) = %q, want pt-alice", got)
	}
	if got := m.participantID("u-agent-alice"); got != "pt-alice" {
		t.Fatalf("participantID(u-agent-alice) = %q, want pt-alice", got)
	}
	if got := m.participantID("pt-agent-alice-d59735ad"); got != "pt-alice" {
		t.Fatalf("participantID(pt-agent-alice-d59735ad) = %q, want pt-alice", got)
	}
	if got := m.userID("pt-alice"); got != "user-alice" {
		t.Fatalf("userID(pt-alice) = %q, want user-alice", got)
	}
	if got := m.userID("user-agent-alice"); got != "user-alice" {
		t.Fatalf("userID(user-agent-alice) = %q, want user-alice", got)
	}
	if got := m.agentID("agent-agent-alice"); got != "agent-alice" {
		t.Fatalf("agentID(agent-agent-alice) = %q, want agent-alice", got)
	}
	if got := m.agentID("u-agent-alice"); got != "agent-alice" {
		t.Fatalf("agentID(u-agent-alice) = %q, want agent-alice", got)
	}

	if got := m.participantID("alice"); got != "pt-alice" {
		t.Fatalf("participantID(alice) = %q, want pt-alice", got)
	}
	if got := m.participantID("alice!"); got == "pt-alice" || !strings.HasPrefix(got, "pt-alice-") {
		t.Fatalf("participantID(alice!) = %q, want stable collision suffix", got)
	}
}

func TestMigrateTypedIDsNoopsAfterMigration(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".csgclaw")
	mustMkdir(t, filepath.Join(root, "im"))
	writeJSON(t, filepath.Join(root, "state.json"), map[string]any{
		"version":         1,
		"model_providers": map[string]any{"items": map[string]any{}},
		"agents":          map[string]any{"items": []any{}},
		"participants":    map[string]any{"items": []any{}},
	})
	writeJSON(t, filepath.Join(root, "im", "state.json"), map[string]any{
		"current_user_id": "user-admin",
		"users":           []map[string]any{{"id": "user-admin", "name": "admin"}},
		"rooms": []map[string]any{{
			"id":       "room-main",
			"members":  []string{"user-admin"},
			"messages": "sessions/room-main.jsonl",
		}},
	})

	result, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 24, 12, 0, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("MigrateTypedIDs() error = %v", err)
	}
	if result.BackupPath != "" {
		t.Fatalf("BackupPath = %q, want no backup for no-op migration", result.BackupPath)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), ".csgclaw_backup_20260624_001")); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup for no-op migration: %v", err)
	}

	var rootState map[string]any
	readJSON(t, filepath.Join(root, "state.json"), &rootState)
	if rootState["version"].(float64) != 1 {
		t.Fatalf("root state changed after no-op migration: %#v", rootState)
	}
}

func TestMigrateTypedIDsMovesLegacyDefaultModelToAgentProfileDefaults(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, ".csgclaw")
	mustMkdir(t, root)
	writeJSON(t, filepath.Join(root, "state.json"), map[string]any{
		"version": 1,
		"model_providers": map[string]any{
			"default_model": map[string]any{"model_provider_id": "openai", "model_id": "gpt-4.1"},
			"items":         map[string]any{"openai": map[string]any{"display_name": "OpenAI"}},
		},
		"agents":       map[string]any{"items": []any{}},
		"participants": map[string]any{"items": []any{}},
	})

	result, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 24, 12, 0, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("MigrateTypedIDs() error = %v", err)
	}
	if result.BackupPath == "" {
		t.Fatal("BackupPath is empty, want cleanup migration backup")
	}

	var rootState map[string]any
	readJSON(t, filepath.Join(root, "state.json"), &rootState)
	modelProviders := rootState["model_providers"].(map[string]any)
	if _, ok := modelProviders["default_model"]; ok {
		t.Fatalf("model_providers.default_model persisted: %#v", modelProviders)
	}
	agents := rootState["agents"].(map[string]any)
	modelDefaults := agents["model_defaults"].(map[string]any)
	if modelDefaults["model_provider_id"] != "openai" || modelDefaults["model_id"] != "gpt-4.1" {
		t.Fatalf("model_defaults = %#v, want openai/gpt-4.1", modelDefaults)
	}
	if _, ok := agents["profile_defaults"]; ok {
		t.Fatalf("legacy profile_defaults persisted: %#v", agents)
	}
}

func TestMigrateTypedIDsRepairsAlreadyMigratedThreadContext(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, ".csgclaw")
	mustMkdir(t, filepath.Join(root, "im", "sessions"))
	mustMkdir(t, filepath.Join(root, "im", "threads", "room-main"))
	writeJSON(t, filepath.Join(root, "state.json"), map[string]any{
		"version":         1,
		"model_providers": map[string]any{"items": map[string]any{"openai": map[string]any{"display_name": "OpenAI"}}},
		"agents":          map[string]any{"items": []map[string]any{{"id": "agent-manager", "name": "manager"}}},
		"participants":    map[string]any{"items": []map[string]any{{"id": "pt-manager", "name": "manager"}}},
	})
	writeJSON(t, filepath.Join(root, "im", "state.json"), map[string]any{
		"current_user_id": "user-admin",
		"users": []map[string]any{
			{"id": "user-admin", "name": "admin"},
			{"id": "user-manager", "name": "manager"},
		},
		"rooms": []map[string]any{{
			"id":       "room-main",
			"members":  []string{"user-admin", "user-manager"},
			"messages": "sessions/room-main.jsonl",
			"threads": []map[string]any{{
				"root_message_id": "msg-root",
				"created_at":      "2026-06-24T01:06:03Z",
				"summary":         map[string]any{"root_excerpt": "root", "message_count": 1},
			}},
		}},
	})
	writeFile(t, filepath.Join(root, "im", "sessions", "room-main.jsonl"), `{"id":"msg-root","sender_id":"user-manager","content":"hi","created_at":"2026-06-24T01:06:03Z"}`+"\n")
	writeJSON(t, filepath.Join(root, "im", "threads", "room-main", "msg-root.json"), map[string]any{
		"root_message_id": "msg-root",
		"created_at":      "2026-06-24T01:06:03Z",
		"context": []map[string]any{{
			"id":         "msg-root",
			"sender_id":  "pt-manager",
			"content":    `<at user_id="pt-admin">admin</at> hi`,
			"created_at": "2026-06-24T01:06:03Z",
			"mentions":   []map[string]any{{"id": "pt-admin", "name": "admin"}},
			"event":      map[string]any{"actor_id": "pt-manager", "target_ids": []string{"pt-admin"}},
		}},
		"summary": map[string]any{"root_excerpt": "root", "message_count": 1},
	})

	result, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 24, 12, 0, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("MigrateTypedIDs() error = %v", err)
	}
	if result.BackupPath != filepath.Join(parent, ".csgclaw_backup_20260624_001") {
		t.Fatalf("BackupPath = %q, want backup for stale thread context", result.BackupPath)
	}

	var rootState map[string]any
	readJSON(t, filepath.Join(root, "state.json"), &rootState)
	agents := rootState["agents"].(map[string]any)["items"].([]any)
	if agents[0].(map[string]any)["id"] != "agent-manager" {
		t.Fatalf("root state agents changed unexpectedly: %#v", agents)
	}

	var threadState map[string]any
	readJSON(t, filepath.Join(root, "im", "threads", "room-main", "msg-root.json"), &threadState)
	context := threadState["context"].([]any)
	message := context[0].(map[string]any)
	if message["sender_id"] != "user-manager" {
		t.Fatalf("thread sender = %v, want user-manager", message["sender_id"])
	}
	if !strings.Contains(message["content"].(string), `user_id="user-admin"`) {
		t.Fatalf("thread content = %q, want user-admin mention", message["content"])
	}
	mentions := message["mentions"].([]any)
	if mentions[0].(map[string]any)["id"] != "user-admin" {
		t.Fatalf("thread mention = %#v, want user-admin", mentions[0])
	}
	event := message["event"].(map[string]any)
	if event["actor_id"] != "user-manager" {
		t.Fatalf("thread event actor = %v, want user-manager", event["actor_id"])
	}
	assertStringArray(t, event["target_ids"], []string{"user-admin"})

	second, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 24, 12, 1, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("second MigrateTypedIDs() error = %v", err)
	}
	if second.BackupPath != "" {
		t.Fatalf("second BackupPath = %q, want no-op after repair", second.BackupPath)
	}
}

func TestMigrateTypedIDsRepairsAlreadyMigratedSessionFile(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, ".csgclaw")
	mustMkdir(t, filepath.Join(root, "im", "sessions"))
	writeJSON(t, filepath.Join(root, "state.json"), map[string]any{
		"version":         1,
		"model_providers": map[string]any{"items": map[string]any{"openai": map[string]any{"display_name": "OpenAI"}}},
		"agents":          map[string]any{"items": []map[string]any{{"id": "agent-manager", "name": "manager"}}},
		"participants":    map[string]any{"items": []map[string]any{{"id": "pt-admin", "name": "admin"}, {"id": "pt-manager", "name": "manager"}}},
	})
	writeJSON(t, filepath.Join(root, "im", "state.json"), map[string]any{
		"current_user_id": "user-admin",
		"users": []map[string]any{
			{"id": "user-admin", "name": "admin"},
			{"id": "user-manager", "name": "manager"},
		},
		"rooms": []map[string]any{{
			"id":       "room-main",
			"members":  []string{"user-admin", "user-manager"},
			"messages": "sessions/room-main.jsonl",
		}},
	})
	writeFile(t, filepath.Join(root, "im", "sessions", "room-main.jsonl"), `{"id":"msg-root","sender_id":"pt-manager","content":"<at user_id=\"pt-admin\">admin</at> hi","created_at":"2026-06-24T01:06:03Z","mentions":[{"id":"pt-admin","name":"admin"}],"event":{"actor_id":"pt-manager","target_ids":["pt-admin"]}}`+"\n")

	result, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 24, 12, 0, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("MigrateTypedIDs() error = %v", err)
	}
	if result.BackupPath != filepath.Join(parent, ".csgclaw_backup_20260624_001") {
		t.Fatalf("BackupPath = %q, want backup for stale session file", result.BackupPath)
	}

	lines, err := readJSONLinesIfExists(filepath.Join(root, "im", "sessions", "room-main.jsonl"))
	if err != nil {
		t.Fatalf("read migrated session: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("session lines = %#v, want one line", lines)
	}
	message := lines[0]
	if message["sender_id"] != "user-manager" {
		t.Fatalf("session sender = %v, want user-manager", message["sender_id"])
	}
	if !strings.Contains(message["content"].(string), `user_id="user-admin"`) {
		t.Fatalf("session content = %q, want user-admin mention", message["content"])
	}
	mentions := message["mentions"].([]any)
	if mentions[0].(map[string]any)["id"] != "user-admin" {
		t.Fatalf("session mention = %#v, want user-admin", mentions[0])
	}
	event := message["event"].(map[string]any)
	if event["actor_id"] != "user-manager" {
		t.Fatalf("session event actor = %v, want user-manager", event["actor_id"])
	}
	assertStringArray(t, event["target_ids"], []string{"user-admin"})

	second, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 24, 12, 1, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("second MigrateTypedIDs() error = %v", err)
	}
	if second.BackupPath != "" {
		t.Fatalf("second BackupPath = %q, want no-op after repair", second.BackupPath)
	}
}

func TestMigrateTypedIDsRepairsStaleIMUserIndex(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, ".csgclaw")
	mustMkdir(t, filepath.Join(root, "im", "sessions"))
	writeJSON(t, filepath.Join(root, "state.json"), map[string]any{
		"version":         1,
		"model_providers": map[string]any{"items": map[string]any{"openai": map[string]any{"display_name": "OpenAI"}}},
		"agents":          map[string]any{"items": []map[string]any{{"id": "agent-manager", "name": "manager"}}},
		"participants":    map[string]any{"items": []map[string]any{{"id": "pt-admin", "name": "admin"}, {"id": "pt-manager", "name": "manager"}}},
	})
	writeJSON(t, filepath.Join(root, "im", "state.json"), map[string]any{
		"current_user_id": "u-admin",
		"users": []map[string]any{
			{"id": "u-admin", "name": "admin", "handle": "admin"},
			{"id": "pt-manager", "name": "manager", "handle": "manager"},
		},
		"rooms": []map[string]any{{
			"id":       "room-main",
			"members":  []string{"user-admin", "user-manager"},
			"messages": "sessions/room-main.jsonl",
		}},
	})
	writeFile(t, filepath.Join(root, "im", "sessions", "room-main.jsonl"), `{"id":"msg-root","sender_id":"user-manager","content":"hi","created_at":"2026-06-24T01:06:03Z"}`+"\n")

	result, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 24, 12, 0, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("MigrateTypedIDs() error = %v", err)
	}
	if result.BackupPath != filepath.Join(parent, ".csgclaw_backup_20260624_001") {
		t.Fatalf("BackupPath = %q, want backup for stale IM user index", result.BackupPath)
	}

	var imState map[string]any
	readJSON(t, filepath.Join(root, "im", "state.json"), &imState)
	if imState["current_user_id"] != "user-admin" {
		t.Fatalf("current_user_id = %v, want user-admin", imState["current_user_id"])
	}
	users := imState["users"].([]any)
	if users[0].(map[string]any)["id"] != "user-admin" || users[1].(map[string]any)["id"] != "user-manager" {
		t.Fatalf("users = %#v, want user-admin/user-manager", users)
	}
	for _, item := range users {
		if _, ok := item.(map[string]any)["handle"]; ok {
			t.Fatalf("migrated user still has handle: %#v", item)
		}
	}

	second, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 24, 12, 1, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("second MigrateTypedIDs() error = %v", err)
	}
	if second.BackupPath != "" {
		t.Fatalf("second BackupPath = %q, want no-op after repair", second.BackupPath)
	}
}

func TestMigrateTypedIDsRepairsStaleInlineMessagePayload(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, ".csgclaw")
	mustMkdir(t, filepath.Join(root, "im"))
	writeJSON(t, filepath.Join(root, "state.json"), map[string]any{
		"version":         1,
		"model_providers": map[string]any{"items": map[string]any{"openai": map[string]any{"display_name": "OpenAI"}}},
		"agents":          map[string]any{"items": []map[string]any{{"id": "agent-manager", "name": "manager"}}},
		"participants":    map[string]any{"items": []map[string]any{{"id": "pt-admin", "name": "admin"}, {"id": "pt-manager", "name": "manager"}}},
	})
	writeJSON(t, filepath.Join(root, "im", "state.json"), map[string]any{
		"current_user_id": "user-admin",
		"users": []map[string]any{
			{"id": "user-admin", "name": "admin"},
			{"id": "user-manager", "name": "manager"},
		},
		"rooms": []map[string]any{{
			"id":      "room-main",
			"members": []string{"user-admin", "user-manager"},
			"messages": []map[string]any{{
				"id":         "msg-root",
				"sender_id":  "user-manager",
				"content":    `<at user_id="pt-admin">admin</at> hi`,
				"created_at": "2026-06-24T01:06:03Z",
				"mentions":   []map[string]any{{"id": "pt-admin", "name": "admin"}},
				"event":      map[string]any{"actor_id": "pt-manager", "target_ids": []string{"pt-admin"}},
			}},
		}},
	})

	if !needsTypedIDMigration(root) {
		t.Fatal("needsTypedIDMigration() = false, want stale inline payload to trigger typed-ID migration")
	}

	result, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 24, 12, 0, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("MigrateTypedIDs() error = %v", err)
	}
	if result.BackupPath != filepath.Join(parent, ".csgclaw_backup_20260624_001") {
		t.Fatalf("BackupPath = %q, want backup for stale inline message payload", result.BackupPath)
	}

	lines, err := readJSONLinesIfExists(filepath.Join(root, "im", "sessions", "room-main.jsonl"))
	if err != nil {
		t.Fatalf("read migrated inline session: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("session lines = %#v, want one line", lines)
	}
	message := lines[0]
	if !strings.Contains(message["content"].(string), `user_id="user-admin"`) {
		t.Fatalf("session content = %q, want user-admin mention", message["content"])
	}
	mentions := message["mentions"].([]any)
	if mentions[0].(map[string]any)["id"] != "user-admin" {
		t.Fatalf("session mention = %#v, want user-admin", mentions[0])
	}
	event := message["event"].(map[string]any)
	if event["actor_id"] != "user-manager" {
		t.Fatalf("session event actor = %v, want user-manager", event["actor_id"])
	}
	assertStringArray(t, event["target_ids"], []string{"user-admin"})
}

func TestMigrateTypedIDsImportsLegacyAuthAndRenamesCLIProxyAuthDir(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, ".csgclaw")
	writeJSON(t, filepath.Join(root, "state.json"), map[string]any{
		"version":         1,
		"model_providers": map[string]any{"items": map[string]any{"openai": map[string]any{"display_name": "OpenAI"}}},
		"agents":          map[string]any{"items": []map[string]any{{"id": "agent-manager", "name": "manager"}}},
		"participants":    map[string]any{"items": []map[string]any{{"id": "pt-manager", "name": "manager"}}},
		"auth": map[string]any{
			"opencsg": map[string]any{
				"tokens": map[string]any{"access_token": "state-token"},
			},
			"future": map[string]any{"enabled": true},
		},
	})
	writeJSON(t, filepath.Join(root, "auth.json"), map[string]any{
		"tokens": map[string]any{"access_token": "legacy-token"},
		"account": map[string]any{
			"user_id":  "alice",
			"base_url": "https://hub.example.test",
		},
		"last_refresh": "2026-06-25T07:00:00Z",
	})
	writeJSON(t, filepath.Join(root, "auth", "csghub.json"), map[string]any{
		"ai_gateway_builtin_api_key": "gk_legacy",
	})
	writeFile(t, filepath.Join(root, "auth", "config.yaml"), "legacy config\n")
	writeFile(t, filepath.Join(root, "auth", "codex-longyun.json"), `{"provider":"codex"}`)
	writeFile(t, filepath.Join(root, "cliproxy-auth", "config.yaml"), "current config\n")

	result, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 25, 12, 0, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("MigrateTypedIDs() error = %v", err)
	}
	wantBackup := filepath.Join(parent, ".csgclaw_backup_20260625_001")
	if result.BackupPath != wantBackup {
		t.Fatalf("BackupPath = %q, want %q", result.BackupPath, wantBackup)
	}
	if _, err := os.Stat(filepath.Join(wantBackup, "auth.json")); err != nil {
		t.Fatalf("backup missing legacy auth.json: %v", err)
	}

	assertMissing(t, filepath.Join(root, "auth.json"))
	assertMissing(t, filepath.Join(root, "auth", "csghub.json"))
	assertMissing(t, filepath.Join(root, "auth"))
	if _, err := os.Stat(filepath.Join(root, "cliproxy-auth", "codex-longyun.json")); err != nil {
		t.Fatalf("cliproxy token file was not moved: %v", err)
	}
	currentConfig, err := os.ReadFile(filepath.Join(root, "cliproxy-auth", "config.yaml"))
	if err != nil {
		t.Fatalf("read current cliproxy config: %v", err)
	}
	if string(currentConfig) != "current config\n" {
		t.Fatalf("current cliproxy config = %q, want preserved destination", currentConfig)
	}
	matches, err := filepath.Glob(filepath.Join(root, "cliproxy-auth", "config.legacy-*.yaml"))
	if err != nil {
		t.Fatalf("glob legacy config: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("legacy collision files = %#v, want exactly one", matches)
	}
	legacyConfig, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read legacy cliproxy config collision file: %v", err)
	}
	if string(legacyConfig) != "legacy config\n" {
		t.Fatalf("legacy collision config = %q, want moved legacy content", legacyConfig)
	}

	var rootState map[string]any
	readJSON(t, filepath.Join(root, "state.json"), &rootState)
	authState := rootState["auth"].(map[string]any)
	if _, ok := authState["csghub"]; ok {
		t.Fatalf("root state contains auth.csghub: %#v", authState)
	}
	if _, ok := authState["future"].(map[string]any); !ok {
		t.Fatalf("future auth key was not preserved: %#v", authState)
	}
	openCSG := authState["opencsg"].(map[string]any)
	tokens := openCSG["tokens"].(map[string]any)
	if tokens["access_token"] != "state-token" {
		t.Fatalf("state auth token was overwritten: %#v", tokens)
	}
	account := openCSG["account"].(map[string]any)
	if account["user_id"] != "alice" || account["base_url"] != "https://hub.example.test" {
		t.Fatalf("legacy account not imported: %#v", account)
	}
	if openCSG["ai_gateway_builtin_api_key"] != "gk_legacy" {
		t.Fatalf("ai gateway key = %v, want gk_legacy", openCSG["ai_gateway_builtin_api_key"])
	}
	if _, ok := rootState["agents"].(map[string]any); !ok {
		t.Fatalf("existing agents section was not preserved: %#v", rootState)
	}

	second, err := MigrateTypedIDs(MigrateOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 6, 25, 12, 0, 0, 0, time.Local) },
	})
	if err != nil {
		t.Fatalf("second MigrateTypedIDs() error = %v", err)
	}
	if second.BackupPath != "" {
		t.Fatalf("second BackupPath = %q, want no-op migration", second.BackupPath)
	}
}

func TestReconcileTypedAgentDirsFoldsPrefixedNameDirsAfterMigration(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".csgclaw")
	writeJSON(t, filepath.Join(root, "state.json"), map[string]any{
		"version": 1,
		"agents": map[string]any{
			"items": []map[string]any{{
				"id":         "agent-zoyz2k",
				"name":       "dev",
				"runtime_id": "rt-agent-zoyz2k",
			}},
		},
	})
	writeFile(t, filepath.Join(root, "agents", "agent-dev", ".codex", "runtime.json"), `{"agent_id":"agent-zoyz2k","runtime_id":"rt-agent-zoyz2k"}`)
	writeFile(t, filepath.Join(root, "agents", "agent-zoyz2k", ".codex", "session.json"), `{"runtime_id":"rt-agent-zoyz2k"}`)

	if err := ReconcileTypedAgentDirs(root); err != nil {
		t.Fatalf("ReconcileTypedAgentDirs() error = %v", err)
	}

	assertMissing(t, filepath.Join(root, "agents", "agent-dev"))
	if _, err := os.Stat(filepath.Join(root, "agents", "agent-zoyz2k", ".codex", "runtime.json")); err != nil {
		t.Fatalf("runtime metadata was not merged into canonical dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "agents", "agent-zoyz2k", ".codex", "session.json")); err != nil {
		t.Fatalf("existing canonical session metadata was not preserved: %v", err)
	}
}

func TestRenameDirNoOverwriteRemovesMergedNestedSourceDirs(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "agents", "manager")
	dst := filepath.Join(root, "agents", "agent-manager")
	writeFile(t, filepath.Join(src, ".picoclaw", "logs", "gateway.log"), "source log\n")
	writeFile(t, filepath.Join(dst, ".picoclaw", "logs", "gateway.log"), "destination log\n")
	writeFile(t, filepath.Join(src, ".picoclaw", "config.json"), `{"agent_id":"u-manager"}`)

	if err := renameDirNoOverwrite(src, dst); err != nil {
		t.Fatalf("renameDirNoOverwrite() error = %v", err)
	}
	assertMissing(t, src)

	data, err := os.ReadFile(filepath.Join(dst, ".picoclaw", "logs", "gateway.log"))
	if err != nil {
		t.Fatalf("read destination log: %v", err)
	}
	if string(data) != "destination log\n" {
		t.Fatalf("destination log = %q, want destination preserved", data)
	}
	if _, err := os.Stat(filepath.Join(dst, ".picoclaw", "config.json")); err != nil {
		t.Fatalf("unique source file was not moved: %v", err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(%s): %v", path, err)
	}
	writeFile(t, path, string(append(data, '\n')))
}

func readJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("Unmarshal(%s): %v\n%s", path, err, data)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists after migration: %v", path, err)
	}
}

func assertJSONContainsID(t *testing.T, items []any, id string) {
	t.Helper()
	for _, item := range items {
		if item.(map[string]any)["id"] == id {
			return
		}
	}
	t.Fatalf("items do not contain id %q: %#v", id, items)
}

func jsonObjectWithID(t *testing.T, items []any, id string) map[string]any {
	t.Helper()
	for _, item := range items {
		obj := item.(map[string]any)
		if obj["id"] == id {
			return obj
		}
	}
	t.Fatalf("items do not contain id %q: %#v", id, items)
	return nil
}

func assertJSONContainsIDPrefix(t *testing.T, items []any, prefix string) {
	t.Helper()
	for _, item := range items {
		id, _ := item.(map[string]any)["id"].(string)
		if strings.HasPrefix(id, prefix) {
			return
		}
	}
	t.Fatalf("items do not contain prefix %q: %#v", prefix, items)
}

func assertUniqueJSONIDs(t *testing.T, items []any) {
	t.Helper()
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		id, _ := item.(map[string]any)["id"].(string)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate id %q in items: %#v", id, items)
		}
		seen[id] = struct{}{}
	}
}

func assertStringArray(t *testing.T, value any, want []string) {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want []any", value)
	}
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.(string))
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("array = %#v, want %#v", got, want)
	}
}
