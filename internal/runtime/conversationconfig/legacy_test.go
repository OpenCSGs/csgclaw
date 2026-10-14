package conversationconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyNativeHistoryImportPreservesMappingsAndReset(t *testing.T) {
	home := t.TempDir()
	current := filepath.Join(home, ".dsh")
	old := filepath.Join(home, "task-sessions", "old-room-layout", ".dsh")
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(current, "runtime.json"), `{"agent_id":"agent-one","sessions":{"room-a":"original-thread","room-b":"other-thread"}}`)
	write(filepath.Join(old, "runtime.json"), `{"sessions":{"room-a":"latest-native-thread"}}`)
	native := filepath.Join("home", "sessions", "workspace", "latest-native-thread", "session.v3.jsonl.zstd")
	write(filepath.Join(old, native), "original compressed native history")
	paths := []string{"home/sessions"}
	if err := ImportLegacy(home, ".dsh", "runtime.json", []string{"sessions"}, paths); err != nil {
		t.Fatal(err)
	}
	meta, err := readObject(filepath.Join(current, "runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sessions map[string]string
	if err := json.Unmarshal(meta["sessions"], &sessions); err != nil {
		t.Fatal(err)
	}
	if sessions["room-a"] != "latest-native-thread" || sessions["room-b"] != "other-thread" || string(meta["agent_id"]) != `"agent-one"` {
		t.Fatal("lost native mapping or agent metadata")
	}
	data, err := os.ReadFile(filepath.Join(current, native))
	if err != nil || string(data) != "original compressed native history" {
		t.Fatal("lost native state", err)
	}
	if _, err := os.Stat(filepath.Join(old, native)); err != nil {
		t.Fatal("removed migration backup")
	}
	write(filepath.Join(current, "runtime.json"), `{"sessions":{"room-b":"other-thread"}}`)
	if err := ImportLegacy(home, ".dsh", "runtime.json", []string{"sessions"}, paths); err != nil {
		t.Fatal(err)
	}
	meta, _ = readObject(filepath.Join(current, "runtime.json"))
	sessions = nil
	_ = json.Unmarshal(meta["sessions"], &sessions)
	if _, ok := sessions["room-a"]; ok {
		t.Fatal("reimported reset history")
	}
}

func TestLegacyImportNeverOverwritesConflictingNativeFiles(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "task-sessions", "legacy", ".codex")
	target := filepath.Join(home, ".codex")
	for _, dir := range []string{source, target} {
		if err := os.MkdirAll(filepath.Join(dir, "home", "sessions"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(source, "session.json"), []byte(`{"conversation_sessions":{"room":"native"}}`), 0600)
	_ = os.WriteFile(filepath.Join(source, "home/sessions/native.jsonl"), []byte("old"), 0600)
	_ = os.WriteFile(filepath.Join(target, "home/sessions/native.jsonl"), []byte("current"), 0600)
	if err := ImportLegacy(home, ".codex", "session.json", []string{"conversation_sessions"}, []string{"home/sessions"}); err == nil {
		t.Fatal("overwrote conflicting history")
	}
	data, _ := os.ReadFile(filepath.Join(target, "home/sessions/native.jsonl"))
	if string(data) != "current" {
		t.Fatal("changed current history")
	}
	if _, err := os.Stat(filepath.Join(source, "imported-into-agent")); !os.IsNotExist(err) {
		t.Fatal("marked failed import as complete")
	}
}
