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
	pending  activity.UserInputSnapshot
	snapshot activity.UserInputSnapshot
	err      error
	got      activity.UserInputResponseRequest
}

func (r *fakeUserInputResponder) Get(requestID string) (activity.UserInputSnapshot, bool) {
	return r.pending, r.pending.ID == requestID
}

func (r *fakeUserInputResponder) Respond(_ context.Context, req activity.UserInputResponseRequest) (activity.UserInputSnapshot, error) {
	r.got = req
	return r.snapshot, r.err
}

func TestChannelUserInputResponseEndpoint(t *testing.T) {
	t.Parallel()

	responder := &fakeUserInputResponder{
		pending:  activity.UserInputSnapshot{ID: "question-1", Channel: "csgclaw", RoomID: "room-1", Status: activity.UserInputStatusPending},
		snapshot: activity.UserInputSnapshot{ID: "question-1", Status: activity.UserInputStatusAnswered},
	}
	h := newUserInputTestHandler(responder)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/activities/question-1:respond", strings.NewReader(`{"answers":{"color":{"answers":["Green","user_note: darker"]}}}`))
	h.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if responder.got.Channel != "csgclaw" || responder.got.ActivityID != "question-1" || responder.got.RoomID != "room-1" || responder.got.ResponderID != "user-admin" {
		t.Fatalf("response request = %+v, want route and responder binding", responder.got)
	}
	answer := responder.got.Response.Answers["color"].Answers
	if len(answer) != 2 || answer[0] != "Green" || answer[1] != "user_note: darker" {
		t.Fatalf("answer = %+v, want exact Codex response", answer)
	}
}

func TestChannelUserInputResponseEndpointSkipAll(t *testing.T) {
	t.Parallel()

	responder := &fakeUserInputResponder{
		pending:  activity.UserInputSnapshot{ID: "question-1", Channel: "csgclaw", RoomID: "room-1", Status: activity.UserInputStatusPending},
		snapshot: activity.UserInputSnapshot{ID: "question-1", Status: activity.UserInputStatusSkipped},
	}
	h := newUserInputTestHandler(responder)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/activities/question-1:respond", strings.NewReader(`{"answers":{}}`))
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(responder.got.Response.Answers) != 0 {
		t.Fatalf("status=%d answers=%v body=%s", rec.Code, responder.got.Response.Answers, rec.Body.String())
	}
}

func TestChannelUserInputResponseEndpointRejectsLegacySubmissionFields(t *testing.T) {
	t.Parallel()

	responder := &fakeUserInputResponder{pending: activity.UserInputSnapshot{
		ID: "question-1", Channel: "csgclaw", RoomID: "room-1", Status: activity.UserInputStatusPending,
	}}
	for name, body := range map[string]string{
		"missing answers": `{}`,
		"legacy room":     `{"answers":{},"room_id":"room-1"}`,
		"legacy skip":     `{"answers":{},"skip_all":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			h := newUserInputTestHandler(responder)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/activities/question-1:respond", strings.NewReader(body))
			h.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestChannelUserInputResponseEndpointDerivesRoomAndResponder(t *testing.T) {
	t.Parallel()

	for name, responder := range map[string]*fakeUserInputResponder{
		"missing activity": {},
		"wrong channel":    {pending: activity.UserInputSnapshot{ID: "question-1", Channel: "matrix", RoomID: "room-1"}},
		"unknown room":     {pending: activity.UserInputSnapshot{ID: "question-1", Channel: "csgclaw", RoomID: "room-2"}},
	} {
		t.Run(name, func(t *testing.T) {
			h := newUserInputTestHandler(responder)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/activities/question-1:respond", strings.NewReader(`{"answers":{}}`))
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
				pending:  activity.UserInputSnapshot{ID: "question-1", Channel: "csgclaw", RoomID: "room-1", Status: activity.UserInputStatusPending},
				snapshot: activity.UserInputSnapshot{ID: "question-1", Status: tc.status},
				err:      tc.err,
			}
			h := newUserInputTestHandler(responder)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/activities/question-1:respond", strings.NewReader(`{"answers":{}}`))
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
