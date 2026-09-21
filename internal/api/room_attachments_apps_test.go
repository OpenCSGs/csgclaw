package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	csgchannel "csgclaw/internal/channel/csgclaw"
	"csgclaw/internal/im"
)

func TestAppScopedAgentCanDiscoverAndDownloadRoomAttachments(t *testing.T) {
	h, alice, _, aliceToken, bobToken := newAppPlatformAuthFixture(t)
	var err error
	h.im, err = im.NewServiceFromPath(filepath.Join(t.TempDir(), "im", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	h.csgclaw = csgchannel.NewService(h.im)
	h.participantBridge = im.NewParticipantBridge("test-admin-secret")
	if _, _, err = h.im.EnsureAgentUser(im.EnsureAgentUserRequest{ID: alice.ID, Name: alice.Name, Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Scoped attachment", CreatorID: "admin", MemberIDs: []string{alice.ID}})
	if err != nil {
		t.Fatal(err)
	}
	body, contentType := multipartMessageBodyForTest(t, map[string]any{"room_id": room.ID, "sender_id": "admin", "content": "Read attached file"}, "files", "sample.txt", "text/plain", []byte("SCOPED-ATTACHMENT-583"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	var msg im.Message
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil || rec.Code != 201 || len(msg.Attachments) != 1 {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	list := "/api/v1/rooms/" + room.ID + "/attachments"
	download := list + "/" + msg.Attachments[0].ID
	for _, path := range []string{list, download} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+aliceToken)
		req.Header.Set("X-CSGClaw-Caller-Agent", "admin") // Must be replaced by the verified Agent identity.
		rec = httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("member %s: %d %s", path, rec.Code, rec.Body)
		}
		if path == download && rec.Body.String() != "SCOPED-ATTACHMENT-583" {
			t.Fatal("download body changed")
		}
		denied := appAuthRequest(t, h, http.MethodGet, path, "", bobToken, nil)
		if denied.Code != 403 {
			t.Fatalf("non-member attachment access: %d", denied.Code)
		}
	}
	if _, err = h.im.RemoveRoomMembers(im.AddRoomMembersRequest{RoomID: room.ID, InviterID: "admin", UserIDs: []string{alice.ID}}); err != nil {
		t.Fatal(err)
	}
	if rec = appAuthRequest(t, h, http.MethodGet, download, "", aliceToken, nil); rec.Code != 403 {
		t.Fatalf("removed member download: %d", rec.Code)
	}
}
