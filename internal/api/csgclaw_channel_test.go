package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/im"
)

func TestHandleCsgclawChannelRoutesMirrorLocalCollections(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin", Role: "admin"},
				{ID: "u-alice", Name: "Alice", Handle: "alice", Role: "worker"},
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
		if len(got) < 2 || got[0].ID != "u-admin" {
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

func TestHandleCsgclawChannelNestedRoutesMirrorLocalMutations(t *testing.T) {
	srv := &Handler{im: im.NewService()}

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/users", strings.NewReader(`{"id":"u-alice","name":"Alice","handle":"alice","role":"worker"}`)))
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
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/users", strings.NewReader(`{"id":"u-bob","name":"Bob","handle":"bob","role":"worker"}`)))
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
	if !testUsersContain(members, "u-bob") {
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

func testUsersContain(users []im.User, id string) bool {
	for _, user := range users {
		if user.ID == id {
			return true
		}
	}
	return false
}
