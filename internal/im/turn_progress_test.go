package im

import (
	"path/filepath"
	"testing"
)

func TestTransientTurnProgressIsReadableButNotPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	bootstrap := Bootstrap{CurrentUserID: "agent", Users: []User{{ID: "agent", Name: "Agent"}}, Rooms: []Room{{ID: "room", Title: "Room", Members: []string{"agent"}}}}
	if err := SaveBootstrap(path, bootstrap); err != nil {
		t.Fatal(err)
	}
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"csgclaw": map[string]any{"delivery_kind": "activity", "turn_progress": map[string]any{"id": "turn", "revision": float64(1), "status": "running", "started_at": "2026-09-26T00:00:00Z", "updated_at": "2026-09-26T00:00:02Z", "items": []any{}}}}
	req := DeliverMessageRequest{RoomID: "room", SenderID: "agent", MessageID: "turn-final", Content: "partial", Metadata: metadata, Transient: true}
	if _, err := svc.DeliverMessage(req); err != nil {
		t.Fatal(err)
	}
	room, _ := svc.Room("room")
	if len(room.Messages) != 1 {
		t.Fatal("active snapshot not readable")
	}
	saved, err := LoadBootstrap(path)
	if err != nil || len(saved.Rooms[0].Messages) != 0 {
		t.Fatalf("transient update persisted: %v %+v", err, saved)
	}
	req.Transient = false
	if _, err := svc.DeliverMessage(req); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	restartedRoom, _ := restarted.Room("room")
	p := messageTurnProgress(restartedRoom.Messages[0])
	if p["status"] != "interrupted" || p["ended_at"] != p["updated_at"] {
		t.Fatalf("restart keeps stale running state: %+v", p)
	}
	req.Metadata = nil
	req.Transient = true
	if _, err := svc.DeliverMessage(req); err == nil {
		t.Fatal("ordinary message accepted as transient")
	}
}

func TestReloadPreservesLiveTurnProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	bootstrap := Bootstrap{CurrentUserID: "agent", Users: []User{{ID: "agent", Name: "Agent"}}, Rooms: []Room{{ID: "room", Title: "Room", Members: []string{"agent"}}}}
	if err := SaveBootstrap(path, bootstrap); err != nil {
		t.Fatal(err)
	}
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	deliver := func(revision float64, transient bool) {
		t.Helper()
		metadata := map[string]any{"csgclaw": map[string]any{"delivery_kind": "activity", "turn_progress": map[string]any{"id": "turn", "revision": revision, "status": "running", "items": []any{}}}}
		if _, err := svc.DeliverMessage(DeliverMessageRequest{RoomID: "room", SenderID: "agent", MessageID: "turn-final", Content: "latest draft", Metadata: metadata, Transient: transient}); err != nil {
			t.Fatal(err)
		}
	}
	check := func(status string, revision float64) {
		t.Helper()
		if err := svc.Reload(); err != nil {
			t.Fatal(err)
		}
		room, _ := svc.Room("room")
		if len(room.Messages) != 1 {
			t.Fatalf("lost progress: %+v", room.Messages)
		}
		p := messageTurnProgress(room.Messages[0])
		if p["status"] != status || p["revision"] != revision {
			t.Fatalf("reload changed progress: %+v", p)
		}
	}
	// An initial transient event must survive before its first checkpoint.
	deliver(1, true)
	check("running", 1)
	deliver(2, false)
	deliver(3, true)
	check("running", 3)
	check("running", 3)
	// External state updates still load, and newer terminal progress wins.
	saved, err := LoadBootstrap(path)
	if err != nil {
		t.Fatal(err)
	}
	saved.Rooms[0].Title = "Externally renamed"
	p := messageTurnProgress(saved.Rooms[0].Messages[0])
	p["status"], p["revision"] = "succeeded", float64(4)
	if err := SaveBootstrap(path, saved); err != nil {
		t.Fatal(err)
	}
	check("succeeded", 4)
	room, _ := svc.Room("room")
	if room.Title != "Externally renamed" {
		t.Fatal("reload lost external room changes")
	}
	// Restart interrupts an unfinished persisted turn and keeps it interrupted
	// on subsequent reads in the new process.
	deliver(5, false)
	svc, err = NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	check("interrupted", 6)
}
