package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/taskcore"
	"csgclaw/internal/worklease"
)

type scopeLeaseReader struct{ rooms []string }

func (s *scopeLeaseReader) ActiveRooms(string) []string { return s.rooms }
func (*scopeLeaseReader) StartOrRenew(context.Context, worklease.ParticipantWorkLease) (apitypes.ParticipantWorkUpdate, error) {
	return apitypes.ParticipantWorkUpdate{}, nil
}
func (*scopeLeaseReader) Stop(context.Context, string, string) error { return nil }

func TestRoomRuntimeRejectsLegacyAndExternalRoutes(t *testing.T) {
	messages := im.NewService()
	for _, id := range []string{"dev", "outsider"} {
		if _, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: id, Name: id, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Work", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	leases := &scopeLeaseReader{rooms: []string{room.ID}}
	h := &Handler{im: messages, participantWork: leases}
	h.SetRoomTaskCore(taskcore.NewService())
	routes := h.Routes()
	request := func(method, path, caller string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		req.Header.Set("X-CSGClaw-Caller-Agent", caller)
		out := httptest.NewRecorder()
		routes.ServeHTTP(out, req)
		return out
	}
	for _, path := range []string{"/api/v1/agent-tasks", "/api/v1/teams", "/api/v1/rooms/elsewhere/tasks", "/api/v1/channels/csgclaw/rooms"} {
		if got := request(http.MethodPost, path, "agent-manager"); got.Code != http.StatusForbidden {
			t.Fatalf("legacy/external route %s: %d %s", path, got.Code, got.Body.String())
		}
	}
	for _, path := range []string{"/api/v1/agents", "/api/v1/channels/csgclaw/participants", "/api/v1/tasks", "/api/v1/channels/csgclaw/messages?room_id=elsewhere"} {
		if got := request(http.MethodGet, path, "agent-manager"); got.Code != http.StatusForbidden {
			t.Fatalf("global discovery %s: %d", path, got.Code)
		}
	}
	got := request(http.MethodGet, "/api/v1/rooms/"+room.ID+"/task-context", "agent-manager")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "dev") || strings.Contains(got.Body.String(), "outsider") {
		t.Fatalf("room roster: %d %s", got.Code, got.Body.String())
	}
	got = request(http.MethodPost, "/api/v1/rooms/"+room.ID+"/tasks", "agent-dev")
	if got.Code != http.StatusForbidden {
		t.Fatalf("worker created root: %d", got.Code)
	}
	leases.rooms = []string{room.ID, "another-active-room"}
	if got := request(http.MethodGet, "/api/v1/rooms/"+room.ID+"/tasks", "agent-manager"); got.Code != http.StatusOK {
		t.Fatalf("explicit active task scope: %d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/api/v1/agents", "agent-manager"); got.Code != http.StatusConflict {
		t.Fatalf("ambiguous scope not rejected: %d", got.Code)
	}
	leases.rooms = nil
	if got := request(http.MethodGet, "/api/v1/rooms/"+room.ID+"/tasks", "agent-manager"); got.Code != http.StatusOK {
		t.Fatalf("non-IM runtime workflow unexpectedly blocked: %d", got.Code)
	}
	// The browser and a human CLI do not inherit runtime restrictions.
	if got := request(http.MethodGet, "/api/v1/rooms/"+room.ID+"/tasks", ""); got.Code != http.StatusOK {
		t.Fatalf("human request blocked: %d", got.Code)
	}
}

func TestRoomRuntimeDoesNotRestrictFreeRoomWork(t *testing.T) {
	messages := im.NewService()
	if _, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "dev", Name: "dev", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	first, err := messages.CreateRoom(im.CreateRoomRequest{Title: "First", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeFree})
	if err != nil {
		t.Fatal(err)
	}
	second, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Second", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeFree})
	if err != nil {
		t.Fatal(err)
	}
	leases := &scopeLeaseReader{rooms: []string{first.ID, second.ID}}
	h := &Handler{im: messages, participantWork: leases}
	h.SetRoomTaskCore(taskcore.NewService())
	request := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-CSGClaw-Caller-Agent", "agent-dev")
		out := httptest.NewRecorder()
		h.Routes().ServeHTTP(out, req)
		return out
	}
	for _, path := range []string{
		"/api/v1/channels/csgclaw/rooms",
		"/api/v1/channels/csgclaw/messages?room_id=" + first.ID,
		"/api/v1/channels/csgclaw/rooms/" + second.ID + "/members",
	} {
		if got := request(path); got.Code == http.StatusConflict || got.Code == http.StatusForbidden {
			t.Fatalf("free-room request %s was scoped: %d %s", path, got.Code, got.Body.String())
		}
	}
	onDemand, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Managed", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	leases.rooms = []string{first.ID, onDemand.ID}
	if got := request("/api/v1/channels/csgclaw/messages?room_id=" + first.ID); got.Code == http.StatusConflict || got.Code == http.StatusForbidden {
		t.Fatalf("explicit free-room request inherited on-demand scope: %d %s", got.Code, got.Body.String())
	}
}

func TestRoomRuntimeOrdinaryWorkerMentionGoesThroughManager(t *testing.T) {
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
	bridge := im.NewParticipantBridge("")
	h := &Handler{im: messages, participantBridge: bridge, participantWork: &scopeLeaseReader{rooms: []string{room.ID}}}
	h.SetRoomTaskCore(taskcore.NewService())
	manager, closeManager := bridge.Subscribe("manager")
	defer closeManager()
	qa, closeQA := bridge.Subscribe("pt-qa")
	defer closeQA()
	call := func(roomID, sender string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/messages", strings.NewReader(`{"room_id":"`+roomID+`","sender_id":"`+sender+`","mention_id":"qa","content":"Please inspect this"}`))
		req.Header.Set("X-CSGClaw-Caller-Agent", "agent-dev")
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		h.Routes().ServeHTTP(out, req)
		return out
	}
	got := call(room.ID, "dev")
	if got.Code != http.StatusCreated {
		t.Fatal(got.Code, got.Body.String())
	}
	stored, _ := messages.ListMessages(room.ID)
	message := stored[len(stored)-1]
	sender, _ := messages.User(message.SenderID)
	evt := im.Event{Type: im.EventTypeMessageCreated, RoomID: room.ID, Message: &message, Sender: &sender}
	h.PublishParticipantEvent(evt)
	select {
	case e := <-manager:
		if e.SenderID != message.SenderID || !e.RoomManager || len(e.Mentions) != 1 {
			t.Fatalf("lost original mention: %+v", e)
		}
	default:
		t.Fatal("ordinary worker mention did not reach manager")
	}
	select {
	case <-qa:
		t.Fatal("worker directly woke another worker")
	default:
	}
	h.PublishParticipantEvent(evt)
	select {
	case <-manager:
		t.Fatal("replay woke manager twice")
	default:
	}
	if got := call(room.ID, "manager"); got.Code != http.StatusForbidden {
		t.Fatal("forged sender", got.Code)
	}
	if got := call("other-room", "dev"); got.Code != http.StatusForbidden {
		t.Fatal("cross-room message", got.Code)
	}
}
