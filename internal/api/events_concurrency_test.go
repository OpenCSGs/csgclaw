package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/im"
)

func TestIMEventsDisconnectWhilePublishing(t *testing.T) {
	bus := im.NewBus()
	handler := &Handler{imBus: bus, serverNoAuth: true}
	server := httptest.NewServer(handler.Routes())
	defer server.Close()

	stop := make(chan struct{})
	var publishers sync.WaitGroup
	var panics atomic.Int64
	for range 4 {
		publishers.Add(1)
		go func() {
			defer publishers.Done()
			defer func() {
				if recover() != nil {
					panics.Add(1)
				}
			}()
			for {
				select {
				case <-stop:
					return
				default:
					bus.Publish(im.Event{Type: im.EventTypeMessageCreated, RoomID: "test-room"})
					runtime.Gosched()
				}
			}
		}()
	}
	defer func() {
		close(stop)
		publishers.Wait()
		if count := panics.Load(); count != 0 {
			t.Errorf("disconnecting event streams caused %d publisher panics", count)
		}
	}()

	for range 64 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/events", nil)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		response, err := server.Client().Do(request)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		response.Body.Close()
		cancel()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("event stream status = %d, want 200", response.StatusCode)
		}
	}
}
