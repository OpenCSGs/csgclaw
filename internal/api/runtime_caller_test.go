package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
)

func TestRuntimeCallerIsNotGloballyScopedToAnActiveRoom(t *testing.T) {
	h := &Handler{im: im.NewService()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/rooms", nil)
	req.Header.Set("X-CSGClaw-Caller-Agent", "agent-dev")
	out := httptest.NewRecorder()
	h.Routes().ServeHTTP(out, req)
	if out.Code != http.StatusOK {
		t.Fatalf("global room list = %d %s", out.Code, out.Body.String())
	}
}

func TestRuntimeMessageSenderMustMatchCaller(t *testing.T) {
	messages := im.NewService()
	for _, id := range []string{"dev", "qa"} {
		if _, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: id, Name: id, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Work", CreatorID: "admin", MemberIDs: []string{"dev", "qa"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: messages, participantBridge: im.NewParticipantBridge("")}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/messages", strings.NewReader(`{"room_id":"`+room.ID+`","sender_id":"manager","content":"forged"}`))
	req.Header.Set("X-CSGClaw-Caller-Agent", "agent-dev")
	req.Header.Set("Content-Type", "application/json")
	out := httptest.NewRecorder()
	h.Routes().ServeHTTP(out, req)
	if out.Code != http.StatusForbidden || !strings.Contains(out.Body.String(), "sender must match") {
		t.Fatalf("forged sender = %d %s", out.Code, out.Body.String())
	}
}
