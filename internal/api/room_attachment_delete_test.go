package api

import (
	"bytes"
	"context"
	"csgclaw/cli/command"
	roomcmd "csgclaw/cli/room"
	"csgclaw/internal/im"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRoomAttachmentLaterChatDownloadAndDeleteCLI(t *testing.T) {
	root := t.TempDir()
	service, err := im.NewServiceFromPath(filepath.Join(root, "im", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	room, err := service.CreateRoom(im.CreateRoomRequest{Title: "Files", CreatorID: "user-admin"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&Handler{im: service, serverAccessToken: "test-secret"}).Routes())
	defer server.Close()
	body, contentType := multipartMessageBodyForTest(t, map[string]any{"room_id": room.ID, "sender_id": "user-admin", "content": "Save this file"}, "files", "sample.mkv", "video/matroska", []byte("video bytes"))
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/messages", body)
	request.Header.Set("Content-Type", contentType)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var uploaded im.Message
	err = json.NewDecoder(response.Body).Decode(&uploaded)
	response.Body.Close()
	if err != nil || response.StatusCode != 201 || len(uploaded.Attachments) != 1 {
		t.Fatalf("upload: %d %v", response.StatusCode, err)
	}
	if _, err := service.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "user-admin", Content: "Find and delete the earlier video"}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	run := &command.Context{Program: "csgclaw-cli", Stdout: &output, Stderr: io.Discard, HTTPClient: server.Client()}
	globals := command.GlobalOptions{Endpoint: server.URL, Token: "test-secret", Output: "json"}
	local := filepath.Join(root, "saved.mkv")
	for _, args := range [][]string{
		{"attachments", "list", "--room-id", room.ID, "--query", "sample.mkv"},
		{"attachments", "download", "--room-id", room.ID, "--attachment-id", uploaded.Attachments[0].ID, "--output", local},
		{"attachments", "delete", "--room-id", room.ID, "--attachment-id", uploaded.Attachments[0].ID},
	} {
		if err := roomcmd.NewCmd().Run(context.Background(), run, args, globals); err != nil {
			t.Fatal(err)
		}
	}
	request, _ = http.NewRequest(http.MethodGet, server.URL+uploaded.Attachments[0].DownloadURL, nil)
	response, err = server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatalf("deleted original download status=%d", response.StatusCode)
	}
	data, err := os.ReadFile(local)
	if err != nil || string(data) != "video bytes" {
		t.Fatalf("explicit saved copy was changed: %q %v", data, err)
	}
}

func TestRoomAttachmentDeleteRequiresCurrentAgentMembership(t *testing.T) {
	h, alice, _, aliceToken, bobToken := newAppPlatformAuthFixture(t)
	h.serverNoAuth = false
	var err error
	h.im, err = im.NewServiceFromPath(filepath.Join(t.TempDir(), "im", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := h.im.EnsureAgentUser(im.EnsureAgentUserRequest{ID: alice.ID, Name: alice.Name, Role: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Private files", CreatorID: "user-admin", MemberIDs: []string{user.ID}})
	if err != nil {
		t.Fatal(err)
	}
	message, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "user-admin", Attachments: []im.MessageAttachmentUpload{{Name: "private.txt", Data: []byte("private")}}})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/rooms/" + room.ID + "/attachments/" + message.Attachments[0].ID
	for _, token := range []string{"", bobToken} {
		response := appAuthRequest(t, h, http.MethodDelete, path, "", token, nil)
		if response.Code != http.StatusUnauthorized && response.Code != http.StatusForbidden {
			t.Fatalf("unauthorized delete status=%d", response.Code)
		}
	}
	if response := appAuthRequest(t, h, http.MethodDelete, "/api/v1/rooms/"+room.ID+"/attachments", "", aliceToken, nil); response.Code != http.StatusForbidden {
		t.Fatalf("collection delete status=%d", response.Code)
	}
	response := appAuthRequest(t, h, http.MethodDelete, path, "", aliceToken, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("member delete status=%d: %s", response.Code, response.Body.String())
	}
}
