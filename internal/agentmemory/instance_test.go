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
	since := time.Now()
	next, err := Open(agents, path, "local")
	if err != nil {
		t.Fatal(err)
	}
	if next.instance.Epoch == store.instance.Epoch || next.instance.HistoryAfter.Before(since) {
		t.Fatalf("mode transition did not rotate policy and set history boundary: %+v", next.instance)
	}
	var saved instance
	if found, err := localstore.ReadSection(path, "agent_memory", &saved); err != nil || !found || saved.Epoch != next.instance.Epoch || saved.Mode != "local" || !saved.HistoryAfter.Equal(next.instance.HistoryAfter) {
		t.Fatalf("instance metadata not persisted: %+v, %v", saved, err)
	}
	var other map[string]string
	if found, err := localstore.ReadSection(path, "participants", &other); err != nil || !found || other["fixture"] != "preserve" {
		t.Fatalf("other global section changed: %+v, %v", other, err)
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
