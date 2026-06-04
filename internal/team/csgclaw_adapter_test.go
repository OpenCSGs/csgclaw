package team

import (
	"context"
	"strings"
	"testing"

	"csgclaw/internal/im"
)

func TestCSGClawAdapterRejectsNonCanonicalMemberBotID(t *testing.T) {
	imSvc := im.NewService()
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID: "u-p-w-0604", Name: "worker", Handle: "p-w-0604", Role: "worker",
	}); err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}

	adapter := NewCSGClawAdapter(imSvc)
	_, err := adapter.EnsureRoom(context.Background(), EnsureRoomRequest{
		Title:        "team",
		LeadBotID:    "u-manager",
		MemberBotIDs: []string{"p-w-0604"},
	})
	if err == nil || !strings.Contains(err.Error(), "canonical user id") {
		t.Fatalf("EnsureRoom() error = %v, want non-canonical member rejection", err)
	}
}
