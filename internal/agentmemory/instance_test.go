package agentmemory

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/localstore"
)

func TestMemoryGlobalStatePreservesMetadataAndDoesNotWriteOnTurnsOrRestart(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.json")
	want := instance{Mode: "local", Epoch: "existing-instance", HistoryAfter: time.Now().UTC().Add(-time.Hour)}
	if err := localstore.WriteSection(path, "agent_memory", want); err != nil {
		t.Fatal(err)
	}
	if err := localstore.WriteSection(path, "auth", map[string]string{"fixture": "preserve"}); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1000000000, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(root, "agents")
	store, err := Open(agents, path, "local")
	if err != nil {
		t.Fatal(err)
	}
	if store.instance != want {
		t.Fatalf("existing instance metadata changed: %+v", store.instance)
	}
	enableMemory(t, store, testScope)
	Worker{Store: store}.process(context.Background(), testScope)
	enqueueMemory(t, store, "pending", "Remember a queued preference")
	restarted, err := Open(agents, path, "local")
	if err != nil {
		t.Fatal(err)
	}
	if restarted.instance != want || getMemory(t, restarted, testScope).Status != "pending" {
		t.Fatal("restart changed metadata or lost the pending task")
	}
	Worker{Store: restarted, Extract: func(context.Context, string, Extraction) ([]Change, error) {
		return []Change{{Kind: "preference", Text: "Use concise reports", SourceMessageID: "pending"}}, nil
	}}.process(context.Background(), testScope)
	doc := getMemory(t, restarted, testScope)
	if doc.Status != "idle" || len(doc.Entries) != 1 {
		t.Fatalf("pending task did not complete after restart: %+v", doc)
	}
	if _, err := restarted.Forget(testScope, Source{"private", "pending"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || !bytes.Equal(before, after) || !info.ModTime().Equal(stamp) {
		t.Fatalf("memory operations or restart wrote global state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agents, "instance.json")); !os.IsNotExist(err) {
		t.Fatalf("separate instance file exists: %v", err)
	}
}

func TestMemoryGlobalModeTransitionPreservesOtherSections(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.json")
	agents := filepath.Join(root, "agents")
	if err := localstore.WriteSection(path, "participants", map[string]string{"fixture": "preserve"}); err != nil {
		t.Fatal(err)
	}
	store, err := Open(agents, path, "disabled")
	if err != nil {
		t.Fatal(err)
	}
	store.instance.HistoryAfter = time.Now()
	if err := localstore.WriteSection(path, "agent_memory", store.instance); err != nil {
		t.Fatal(err)
	}
	next, err := Open(agents, path, "local")
	if err != nil {
		t.Fatal(err)
	}
	if next.instance.Epoch == store.instance.Epoch || !next.instance.HistoryAfter.IsZero() {
		t.Fatalf("mode transition did not rotate policy and allow all private history: %+v", next.instance)
	}
	var saved instance
	if found, err := localstore.ReadSection(path, "agent_memory", &saved); err != nil || !found || saved.Epoch != next.instance.Epoch || saved.Mode != "local" || !saved.HistoryAfter.Equal(next.instance.HistoryAfter) {
		t.Fatalf("instance metadata not persisted: %+v, %v", saved, err)
	}
	var other map[string]string
	if found, err := localstore.ReadSection(path, "participants", &other); err != nil || !found || other["fixture"] != "preserve" {
		t.Fatalf("other global section changed: %+v, %v", other, err)
	}
	if doc, err := next.GetOrInitialize(testScope); err != nil || !doc.Enabled {
		t.Fatalf("default memory did not start history learning: %+v %v", doc, err)
	}
	worker := Worker{Store: next, History: historyOnce("old-private-message", "Historical reporting preference"),
		Extract: func(_ context.Context, _ string, input Extraction) ([]Change, error) {
			return []Change{{Kind: "preference", Text: input.Messages[0].Text, SourceMessageID: input.Messages[0].MessageID}}, nil
		}}
	worker.process(context.Background(), testScope)
	worker.process(context.Background(), testScope)
	if doc := getMemory(t, next, testScope); len(doc.Entries) != 1 || doc.Entries[0].Text != "Historical reporting preference" {
		t.Fatalf("history from before enabling memory was lost: %+v", doc)
	}
}

func TestMemoryRejectsInvalidGlobalStateWithoutOverwriting(t *testing.T) {
	for _, content := range []string{`broken`, `{"agent_memory":{"epoch":12}}`, `{"agent_memory":[]}`} {
		t.Run(content, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "state.json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(filepath.Join(root, "agents"), path, "local"); err == nil {
				t.Fatal("invalid global state accepted")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != content {
				t.Fatalf("invalid global state overwritten: %v", err)
			}
		})
	}
}

func TestMemoryFirstStartupPreservesExistingRootSections(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"agents":{"items":[]},"auth":{"fixture":"preserve"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(root, "agents"), path, "local")
	if err != nil {
		t.Fatal(err)
	}
	var sections map[string]json.RawMessage
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &sections) != nil || store.instance.Epoch == "" || len(sections) != 4 {
		t.Fatalf("first startup did not preserve other root sections: %v", err)
	}
	var auth map[string]string
	if err := json.Unmarshal(sections["auth"], &auth); err != nil || auth["fixture"] != "preserve" {
		t.Fatalf("first startup changed auth: %v", err)
	}
}

func TestDefaultMemoryAndManualOptOutSurviveRestart(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.json")
	agents := filepath.Join(root, "agents")
	store, err := Open(agents, path, "local")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := store.GetOrInitialize(testScope)
	if err != nil || !doc.Enabled {
		t.Fatalf("default memory is not enabled: %+v %v", doc, err)
	}
	disabled := false
	if _, err := store.Update(testScope, Update{Revision: &doc.Revision, Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(agents, path, "local")
	if err != nil {
		t.Fatal(err)
	}
	if doc := getMemory(t, restarted, testScope); doc.Enabled {
		t.Fatal("restart overwrote manual opt-out")
	}
	if doc := getMemory(t, restarted, Scope{AgentID: testScope.AgentID, UserID: "user-other"}); !doc.Enabled {
		t.Fatal("manual opt-out changed the default for another scope")
	}
}
