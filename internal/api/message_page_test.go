package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/im"
)

func TestMessagePagesBoundHistoryAndRefresh(t *testing.T) {
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
	if missing := requestPage("&limit=100&before=removed", 0); !missing.CursorMissing {
		t.Fatal("a removed cursor must be reported explicitly")
	}
	refreshed := requestPage("&limit=100&refresh_id=msg-0001&refresh_id=msg-0001&refresh_id=msg-4146&refresh_id=removed", 100)
	if len(refreshed.Refreshed) != 1 || refreshed.Refreshed[0].ID != "msg-0001" || !refreshed.HasMore {
		t.Fatal("refresh IDs should return out-of-window snapshots without changing the page")
	}
	scoped := requestPage("&limit=100&thread_root_id=msg-0001&refresh_id=msg-0002", 1)
	if len(scoped.Refreshed) != 0 {
		t.Fatal("refresh IDs must respect the page's message scope")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/messages?room_id=room-1&limit=100"+strings.Repeat("&refresh_id=msg-0001", im.MaxMessagePageSize+1), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("too many refresh IDs returned status %d", rec.Code)
	}
}
