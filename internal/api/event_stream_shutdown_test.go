package api

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/im"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/worklease"
)

func TestFeishuEventStreamShutdownUnsubscribes(t *testing.T) {
	service := feishu.NewService()
	handler := &Handler{feishu: service, serverAccessToken: "shutdown-test"}
	shutdown := make(chan struct{})
	handler.SetEventStreamShutdown(shutdown)
	response, done := openShutdownEventStream(t, func(w http.ResponseWriter, r *http.Request) {
		handler.streamFeishuEvents(w, r, feishuEventTarget{IDs: []string{"pt-worker"}})
	})

	close(shutdown)
	waitForEventStreamShutdown(t, response, done)
	// The bus has no public subscriber count. Inspect it after the handler has
	// returned to verify unsubscribe without adding a production API.
	subscribers := reflect.ValueOf(service.MessageBus()).Elem().FieldByName("subscribers")
	if got := subscribers.Len(); got != 0 {
		t.Fatalf("subscriptions after shutdown = %d, want 0", got)
	}
}

func TestParticipantEventStreamShutdownUnsubscribes(t *testing.T) {
	bridge := im.NewParticipantBridge("")
	controls := worklease.NewControlBus()
	handler := &Handler{participantBridge: bridge, workControlBus: controls}
	shutdown := make(chan struct{})
	handler.SetEventStreamShutdown(shutdown)
	response, done := openShutdownEventStream(t, func(w http.ResponseWriter, r *http.Request) {
		handler.handleParticipantEventsStream(w, r, "pt-worker")
	})
	if got := bridge.SubscriberCount("pt-worker"); got != 1 {
		t.Fatalf("active subscriptions = %d, want 1", got)
	}
	if err := controls.StopTurn(context.Background(), agentruntime.TurnRef{ParticipantID: "pt-worker"}); err != nil {
		t.Fatalf("active control subscription: %v", err)
	}

	close(shutdown)
	waitForEventStreamShutdown(t, response, done)
	if got := bridge.SubscriberCount("pt-worker"); got != 0 {
		t.Fatalf("subscriptions after shutdown = %d, want 0", got)
	}
	if err := controls.StopTurn(context.Background(), agentruntime.TurnRef{ParticipantID: "pt-worker"}); !errors.Is(err, agentruntime.ErrTurnControlUnavailable) {
		t.Fatalf("control after shutdown = %v, want no subscriber", err)
	}
}

func TestParticipantEventStreamShutdownRequeuesBufferedEvents(t *testing.T) {
	bridge := im.NewParticipantBridge("")
	handler := &Handler{participantBridge: bridge}
	shutdown := make(chan struct{})
	handler.SetEventStreamShutdown(shutdown)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	writer := &shutdownBufferedEventWriter{
		failingBotEventWriter: failingBotEventWriter{header: make(http.Header)},
		ready:                 make(chan struct{}),
		release:               release,
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
		handler.handleParticipantEventsStream(writer, req, "pt-worker")
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-release:
		default:
			close(release)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("handler did not stop during cleanup")
		}
	})
	select {
	case <-writer.ready:
	case <-time.After(time.Second):
		t.Fatal("participant stream did not subscribe")
	}
	room := im.Room{ID: "room-shutdown", IsDirect: true, Members: []string{"user-admin", "pt-worker"}}
	sender := im.User{ID: "user-admin", Name: "admin"}
	want := map[string]string{"msg-first": "first", "msg-second": "second", "msg-third": "third"}
	for id, content := range want {
		message := im.Message{ID: id, SenderID: sender.ID, Content: content}
		if !bridge.EnqueueMessageEvent(room, sender, message, "pt-worker") {
			t.Fatalf("event %s was not buffered in the active subscription", id)
		}
	}
	// Shutdown can race with selecting an already-buffered event. The writer
	// models the disconnected client so either exit path must preserve all events.
	close(shutdown)
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("participant stream did not stop on shutdown")
	}
	if got := bridge.SubscriberCount("pt-worker"); got != 0 {
		t.Fatalf("subscriptions after shutdown = %d, want 0", got)
	}
	events, unsubscribe := bridge.Subscribe("pt-worker")
	defer unsubscribe()
	for range len(want) {
		select {
		case event := <-events:
			content, ok := want[event.MessageID]
			if !ok || event.Text != content || event.RoomID != room.ID {
				t.Fatalf("unexpected or duplicate replayed event: %+v", event)
			}
			delete(want, event.MessageID)
			bridge.Ack("pt-worker", event.MessageID)
		case <-time.After(time.Second):
			t.Fatalf("buffered events lost on shutdown: %v", want)
		}
	}
	select {
	case event := <-events:
		t.Fatalf("unexpected extra replayed event: %+v", event)
	default:
	}
}

type shutdownBufferedEventWriter struct {
	failingBotEventWriter
	ready   chan struct{}
	release <-chan struct{}
}

func (w *shutdownBufferedEventWriter) Flush() {
	close(w.ready)
	<-w.release
}

func openShutdownEventStream(t *testing.T, handler http.HandlerFunc) (*http.Response, <-chan struct{}) {
	t.Helper()
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer shutdown-test")
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("unexpected event stream response: %s %q", response.Status, response.Header.Get("Content-Type"))
	}
	if line, err := bufio.NewReader(response.Body).ReadString('\n'); err != nil || line != ": connected\n" {
		t.Fatalf("stream greeting = %q, error = %v", line, err)
	}
	return response, done
}

func waitForEventStreamShutdown(t *testing.T, response *http.Response, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not return after closing the shutdown channel")
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatalf("stream did not close cleanly: %v", err)
	}
}
