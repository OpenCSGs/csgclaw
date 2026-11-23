package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"csgclaw/internal/im"
)

func TestMessagePagesBoundHistoryAndCatchUp(t *testing.T) {
	messages := make([]im.Message, 4146)
	for index := range messages {
		messages[index] = im.Message{ID: fmt.Sprintf("msg-%04d", index+1), SenderID: "u-admin", Content: "tool output"}
	}
	srv := &Handler{im: im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users:         []im.User{{ID: "u-admin"}},
		Rooms:         []im.Room{{ID: "room-1", Members: []string{"u-admin"}, Messages: messages}},
	})}
	handler := srv.Routes()
	requestPage := func(query string, limit int) im.MessagePage {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/messages?room_id=room-1"+query, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
		}
		var page im.MessagePage
		if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
			t.Fatal(err)
		}
		if len(page.Messages) != limit {
			t.Fatalf("page has %d messages, want %d", len(page.Messages), limit)
		}
		return page
	}
	latest := requestPage("&limit=100", 100)
	if !latest.HasMore || latest.Messages[0].ID != "msg-4047" || latest.Messages[99].ID != "msg-4146" {
		t.Fatal("initial page did not return the latest 100 messages in order")
	}
	older := requestPage("&limit=100&before=msg-4047", 100)
	if !older.HasMore || older.Messages[0].ID != "msg-3947" || older.Messages[99].ID != "msg-4046" {
		t.Fatal("older page skipped or duplicated messages at the boundary")
	}
	forward := requestPage("&limit=100&after=msg-3900", 100)
	if !forward.HasMore || forward.Anchor == nil || forward.Anchor.ID != "msg-3900" ||
		forward.Messages[0].ID != "msg-3901" || forward.Messages[99].ID != "msg-4000" {
		t.Fatal("catch-up page should return the anchor revision and the first 100 messages after it")
	}
	if empty := requestPage("&limit=100&after=msg-4146", 0); empty.HasMore || empty.Anchor == nil {
		t.Fatal("an unchanged tail should return only the anchor revision")
	}
	if missing := requestPage("&limit=100&before=removed", 0); !missing.CursorMissing {
		t.Fatal("a removed cursor must be reported explicitly")
	}
}
