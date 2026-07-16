package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/activity"
	"csgclaw/internal/im"
)

type fakeUserInputResponder struct {
	snapshot activity.UserInputSnapshot
	err      error
	got      activity.UserInputResponseRequest
}

func (r *fakeUserInputResponder) Respond(_ context.Context, req activity.UserInputResponseRequest) (activity.UserInputSnapshot, error) {
	r.got = req
	return r.snapshot, r.err
}

func TestChannelUserInputResponseEndpoint(t *testing.T) {
	t.Parallel()

	responder := &fakeUserInputResponder{snapshot: activity.UserInputSnapshot{ID: "question-1", Status: activity.UserInputStatusAnswered}}
	h := newUserInputTestHandler(responder)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/activities/question-1:respond", strings.NewReader(`{
		"room_id":"room-1",
		"responder_id":"user-admin",
		"answers":{"color":{"option_index":2,"text":"darker","skip":false}},
		"skip_all":false
	}`))
	h.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if responder.got.Channel != "csgclaw" || responder.got.ActivityID != "question-1" || responder.got.RoomID != "room-1" || responder.got.ResponderID != "user-admin" {
		t.Fatalf("response request = %+v, want route and responder binding", responder.got)
	}
	answer := responder.got.Answers["color"]
	if answer.OptionIndex != 2 || answer.Text != "darker" || answer.Skip {
		t.Fatalf("answer = %+v, want option 2 with note", answer)
	}
}

func TestChannelUserInputResponseEndpointSkipAll(t *testing.T) {
	t.Parallel()

	responder := &fakeUserInputResponder{snapshot: activity.UserInputSnapshot{ID: "question-1", Status: activity.UserInputStatusSkipped}}
	h := newUserInputTestHandler(responder)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/activities/question-1:respond", strings.NewReader(`{"room_id":"room-1","responder_id":"user-admin","skip_all":true}`))
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !responder.got.SkipAll {
		t.Fatalf("status=%d skip_all=%v body=%s", rec.Code, responder.got.SkipAll, rec.Body.String())
	}
}

func TestChannelUserInputResponseEndpointRejectsWrongRoomOrResponder(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"unknown room":      `{"room_id":"room-2","responder_id":"user-admin","skip_all":true}`,
		"unknown responder": `{"room_id":"room-1","responder_id":"user-missing","skip_all":true}`,
		"non-member":        `{"room_id":"room-1","responder_id":"user-outsider","skip_all":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			h := newUserInputTestHandler(&fakeUserInputResponder{})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/activities/question-1:respond", strings.NewReader(body))
			h.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestChannelUserInputResponseEndpointErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		status   activity.UserInputStatus
		wantCode int
	}{
		{name: "malformed answer", err: activity.ErrUserInputInvalidResponse, status: activity.UserInputStatusPending, wantCode: http.StatusBadRequest},
		{name: "missing", err: activity.ErrUserInputNotFound, status: activity.UserInputStatusPending, wantCode: http.StatusNotFound},
		{name: "conflict", err: activity.ErrUserInputAlreadyResolved, status: activity.UserInputStatusAnswered, wantCode: http.StatusConflict},
		{name: "expired", err: activity.ErrUserInputGone, status: activity.UserInputStatusExpired, wantCode: http.StatusGone},
		{name: "unexpected", err: errors.New("boom"), status: activity.UserInputStatusPending, wantCode: http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			responder := &fakeUserInputResponder{
				snapshot: activity.UserInputSnapshot{ID: "question-1", Status: tc.status},
				err:      tc.err,
			}
			h := newUserInputTestHandler(responder)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/activities/question-1:respond", strings.NewReader(`{"room_id":"room-1","responder_id":"user-admin","skip_all":true}`))
			h.Routes().ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantCode, rec.Body.String())
			}
			if (tc.wantCode == http.StatusConflict || tc.wantCode == http.StatusGone) && !strings.Contains(rec.Body.String(), `"id":"question-1"`) {
				t.Fatalf("body = %s, want winning snapshot", rec.Body.String())
			}
		})
	}
}

func newUserInputTestHandler(responder UserInputResponder) *Handler {
	h := &Handler{im: im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "user-admin",
		Users: []im.User{
			{ID: "user-admin", Name: "Admin", Role: "admin"},
			{ID: "user-agent", Name: "Agent", Role: "worker"},
			{ID: "user-outsider", Name: "Outsider", Role: "worker"},
		},
		Rooms: []im.Room{{ID: "room-1", Title: "Room", Members: []string{"user-admin", "user-agent"}}},
	})}
	h.SetUserInputResponder(responder)
	return h
}
