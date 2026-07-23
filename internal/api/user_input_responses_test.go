package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/im"
	runtimecodex "csgclaw/internal/runtime/codex"
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

func TestChannelUserInputResponsePersistsReadableLocalUserTranscript(t *testing.T) {
	t.Parallel()

	broker := runtimecodex.NewUserInputBroker(nil)
	h, statePath, bus := newPersistentUserInputTestHandler(t, broker)
	if _, err := h.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID: "room-1", SenderID: "user-agent", Content: "Thread root", MessageID: "thread-root",
	}); err != nil {
		t.Fatalf("create thread root: %v", err)
	}
	snapshot, err := broker.CreateDetached(runtimecodex.PendingUserInputRequest{
		Questions: []activity.UserInputQuestionSnapshot{
			{
				ID: "kind", Header: "Kind", Question: "Choose",
				Options: []activity.UserInputOptionSnapshot{{Label: "Standard", Description: "Normal checks."}},
			},
			{ID: "note", Header: "Note", Question: "Add note", IsOther: true},
			{ID: "secret", Header: "Secret", Question: "Disposable only", IsOther: true, IsSecret: true},
		},
	}, runtimecodex.DetachedUserInputContext{
		Channel: "csgclaw", RoomID: "room-1", ThreadRootID: "thread-root", SourceMessageID: "source-1",
	})
	if err != nil {
		t.Fatalf("CreateDetached() error = %v", err)
	}
	events, cancelEvents := bus.Subscribe()
	defer cancelEvents()

	body := `{"answers":{"kind":{"answers":["Standard"]},"note":{"answers":[]},"secret":{"answers":["user_note: disposable-secret"]}}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/activities/"+snapshot.ID+":respond", strings.NewReader(body))
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	select {
	case event := <-events:
		if event.Type != im.EventTypeMessageCreated || event.Message == nil || event.Message.ID != "answer-"+snapshot.ID {
			t.Fatalf("IM event = %+v, want visible answer transcript", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for answer transcript IM event")
	}

	messages, err := h.im.ListMessagesWithOptions("room-1", im.ListMessagesOptions{IncludeThreadReplies: true})
	if err != nil {
		t.Fatalf("ListMessagesWithOptions() error = %v", err)
	}
	var answer im.Message
	for _, message := range messages {
		if message.ID == "answer-"+snapshot.ID {
			answer = message
		}
	}
	if answer.ID == "" || answer.SenderID != "user-admin" || answer.RelatesTo == nil || answer.RelatesTo.EventID != "thread-root" {
		t.Fatalf("answer message = %+v, want local user reply in question thread", answer)
	}
	want := "## Answers\n\n- kind：Standard (Normal checks.)\n- note：Skipped (No answer provided)\n- secret：Secret recorded (Secret value redacted)"
	if answer.Content != want {
		t.Fatalf("answer content = %q, want %q", answer.Content, want)
	}
	encoded, _ := json.Marshal(answer)
	if strings.Contains(string(encoded), "disposable-secret") || !isUserInputAnswerTranscript(&answer) {
		t.Fatalf("answer = %s, want redacted transcript metadata", encoded)
	}
	root := filepath.Dir(statePath)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), "disposable-secret") {
			t.Errorf("persisted file %s contains unredacted test secret", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan persisted IM state: %v", err)
	}

	duplicate := httptest.NewRecorder()
	h.Routes().ServeHTTP(duplicate, httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/activities/"+snapshot.ID+":respond", strings.NewReader(body)))
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409", duplicate.Code)
	}
	messages, err = h.im.ListMessagesWithOptions("room-1", im.ListMessagesOptions{IncludeThreadReplies: true})
	if err != nil {
		t.Fatalf("ListMessagesWithOptions() after duplicate error = %v", err)
	}
	count := 0
	for _, message := range messages {
		if message.ID == answer.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("answer message count = %d, want 1", count)
	}
}

func TestChannelUserInputResponseSkipAllDoesNotPersistTranscript(t *testing.T) {
	t.Parallel()

	broker := runtimecodex.NewUserInputBroker(nil)
	h := newUserInputTestHandler(broker)
	snapshot, err := broker.CreateDetached(runtimecodex.PendingUserInputRequest{
		Questions: []activity.UserInputQuestionSnapshot{{ID: "q", Header: "Q", Question: "Answer?"}},
	}, runtimecodex.DetachedUserInputContext{Channel: "csgclaw", RoomID: "room-1"})
	if err != nil {
		t.Fatalf("CreateDetached() error = %v", err)
	}
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/activities/"+snapshot.ID+":respond", strings.NewReader(`{"answers":{}}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	room, _ := h.im.Room("room-1")
	for _, message := range room.Messages {
		if strings.HasPrefix(message.ID, "answer-") {
			t.Fatalf("unexpected answer transcript: %+v", message)
		}
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
	h := &Handler{im: im.NewServiceFromBootstrap(userInputTestBootstrap())}
	h.SetUserInputResponder(responder)
	return h
}

func newPersistentUserInputTestHandler(t *testing.T, responder UserInputResponder) (*Handler, string, *im.Bus) {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "im.json")
	if err := im.SaveBootstrap(statePath, userInputTestBootstrap()); err != nil {
		t.Fatalf("SaveBootstrap() error = %v", err)
	}
	bus := im.NewBus()
	imSvc, err := im.NewServiceFromPathWithBus(statePath, bus)
	if err != nil {
		t.Fatalf("NewServiceFromPath() error = %v", err)
	}
	h := &Handler{im: imSvc}
	h.SetUserInputResponder(responder)
	return h, statePath, bus
}

func userInputTestBootstrap() im.Bootstrap {
	return im.Bootstrap{
		CurrentUserID: "user-admin",
		Users: []im.User{
			{ID: "user-admin", Name: "Admin", Role: "admin"},
			{ID: "user-agent", Name: "Agent", Role: "worker"},
			{ID: "user-outsider", Name: "Outsider", Role: "worker"},
		},
		Rooms: []im.Room{{ID: "room-1", Title: "Room", Members: []string{"user-admin", "user-agent"}}},
	}
}
