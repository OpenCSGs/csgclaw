package im

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveLargeMessageSpillsToBlob(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	roomID := "room-large"
	large := strings.Repeat("Z", 70*1024)

	state := Bootstrap{
		CurrentUserID: "u-admin",
		Users:         []User{{ID: "u-admin", Name: "admin", Handle: "admin"}},
		Rooms: []Room{{
			ID:      roomID,
			Title:   "large",
			Members: []string{"u-admin"},
			Messages: []Message{{
				ID:        "msg-large-1",
				SenderID:  "u-admin",
				Kind:      MessageKindMessage,
				Content:   large,
				CreatedAt: time.Date(2026, 5, 25, 3, 0, 0, 0, time.UTC),
			}},
		}},
	}
	if err := SaveBootstrap(statePath, state); err != nil {
		t.Fatalf("SaveBootstrap() error = %v", err)
	}

	sessionPath := filepath.Join(dir, "sessions", roomID+".jsonl")
	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("ReadFile(session) error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(sessionData)), "\n")
	if len(lines) != 1 {
		t.Fatalf("session lines = %d, want 1", len(lines))
	}
	if len(lines[0]) > maxSessionJSONLLineBytes {
		t.Fatalf("jsonl line bytes = %d, want <= %d", len(lines[0]), maxSessionJSONLLineBytes)
	}

	var record sessionMessageLine
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("Unmarshal(session line) error = %v", err)
	}
	if record.BlobRef == "" {
		t.Fatal("blob_ref is empty, want spillover reference")
	}
	if record.Content != "" {
		t.Fatalf("inline content = %d bytes, want empty with blob_ref", len(record.Content))
	}

	blobPath := filepath.Join(dir, "sessions", filepath.FromSlash(record.BlobRef))
	blobData, err := os.ReadFile(blobPath)
	if err != nil {
		t.Fatalf("ReadFile(blob) error = %v", err)
	}
	var blob sessionMessageBlob
	if err := json.Unmarshal(blobData, &blob); err != nil {
		t.Fatalf("Unmarshal(blob) error = %v", err)
	}
	if blob.Content != large {
		t.Fatalf("blob content len = %d, want %d", len(blob.Content), len(large))
	}

	loaded, err := LoadBootstrap(statePath)
	if err != nil {
		t.Fatalf("LoadBootstrap() error = %v", err)
	}
	if len(loaded.Rooms) != 1 || len(loaded.Rooms[0].Messages) != 1 {
		t.Fatalf("loaded rooms = %+v, want one large message", loaded.Rooms)
	}
	if loaded.Rooms[0].Messages[0].Content != large {
		t.Fatalf("loaded content len = %d, want %d", len(loaded.Rooms[0].Messages[0].Content), len(large))
	}
}

func TestSaveMessageKeepsSlashInvocationAsContentOnly(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	roomID := "room-skill"

	state := Bootstrap{
		CurrentUserID: "u-admin",
		Users:         []User{{ID: "u-admin", Name: "admin", Handle: "admin"}},
		Rooms: []Room{{
			ID:      roomID,
			Title:   "skill",
			Members: []string{"u-admin"},
			Messages: []Message{{
				ID:        "msg-skill-1",
				SenderID:  "u-admin",
				Kind:      MessageKindMessage,
				Content:   "/custom do it",
				CreatedAt: time.Date(2026, 5, 25, 3, 30, 0, 0, time.UTC),
			}},
		}},
	}
	if err := SaveBootstrap(statePath, state); err != nil {
		t.Fatalf("SaveBootstrap() error = %v", err)
	}

	sessionPath := filepath.Join(dir, "sessions", roomID+".jsonl")
	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("ReadFile(session) error = %v", err)
	}
	if !strings.Contains(string(sessionData), `"content":"/custom do it"`) {
		t.Fatalf("session = %q, want original slash invocation content", string(sessionData))
	}
	if strings.Contains(string(sessionData), "agent_content") || strings.Contains(string(sessionData), "Follow custom rules") {
		t.Fatalf("session = %q, want no hidden skill payload", string(sessionData))
	}

	loaded, err := LoadBootstrap(statePath)
	if err != nil {
		t.Fatalf("LoadBootstrap() error = %v", err)
	}
	if len(loaded.Rooms) != 1 || len(loaded.Rooms[0].Messages) != 1 {
		t.Fatalf("loaded rooms = %+v, want one slash invocation message", loaded.Rooms)
	}
	if loaded.Rooms[0].Messages[0].Content != "/custom do it" {
		t.Fatalf("loaded content = %q, want original slash invocation", loaded.Rooms[0].Messages[0].Content)
	}
}

func TestLoadLegacyOversizedInlineLine(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	roomID := "room-legacy"
	large := strings.Repeat("L", 70*1024)

	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacy := Message{
		ID:        "msg-legacy-fat",
		SenderID:  "u-admin",
		Kind:      MessageKindMessage,
		Content:   large,
		CreatedAt: time.Date(2026, 5, 25, 4, 0, 0, 0, time.UTC),
	}
	line, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("Marshal(legacy) error = %v", err)
	}
	if len(line) <= maxSessionJSONLLineBytes {
		t.Fatalf("legacy line bytes = %d, want > %d for repro", len(line), maxSessionJSONLLineBytes)
	}
	sessionPath := filepath.Join(dir, "sessions", roomID+".jsonl")
	if err := os.WriteFile(sessionPath, append(line, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile(session) error = %v", err)
	}

	stateJSON := `{
  "current_user_id": "u-admin",
  "users": [{"id": "u-admin", "name": "admin", "handle": "admin"}],
  "rooms": [{
    "id": "` + roomID + `",
    "title": "legacy",
    "members": ["u-admin"],
    "messages": "sessions/` + roomID + `.jsonl"
  }]
}`
	if err := os.WriteFile(statePath, []byte(stateJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(state.json) error = %v", err)
	}

	loaded, err := LoadBootstrap(statePath)
	if err != nil {
		t.Fatalf("LoadBootstrap() error = %v", err)
	}
	if len(loaded.Rooms[0].Messages) != 1 || loaded.Rooms[0].Messages[0].Content != large {
		t.Fatalf("loaded message = %+v, want legacy fat content", loaded.Rooms[0].Messages[0])
	}

	if err := SaveBootstrap(statePath, loaded); err != nil {
		t.Fatalf("SaveBootstrap(migrate) error = %v", err)
	}
	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("ReadFile(session after migrate) error = %v", err)
	}
	migratedLine := strings.TrimSpace(strings.Split(string(sessionData), "\n")[0])
	if len(migratedLine) > maxSessionJSONLLineBytes {
		t.Fatalf("migrated jsonl line bytes = %d, want <= %d", len(migratedLine), maxSessionJSONLLineBytes)
	}
	var record sessionMessageLine
	if err := json.Unmarshal([]byte(migratedLine), &record); err != nil {
		t.Fatalf("Unmarshal(migrated line) error = %v", err)
	}
	if record.BlobRef == "" {
		t.Fatal("migrated line missing blob_ref")
	}
}
