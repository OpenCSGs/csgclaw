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
	"csgclaw/internal/taskconfig"
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
	if first.Rooms[0].SearchMatch == nil || first.Rooms[0].SearchMatch.Content != "needle in history" {
		t.Fatalf("search match = %+v", first.Rooms[0].SearchMatch)
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

func TestTaskConversationFiltersApplyBeforePagination(t *testing.T) {
	members := []string{"user-admin", "user-worker"}
	rooms := []im.Room{
		{ID: "other", Pinned: true, Members: []string{"user-admin"}, Configuration: &taskconfig.Config{WorkspaceDir: "/project"}},
		{ID: "wrong-directory", Pinned: true, IsDirect: true, TaskSession: true, Members: members, Configuration: &taskconfig.Config{WorkspaceDir: "/other"}},
		{ID: "explicit-empty", Pinned: true, IsDirect: true, TaskSession: true, Members: members, Configuration: &taskconfig.Config{}},
		{ID: "legacy", Title: "needle", IsDirect: true, TaskSession: true, Members: members},
		{ID: "configured", Title: "needle", IsDirect: true, TaskSession: true, Members: members, Configuration: &taskconfig.Config{WorkspaceDir: "/project"}},
		{ID: "archived", Archived: true, IsDirect: true, TaskSession: true, Members: members, Configuration: &taskconfig.Config{WorkspaceDir: "/project"}},
	}
	h := &Handler{im: im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "user-admin",
		Users:         []im.User{{ID: "user-admin", Role: "admin"}, {ID: "user-worker", Role: "worker"}},
		Rooms:         rooms,
	})}
	for _, tc := range []struct {
		query, id string
		total     int
		more      bool
	}{
		{"", "configured", 1, false},
		{"&legacy_peer=user-worker", "configured", 2, true},
		{"&legacy_peer=user-worker&offset=1", "legacy", 2, false},
		{"&archived=true", "archived", 1, false},
	} {
		rec := httptest.NewRecorder()
		url := "/api/v1/conversations?owner=direct:user-worker&directory=%2Fproject&limit=1" + tc.query
		h.listTaskConversations(rec, httptest.NewRequest(http.MethodGet, url, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d: %s", url, rec.Code, rec.Body.String())
		}
		var page taskConversationPage
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Rooms) != 1 || page.Rooms[0].ID != tc.id || page.Total != tc.total || page.HasMore != tc.more {
			t.Fatalf("%s: unexpected page %+v", url, page)
		}
	}
}

func TestTaskConversationFilterGroups(t *testing.T) {
	members := []string{"user-admin", "user-worker"}
	config := &taskconfig.Config{WorkspaceDir: "/project"}
	svc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "user-admin",
		Users:         []im.User{{ID: "user-admin", Role: "admin"}, {ID: "user-worker", Role: "worker"}},
		Rooms: []im.Room{
			{ID: "first", IsDirect: true, TaskSession: true, Members: members, Configuration: config},
			{ID: "second", IsDirect: true, TaskSession: true, Members: members, Configuration: config},
			{ID: "legacy", IsDirect: true, TaskSession: true, Members: members},
			{ID: "explicit-empty", IsDirect: true, TaskSession: true, Members: members, Configuration: &taskconfig.Config{}},
			{ID: "team", Members: members, Configuration: config},
			{ID: "archived", IsDirect: true, Archived: true, Members: members, Configuration: config},
			{ID: "provisioned", IsDirect: true, Members: members},
			{ID: "not-mine", Members: []string{"user-worker"}, Configuration: config},
		},
	})
	h := &Handler{im: svc}
	query := func(suffix string) []taskConversationFilterGroup {
		t.Helper()
		rec := httptest.NewRecorder()
		h.listTaskConversationFilters(rec, httptest.NewRequest(http.MethodGet, "/api/v1/conversations/filters"+suffix, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%d: %s", rec.Code, rec.Body.String())
		}
		var groups []taskConversationFilterGroup
		if err := json.Unmarshal(rec.Body.Bytes(), &groups); err != nil {
			t.Fatal(err)
		}
		return groups
	}
	groups := query("?q=ignored&limit=1")
	if len(groups) != 4 {
		t.Fatalf("expected four groups, got %+v", groups)
	}
	for _, group := range groups {
		wantCount := 1
		if group.Direct {
			if group.Peer != "user-worker" {
				t.Fatalf("unexpected peer: %+v", group)
			}
			if group.WorkspaceDir != nil && *group.WorkspaceDir == "/project" {
				wantCount = 2
			}
		}
		if group.Count != wantCount {
			t.Fatalf("expected count %d, got %+v", wantCount, group)
		}
	}
	archived := query("?archived=true")
	if len(archived) != 1 || archived[0].Count != 1 || !archived[0].Direct {
		t.Fatalf("unexpected archived groups: %+v", archived)
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
		{ID: "thread-room", Title: "Review", Type: apitypes.RoomTypeAutonomous, Members: []string{"user-admin"}, Messages: []im.Message{
			{ID: "root", SenderID: "user-admin", Content: "root"},
			{ID: "reply", SenderID: "user-admin", Content: "hidden-thread-needle", RelatesTo: &apitypes.MessageRelation{RelType: im.RelationTypeThread, EventID: "root"}},
			{ID: "notice", SenderID: "user-admin", Content: "claimed task-17", Metadata: map[string]any{"csgclaw": map[string]any{"delivery_kind": "autonomous_task_update"}}},
		}},
	}})
	h := &Handler{im: svc}
	rec := httptest.NewRecorder()
	h.listTaskConversations(rec, httptest.NewRequest(http.MethodGet, "/api/v1/conversations?q=hidden-thread-needle", nil))
	var page taskConversationPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || page.Total != 1 {
		t.Fatalf("thread search %s: %v", rec.Body.String(), err)
	}
	if len(page.Rooms[0].Messages) != 1 || page.Rooms[0].Messages[0].ID != "reply" {
		t.Fatalf("preview must use the latest public message: %+v", page.Rooms[0].Messages)
	}
	rec = httptest.NewRecorder()
	h.listTaskConversations(rec, httptest.NewRequest(http.MethodGet, "/api/v1/conversations?q=task-17", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || page.Total != 0 {
		t.Fatalf("internal notice matched conversation search %s: %v", rec.Body.String(), err)
	}
	for _, q := range []string{"limit=0", "offset=-1", "limit=101", "limit=bad"} {
		rec := httptest.NewRecorder()
		h.listTaskConversations(rec, httptest.NewRequest(http.MethodGet, "/api/v1/conversations?"+q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", q, rec.Code)
		}
	}
}
