package retiredclaws_test

import (
	"csgclaw/internal/im"
	"csgclaw/internal/migrations/retiredclaws"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoricalRuntimeMetadataMigratesInlineAndBlobActivity(t *testing.T) {
	for _, key := range []string{"openclaw", "picoclaw"} {
		for _, blob := range []bool{false, true} {
			root := t.TempDir()
			path := filepath.Join(root, "state.json")
			state := im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Name: "Admin"}, {ID: "agent-old", Name: "Old", Role: "worker"}}, Rooms: []im.Room{{ID: "room-old", Members: []string{im.AdminUserID, "agent-old"}}}}
			if err := im.SaveBootstrap(path, state); err != nil {
				t.Fatal(err)
			}
			metadata := map[string]any{key: map[string]any{"delivery_kind": "tool", "tool_call_id": "tool-one", "request_id": "source-one"}, "custom": "preserved"}
			line := map[string]any{"id": "message-one", "sender_id": "agent-old", "content": "tool result", "created_at": time.Now().UTC().Format(time.RFC3339Nano), "metadata": metadata}
			if blob {
				line["blob_ref"] = "blobs/old.json"
				delete(line, "metadata")
				if err := os.MkdirAll(filepath.Join(root, "sessions", "blobs"), 0700); err != nil {
					t.Fatal(err)
				}
				data, _ := json.Marshal(map[string]any{"content": "tool result", "metadata": metadata})
				if err := os.WriteFile(filepath.Join(root, "sessions", "blobs", "old.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			data, _ := json.Marshal(line)
			if err := os.WriteFile(filepath.Join(root, "sessions", "room-old.jsonl"), append(data, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := im.LoadBootstrap(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded.Rooms) != 1 || len(loaded.Rooms[0].Messages) != 1 {
				t.Fatalf("historical messages not loaded: %+v", loaded.Rooms)
			}
			message := loaded.Rooms[0].Messages[0]
			if !im.IsAgentActivityMessage(message) {
				t.Fatalf("%s blob=%t became a conversational wake-up", key, blob)
			}
			if _, ok := message.Metadata[key]; ok {
				t.Fatal("old envelope retained")
			}
			current, _ := message.Metadata["codex"].(map[string]any)
			if current["request_id"] != "source-one" || current["tool_call_id"] != "tool-one" || message.Metadata["custom"] != "preserved" {
				t.Fatal("activity context lost")
			}
		}
	}
}

func TestHistoricalRuntimeMetadataPreservesCurrentFieldsAndSourceSnapshot(t *testing.T) {
	source := map[string]any{"codex": map[string]any{"request_id": "current"}, "openclaw": map[string]any{"request_id": "old", "tool_call_id": "tool"}}
	result := retiredclaws.MessageMetadata(source)
	current := result["codex"].(map[string]any)
	if current["request_id"] != "current" || current["tool_call_id"] != "tool" {
		t.Fatalf("metadata merge: %v", result)
	}
	if _, ok := source["openclaw"]; !ok {
		t.Fatal("source snapshot mutated")
	}
	if _, ok := source["codex"].(map[string]any)["tool_call_id"]; ok {
		t.Fatal("source envelope mutated")
	}
}
