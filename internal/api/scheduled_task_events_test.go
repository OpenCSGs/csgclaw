package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/im"
	"csgclaw/internal/scheduledtask"
)

func TestScheduledTaskChangesReachEventStream(t *testing.T) {
	root := filepath.Join(t.TempDir(), "schedules")
	store, err := scheduledtask.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := scheduledtask.NewService(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	bus := im.NewBus()
	events, unsubscribe := bus.Subscribe()
	defer unsubscribe()
	h := &Handler{scheduledTaskSvc: svc, imBus: bus, serverNoAuth: true}
	routes := h.Routes()
	server := httptest.NewServer(routes)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	if stream.StatusCode != http.StatusOK {
		t.Fatalf("event stream status = %d", stream.StatusCode)
	}
	scanner := bufio.NewScanner(stream.Body)
	call := func(method, path, body string, status int, changed bool) {
		t.Helper()
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body)
		}
		if !changed {
			select {
			case event := <-events:
				t.Fatalf("unsuccessful mutation published %+v", event)
			default:
			}
			return
		}
		if event := mustReceiveIMEvent(t, events); event.Type != im.EventTypeScheduledTaskChanged {
			t.Fatalf("unexpected change event: %+v", event)
		}
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var event imEventResponse
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
				t.Fatal(err)
			}
			if event.Type != "scheduled_task.changed" {
				t.Fatalf("browser received unexpected event: %+v", event)
			}
			return
		}
		t.Fatalf("event stream ended before schedule notification: %v", scanner.Err())
	}
	const collection = "/api/v1/scheduled-tasks"
	const body = `{"title":"Weekly report","agent_id":"agent-manager","prompt":"Summarize last week","recurrence":"weekly","first_run_at":"2026-11-02T10:00:00+08:00"}`
	call(http.MethodPost, collection, `{}`, http.StatusBadRequest, false)
	call(http.MethodPost, collection, body, http.StatusCreated, true)
	items := svc.List()
	if len(items) != 1 {
		t.Fatalf("notification arrived without a saved schedule: %+v", items)
	}
	path := collection + "/" + items[0].ID
	call(http.MethodPatch, path, `{"recurrence":"invalid"}`, http.StatusBadRequest, false)
	call(http.MethodPatch, path, `{"title":"Revised report"}`, http.StatusOK, true)
	call(http.MethodDelete, path, "", http.StatusNoContent, true)
	call(http.MethodDelete, path, "", http.StatusNotFound, false)
	// A failed disk write must not tell the browser that a schedule was saved.
	if err := os.Rename(root, root+"-saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("block directory creation"), 0o600); err != nil {
		t.Fatal(err)
	}
	call(http.MethodPost, collection, body, http.StatusBadRequest, false)
}
