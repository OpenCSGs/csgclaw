package im

import (
	"csgclaw/internal/modelcap"
	"path/filepath"
	"testing"
)

func TestCompactionPersistenceDedupeAndEpoch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveBootstrap(path, Bootstrap{CurrentUserID: "user-admin", Users: []User{{ID: "user-admin"}, {ID: "user-worker"}}, Rooms: []Room{{ID: "r", Members: []string{"user-admin", "user-worker"}}}}); err != nil {
		t.Fatal(err)
	}
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	op := modelcap.CompactionOperation{ID: "request", Targets: []modelcap.CompactionTarget{{AgentID: "worker", UserID: "user-worker", Status: "pending"}}}
	first, created, err := svc.StartContextCompaction("r", op)
	if err != nil || !created {
		t.Fatalf("start %v %v", created, err)
	}
	_, created, err = svc.StartContextCompaction("r", op)
	if err != nil || created {
		t.Fatalf("duplicate %v %v", created, err)
	}
	conflict := op
	conflict.Targets = []modelcap.CompactionTarget{{AgentID: "other"}}
	if _, _, err = svc.StartContextCompaction("r", conflict); err == nil {
		t.Fatal("request ID reused for different targets")
	}
	reloaded, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Reload(); err != nil {
		t.Fatal(err)
	}
	room, _ := reloaded.Room("r")
	saved := MessageCompaction(room.Messages[0])
	if saved == nil || saved.Running || saved.Targets[0].Status != "interrupted" {
		t.Fatalf("restart: %+v", saved)
	}
	first.Targets[0].Status = "completed"
	first.Running = false
	if err = svc.UpdateContextCompaction("r", first); err != nil {
		t.Fatal(err)
	}
	room, _ = svc.Room("r")
	if len(room.Messages) != 1 || !room.Messages[0].MemoryExcluded || room.Messages[0].Kind != MessageKindEvent {
		t.Fatal("control leaked into ordinary chat")
	}
	if _, err = svc.ClearRoomMessages("r"); err != nil {
		t.Fatal(err)
	}
	if err = svc.UpdateContextCompaction("r", first); err == nil {
		t.Fatal("late completion survived clear")
	}
	used := int64(1)
	usage := modelcap.ContextUsage{SessionID: "s", ModelID: "m", UsedTokens: &used, ContextWindow: 100000, ContextSource: "default"}
	if _, err = svc.RecordCompactedContextUsage("r", "user-worker", usage, 0); err == nil {
		t.Fatal("late usage survived clear")
	}
}
func TestCompactionNoOpAndUnknownCountReconciliation(t *testing.T) {
	svc := NewServiceFromBootstrap(Bootstrap{CurrentUserID: "user-admin", Users: []User{{ID: "user-admin"}, {ID: "user-worker"}}, Rooms: []Room{{ID: "r", Members: []string{"user-admin", "user-worker"}}}})
	op := modelcap.CompactionOperation{ID: "small", Targets: []modelcap.CompactionTarget{{AgentID: "worker", UserID: "user-worker", Status: "unnecessary"}}}
	if _, created, err := svc.StartContextCompaction("r", op); err != nil || created {
		t.Fatalf("no-op: %v %v", created, err)
	}
	used := int64(50000)
	usage := modelcap.ContextUsage{SessionID: "s", ModelID: "m", UsedTokens: &used, ContextWindow: 100000, ContextSource: "default"}
	op.ID = "pending"
	op.Targets[0].Status = "pending"
	op.Targets[0].Before = &usage
	op, _, _ = svc.StartContextCompaction("r", op)
	after := usage
	after.UsedTokens = nil
	op.Targets[0].After = &after
	op.Targets[0].Status = "completed"
	op.Running = false
	if err := svc.UpdateContextCompaction("r", op); err != nil {
		t.Fatal(err)
	}
	used = 1000
	if _, err := svc.RecordContextUsage("r", "user-worker", usage); err != nil {
		t.Fatal(err)
	}
	room, _ := svc.Room("r")
	saved := MessageCompaction(room.Messages[0])
	if saved.Targets[0].After.UsedTokens == nil || *saved.Targets[0].After.UsedTokens != 1000 {
		t.Fatalf("missing late count: %+v", saved)
	}
}

func TestCompactionRevisionAdvancesAcrossUpdatesAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveBootstrap(path, Bootstrap{CurrentUserID: "user-admin", Users: []User{{ID: "user-admin"}, {ID: "user-worker"}}, Rooms: []Room{{ID: "r", Members: []string{"user-admin", "user-worker"}}}}); err != nil {
		t.Fatal(err)
	}
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	op, _, err := svc.StartContextCompaction("r", modelcap.CompactionOperation{ID: "versioned", Targets: []modelcap.CompactionTarget{{AgentID: "a", UserID: "user-worker", Status: "pending"}}})
	if err != nil || op.Revision != 1 {
		t.Fatalf("initial revision: %+v %v", op, err)
	}
	for i := 0; i < 2; i++ {
		op.Targets[0].Status = "running"
		if err := svc.UpdateContextCompaction("r", op); err != nil {
			t.Fatal(err)
		}
	}
	room, _ := svc.Room("r")
	if got := MessageCompaction(room.Messages[0]).Revision; got != 3 {
		t.Fatalf("updates reused caller revision: %d", got)
	}
	restarted, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	room, _ = restarted.Room("r")
	saved := MessageCompaction(room.Messages[0])
	if saved.Revision != 4 || saved.Running {
		t.Fatalf("restart did not version interruption: %+v", saved)
	}
}
