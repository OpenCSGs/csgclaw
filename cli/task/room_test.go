package task

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"csgclaw/cli/command"
)

type testTransport func(*http.Request) (*http.Response, error)

func (transport testTransport) Do(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func testJSONResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestRoomActionResolvesRoomFromTaskID(t *testing.T) {
	paths := make([]string, 0, 2)
	run := &command.Context{
		Program: "test", Stdout: io.Discard, Stderr: io.Discard,
		HTTPClient: testTransport(func(request *http.Request) (*http.Response, error) {
			paths = append(paths, request.Method+" "+request.URL.Path)
			switch request.URL.Path {
			case "/api/v1/tasks/task-8":
				return testJSONResponse(`{"id":"task-8","assignment_type":"room","assignment_id":"room-3"}`), nil
			case "/api/v1/rooms/room-3/tasks/task-8":
				return testJSONResponse(`{"id":"task-8","assignment_type":"room","assignment_id":"room-3"}`), nil
			default:
				t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
				return nil, nil
			}
		}),
	}
	if err := NewCmd().Run(context.Background(), run, []string{"get", "--task", "task-8"}, command.GlobalOptions{Endpoint: "http://example.test"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /api/v1/tasks/task-8", "GET /api/v1/rooms/room-3/tasks/task-8"}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("requests = %v, want %v", paths, want)
	}
}

func TestRoomActionRejectsMismatchedExplicitRoom(t *testing.T) {
	calls := 0
	run := &command.Context{
		Program: "test", Stdout: io.Discard, Stderr: io.Discard,
		HTTPClient: testTransport(func(request *http.Request) (*http.Response, error) {
			calls++
			return testJSONResponse(`{"id":"task-8","assignment_type":"room","assignment_id":"room-3"}`), nil
		}),
	}
	err := NewCmd().Run(context.Background(), run, []string{"get", "--task", "task-8", "--room", "room-other"}, command.GlobalOptions{Endpoint: "http://example.test"})
	if err == nil || !strings.Contains(err.Error(), "belongs to room room-3") {
		t.Fatalf("error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want only assignment resolution", calls)
	}
}
