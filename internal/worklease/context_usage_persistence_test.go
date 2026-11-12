package worklease

import (
	"context"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/participant"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestContextUsageAvailableAcrossServiceRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := im.SaveBootstrap(path, im.Bootstrap{CurrentUserID: "user-admin", Users: []im.User{{ID: "user-admin"}, {ID: "user-worker"}}, Rooms: []im.Room{{ID: "room-1", Members: []string{"user-admin", "user-worker"}}}}); err != nil {
		t.Fatal(err)
	}
	chat, err := im.NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	directory := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{ID: "pt-worker", Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent, ChannelUserRef: "user-worker", LifecycleStatus: participant.LifecycleStatusActive}}))
	registry := NewRegistry(directory, chat, NewBus())
	lease := testLease(NewID())
	if _, err := registry.StartOrRenew(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	used := int64(9000)
	usage := &modelcap.ContextUsage{SessionID: "s", ModelID: "m", UsedTokens: &used, ContextWindow: 32768, ContextSource: "default", CompactThreshold: 24576, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if _, ok, err := registry.UpdateStatus(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{Sequence: 1, Phase: "working", ContextUsage: usage}); err != nil || !ok {
		t.Fatalf("update: %v %v", ok, err)
	}
	if err := registry.Finish(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkOutcomeReleased); err != nil {
		t.Fatal(err)
	}
	// A separate thread has its own context and must not replace the main room.
	thread := testLease(NewID())
	thread.ThreadRootID = "thread-root"
	thread.RequestID = "thread-request"
	if _, err := registry.StartOrRenew(context.Background(), thread); err != nil {
		t.Fatal(err)
	}
	threadUsed := int64(30000)
	threadUsage := *usage
	threadUsage.SessionID, threadUsage.UsedTokens = "thread-session", &threadUsed
	if _, accepted, err := registry.UpdateStatus(context.Background(), "worker", thread.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{Sequence: 1, Phase: "working", ContextUsage: &threadUsage}); err != nil || !accepted {
		t.Fatalf("thread status: %v %v", accepted, err)
	}
	if err := chat.Reload(); err != nil {
		t.Fatal(err)
	}
	room, _ := chat.Room("room-1")
	raw, _ := json.Marshal(room)
	var result struct {
		Usage map[string]modelcap.ContextUsage `json:"context_usage"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	snapshot, ok := result.Usage["user-worker"]
	if !ok || snapshot.UsedTokens == nil || *snapshot.UsedTokens != 9000 {
		t.Fatalf("idle/reloaded room lost context usage: %s", raw)
	}
	restarted, err := im.NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	room, _ = restarted.Room("room-1")
	if restored, ok := room.ContextUsage["user-worker"]; !ok || restored.UsedTokens == nil || *restored.UsedTokens != 9000 {
		t.Fatalf("service restart lost usage: %+v", room.ContextUsage)
	}

}
