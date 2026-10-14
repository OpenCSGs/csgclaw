package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/worklease"
)

func TestTaskConversationSearchPaginationAndArchive(t *testing.T) {
	svc := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: "user-admin", Users: []im.User{{ID: "user-admin", Role: "admin"}}, Rooms: []im.Room{
		{ID: "older", Title: "Old", Pinned: true, Members: []string{"user-admin"}, Messages: []im.Message{{Content: "needle in history", CreatedAt: time.Now().Add(-time.Hour)}, {Content: "latest message"}}},
		{ID: "newer", Title: "New needle", Members: []string{"user-admin"}, Messages: []im.Message{{Content: "hello", CreatedAt: time.Now()}}},
		{ID: "not-mine", Title: "needle", Members: []string{"manager"}},
		{ID: "archived", Title: "needle", Members: []string{"user-admin"}, Archived: true},
	}})
	h := &Handler{im: svc}
	query := func(url string) taskConversationPage {
		r := httptest.NewRecorder()
		h.listTaskConversations(r, httptest.NewRequest(http.MethodGet, url, nil))
		if r.Code != http.StatusOK {
			t.Fatalf("%d: %s", r.Code, r.Body.String())
		}
		var page taskConversationPage
		if err := json.Unmarshal(r.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	first := query("/api/v1/conversations?q=needle&limit=1")
	if first.Total != 2 || len(first.Rooms[0].Messages) != 1 || !first.HasMore || len(first.Rooms) != 1 || first.Rooms[0].ID != "older" {
		t.Fatalf("first = %+v", first)
	}
	if stored, _ := svc.Room("older"); len(stored.Messages) != 2 {
		t.Fatal("pagination mutated stored history")
	}
	if combined := query("/api/v1/conversations?q=needle+history"); combined.Total != 1 {
		t.Fatalf("combined = %+v", combined)
	}
	second := query("/api/v1/conversations?q=needle&limit=1&offset=1")
	if second.HasMore || len(second.Rooms) != 1 || second.Rooms[0].ID != "newer" {
		t.Fatalf("second = %+v", second)
	}
	archived := query("/api/v1/conversations?q=needle&archived=true")
	if len(archived.Rooms) != 1 || archived.Rooms[0].ID != "archived" {
		t.Fatalf("archived = %+v", archived)
	}
}

type taskEntryWorkReporter struct{}

func (taskEntryWorkReporter) StartOrRenew(context.Context, worklease.ParticipantWorkLease) (apitypes.ParticipantWorkUpdate, error) {
	return apitypes.ParticipantWorkUpdate{}, nil
}
func (taskEntryWorkReporter) Stop(context.Context, string, string) error { return nil }
func (taskEntryWorkReporter) ActiveWork(string) []apitypes.ParticipantWorkUpdate {
	return []apitypes.ParticipantWorkUpdate{{}}
}

func TestTaskConversationArchiveAndDeleteRejectRunningWork(t *testing.T) {
	svc := im.NewService()
	room, err := svc.CreateRoom(im.CreateRoomRequest{Title: "Task", CreatorID: "user-admin", MemberIDs: []string{"manager"}})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: svc, participantWork: taskEntryWorkReporter{}}
	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.handleLocalRoomByID(rec, httptest.NewRequest(method, "/", strings.NewReader(`{"archived":true}`)), room.ID)
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s: %d %s", method, rec.Code, rec.Body.String())
		}
	}
	if preserved, ok := svc.Room(room.ID); !ok || preserved.Archived {
		t.Fatal("running room was altered")
	}
}

func TestTaskConversationSearchIncludesThreadRepliesAndRejectsPagination(t *testing.T) {
	svc := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: "user-admin", Users: []im.User{{ID: "user-admin", Role: "admin"}}, Rooms: []im.Room{
		{ID: "thread-room", Title: "Review", Members: []string{"user-admin"}, Messages: []im.Message{
			{ID: "root", SenderID: "user-admin", Content: "root"},
			{ID: "reply", SenderID: "user-admin", Content: "hidden-thread-needle", RelatesTo: &apitypes.MessageRelation{RelType: im.RelationTypeThread, EventID: "root"}},
		}},
	}})
	h := &Handler{im: svc}
	rec := httptest.NewRecorder()
	h.listTaskConversations(rec, httptest.NewRequest(http.MethodGet, "/api/v1/conversations?q=hidden-thread-needle", nil))
	var page taskConversationPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || page.Total != 1 {
		t.Fatalf("thread search %s: %v", rec.Body.String(), err)
	}
	for _, q := range []string{"limit=0", "offset=-1", "limit=101", "limit=bad"} {
		rec := httptest.NewRecorder()
		h.listTaskConversations(rec, httptest.NewRequest(http.MethodGet, "/api/v1/conversations?"+q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", q, rec.Code)
		}
	}
}
