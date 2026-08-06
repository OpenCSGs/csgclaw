package agentsession

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"csgclaw/internal/localstore"
)

func TestStorePersistsAgentScopedBindings(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session-bindings")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.GetOrCreate("agent-a", "shared-session")
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.GetOrCreate("agent-a", "shared-session")
	if err != nil {
		t.Fatal(err)
	}
	otherAgent, err := store.GetOrCreate("agent-b", "shared-session")
	if err != nil {
		t.Fatal(err)
	}
	if first.ConversationKey != again.ConversationKey {
		t.Fatalf("conversation key changed: %q != %q", first.ConversationKey, again.ConversationKey)
	}
	if first.ConversationKey == otherAgent.ConversationKey {
		t.Fatalf("different Agents share conversation key %q", first.ConversationKey)
	}

	reloaded, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := reloaded.GetOrCreate("agent-a", "shared-session")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ConversationKey != first.ConversationKey {
		t.Fatalf("reloaded conversation key = %q, want %q", persisted.ConversationKey, first.ConversationKey)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("binding files = %d, want one file per agent", len(entries))
	}
}

func TestImportLegacyStatePreservesBindingsAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	dir := filepath.Join(root, "session-bindings")
	want := Binding{AgentID: "agent-a", ExternalSession: "existing-session", ConversationKey: "conv_existing"}
	if err := localstore.WriteSection(statePath, legacyRootStateSection, legacyPersistedState{Items: []Binding{want}}); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ImportLegacyState(statePath, store); err != nil {
		t.Fatal(err)
	}
	if err := ImportLegacyState(statePath, store); err != nil {
		t.Fatalf("repeat import: %v", err)
	}
	got, err := store.GetOrCreate(want.AgentID, want.ExternalSession)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("binding = %#v, want %#v", got, want)
	}
	data, err := os.ReadFile(filepath.Join(dir, bindingFileName(want.AgentID)))
	if err != nil {
		t.Fatal(err)
	}
	if records := bytes.Count(data, []byte{'\n'}); records != 1 {
		t.Fatalf("persisted records = %d, want 1", records)
	}
	reloaded, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err = reloaded.GetOrCreate(want.AgentID, want.ExternalSession)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("reloaded binding = %#v, want %#v", got, want)
	}
}

func TestStoreConcurrentGetOrCreatePersistsOneBinding(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session-bindings")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 100
	results := make(chan Binding, workers)
	errors := make(chan error, workers)
	start := make(chan struct{})
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			binding, err := store.GetOrCreate("agent-a", "shared-session")
			if err != nil {
				errors <- err
				return
			}
			results <- binding
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}

	conversationKey := ""
	for binding := range results {
		if conversationKey == "" {
			conversationKey = binding.ConversationKey
		}
		if binding.ConversationKey != conversationKey {
			t.Fatalf("conversation key = %q, want %q", binding.ConversationKey, conversationKey)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, bindingFileName("agent-a")))
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(data, []byte{'\n'}); got != 1 {
		t.Fatalf("persisted records = %d, want 1; data=%s", got, data)
	}
}

func TestStoreConcurrentSessionsAppendCompleteRecords(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session-bindings")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	const sessions = 200
	start := make(chan struct{})
	errors := make(chan error, sessions)
	var group sync.WaitGroup
	for index := range sessions {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, err := store.GetOrCreate("agent-a", fmt.Sprintf("session-%03d", index))
			if err != nil {
				errors <- err
			}
		}()
	}
	close(start)
	group.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	if got := len(store.Bindings()); got != sessions {
		t.Fatalf("bindings = %d, want %d", got, sessions)
	}
	reloaded, err := NewStore(dir)
	if err != nil {
		t.Fatalf("reload concurrent appends: %v", err)
	}
	if got := len(reloaded.Bindings()); got != sessions {
		t.Fatalf("reloaded bindings = %d, want %d", got, sessions)
	}
}

func TestStoreRepairsIncompleteTrailingRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session-bindings")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	first := Binding{AgentID: "agent-a", ExternalSession: "first", ConversationKey: "conv_first"}
	data, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	data = append(append(data, '\n'), []byte(`{"agent_id":"agent-a","external_session_id":"partial"`)...)
	path := filepath.Join(dir, bindingFileName("agent-a"))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Bindings(); len(got) != 1 || got[0] != first {
		t.Fatalf("bindings = %#v, want only %#v", got, first)
	}
	if _, err := store.GetOrCreate("agent-a", "second"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(dir); err != nil {
		t.Fatalf("reload repaired store: %v", err)
	}
	finalData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(finalData), "partial") {
		t.Fatalf("incomplete tail was not removed: %s", finalData)
	}
}

func TestStoreRepairsMissingFinalNewline(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session-bindings")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	binding := Binding{AgentID: "agent-a", ExternalSession: "first", ConversationKey: "conv_first"}
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, bindingFileName("agent-a"))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(dir); err != nil {
		t.Fatal(err)
	}
	repaired, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(repaired) == 0 || repaired[len(repaired)-1] != '\n' {
		t.Fatalf("repaired data = %q, want trailing newline", repaired)
	}
}

func TestStoreRejectsConflictingBindings(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session-bindings")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, bindingFileName("agent-a"))
	content := ""
	for _, binding := range []Binding{
		{AgentID: "agent-a", ExternalSession: "shared", ConversationKey: "conv_first"},
		{AgentID: "agent-a", ExternalSession: "shared", ConversationKey: "conv_second"},
	} {
		data, err := json.Marshal(binding)
		if err != nil {
			t.Fatal(err)
		}
		content += string(data) + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(dir); err == nil || !strings.Contains(err.Error(), "conflicting binding") {
		t.Fatalf("NewStore() error = %v, want conflicting binding", err)
	}
}
