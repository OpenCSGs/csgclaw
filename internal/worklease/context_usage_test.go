package worklease

import (
	"context"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/modelcap"
	"testing"
	"time"
)

func TestWorkStatusPreservesUsageAcrossToolUpdates(t *testing.T) {
	now := time.Now()
	r, _ := newTestRegistry(t, &now)
	lease := testLease(NewID())
	if _, err := r.StartOrRenew(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	used := int64(9000)
	u := &modelcap.ContextUsage{SessionID: "s", ModelID: "m", ContextWindow: 32768, UsedTokens: &used, ContextSource: "default", CompactThreshold: 24576}
	request := apitypes.ParticipantWorkStatusPatchRequest{Sequence: 1, Phase: "working", ContextUsage: u}
	first, ok, err := r.UpdateStatus(context.Background(), "worker", lease.LeaseID, request)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	*first.Status.ContextUsage.UsedTokens = 123
	second, ok, err := r.UpdateStatus(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{Sequence: 2, Phase: "working"})
	if err != nil || !ok || *second.Status.ContextUsage.UsedTokens != 9000 {
		t.Fatalf("usage lost: %+v %v", second.Status, err)
	}
	request.Sequence = 1
	*u.UsedTokens = 0
	if _, accepted, err := r.UpdateStatus(context.Background(), "worker", lease.LeaseID, request); err != nil || accepted {
		t.Fatal("accepted stale status")
	}
	active := r.ActiveWork(lease.RoomID)
	if len(active) != 1 || *active[0].Status.ContextUsage.UsedTokens != 9000 {
		t.Fatal("snapshot lost usage")
	}
}

func TestClearRoomOnlyAcceptsFreshContextReports(t *testing.T) {
	now := time.Now()
	registry, _ := newTestRegistry(t, &now)
	lease := testLease(NewID())
	if _, err := registry.StartOrRenew(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	used := int64(9000)
	usage := &modelcap.ContextUsage{SessionID: "s", ModelID: "m", UsedTokens: &used, ContextWindow: 32768, ContextSource: "default", CompactThreshold: 24576}
	initial, _, err := registry.UpdateStatus(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{Sequence: 1, Phase: "working", ContextUsage: usage})
	if err != nil {
		t.Fatal(err)
	}
	chat := registry.im.(*im.Service)
	cleared, err := chat.ClearRoomMessages(lease.RoomID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.ContextUsageEpoch == initial.Status.ContextUsageEpoch {
		t.Fatal("clear did not advance the epoch")
	}
	stale, _, err := registry.UpdateStatus(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{Sequence: 2, Phase: "working"})
	if err != nil {
		t.Fatal(err)
	}
	if stale.Status.ContextUsageEpoch != initial.Status.ContextUsageEpoch {
		t.Fatal("status-only update promoted old usage")
	}
	room, _ := chat.Room(lease.RoomID)
	if len(room.ContextUsage) != 0 {
		t.Fatal("status-only update restored usage")
	}
	fresh, _, err := registry.UpdateStatus(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{Sequence: 3, Phase: "working", ContextUsage: usage})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status.ContextUsageEpoch != cleared.ContextUsageEpoch {
		t.Fatal("fresh report did not receive current epoch")
	}
	if err := registry.Finish(context.Background(), "worker", lease.LeaseID, apitypes.ParticipantWorkOutcomeReleased); err != nil {
		t.Fatal(err)
	}
}
