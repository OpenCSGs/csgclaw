package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

func TestHandleCsgclawChannelRoutesMirrorLocalCollections(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Role: "admin"},
				{ID: "u-alice", Name: "Alice", Role: "worker"},
			},
			Rooms: []im.Room{{
				ID:      "room-1",
				Title:   "Room One",
				Members: []string{"u-admin", "u-alice"},
				Messages: []im.Message{{
					ID:        "msg-1",
					SenderID:  "u-admin",
					Content:   "hello",
					CreatedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
				}},
			}},
		}),
	}

	t.Run("users", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/users", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var got []im.User
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode users: %v", err)
		}
		if len(got) < 2 || got[0].ID != "user-admin" {
			t.Fatalf("users = %+v, want local users through csgclaw channel route", got)
		}
	})

	t.Run("rooms", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/rooms", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var got []im.Room
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode rooms: %v", err)
		}
		if len(got) != 1 || got[0].ID != "room-1" {
			t.Fatalf("rooms = %+v, want room-1 through csgclaw channel route", got)
		}
	})

	t.Run("messages", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/messages?room_id=room-1", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var got []im.Message
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode messages: %v", err)
		}
		if len(got) != 1 || got[0].ID != "msg-1" {
			t.Fatalf("messages = %+v, want msg-1 through csgclaw channel route", got)
		}
	})
}

func TestHandleCsgclawMessageMultipartAttachmentAndDownload(t *testing.T) {
	imSvc, err := im.NewServiceFromPath(filepath.Join(t.TempDir(), "im", "state.json"))
	if err != nil {
		t.Fatalf("NewServiceFromPath() error = %v", err)
	}
	worker, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "worker", Name: "worker", Role: "worker"})
	if err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}
	room, err := imSvc.CreateRoom(im.CreateRoomRequest{
		Title:     "Uploads",
		CreatorID: "user-admin",
		MemberIDs: []string{worker.ID},
	})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	srv := &Handler{im: imSvc, participantBridge: im.NewParticipantBridge(""), serverAccessToken: "secret"}
	fileBytes := []byte("hello from an uploaded note")

	body, contentType := multipartMessageBodyForTest(t, map[string]any{
		"room_id":   room.ID,
		"sender_id": "user-admin",
		"content":   "",
	}, "files", "plan.txt", "text/plain", fileBytes)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/messages", body)
	req.Header.Set("Content-Type", contentType)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create message status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var msg im.Message
	if err := json.NewDecoder(rec.Body).Decode(&msg); err != nil {
		t.Fatalf("decode message: %v", err)
	}
	if len(msg.Attachments) != 1 {
		t.Fatalf("attachments = %+v, want one uploaded file", msg.Attachments)
	}
	att := msg.Attachments[0]
	if att.Name != "plan.txt" || att.Kind != "file" || att.MediaType != "text/plain" {
		t.Fatalf("attachment = %+v, want sanitized file metadata", att)
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, att.DownloadURL, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("capability download status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Body.String(); got != string(fileBytes) {
		t.Fatalf("capability download body = %q, want original upload", got)
	}

	bareDownloadURL, _, _ := strings.Cut(att.DownloadURL, "?")
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, bareDownloadURL, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("download status without capability or auth = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, bareDownloadURL, nil)
	req.Header.Set("Authorization", "Bearer secret")
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Body.String(); got != string(fileBytes) {
		t.Fatalf("download body = %q, want original upload", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff header = %q, want nosniff", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "sandbox") {
		t.Fatalf("Content-Security-Policy = %q, want sandbox", got)
	}

	body, contentType = multipartMessageBodyForTest(t, map[string]any{
		"room_id":   room.ID,
		"sender_id": "user-admin",
		"content":   "",
	}, "files", "../secret.txt", "text/plain", []byte("secret"))
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/messages", body)
	req.Header.Set("Content-Type", contentType)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unsafe") {
		t.Fatalf("unsafe filename status = %d, body=%q, want 400 unsafe filename", rec.Code, rec.Body.String())
	}

	oversized := bytes.Repeat([]byte("x"), im.MaxAttachmentFileBytes+1)
	body, contentType = multipartMessageBodyForTest(t, map[string]any{
		"room_id":   room.ID,
		"sender_id": "user-admin",
		"content":   "",
	}, "files", "large.bin", "application/octet-stream", oversized)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/messages", body)
	req.Header.Set("Content-Type", contentType)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized attachment status = %d, body=%q, want %d", rec.Code, rec.Body.String(), http.StatusRequestEntityTooLarge)
	}

	body, contentType = multipartMessageBodyForTest(t, map[string]any{
		"room_id":        room.ID,
		"text":           "",
		"thread_root_id": msg.ID,
	}, "files", "report.txt", "text/plain", []byte("generated report"))
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants/worker/messages", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", contentType)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("participant multipart status = %d, body=%q, want %d", rec.Code, rec.Body.String(), http.StatusOK)
	}
	var participantResponse map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&participantResponse); err != nil {
		t.Fatalf("decode participant response: %v", err)
	}
	messages, err := imSvc.ListMessagesWithOptions(room.ID, im.ListMessagesOptions{IncludeThreadReplies: true})
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	var participantMessage im.Message
	for _, message := range messages {
		if message.ID == participantResponse["message_id"] {
			participantMessage = message
			break
		}
	}
	if len(participantMessage.Attachments) != 1 || participantMessage.Attachments[0].Name != "report.txt" {
		t.Fatalf("participant message = %+v, want generated report attachment", participantMessage)
	}
	if participantMessage.RelatesTo == nil || participantMessage.RelatesTo.EventID != msg.ID {
		t.Fatalf("participant message relation = %+v, want thread root %q", participantMessage.RelatesTo, msg.ID)
	}
}

func multipartMessageBodyForTest(t *testing.T, payload map[string]any, fieldName, filename, mediaType string, data []byte) (io.Reader, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	payloadWriter, err := writer.CreateFormField("payload")
	if err != nil {
		t.Fatalf("CreateFormField(payload) error = %v", err)
	}
	if err := json.NewEncoder(payloadWriter).Encode(payload); err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="`+filename+`"`)
	header.Set("Content-Type", mediaType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("CreatePart(file) error = %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return bytes.NewReader(body.Bytes()), writer.FormDataContentType()
}

func TestHandleCsgclawUsersShowsHumanDescriptionAndBoundChannels(t *testing.T) {
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "admin",
		Channel:         participant.ChannelFeishu,
		Type:            participant.TypeHuman,
		Name:            "admin",
		ChannelUserRef:  "ou_admin",
		ChannelUserKind: participant.ChannelUserKindOpenID,
		ChannelAppConfig: map[string]any{
			"app_secret": "should-redact",
		},
	}, {
		ID:              "dev",
		Channel:         participant.ChannelFeishu,
		Type:            participant.TypeAgent,
		Name:            "dev",
		ChannelUserRef:  "cli_dev",
		ChannelUserKind: participant.ChannelUserKindAppID,
		AgentID:         "u-dev",
	}}))
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "admin",
			Users: []im.User{{
				ID:       "admin",
				Name:     "admin",
				Role:     "admin",
				IsOnline: true,
			}},
		}),
		participant: participantSvc,
	}

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/users", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	admin, ok := findUserByID(got, "user-admin")
	if !ok {
		t.Fatalf("users = %+v, want admin", got)
	}
	if !strings.Contains(admin.Description, "@admin") || !strings.Contains(admin.Description, "double-check") {
		t.Fatalf("admin description = %q, want default prompt for agent requests", admin.Description)
	}
	if len(admin.Participants) != 1 {
		t.Fatalf("admin participants = %+v, want only bound Feishu human participant", admin.Participants)
	}
	bound := admin.Participants[0]
	if bound.Channel != participant.ChannelFeishu || bound.Type != participant.TypeHuman || bound.ChannelUserRef != "ou_admin" {
		t.Fatalf("admin participant = %+v, want Feishu human open_id binding", bound)
	}
	if gotSecret := bound.ChannelAppConfig["app_secret"]; gotSecret == "should-redact" {
		t.Fatalf("admin participant leaked app_secret: %+v", bound.ChannelAppConfig)
	}
}

func TestHandleCsgclawUserPatchUpdatesDescription(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "admin",
			Users: []im.User{{
				ID:          "admin",
				Name:        "admin",
				Role:        "admin",
				Description: "old prompt",
			}},
		}),
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/channels/csgclaw/users/admin", strings.NewReader(`{"description":"Ask this human to confirm risky changes."}`))
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode user: %v", err)
	}
	if got.Description != "Ask this human to confirm risky changes." {
		t.Fatalf("description = %q, want patched description", got.Description)
	}
	reloaded, ok := srv.im.User("admin")
	if !ok || reloaded.Description != got.Description {
		t.Fatalf("stored user = %+v, ok=%v, want patched description persisted", reloaded, ok)
	}
}

func findUserByID(users []im.User, id string) (im.User, bool) {
	for _, user := range users {
		if user.ID == id {
			return user, true
		}
	}
	return im.User{}, false
}

func TestHandleCsgclawChannelNestedRoutesMirrorLocalMutations(t *testing.T) {
	srv := &Handler{im: im.NewService()}

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/users", strings.NewReader(`{"id":"u-alice","name":"Alice","role":"worker"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/rooms", strings.NewReader(`{"title":"room","creator_id":"u-admin","member_ids":["u-alice"]}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create room status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var room im.Room
	if err := json.NewDecoder(rec.Body).Decode(&room); err != nil {
		t.Fatalf("decode room: %v", err)
	}
	if room.ID == "" {
		t.Fatal("created room ID is empty")
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/users", strings.NewReader(`{"id":"u-bob","name":"Bob","role":"worker"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bob status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/rooms/"+room.ID+"/members", strings.NewReader(`{"inviter_id":"u-admin","user_ids":["u-bob"]}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("add member status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/rooms/"+room.ID+"/members", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list members status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var members []im.User
	if err := json.NewDecoder(rec.Body).Decode(&members); err != nil {
		t.Fatalf("decode members: %v", err)
	}
	if !testUsersContain(members, "user-bob") {
		t.Fatalf("members = %+v, want u-bob through csgclaw channel route", members)
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/messages", strings.NewReader(`{"room_id":"`+room.ID+`","sender_id":"u-admin","content":"hello"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create message status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/channels/csgclaw/rooms/"+room.ID, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete room status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/channels/csgclaw/users/u-bob", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete user status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestHandleClearRoomMessages(t *testing.T) {
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()
	srv := &Handler{
		im: im.NewServiceFromBootstrapWithBus(im.Bootstrap{
			CurrentUserID: "u-admin",
			Rooms: []im.Room{{
				ID:       "room-1",
				Title:    "Room One",
				Members:  []string{"u-admin"},
				Messages: []im.Message{{ID: "msg-1", SenderID: "u-admin", Content: "hello"}},
				Threads:  []im.ThreadState{{RootMessageID: "msg-1"}},
			}},
		}, bus),
		imBus: bus,
	}

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/rooms/room-1:clearMessages", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var room im.Room
	if err := json.NewDecoder(rec.Body).Decode(&room); err != nil {
		t.Fatalf("decode room: %v", err)
	}
	if room.ID != "room-1" || len(room.Members) != 1 {
		t.Fatalf("room = %+v, want preserved room and members", room)
	}
	if len(room.Messages) != 0 || len(room.Threads) != 0 {
		t.Fatalf("messages/threads = %d/%d, want 0/0", len(room.Messages), len(room.Threads))
	}
	evt := mustReceiveEvent(t, events)
	if evt.Type != im.EventTypeRoomMessagesCleared || evt.RoomID != "room-1" || evt.Room == nil {
		t.Fatalf("event = %+v, want room.messages_cleared for room-1", evt)
	}
	if len(evt.Room.Messages) != 0 || len(evt.Room.Threads) != 0 {
		t.Fatalf("event room messages/threads = %d/%d, want 0/0", len(evt.Room.Messages), len(evt.Room.Threads))
	}
}

func testUsersContain(users []im.User, id string) bool {
	for _, user := range users {
		if user.ID == id {
			return true
		}
	}
	return false
}
