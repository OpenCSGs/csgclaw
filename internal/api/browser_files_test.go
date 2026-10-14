package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"csgclaw/internal/im"
)

func TestRoomAttachmentHeadersCannotAuthorizeReads(t *testing.T) {
	h, roomID, _, _, _ := referenceTestHandler(t)
	h.serverAccessToken = "test-admin-credential"
	message, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: roomID, SenderID: "admin", Attachments: []im.MessageAttachmentUpload{{Name: "private.txt", Data: []byte("fixture-private-content")}}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h.Routes())
	defer server.Close()
	for _, suffix := range []string{"", "/" + message.Attachments[0].ID} {
		for _, mode := range []string{"cors", "no-cors", "navigate"} {
			req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/rooms/"+roomID+"/attachments"+suffix, nil)
			req.Header.Set("Sec-Fetch-Site", "same-origin")
			req.Header.Set("Sec-Fetch-Mode", mode)
			req.Header.Set("Sec-Fetch-User", "?1")
			req.Header.Set("Sec-Fetch-Dest", "empty")
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusUnauthorized {
				t.Fatalf("forged %s headers authorized attachment read: %d", mode, response.StatusCode)
			}
			req.Header.Set("Authorization", "Bearer test-admin-credential")
			response, err = server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("authenticated control denied: %d", response.StatusCode)
			}
		}
	}
}

func TestBrowserRoomFilesRequireReadCapabilities(t *testing.T) {
	messages, err := im.NewServiceFromPath(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Browser", CreatorID: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Other", CreatorID: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := messages.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Attachments: []im.MessageAttachmentUpload{{Name: "file.txt", Data: []byte("fixture-content")}}})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: messages, serverAccessToken: "test-token", desktopSessionToken: "test-desktop-token"}
	object, err := messages.AttachmentFile(message.Attachments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, room, token, auth, caller, method string
		listStatus, fileStatus                  int
	}{
		{"headers only", room.ID, "", "", "", http.MethodGet, 401, 401},
		{"file capability", room.ID, object.DownloadToken, "", "", http.MethodGet, 401, 200},
		{"wrong room", other.ID, object.DownloadToken, "", "", http.MethodGet, 401, 401},
		{"invalid capability", room.ID, "forged", "", "", http.MethodGet, 401, 401},
		{"invalid bearer cannot downgrade", room.ID, object.DownloadToken, "Bearer wrong", "", http.MethodGet, 401, 401},
		{"caller header cannot downgrade", room.ID, object.DownloadToken, "", "admin", http.MethodGet, 401, 401},
		{"admin", room.ID, "", "Bearer test-token", "", http.MethodGet, 200, 200},
		{"desktop", room.ID, "", "Bearer test-desktop-token", "", http.MethodGet, 200, 200},
		{"file capability cannot delete", room.ID, object.DownloadToken, "", "", http.MethodDelete, 401, 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, suffix := range []string{"", "/" + object.Attachment.ID} {
				if test.method == http.MethodDelete && suffix == "" {
					continue
				}
				req := httptest.NewRequest(test.method, "/api/v1/rooms/"+test.room+"/attachments"+suffix+"?token="+test.token, nil)
				req.Header.Set("Sec-Fetch-Site", "same-origin")
				req.Header.Set("Sec-Fetch-Mode", "cors")
				req.Header.Set("Authorization", test.auth)
				req.Header.Set("X-CSGClaw-Caller-Agent", test.caller)
				rec := httptest.NewRecorder()
				h.Routes().ServeHTTP(rec, req)
				want := test.listStatus
				if suffix != "" {
					want = test.fileStatus
				}
				if rec.Code != want {
					t.Fatalf("%s %s: status %d, want %d", test.method, suffix, rec.Code, want)
				}
			}
		})
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+room.ID+"/attachments/"+object.Attachment.ID+"?token="+object.DownloadToken, nil)
	req.Header.Set("Range", "bytes=0-6")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "fixture" {
		t.Fatalf("capability Range status %d", rec.Code)
	}
	newMessage, err := messages.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Attachments: []im.MessageAttachmentUpload{{Name: "later.txt", Data: []byte("later private data")}}})
	if err != nil {
		t.Fatal(err)
	}
	later := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+room.ID+"/attachments/"+newMessage.Attachments[0].ID+"?token="+object.DownloadToken, nil)
	laterResponse := httptest.NewRecorder()
	h.Routes().ServeHTTP(laterResponse, later)
	if laterResponse.Code != http.StatusUnauthorized {
		t.Fatal("a file capability granted future attachment access")
	}
	if _, err := messages.ClearRoomMessages(room.ID); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatal("capability ignored removed live references")
	}
	h.serverNoAuth = true
	req = httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+room.ID+"/attachments", nil)
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatal("explicit NoAuth mode stopped working")
	}
}
