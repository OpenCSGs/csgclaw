package dsh

import (
	"context"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/dshcli"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/taskconfig"
	"encoding/json"

	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRoomConfigurationDSHPersistsAndIsolatesPermissionAndDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX helper")
	}
	root := t.TempDir()
	launcher := filepath.Join(root, "dsh-test")
	_ = os.WriteFile(launcher, []byte("#!/bin/sh\nexec \"$DSH_TEST_BINARY\" -test.run=TestDSHHelperProcess\n"), 0755)
	t.Setenv("GO_WANT_DSH_HELPER_PROCESS", "1")
	t.Setenv("DSH_TEST_UNIQUE_SESSIONS", "1")
	t.Setenv("DSH_TEST_BINARY", os.Args[0])
	record := filepath.Join(root, "record.json")
	t.Setenv("DSH_TEST_SESSION_RECORD", record)
	profile := agentruntime.Profile{BaseURL: "https://gateway.example/v1", APIKey: "test-key", ModelID: "test-model", ReasoningEffort: "auto"}
	ref := AgentRef{ID: "agent-test", Name: "test", RuntimeID: "rt-agent-test", Profile: profile}
	home := filepath.Join(root, "agent")
	rt := New(Dependencies{ResolveBinary: func(context.Context) (dshcli.Info, error) { return dshcli.Info{Path: launcher}, nil }, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) { return ref, nil }, AgentHome: func(string) (string, error) { return home, nil }, ResolveRoomProfile: func(string, string, taskconfig.Config) (agentruntime.Profile, error) { return profile, nil }})
	t.Cleanup(func() { _ = rt.Close() })
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	for _, dir := range []string{first, second} {
		_ = os.MkdirAll(dir, 0700)
		_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("hello"), 0600)
	}
	identities := map[string]string{}
	run := func(key, dir, mode, wantMethod string) {
		request := contract.TurnRequest{ID: "turn", ConversationKey: contract.ConversationKey(key), Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "hello"}}, RoomConfiguration: &contract.RoomConfiguration{RoomID: key, Configuration: taskconfig.Config{WorkspaceDir: dir, ExecutionMode: mode}}}
		result := rt.Conversation(ref.RuntimeID).Run(context.Background(), request, contract.EventSinkFunc(func(context.Context, contract.TurnEvent) error { return nil }))
		for _, file := range result.RuntimeFiles {
			file.Cleanup()
		}
		if result.Status != contract.TurnSucceeded {
			t.Fatalf("%s: %+v", key, result)
		}
		data, _ := os.ReadFile(record)
		var call struct {
			Method string
			Params struct {
				CWD string `json:"cwd"`
			}
		}
		_ = json.Unmarshal(data, &call)
		if call.Method != wantMethod || call.Params.CWD != dir {
			t.Fatalf("wrong resume/workspace: %s", data)
		}
		meta, err := readMetadata(filepath.Join(home, hostStateDirName))
		if err != nil {
			t.Fatal(err)
		}
		if meta.PermissionMode != mode || meta.Sessions[key] == "" {
			t.Fatalf("settings/history: %+v", meta)
		}
		if previous := identities[key]; previous != "" && previous != meta.Sessions[key] {
			t.Fatal("native session was replaced")
		}
		identities[key] = meta.Sessions[key]
	}
	run("one", first, "read-only", "session/new")
	before, _ := rt.process(ref.RuntimeID)
	run("one", first, "read-only", "session/new")
	after, _ := rt.process(ref.RuntimeID)
	if before != after {
		t.Fatal("unchanged configuration restarted the process")
	}
	run("two", second, "workspace-write", "session/new")
	run("one", first, "workspace-write", "session/resume")
	if identities["one"] == identities["two"] {
		t.Fatal("rooms share native session")
	}
	if _, err := rt.Stop(context.Background(), agentruntime.Handle{RuntimeID: ref.RuntimeID}); err != nil {
		t.Fatal(err)
	}
	rt = New(rt.deps)
	run("one", first, "workspace-write", "session/resume")
	if _, err := os.Stat(filepath.Join(home, "task-sessions")); !os.IsNotExist(err) {
		t.Fatal("created a second conversation store")
	}
	if ref.RuntimeOptions != nil || ref.Profile.ModelID != "test-model" {
		t.Fatal("changed agent defaults")
	}
}
