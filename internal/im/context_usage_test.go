package im

import (
	"csgclaw/internal/modelcap"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestContextUsagePersistsWithoutRewritingTranscripts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveBootstrap(path, Bootstrap{CurrentUserID: "user-admin", Users: []User{{ID: "user-admin"}, {ID: "user-worker"}}, Rooms: []Room{{ID: "r", Members: []string{"user-admin", "user-worker"}, Messages: []Message{{ID: "m", Content: "Retain transcript", SenderID: "user-admin"}}}}}); err != nil {
		t.Fatal(err)
	}
	service, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(filepath.Dir(path), sessionRelativePath("r"))
	before, err := os.Stat(transcript)
	if err != nil {
		t.Fatal(err)
	}
	used := int64(123)
	usage := modelcap.ContextUsage{SessionID: "s", ModelID: "m", ContextWindow: 32768, ContextSource: "default", UsedTokens: &used, CompactThreshold: 24000, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if _, err := service.RecordContextUsage("r", "user-worker", usage); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(transcript)
	if err != nil || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("snapshot rewrote transcript: %v", err)
	}
	metadata, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordContextUsage("r", "user-worker", usage); err != nil {
		t.Fatal(err)
	}
	unchanged, err := os.Stat(path)
	if err != nil || !os.SameFile(metadata, unchanged) || !metadata.ModTime().Equal(unchanged.ModTime()) {
		t.Fatalf("identical snapshot rewrote metadata: %v", err)
	}
	restarted, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	room, _ := restarted.Room("r")
	snapshot, ok := room.ContextUsage["user-worker"]
	if !ok || snapshot.UsedTokens == nil || *snapshot.UsedTokens != 123 {
		t.Fatalf("restart lost snapshot: %+v", room.ContextUsage)
	}
	*snapshot.UsedTokens = 999
	room, _ = restarted.Room("r")
	if *room.ContextUsage["user-worker"].UsedTokens != 123 {
		t.Fatal("snapshot alias leaked")
	}
	blocker := filepath.Join(filepath.Dir(path), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	service.statePath = filepath.Join(blocker, "state.json")
	changed := int64(456)
	usage.UsedTokens = &changed
	if _, err := service.RecordContextUsage("r", "user-worker", usage); err == nil {
		t.Fatal("storage failure was hidden")
	}
	room, _ = service.Room("r")
	if *room.ContextUsage["user-worker"].UsedTokens != 123 {
		t.Fatal("failed write replaced the durable snapshot")
	}
	service.statePath = path
	if _, err := service.ClearRoomMessages("r"); err != nil {
		t.Fatal(err)
	}
	cleared, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	room, _ = cleared.Room("r")
	if len(room.ContextUsage) != 0 || room.ContextUsageEpoch == 0 {
		t.Fatal("cleared room lost its usage boundary after restart")
	}
}
