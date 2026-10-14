package codex

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	agentruntime "csgclaw/internal/runtime"
	skill "csgclaw/internal/skill/state"
	"csgclaw/internal/taskconfig"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoomConfigurationCodexKeepsHistoryWhenModelChanges(t *testing.T) {
	rt := New(Dependencies{ResolveRoomProfile: func(string, string, taskconfig.Config) (agentruntime.Profile, error) {
		return agentruntime.Profile{}, nil
	}})
	meta := sessionMetadata{ConversationProfileFingerprint: "old-model", ConversationSessions: map[string]string{"room": "native-thread"}, FilePublishingConversations: map[string]bool{"room": true}}
	threads, files := rt.persistedRoomConversations(meta, agentruntime.Profile{ModelID: "new-model"})
	if threads["room"] != "native-thread" || !files["room"] {
		t.Fatal("model change discarded native history")
	}
	threads["room"] = "changed"
	if meta.ConversationSessions["room"] != "native-thread" {
		t.Fatal("mutated persistent snapshot")
	}
	rt.deps.ResolveRoomProfile = nil
	threads, _ = rt.persistedRoomConversations(meta, agentruntime.Profile{ModelID: "new-model"})
	if len(threads) != 0 {
		t.Fatal("changed existing global reset behavior")
	}
}

func TestRoomConfigurationCodexCannotElevateServerReadOnlyMode(t *testing.T) {
	t.Setenv("CSGCLAW_EXECUTION_MODE", "read_only")

	opts, err := conversationRuntimeOptions(nil)
	if err != nil || opts.ExecutionMode != "read_only" {
		t.Fatal("elevated server policy")
	}
	t.Setenv("CSGCLAW_EXECUTION_MODE", "standard")

	opts, err = conversationRuntimeOptions(map[string]any{conversationReadOnlyOption: true})
	if err != nil || opts.ExecutionMode != "read_only" {
		t.Fatal("did not apply room read-only mode")
	}
}

func TestRoomConfigurationReusesProcessAndNativeMappings(t *testing.T) {
	withAppServerHelperCommand(t, "room-configuration")
	t.Setenv("CSGCLAW_EXECUTION_MODE", "standard")
	root := t.TempDir()
	assignedSkill := filepath.Join(root, "agent-one", hostStateDirName, homeDirName, "skills", "assigned", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(assignedSkill), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assignedSkill, []byte("# Assigned skill"), 0600); err != nil {
		t.Fatal(err)
	}
	profile := agentruntime.Profile{ModelID: "first-model", BaseURL: "https://gateway.example/v1", APIKey: "test-key"}
	deps := Dependencies{BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"}, EventSink: NewEventSink(), AgentHome: func(id string) (string, error) { return filepath.Join(root, id), nil }, ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: h.RuntimeID, Name: h.RuntimeID, RuntimeID: h.RuntimeID, Profile: profile, RuntimeOptions: map[string]any{memoryModeOptionKey: MemoryModeDisabled}}, nil
	}, ResolveRoomProfile: func(_, _ string, cfg taskconfig.Config) (agentruntime.Profile, error) {
		p := profile
		p.ModelID = cfg.ModelID
		return p, nil
	}}
	rt := New(deps)
	t.Cleanup(func() { _ = rt.Close() })
	ids := map[string]string{}
	run := func(agent, room, model string, modes ...string) int {
		mode := ExecutionModeReadOnly
		if len(modes) > 0 {
			mode = modes[0]
		}
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req := contract.TurnRequest{ID: "turn", ConversationKey: contract.ConversationKey(room), Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "hello"}}, RoomConfiguration: &contract.RoomConfiguration{RoomID: room, Configuration: taskconfig.Config{ModelID: model, ExecutionMode: mode}}}
		result := rt.Conversation(agent).Run(ctx, req, contract.EventSinkFunc(func(context.Context, contract.TurnEvent) error { return nil }))
		if result.Status != contract.TurnSucceeded {
			t.Fatalf("run %s/%s: %+v", agent, room, result)
		}
		meta, err := rt.readSessionMetadata(agent)
		if err != nil {
			t.Fatal(err)
		}
		key := agent + ":" + room
		if old := ids[key]; old != "" && old != meta.ConversationSessions[room] {
			t.Fatal("replaced native history", old, meta.ConversationSessions[room])
		}
		ids[key] = meta.ConversationSessions[room]
		live := rt.sessionManager().(*appServerManager).liveSession(agent)
		if live.spec.Profile.ModelID != model || live.spec.ExecutionMode != mode {
			t.Fatal("incorrect process settings")
		}
		return live.cmd.Process.Pid
	}
	first := run("agent-one", "room-shared", "model-a")
	if next := run("agent-one", "room-shared", "model-a"); first != next {
		t.Fatal("unchanged settings restarted runtime")
	}
	if next := run("agent-one", "room-shared", "model-b"); first == next {
		t.Fatal("process configuration was not refreshed")
	}
	standard := run("agent-one", "room-shared", "model-b", ExecutionModeStandard)
	if readonly := run("agent-one", "room-shared", "model-b"); standard == readonly {
		t.Fatal("mode change did not restart the process")
	}
	run("agent-one", "room-shared", "model-b", ExecutionModeStandard)
	run("agent-one", "room-other", "model-c")
	run("agent-one", "room-shared", "model-a")
	run("agent-two", "room-shared", "model-a")
	if ids["agent-one:room-shared"] == ids["agent-two:room-shared"] || ids["agent-one:room-shared"] == ids["agent-one:room-other"] {
		t.Fatal("shared native identity across agents or rooms")
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	rt = New(deps)
	run("agent-one", "room-shared", "model-b")
	if data, err := os.ReadFile(assignedSkill); err != nil || string(data) != "# Assigned skill" {
		t.Fatal("runtime startup removed assigned skills", err)
	}
	if _, err := os.Stat(filepath.Join(root, "agent-one", "task-sessions")); !os.IsNotExist(err) {
		t.Fatal("created another conversation store")
	}
}

func TestRoomReadOnlyPreservesAgentSkills(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "skills", "assigned", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Assigned skill"), 0600); err != nil {
		t.Fatal(err)
	}
	original := map[string]skill.State{"assigned": {Enabled: true}}
	rt := New(Dependencies{ResolveRoomProfile: func(string, string, taskconfig.Config) (agentruntime.Profile, error) {
		return agentruntime.Profile{}, nil
	}})
	restricted, err := rt.conversationSkillStates(root, ExecutionModeReadOnly, original)
	if err != nil || restricted["assigned"].Enabled {
		t.Fatal("read-only skill restriction missing", err)
	}
	if !original["assigned"].Enabled {
		t.Fatal("changed saved Agent skill state")
	}
	restored, err := rt.conversationSkillStates(root, ExecutionModeStandard, original)
	if err != nil || !restored["assigned"].Enabled {
		t.Fatal("skill did not restore after mode change", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "# Assigned skill" {
		t.Fatal("removed skill content", err)
	}
}
