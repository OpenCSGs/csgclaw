package task

import (
	"context"
	"encoding/json"
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

func TestClaimAndUpdateRoomTaskUseRuntimeCallerWithoutActorFlag(t *testing.T) {
	t.Setenv("CSGCLAW_CALLER_AGENT_ID", "agent-dev")
	tests := []struct {
		name   string
		args   []string
		method string
	}{
		{name: "claim", args: []string{"claim", "--task", "task-8", "--attempt", "2"}, method: http.MethodPost},
		{name: "update", args: []string{"update", "--task", "task-8", "--attempt", "2", "--status", "completed", "--result", "done"}, method: http.MethodPatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			run := &command.Context{
				Program: "test", Stdout: io.Discard, Stderr: io.Discard,
				HTTPClient: testTransport(func(request *http.Request) (*http.Response, error) {
					calls++
					if calls == 1 {
						return testJSONResponse(`{"id":"task-8","assignment_type":"room","assignment_id":"room-3"}`), nil
					}
					if request.Method != tt.method || request.Header.Get("X-CSGClaw-Caller-Agent") != "agent-dev" {
						t.Fatalf("request = %s %s, caller=%q", request.Method, request.URL.Path, request.Header.Get("X-CSGClaw-Caller-Agent"))
					}
					var body map[string]any
					if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if _, found := body["actor_id"]; found {
						t.Fatalf("request unexpectedly contains actor_id: %#v", body)
					}
					if _, found := body["participant_id"]; found {
						t.Fatalf("request unexpectedly contains participant_id: %#v", body)
					}
					return testJSONResponse(`{"id":"task-8","assignment_type":"room","assignment_id":"room-3"}`), nil
				}),
			}
			if err := NewCmd().Run(context.Background(), run, tt.args, command.GlobalOptions{Endpoint: "http://example.test"}); err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatalf("calls = %d, want assignment lookup and room mutation", calls)
			}
		})
	}
}

func TestClaimDirectAgentTaskAcceptsActorID(t *testing.T) {
	paths := []string{}
	run := &command.Context{
		Program: "test", Stdout: io.Discard, Stderr: io.Discard,
		HTTPClient: testTransport(func(request *http.Request) (*http.Response, error) {
			paths = append(paths, request.Method+" "+request.URL.Path)
			switch request.URL.Path {
			case "/api/v1/tasks/task-8":
				return testJSONResponse(`{"id":"task-8","assignment_type":"agent","assignment_id":"agent-dev"}`), nil
			case "/api/v1/agent-tasks/task-8/claim":
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(body), `"participant_id":"pt-dev"`) {
					t.Fatalf("claim body = %s", body)
				}
				return testJSONResponse(`{"id":"task-8","assignment_type":"agent","assignment_id":"agent-dev","status":"in_progress"}`), nil
			default:
				t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
				return nil, nil
			}
		}),
	}
	if err := NewCmd().Run(context.Background(), run, []string{"claim", "--task", "task-8", "--actor-id", "pt-dev"}, command.GlobalOptions{Endpoint: "http://example.test"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /api/v1/tasks/task-8", "POST /api/v1/agent-tasks/task-8/claim"}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("requests = %v, want %v", paths, want)
	}
}
