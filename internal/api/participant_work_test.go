package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/worklease"
)

type apiTurnControllerFunc func(context.Context, agentruntime.TurnRef) error

func (f apiTurnControllerFunc) StopTurn(ctx context.Context, ref agentruntime.TurnRef) error {
	return f(ctx, ref)
}

type participantWorkSSEWriter struct {
	header  http.Header
	body    bytes.Buffer
	flushed chan struct{}
}

func (w *participantWorkSSEWriter) Header() http.Header { return w.header }
func (w *participantWorkSSEWriter) WriteHeader(int)     {}
func (w *participantWorkSSEWriter) Write(data []byte) (int, error) {
	return w.body.Write(data)
}
func (w *participantWorkSSEWriter) Flush() {
	w.flushed <- struct{}{}
}

func TestParticipantWorkLeaseAPI(t *testing.T) {
	handler, registry := newParticipantWorkTestHandler(t, true)
	leaseID := worklease.NewID()
	path := "/api/v1/channels/csgclaw/participants/worker/work-leases/" + leaseID

	started := performParticipantWorkRequest(t, handler, http.MethodPut, path, map[string]any{
		"room_id":    "room-1",
		"request_id": "message-1",
		"kind":       "agent_turn",
	})
	if started.Code != http.StatusOK {
		t.Fatalf("start status = %d, body=%s", started.Code, started.Body.String())
	}
	var startUpdate apitypes.ParticipantWorkUpdate
	if err := json.Unmarshal(started.Body.Bytes(), &startUpdate); err != nil {
		t.Fatal(err)
	}
	if startUpdate.Revision != 1 || startUpdate.ParticipantID != "pt-worker" || startUpdate.UserID != "user-worker" {
		t.Fatalf("start update = %#v", startUpdate)
	}
	if delta := startUpdate.ExpiresAt.Sub(time.Now().UTC()); delta < 14*time.Second || delta > 16*time.Second {
		t.Fatalf("default ttl delta = %s", delta)
	}

	renewed := performParticipantWorkRequest(t, handler, http.MethodPut, path, map[string]any{
		"room_id":     "room-1",
		"request_id":  "message-1",
		"kind":        "agent_turn",
		"ttl_seconds": 90,
	})
	if renewed.Code != http.StatusOK {
		t.Fatalf("renew status = %d, body=%s", renewed.Code, renewed.Body.String())
	}
	var renewUpdate apitypes.ParticipantWorkUpdate
	if err := json.Unmarshal(renewed.Body.Bytes(), &renewUpdate); err != nil {
		t.Fatal(err)
	}
	if renewUpdate.Revision != 2 {
		t.Fatalf("renew revision = %d", renewUpdate.Revision)
	}
	if delta := renewUpdate.ExpiresAt.Sub(time.Now().UTC()); delta < 59*time.Second || delta > 61*time.Second {
		t.Fatalf("upper-clamped ttl delta = %s", delta)
	}

	conflict := performParticipantWorkRequest(t, handler, http.MethodPut, path, map[string]any{
		"room_id":    "room-1",
		"request_id": "message-other",
		"kind":       "agent_turn",
	})
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, body=%s", conflict.Code, conflict.Body.String())
	}

	released := performParticipantWorkRequest(t, handler, http.MethodDelete, path, nil)
	if released.Code != http.StatusNoContent {
		t.Fatalf("release status = %d, body=%s", released.Code, released.Body.String())
	}
	if got := registry.ActiveCount("room-1", "pt-worker"); got != 0 {
		t.Fatalf("active count = %d", got)
	}
	duplicate := performParticipantWorkRequest(t, handler, http.MethodDelete, path, nil)
	if duplicate.Code != http.StatusNoContent {
		t.Fatalf("duplicate release status = %d", duplicate.Code)
	}
	closed := performParticipantWorkRequest(t, handler, http.MethodPut, path, map[string]any{
		"room_id":    "room-1",
		"request_id": "message-1",
		"kind":       "agent_turn",
	})
	if closed.Code != http.StatusGone {
		t.Fatalf("closed status = %d, body=%s", closed.Code, closed.Body.String())
	}
}

func TestParticipantWorkLeaseAPIValidationAndAuth(t *testing.T) {
	handler, _ := newParticipantWorkTestHandler(t, false)
	leaseID := worklease.NewID()
	path := "/api/v1/channels/csgclaw/participants/worker/work-leases/" + leaseID

	unauthorized := performParticipantWorkRequest(t, handler, http.MethodPut, path, map[string]any{
		"room_id":    "room-1",
		"request_id": "message-1",
		"kind":       "agent_turn",
	})
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(`{"room_id":"room-1","request_id":"message-1","kind":"other"}`))
	request.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid kind status = %d", recorder.Code)
	}

	unknownRoom := httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(`{"room_id":"missing","request_id":"message-1","kind":"agent_turn"}`))
	unknownRoom.Header.Set("Authorization", "Bearer secret")
	recorder = httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, unknownRoom)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown room status = %d", recorder.Code)
	}

	wrongChannelPath := "/api/v1/channels/feishu/participants/worker/work-leases/" + leaseID
	wrongChannel := httptest.NewRequest(http.MethodDelete, wrongChannelPath, nil)
	wrongChannel.Header.Set("Authorization", "Bearer secret")
	recorder = httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, wrongChannel)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("wrong channel status = %d", recorder.Code)
	}
}

func TestParticipantWorkStatusAndStopAPI(t *testing.T) {
	handler, registry := newParticipantWorkTestHandler(t, true)
	leaseID := worklease.NewID()
	leasePath := "/api/v1/channels/csgclaw/participants/worker/work-leases/" + leaseID
	started := performParticipantWorkRequest(t, handler, http.MethodPut, leasePath, map[string]any{
		"room_id":    "room-1",
		"request_id": "message-1",
		"kind":       "agent_turn",
	})
	if started.Code != http.StatusOK {
		t.Fatalf("start status = %d, body=%s", started.Code, started.Body.String())
	}

	patchBody := map[string]any{
		"capabilities": []string{"thinking_status_v1", "turn_stop_v1", "work_stage_v1"},
		"sequence":     1,
		"phase":        "thinking",
		"stage":        "thinking",
		"thinking": map[string]any{
			"format":    "plain_text",
			"text":      "checking configuration",
			"truncated": false,
		},
	}
	patched := performParticipantWorkRequest(t, handler, http.MethodPatch, leasePath, patchBody)
	if patched.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body=%s", patched.Code, patched.Body.String())
	}
	if !bytes.Contains(patched.Body.Bytes(), []byte(`"thinking":{"format":"plain_text","text":"checking configuration"`)) {
		t.Fatalf("patch body = %s", patched.Body.String())
	}
	if !bytes.Contains(patched.Body.Bytes(), []byte(`"stage":"thinking"`)) {
		t.Fatalf("patch stage body = %s", patched.Body.String())
	}
	stale := performParticipantWorkRequest(t, handler, http.MethodPatch, leasePath, patchBody)
	if stale.Code != http.StatusNoContent {
		t.Fatalf("stale patch status = %d, body=%s", stale.Code, stale.Body.String())
	}

	stopPath := "/api/v1/channels/csgclaw/participants/worker/work:stop"
	stopped := performParticipantWorkRequest(t, handler, http.MethodPost, stopPath, map[string]any{
		"room_id":    "room-1",
		"lease_id":   leaseID,
		"request_id": "message-1",
	})
	if stopped.Code != http.StatusAccepted {
		t.Fatalf("stop status = %d, body=%s", stopped.Code, stopped.Body.String())
	}
	var stopResponse apitypes.ParticipantWorkStopResponse
	if err := json.Unmarshal(stopped.Body.Bytes(), &stopResponse); err != nil {
		t.Fatal(err)
	}
	if !stopResponse.Accepted || stopResponse.LeaseID != leaseID || stopResponse.State != "stop_requested" {
		t.Fatalf("stop response = %#v", stopResponse)
	}
	messages, err := handler.im.ListMessages("room-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages before runtime confirmation = %#v, want none", messages)
	}
	repeatedStop := performParticipantWorkRequest(t, handler, http.MethodPost, stopPath, map[string]any{
		"room_id":    "room-1",
		"lease_id":   leaseID,
		"request_id": "message-1",
	})
	if repeatedStop.Code != http.StatusAccepted {
		t.Fatalf("repeated stop status = %d, body=%s", repeatedStop.Code, repeatedStop.Body.String())
	}
	messages, err = handler.im.ListMessages("room-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages after repeated stop request = %#v, want none", messages)
	}

	renewed := performParticipantWorkRequest(t, handler, http.MethodPut, leasePath, map[string]any{
		"room_id":    "room-1",
		"request_id": "message-1",
		"kind":       "agent_turn",
	})
	if renewed.Code != http.StatusOK || !bytes.Contains(renewed.Body.Bytes(), []byte(`"stop_requested_at"`)) {
		t.Fatalf("renew after stop status=%d body=%s", renewed.Code, renewed.Body.String())
	}
	released := performParticipantWorkRequest(t, handler, http.MethodDelete, leasePath, map[string]any{
		"outcome": apitypes.ParticipantWorkOutcomeStopped,
	})
	if released.Code != http.StatusNoContent || registry.ActiveCount("room-1", "worker") != 0 {
		t.Fatalf("release status=%d active=%d", released.Code, registry.ActiveCount("room-1", "worker"))
	}
	messages, err = handler.im.ListMessages("room-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 ||
		messages[0].ID != "msg-turn-stopped-"+leaseID ||
		messages[0].SenderID != "user-worker" ||
		messages[0].Content != "Conversation interrupted" {
		t.Fatalf("confirmed stop messages = %#v", messages)
	}
	csgclawMetadata, ok := messages[0].Metadata["csgclaw"].(map[string]any)
	if !ok || csgclawMetadata["delivery_kind"] != "turn_stopped" || csgclawMetadata["lease_id"] != leaseID {
		t.Fatalf("stop message metadata = %#v", messages[0].Metadata)
	}
	repeatedRelease := performParticipantWorkRequest(t, handler, http.MethodDelete, leasePath, map[string]any{
		"outcome": apitypes.ParticipantWorkOutcomeStopped,
	})
	if repeatedRelease.Code != http.StatusNoContent {
		t.Fatalf("repeated release status = %d, body=%s", repeatedRelease.Code, repeatedRelease.Body.String())
	}
	messages, err = handler.im.ListMessages("room-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("repeated release messages = %#v, want one idempotent record", messages)
	}
	late := performParticipantWorkRequest(t, handler, http.MethodPatch, leasePath, map[string]any{
		"capabilities": []string{"thinking_status_v1", "turn_stop_v1", "work_stage_v1"},
		"sequence":     2,
		"phase":        "working",
	})
	if late.Code != http.StatusGone {
		t.Fatalf("late patch status = %d, body=%s", late.Code, late.Body.String())
	}
}

func TestParticipantControlRecordIsNotDispatchedAsAgentInput(t *testing.T) {
	if !isParticipantControlRecord(im.Message{Metadata: map[string]any{
		"csgclaw": map[string]any{"delivery_kind": "turn_stopped"},
	}}) {
		t.Fatal("turn stop record was not recognized as participant control")
	}
	if isParticipantControlRecord(im.Message{Metadata: map[string]any{
		"csgclaw": map[string]any{"delivery_kind": "final"},
	}}) {
		t.Fatal("ordinary participant message was recognized as control")
	}
}

func TestParticipantWorkStatusValidation(t *testing.T) {
	handler, _ := newParticipantWorkTestHandler(t, true)
	leaseID := worklease.NewID()
	path := "/api/v1/channels/csgclaw/participants/worker/work-leases/" + leaseID
	performParticipantWorkRequest(t, handler, http.MethodPut, path, map[string]any{
		"room_id": "room-1", "request_id": "message-1", "kind": "agent_turn",
	})
	unknown := httptest.NewRequest(
		http.MethodPatch,
		path,
		bytes.NewBufferString(`{"capabilities":["thinking_status_v1"],"sequence":1,"phase":"working","extra":true}`),
	)
	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, unknown)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d", recorder.Code)
	}
	emptyThinking := performParticipantWorkRequest(t, handler, http.MethodPatch, path, map[string]any{
		"capabilities": []string{"thinking_status_v1", "turn_stop_v1", "work_stage_v1"},
		"sequence":     1,
		"phase":        "thinking",
		"stage":        "thinking",
		"thinking": map[string]any{
			"format":    "plain_text",
			"text":      "  ",
			"truncated": false,
		},
	})
	if emptyThinking.Code != http.StatusBadRequest {
		t.Fatalf("empty thinking status = %d, body=%s", emptyThinking.Code, emptyThinking.Body.String())
	}
}

func TestParticipantWorkStopReportsDeliveryFailureAndTimeout(t *testing.T) {
	handler, registry := newParticipantWorkTestHandler(t, true)
	leaseID := worklease.NewID()
	leasePath := "/api/v1/channels/csgclaw/participants/worker/work-leases/" + leaseID
	performParticipantWorkRequest(t, handler, http.MethodPut, leasePath, map[string]any{
		"room_id": "room-1", "request_id": "message-1", "kind": "agent_turn",
	})
	performParticipantWorkRequest(t, handler, http.MethodPatch, leasePath, map[string]any{
		"capabilities": []string{"turn_stop_v1"},
		"sequence":     1,
		"phase":        "working",
	})
	stopPath := "/api/v1/channels/csgclaw/participants/worker/work:stop"
	stopBody := map[string]any{"room_id": "room-1", "lease_id": leaseID, "request_id": "message-1"}

	unregister := registry.RegisterTurnController("pt-worker", apiTurnControllerFunc(func(context.Context, agentruntime.TurnRef) error {
		return agentruntime.ErrTurnControlUnavailable
	}))
	failed := performParticipantWorkRequest(t, handler, http.MethodPost, stopPath, stopBody)
	unregister()
	if failed.Code != http.StatusBadGateway {
		t.Fatalf("failed stop status = %d, body=%s", failed.Code, failed.Body.String())
	}
	messages, err := handler.im.ListMessages("room-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("failed stop messages = %#v, want none", messages)
	}

	unregister = registry.RegisterTurnController("pt-worker", apiTurnControllerFunc(func(context.Context, agentruntime.TurnRef) error {
		return context.DeadlineExceeded
	}))
	defer unregister()
	timedOut := performParticipantWorkRequest(t, handler, http.MethodPost, stopPath, stopBody)
	if timedOut.Code != http.StatusGatewayTimeout {
		t.Fatalf("timed out stop status = %d, body=%s", timedOut.Code, timedOut.Body.String())
	}
	finished := performParticipantWorkRequest(t, handler, http.MethodDelete, leasePath, map[string]any{
		"outcome": apitypes.ParticipantWorkOutcomeStopTimedOut,
	})
	if finished.Code != http.StatusNoContent {
		t.Fatalf("timed out finish status = %d, body=%s", finished.Code, finished.Body.String())
	}
	messages, err = handler.im.ListMessages("room-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("failed or timed out stop messages = %#v, want none", messages)
	}
}

func TestParticipantWorkEventsMergeWithIMEvents(t *testing.T) {
	imBus := im.NewBus()
	workBus := worklease.NewBus()
	handler := &Handler{imBus: imBus, workBus: workBus}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx)
	writer := &participantWorkSSEWriter{header: make(http.Header), flushed: make(chan struct{}, 4)}
	done := make(chan struct{})
	go func() {
		handler.handleIMEvents(writer, request)
		close(done)
	}()

	waitForParticipantWorkFlush(t, writer.flushed)
	workBus.Publish(worklease.Event{
		Type:   worklease.EventTypeParticipantWorkUpdated,
		RoomID: "room-1",
		Work: apitypes.ParticipantWorkUpdate{
			RegistryEpoch: "epoch-1",
			LeaseID:       worklease.NewID(),
			ParticipantID: "pt-worker",
			UserID:        "user-worker",
			RoomID:        "room-1",
			RequestID:     "message-1",
			Kind:          apitypes.ParticipantWorkKindAgentTurn,
			State:         apitypes.ParticipantWorkStateWorking,
			Reason:        apitypes.ParticipantWorkReasonStarted,
			Revision:      1,
			ExpiresAt:     time.Now().Add(15 * time.Second),
		},
	})
	waitForParticipantWorkFlush(t, writer.flushed)
	imBus.Publish(im.Event{Type: im.EventTypeRoomDeleted, RoomID: "room-2"})
	waitForParticipantWorkFlush(t, writer.flushed)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not stop")
	}

	body, err := io.ReadAll(bytes.NewReader(writer.body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"type":"participant.work.updated"`)) ||
		!bytes.Contains(body, []byte(`"work":{"registry_epoch":"epoch-1"`)) ||
		!bytes.Contains(body, []byte(`"type":"room.deleted"`)) {
		t.Fatalf("merged SSE body = %s", body)
	}
}

func TestParticipantWorkStopControlMergesWithParticipantEvents(t *testing.T) {
	handler, _ := newParticipantWorkTestHandler(t, true)
	leaseID := worklease.NewID()
	leasePath := "/api/v1/channels/csgclaw/participants/worker/work-leases/" + leaseID
	performParticipantWorkRequest(t, handler, http.MethodPut, leasePath, map[string]any{
		"room_id": "room-1", "request_id": "message-1", "kind": "agent_turn",
	})
	performParticipantWorkRequest(t, handler, http.MethodPatch, leasePath, map[string]any{
		"capabilities": []string{"thinking_status_v1", "turn_stop_v1"},
		"sequence":     1,
		"phase":        "working",
	})

	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/channels/csgclaw/participants/worker/events",
		nil,
	).WithContext(ctx)
	writer := &participantWorkSSEWriter{header: make(http.Header), flushed: make(chan struct{}, 4)}
	done := make(chan struct{})
	go func() {
		handler.Routes().ServeHTTP(writer, request)
		close(done)
	}()
	waitForParticipantWorkFlush(t, writer.flushed)

	stop := performParticipantWorkRequest(
		t,
		handler,
		http.MethodPost,
		"/api/v1/channels/csgclaw/participants/worker/work:stop",
		map[string]any{"room_id": "room-1", "lease_id": leaseID, "request_id": "message-1"},
	)
	if stop.Code != http.StatusAccepted {
		t.Fatalf("stop status = %d, body=%s", stop.Code, stop.Body.String())
	}
	waitForParticipantWorkFlush(t, writer.flushed)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("participant SSE handler did not stop")
	}
	body := writer.body.String()
	if !strings.Contains(body, "event: participant.work.stop_requested") ||
		!strings.Contains(body, `"lease_id":"`+leaseID+`"`) {
		t.Fatalf("participant control SSE body = %s", body)
	}
}

func newParticipantWorkTestHandler(t *testing.T, noAuth bool) (*Handler, *worklease.Registry) {
	t.Helper()
	imService := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "user-admin",
		Users: []im.User{
			{ID: "user-admin", Name: "Admin"},
			{ID: "user-worker", Name: "Worker"},
		},
		Rooms: []im.Room{{ID: "room-1", Members: []string{"user-admin", "u-worker"}}},
	})
	participantService := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "pt-worker",
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		ChannelUserRef:  "u-worker",
		LifecycleStatus: participant.LifecycleStatusActive,
	}}))
	bus := worklease.NewBus()
	controlBus := worklease.NewControlBus()
	_, cancelControl := controlBus.Subscribe("pt-worker")
	t.Cleanup(cancelControl)
	registry := worklease.NewRegistry(
		participantService,
		imService,
		bus,
		worklease.WithEpoch("epoch-api-test"),
		worklease.WithControlBus(controlBus),
	)
	handler := NewHandlerWithAuth(
		nil,
		imService,
		im.NewBus(),
		im.NewParticipantBridge("secret"),
		nil,
		nil,
		"secret",
		noAuth,
	)
	handler.SetParticipantService(participantService)
	handler.SetParticipantWorkService(registry, bus, controlBus)
	return handler, registry
}

func performParticipantWorkRequest(t *testing.T, handler *Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody []byte
	if body != nil {
		var err error
		requestBody, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(requestBody))
	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, request)
	return recorder
}

func waitForParticipantWorkFlush(t *testing.T, flushed <-chan struct{}) {
	t.Helper()
	select {
	case <-flushed:
	case <-time.After(time.Second):
		t.Fatal("SSE event was not flushed")
	}
}
