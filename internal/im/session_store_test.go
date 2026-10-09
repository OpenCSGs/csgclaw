package im

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSaveLargeMetadataSpillsToBlob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions", "room.jsonl")
	message := Message{
		ID: "turn-final", SenderID: "agent", Content: "Short answer", CreatedAt: time.Now().UTC(),
		Metadata: map[string]any{"csgclaw": map[string]any{
			"delivery_kind": "final",
			"turn_progress": map[string]any{"items": []any{
				map[string]any{"kind": "tool", "output": strings.Repeat("工具输出\n", 12000)},
			}},
		}},
	}
	if err := saveMessagesJSONL(path, "room", []Message{message}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(bytes.TrimSpace(data)) > maxSessionJSONLLineBytes {
		t.Fatalf("session line exceeds budget: %d bytes", len(data))
	}
	loaded, err := loadMessagesJSONL(path, "room")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Content != message.Content || !reflect.DeepEqual(loaded[0].Metadata, message.Metadata) {
		t.Fatal("large message metadata did not survive save and reload")
	}
}

func TestLoadLegacyBlobKeepsInlineMetadata(t *testing.T) {
	dir := t.TempDir()
	metadata := map[string]any{"csgclaw": map[string]any{"delivery_kind": "final"}}
	line := messageToSessionLine(Message{ID: "legacy", SenderID: "agent", CreatedAt: time.Now().UTC(), Metadata: metadata})
	line.BlobRef = "blobs/room/legacy.json"
	path := filepath.Join(dir, filepath.FromSlash(line.BlobRef))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"content":"Legacy response"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	message, err := decodeSessionMessageLine(dir, data)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "Legacy response" || !reflect.DeepEqual(message.Metadata, metadata) {
		t.Fatal("legacy blob lost its inline metadata")
	}
}

func TestLegacyOversizedMetadataMigratesOnStartup(t *testing.T) {
	for _, format := range []string{"inline", "legacy_blob"} {
		t.Run(format, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			state := Bootstrap{
				CurrentUserID: AdminUserID,
				Users:         []User{{ID: AdminUserID, Name: "Admin"}, {ID: ManagerUserID, Name: "Manager"}},
				Rooms: []Room{
					{ID: "old-room", Title: "Historical room", Members: []string{AdminUserID, ManagerUserID}},
					{ID: "other-room", Title: "Other room", Members: []string{AdminUserID, ManagerUserID}},
				},
			}
			if err := SaveBootstrap(path, state); err != nil {
				t.Fatal(err)
			}
			original := Message{
				ID: "historical-final", SenderID: ManagerUserID, Content: "Historical answer", CreatedAt: time.Now().UTC(),
				Metadata: map[string]any{"csgclaw": map[string]any{
					"delivery_kind": "final",
					"turn_progress": map[string]any{"status": "succeeded", "items": []any{
						map[string]any{"kind": "reasoning", "text": strings.Repeat("R", 70*1024)},
					}},
				}},
			}
			// Write an old-format row directly: the new writer would already spill it.
			row := messageToSessionLine(original)
			if format == "legacy_blob" {
				row.Content = ""
				row.BlobRef = "blobs/old-room/historical-final.json"
				blobPath := filepath.Join(filepath.Dir(path), "sessions", filepath.FromSlash(row.BlobRef))
				if err := os.MkdirAll(filepath.Dir(blobPath), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(blobPath, []byte(`{"content":"Historical answer"}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			data, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			if len(data) <= maxSessionJSONLLineBytes {
				t.Fatal("fixture must exceed the old write limit")
			}
			sessionPath := filepath.Join(filepath.Dir(path), "sessions", "old-room.jsonl")
			if err := os.WriteFile(sessionPath, append(data, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
			// Inspection remains read-only. Only service startup migrates history.
			if _, err := LoadBootstrap(path); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(sessionPath)
			if err != nil || !bytes.Equal(before, append(data, '\n')) {
				t.Fatalf("read-only load changed history: %v", err)
			}
			otherPath := filepath.Join(filepath.Dir(path), "sessions", "other-room.jsonl")
			otherInfo, err := os.Stat(otherPath)
			if err != nil {
				t.Fatal(err)
			}
			service, err := NewServiceFromPath(path)
			if err != nil {
				t.Fatalf("load historical oversized metadata: %v", err)
			}
			data, err = os.ReadFile(sessionPath)
			if err != nil || len(bytes.TrimSpace(data)) > maxSessionJSONLLineBytes {
				t.Fatalf("historical record was not migrated: bytes=%d, error=%v", len(data), err)
			}
			loaded, err := loadMessagesJSONL(sessionPath, "old-room")
			if err != nil || len(loaded) != 1 || !reflect.DeepEqual(loaded[0], original) {
				t.Fatalf("migration changed historical message content or metadata: %v", err)
			}
			assertSessionFileUnchanged(t, otherPath, otherInfo)
			migratedInfo, err := os.Stat(sessionPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := NewServiceFromPath(path); err != nil {
				t.Fatal(err)
			}
			assertSessionFileUnchanged(t, sessionPath, migratedInfo)
			if _, err := service.CreateMessage(CreateMessageRequest{RoomID: "other-room", SenderID: AdminUserID, Content: "Hello"}); err != nil {
				t.Fatalf("historical message blocks another room: %v", err)
			}
			if _, err := service.CreateRoom(CreateRoomRequest{Type: "on_demand", Title: "New room", CreatorID: AdminUserID, ManagerID: ManagerUserID}); err != nil {
				t.Fatalf("historical message blocks room creation: %v", err)
			}
			assertSessionFileUnchanged(t, sessionPath, migratedInfo)
		})
	}
}

func assertSessionFileUnchanged(t *testing.T, path string, before os.FileInfo) {
	t.Helper()
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("unchanged file was rewritten: %s", path)
	}
}

func TestStartupMigrationFailurePreservesHistoryAndCanRetry(t *testing.T) {
	for _, failure := range []string{"decode_later_room", "write_blob"} {
		t.Run(failure, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			state := Bootstrap{Rooms: []Room{{ID: "old-room"}, {ID: "later-room"}}}
			if err := SaveBootstrap(path, state); err != nil {
				t.Fatal(err)
			}
			message := Message{ID: "legacy", SenderID: "agent", CreatedAt: time.Now().UTC(), Metadata: map[string]any{"large": strings.Repeat("X", 70*1024)}}
			row, err := json.Marshal(messageToSessionLine(message))
			if err != nil {
				t.Fatal(err)
			}
			row = append(row, '\n')
			sessionPath := filepath.Join(filepath.Dir(path), "sessions", "old-room.jsonl")
			if err := os.WriteFile(sessionPath, row, 0o600); err != nil {
				t.Fatal(err)
			}
			blockedPath := filepath.Join(filepath.Dir(sessionPath), "blobs")
			if failure == "decode_later_room" {
				blockedPath = filepath.Join(filepath.Dir(sessionPath), "later-room.jsonl")
			}
			if err := os.WriteFile(blockedPath, []byte("invalid"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewServiceFromPath(path); err == nil {
				t.Fatal("expected migration or loading error")
			}
			after, err := os.ReadFile(sessionPath)
			if err != nil || !bytes.Equal(after, row) {
				t.Fatalf("startup failure changed original history: %v", err)
			}
			if err := os.Remove(blockedPath); err != nil {
				t.Fatal(err)
			}
			if _, err := NewServiceFromPath(path); err != nil {
				t.Fatalf("startup migration cannot retry: %v", err)
			}
			loaded, err := loadMessagesJSONL(sessionPath, "old-room")
			if err != nil || len(loaded) != 1 || !reflect.DeepEqual(loaded[0], message) {
				t.Fatalf("retry lost history: %v", err)
			}
		})
	}
}

func TestSessionSaveReusesUnchangedFilesAndRepairsMissingBlob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions", "room.jsonl")
	messages := []Message{{ID: "large", SenderID: "agent", Content: strings.Repeat("A", 70*1024), CreatedAt: time.Now().UTC()}}
	if err := saveMessagesJSONL(path, "room", messages); err != nil {
		t.Fatal(err)
	}
	blobDir := filepath.Join(filepath.Dir(path), sessionBlobsDirName, "room")
	entries, err := os.ReadDir(blobDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one blob: %v", err)
	}
	blobPath := filepath.Join(blobDir, entries[0].Name())
	sessionInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	blobInfo, err := os.Stat(blobPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveMessagesJSONL(path, "room", messages); err != nil {
		t.Fatal(err)
	}
	assertSessionFileUnchanged(t, path, sessionInfo)
	assertSessionFileUnchanged(t, blobPath, blobInfo)
	if err := os.Remove(blobPath); err != nil {
		t.Fatal(err)
	}
	if err := saveMessagesJSONL(path, "room", messages); err != nil {
		t.Fatal(err)
	}
	assertSessionFileUnchanged(t, path, sessionInfo)
	loaded, err := loadMessagesJSONL(path, "room")
	if err != nil || !reflect.DeepEqual(loaded, messages) {
		t.Fatalf("missing blob was not restored: %v", err)
	}
}

func TestSessionStreamingUpdates(t *testing.T) {
	original := make([]Message, 100)
	for i := range original {
		original[i] = Message{ID: fmt.Sprintf("msg-%d", i), SenderID: "agent", Content: strings.Repeat("A", 1024), CreatedAt: time.Now().UTC()}
	}
	for _, change := range []string{"first", "middle", "last", "append", "shorten", "empty", "unchanged", "late_error"} {
		t.Run(change, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "room.jsonl")
			if err := saveMessagesJSONL(path, "room", original); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			updated := append([]Message(nil), original...)
			switch change {
			case "first":
				updated[0].Content = "First edit"
			case "middle":
				updated[50].Content = "Middle edit"
			case "last":
				updated[99].Content = "Last edit"
			case "append":
				updated = append(updated, Message{ID: "new", SenderID: "agent", Content: "New reply", CreatedAt: time.Now().UTC()})
			case "shorten":
				updated = updated[:60]
			case "empty":
				updated = nil
			case "late_error":
				updated[0].Content = "Start writing before the last row fails"
				updated[99].Metadata = map[string]any{"invalid": make(chan struct{})}
			}
			err = saveMessagesJSONL(path, "room", updated)
			if change == "late_error" {
				if err == nil {
					t.Fatal("expected encoding error after temporary file creation")
				}
				assertSessionFileUnchanged(t, path, before)
				updated = original
			} else if err != nil {
				t.Fatal(err)
			}
			loaded, err := loadMessagesJSONL(path, "room")
			if err != nil || (len(updated) != 0 && !reflect.DeepEqual(loaded, updated)) || len(loaded) != len(updated) {
				t.Fatalf("streaming update changed messages: %v", err)
			}
			if change == "unchanged" {
				assertSessionFileUnchanged(t, path, before)
			}
			temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".room.jsonl.tmp-*"))
			if err != nil || len(temps) != 0 {
				t.Fatalf("temporary files remain: %v %v", temps, err)
			}
		})
	}
}

func TestFailedSessionSavePreservesCommittedMessagesAndBlobs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions", "room.jsonl")
	original := []Message{
		{ID: "large", SenderID: "agent", Content: strings.Repeat("A", 70*1024), CreatedAt: time.Now().UTC()},
		{ID: "later", SenderID: "agent", Content: "Later message", CreatedAt: time.Now().UTC()},
	}
	if err := saveMessagesJSONL(path, "room", original); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := append([]Message(nil), original...)
	updated[0].Content = strings.Repeat("B", 70*1024)
	updated[1].Metadata = map[string]any{"invalid": make(chan struct{})}
	if err := saveMessagesJSONL(path, "room", updated); err == nil {
		t.Fatal("expected message encoding failure")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed save changed the committed session: %v", err)
	}
	loaded, err := loadMessagesJSONL(path, "room")
	if err != nil || !reflect.DeepEqual(loaded, original) {
		t.Fatalf("failed save changed committed messages or blobs: %v", err)
	}
	// A later successful write also collects blobs left by the failed attempt.
	if err := saveMessagesJSONL(path, "room", original); err != nil {
		t.Fatal(err)
	}
	blobs, err := os.ReadDir(filepath.Join(filepath.Dir(path), sessionBlobsDirName, "room"))
	if err != nil || len(blobs) != 1 {
		t.Fatalf("unreferenced blobs remain: count=%d, error=%v", len(blobs), err)
	}
}

func TestSaveLargeMessageSpillsToBlob(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	roomID := "room-large"
	large := strings.Repeat("Z", 70*1024)

	state := Bootstrap{
		CurrentUserID: "u-admin",
		Users:         []User{{ID: "u-admin", Name: "admin"}},
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
		Users:         []User{{ID: "u-admin", Name: "admin"}},
		Rooms: []Room{{
			ID:      roomID,
			Title:   "skill",
			Members: []string{"u-admin"},
			Messages: []Message{{
				ID:        "msg-skill-1",
				SenderID:  "u-admin",
				Kind:      MessageKindMessage,
				Content:   "/custom do it",
				Metadata:  map[string]any{"openclaw": map[string]any{"delivery_kind": "final"}},
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
	if !strings.Contains(string(sessionData), `"metadata":{"openclaw":{"delivery_kind":"final"}}`) {
		t.Fatalf("session = %q, want message metadata persisted", string(sessionData))
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
	openclaw, ok := loaded.Rooms[0].Messages[0].Metadata["openclaw"].(map[string]any)
	if !ok || openclaw["delivery_kind"] != "final" {
		t.Fatalf("loaded metadata = %#v, want openclaw final", loaded.Rooms[0].Messages[0].Metadata)
	}
}

func TestSaveEmptyMessagesTruncatesSessionAndRemovesBlobs(t *testing.T) {
	dir := t.TempDir()
	roomID := "room-clear"
	path := filepath.Join(dir, "sessions", roomID+".jsonl")
	large := strings.Repeat("B", 70*1024)

	if err := saveMessagesJSONL(path, roomID, []Message{{
		ID:        "msg-large",
		SenderID:  "u-admin",
		Content:   large,
		CreatedAt: time.Date(2026, 5, 25, 5, 0, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatalf("saveMessagesJSONL(large) error = %v", err)
	}
	blobDir := filepath.Join(dir, "sessions", sessionBlobsDirName, roomID)
	if _, err := os.Stat(blobDir); err != nil {
		t.Fatalf("Stat(blob dir) error = %v", err)
	}

	if err := saveMessagesJSONL(path, roomID, nil); err != nil {
		t.Fatalf("saveMessagesJSONL(empty) error = %v", err)
	}
	sessionData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(session) error = %v", err)
	}
	if len(sessionData) != 0 {
		t.Fatalf("session bytes = %d, want 0", len(sessionData))
	}
	if _, err := os.Stat(blobDir); !os.IsNotExist(err) {
		t.Fatalf("Stat(blob dir) error = %v, want not exist", err)
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
  "users": [{"id": "u-admin", "name": "admin"}],
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
