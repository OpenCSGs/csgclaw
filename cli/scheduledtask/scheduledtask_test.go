package scheduledtask

import (
	"bytes"
	"context"
	"csgclaw/cli/command"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) Do(r *http.Request) (*http.Response, error) { return f(r) }
func TestCreateManagerScheduleUsesRealAPI(t *testing.T) {
	var output bytes.Buffer
	calls := 0
	run := &command.Context{Program: "csgclaw-cli", Stdout: &output, Stderr: io.Discard, HTTPClient: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/v1/scheduled-tasks" {
			t.Fatalf("request %s %s", r.Method, r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]any{"agent_id": "u-manager", "prompt": "Summarize work", "recurrence": "weekly", "first_run_at": "2026-11-02T10:00:00+08:00", "enabled": true} {
			if payload[key] != want {
				t.Fatalf("%s = %#v, want %#v", key, payload[key], want)
			}
		}
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"id":"scheduled-task-1","agent_id":"u-manager"}`))}, nil
	})}
	err := NewCmd().Run(context.Background(), run, []string{"create", "--title", "Weekly summary", "--agent-id", "u-manager", "--prompt", "Summarize work", "--recurrence", "weekly", "--first-run-at", "2026-11-02T10:00:00+08:00"}, command.GlobalOptions{Endpoint: "http://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(output.String(), "scheduled-task-1") {
		t.Fatalf("calls=%d output=%s", calls, output.String())
	}
}
func TestUpdateSendsOnlySpecifiedFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want map[string]any
	}{
		{"omitted expiration", []string{"--enabled=false"}, map[string]any{"enabled": false}},
		{"clear expiration", []string{"--expires-at", ""}, map[string]any{"expires_at": nil}},
		{"clear expiration with equals", []string{"--expires-at="}, map[string]any{"expires_at": nil}},
		{"replace expiration", []string{"--expires-at", "2026-11-30T23:59:00+08:00"}, map[string]any{"expires_at": "2026-11-30T23:59:00+08:00"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			run := &command.Context{Program: "test", Stdout: io.Discard, Stderr: io.Discard, HTTPClient: transport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "PATCH" || r.URL.Path != "/api/v1/scheduled-tasks/scheduled-task-1" {
					t.Fatalf("request %s %s", r.Method, r.URL.Path)
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(payload, tc.want) {
					t.Fatalf("payload=%#v, want %#v", payload, tc.want)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"scheduled-task-1"}`))}, nil
			})}
			args := append([]string{"update", "--id", "scheduled-task-1"}, tc.args...)
			if err := NewCmd().Run(context.Background(), run, args, command.GlobalOptions{Endpoint: "http://example.test"}); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("calls=%d, want 1", calls)
			}
		})
	}
}
func TestInvalidInputDoesNotCallAPI(t *testing.T) {
	for _, args := range [][]string{
		{"create", "--title", "x", "--agent-id", "u-manager", "--prompt", "x", "--first-run-at", "2026-11-02T10:00"},
		{"create", "--title", "x", "--first-run-at", "2026-11-02T10:00:00Z"},
		{"update"}, {"update", "--id", "scheduled-task-1"},
		{"update", "--id", "scheduled-task-1", "--expires-at", "invalid"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			run := &command.Context{Program: "test", Stdout: io.Discard, Stderr: io.Discard, HTTPClient: transport(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected request"); return nil, nil })}
			if err := NewCmd().Run(context.Background(), run, args, command.GlobalOptions{}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
