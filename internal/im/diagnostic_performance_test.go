package im

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/diagnostics"
)

func TestDiagnosticDeliveryMeasuresLockAndDurablePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "im", "state.json")
	state := Bootstrap{CurrentUserID: "admin", Users: []User{{ID: "admin", Name: "Admin", Role: "admin"}}, Rooms: []Room{{ID: "room", Members: []string{"admin"}}, {ID: "other", Members: []string{"admin"}}}}
	for i := 0; i < 128; i++ {
		state.Rooms[1].Messages = append(state.Rooms[1].Messages, Message{
			ID:        fmt.Sprintf("history-%d", i),
			SenderID:  "admin",
			Content:   "Unrelated history",
			CreatedAt: time.Now().UTC(),
		})
	}
	if err := SaveBootstrap(path, state); err != nil {
		t.Fatal(err)
	}
	service, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	record := service.Diagnostics().Begin("room", "source", "", "agent", "turn")
	record.Running()
	ctx := diagnostics.WithRecord(context.Background(), record)
	request := DeliverMessageRequest{RoomID: "room", SenderID: "admin", MessageID: "reply", Content: "persisted reply"}
	service.mu.Lock()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() { close(started); _, err := service.DeliverMessageContext(ctx, request); done <- err }()
	<-started
	select {
	case err := <-done:
		service.mu.Unlock()
		t.Fatalf("delivery bypassed IM lock: %v", err)
	case <-time.After(15 * time.Millisecond):
	}
	service.mu.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	metrics := record.Snapshot().Performance
	for _, name := range []string{"im.lock_wait", "im.lock_hold", "im.snapshot", "im.serialize", "im.write", "im.fsync", "im.append", "im.file_skipped"} {
		if metrics[name].Count == 0 {
			t.Fatalf("missing actual metric %s: %+v", name, metrics)
		}
	}
	if metrics["im.rooms_saved"].Items != 1 || metrics["im.write"].Bytes == 0 {
		t.Fatalf("wrong persistence counters: %+v", metrics)
	}
	if metrics["im.serialize"].Count > 4 {
		t.Fatalf("delivery serialized unrelated history: %+v", metrics["im.serialize"])
	}
	if _, err := service.DeliverMessageContext(ctx, request); err != nil {
		t.Fatal(err)
	}
	after := record.Snapshot().Performance
	if after["im.fsync"].Count != metrics["im.fsync"].Count {
		t.Fatal("measurement changed unchanged-file write behavior")
	}
	restored, err := LoadBootstrap(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Rooms[0].Messages)+len(restored.Rooms[1].Messages) != 129 {
		t.Fatal("persistence behavior changed")
	}
	var observed map[string]any
	if err := service.Diagnostics().ReadContext(ctx, record.Snapshot().ID, func(entry diagnostics.ContextEntry) error {
		if entry.Kind == "performance.operation" {
			return json.Unmarshal(entry.Data, &observed)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if observed["inclusive"] != true {
		t.Fatal("inclusive metrics were not labeled")
	}
	if err := service.Diagnostics().Flush(ctx); err != nil {
		t.Fatal(err)
	}
}
