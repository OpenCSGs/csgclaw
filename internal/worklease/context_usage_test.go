package worklease

import (
	"context"
	"csgclaw/internal/apitypes"
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
