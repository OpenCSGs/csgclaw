package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"csgclaw/internal/agent"
	"csgclaw/internal/bot"
	appversion "csgclaw/internal/version"
)

func TestExecuteAgentStatusUsesHTTPClient(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
			}
			return jsonResponse(http.StatusOK, `[{"id":"u-alice","name":"alice","role":"worker","status":"running","created_at":"2026-04-01T12:00:00Z"}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "agent", "status"})
	if err != nil {
		t.Fatalf("Execute() error = %v; stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id": "u-alice"`) {
		t.Fatalf("stdout = %q, want JSON agent payload", stdout.String())
	}
}

func TestExecuteAgentListUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
			}
			return jsonResponse(http.StatusOK, `[{"id":"u-alice","name":"alice","role":"worker","status":"running","created_at":"2026-04-01T12:00:00Z","profile":"codex-main"}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "worker", "running", "codex-main")
}

func TestExecuteBotListUsesDefaultChannel(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/bots?channel=csgclaw" {
				t.Fatalf("url = %q, want csgclaw bot list route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"bot-alice","name":"alice","role":"worker","channel":"csgclaw","agent_id":"u-alice","user_id":"u-alice","created_at":"2026-04-12T09:00:00Z"}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "bot", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "bot-alice", "alice", "worker", "csgclaw", "u-alice", "u-alice")
}

func TestExecuteBotListFeishuUsesChannelQuery(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/bots?channel=feishu" {
				t.Fatalf("url = %q, want feishu bot list route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"bot-feishu","name":"feishu","role":"manager","channel":"feishu","agent_id":"u-manager","user_id":"fsu-manager","created_at":"2026-04-12T09:00:00Z"}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "bot", "list", "--channel", "feishu"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "bot-feishu"`) || !strings.Contains(stdout.String(), `"channel": "feishu"`) {
		t.Fatalf("stdout = %q, want JSON bot payload", stdout.String())
	}
}

func TestExecuteBotCreateUsesDefaultChannel(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/bots" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/bots")
			}
			var payload bot.CreateRequest
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload.Name != "alice" || payload.Role != "worker" || payload.Channel != "csgclaw" {
				t.Fatalf("payload = %+v, want alice worker csgclaw", payload)
			}
			if payload.Description != "test lead" || payload.ModelID != "gpt-test" {
				t.Fatalf("payload = %+v, want description/model_id", payload)
			}
			return jsonResponse(http.StatusCreated, `{"id":"u-alice","name":"alice","role":"worker","channel":"csgclaw","agent_id":"u-alice","user_id":"u-alice","created_at":"2026-04-12T09:00:00Z"}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "bot", "create", "--name", "alice", "--description", "test lead", "--role", "worker", "--model-id", "gpt-test"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "worker", "csgclaw", "u-alice", "u-alice")
}

func TestExecuteBotCreateFeishuSendsChannelPayload(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			var payload bot.CreateRequest
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload.ID != "u-alice" || payload.Name != "alice" || payload.Role != "worker" || payload.Channel != "feishu" {
				t.Fatalf("payload = %+v, want u-alice alice worker feishu", payload)
			}
			return jsonResponse(http.StatusCreated, `{"id":"u-alice","name":"alice","role":"worker","channel":"feishu","agent_id":"u-alice","user_id":"u-alice","created_at":"2026-04-12T09:00:00Z"}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "bot", "create", "--id", "u-alice", "--name", "alice", "--role", "worker", "--channel", "feishu"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "u-alice"`) || !strings.Contains(stdout.String(), `"channel": "feishu"`) {
		t.Fatalf("stdout = %q, want JSON feishu bot payload", stdout.String())
	}
}

func TestExecuteBotCreateRequiresNameAndRole(t *testing.T) {
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "bot", "create", "--role", "worker"})
	if err == nil || !strings.Contains(err.Error(), "requires --name") {
		t.Fatalf("Execute(missing name) error = %v, want --name error", err)
	}

	err = app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "bot", "create", "--name", "alice"})
	if err == nil || !strings.Contains(err.Error(), "requires --role") {
		t.Fatalf("Execute(missing role) error = %v, want --role error", err)
	}
}

func TestExecuteMessageSendsToDefaultChannel(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/messages" {
				t.Fatalf("url = %q, want csgclaw messages route", req.URL.String())
			}
			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["room_id"] != "room-1" || payload["sender_id"] != "u-admin" || payload["content"] != "hello" {
				t.Fatalf("payload = %#v, want room/sender/content", payload)
			}
			return jsonResponse(http.StatusCreated, `{"id":"msg-1","sender_id":"u-admin","kind":"message","content":"hello","created_at":"2026-04-12T09:00:00Z","mentions":[]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "message", "--room-id", "room-1", "--sender-id", "u-admin", "--content", "hello"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "msg-1", "u-admin", "message", "hello")
}

func TestExecuteMessageSendsToFeishuChannel(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/messages" {
				t.Fatalf("url = %q, want feishu messages route", req.URL.String())
			}
			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["room_id"] != "oc_alpha" || payload["sender_id"] != "u-manager" || payload["content"] != "hello" || payload["mention_id"] != "u-dev" {
				t.Fatalf("payload = %#v, want room/sender/content/mention_id", payload)
			}
			return jsonResponse(http.StatusCreated, `{"id":"om_1","sender_id":"u-manager","kind":"message","content":"hello","created_at":"2026-04-12T09:00:00Z","mentions":[]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "message", "--channel", "feishu", "--room-id", "oc_alpha", "--sender-id", "u-manager", "--content", "hello", "--mention-id", "u-dev"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "om_1"`) || !strings.Contains(stdout.String(), `"sender_id": "u-manager"`) {
		t.Fatalf("stdout = %q, want JSON message payload", stdout.String())
	}
}

func TestRenderAgentsTableAlignsLongColumns(t *testing.T) {
	var buf bytes.Buffer
	agents := []agent.Agent{
		{ID: "u-manager", Name: "manager", Role: "manager", Status: "running", Profile: "codex-main"},
		{ID: "u-dev", Name: "dev", Role: "worker", Status: "running", Profile: "claude-main"},
		{ID: "u-alex", Name: "alex", Role: "worker", Status: "running"},
	}

	if err := renderAgentsTable(&buf, agents); err != nil {
		t.Fatalf("renderAgentsTable() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("line count = %d, want 4; output=%q", len(lines), buf.String())
	}

	re := regexp.MustCompile(`^(\S+)(\s{2,})(\S+)(\s{2,})(\S+)(\s{2,})(\S+)(\s{2,})(\S+)$`)
	if re.FindStringSubmatchIndex(lines[0]) == nil {
		t.Fatalf("header not aligned: %q", lines[0])
	}
	wantStarts := fieldStartIndexes(lines[0])
	for i, line := range lines[1:] {
		if re.FindStringSubmatchIndex(line) == nil {
			t.Fatalf("row %d not aligned: %q", i+1, line)
		}
		if gotStarts := fieldStartIndexes(line); !slicesEqualInts(gotStarts, wantStarts) {
			t.Fatalf("row %d columns not aligned with header:\nheader=%q\nrow=%q", i+1, lines[0], line)
		}
	}
}

func TestRenderAgentsTableUsesDashForMissingProfile(t *testing.T) {
	var buf bytes.Buffer

	if err := renderAgentsTable(&buf, []agent.Agent{{ID: "u-alice", Name: "alice", Role: "worker", Status: "running"}}); err != nil {
		t.Fatalf("renderAgentsTable() error = %v", err)
	}

	assertTableHasRow(t, buf.String(), "u-alice", "alice", "worker", "running", "-")
}

func TestExecuteAgentCreateUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
			}
			if got := req.Header.Get("Authorization"); got != "Bearer secret-token" {
				t.Fatalf("Authorization = %q, want %q", got, "Bearer secret-token")
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["name"] != "alice" {
				t.Fatalf("payload[name] = %#v, want %q", payload["name"], "alice")
			}
			if payload["description"] != "worker" {
				t.Fatalf("payload[description] = %#v, want %q", payload["description"], "worker")
			}
			if payload["profile"] != "cliproxy-codex" {
				t.Fatalf("payload[profile] = %#v, want %q", payload["profile"], "cliproxy-codex")
			}

			return jsonResponse(http.StatusCreated, `{"id":"u-alice","name":"alice","role":"worker","status":"running","created_at":"2026-04-01T12:00:00Z","profile":"codex-main"}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--token", "secret-token", "agent", "create", "--name", "alice", "--description", "worker", "--profile", "cliproxy-codex"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "worker", "running", "codex-main")
}

func TestExecuteAgentDeleteUsesHTTPClient(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			if req.URL.String() != "http://example.test/api/v1/agents/u-alice" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents/u-alice")
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Status:     http.StatusText(http.StatusNoContent),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "delete", "u-alice"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteAgentStatusByIDUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://example.test/api/v1/agents/u-alice" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents/u-alice")
			}
			return jsonResponse(http.StatusOK, `{"id":"u-alice","name":"alice","role":"worker","status":"running","created_at":"2026-04-01T12:00:00Z","profile":"codex-main"}`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "status", "u-alice"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "worker", "running", "codex-main")
}

func TestExecuteAgentLogsUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/agents/u-alice/logs?follow=true&lines=80" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents/u-alice/logs?follow=true&lines=80")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("line-1\nline-2\n")),
			}, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "logs", "u-alice", "-f", "-n", "80"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout.String() != "line-1\nline-2\n" {
		t.Fatalf("stdout = %q, want streamed logs", stdout.String())
	}
}

func TestExecuteRoomListUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/rooms" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/rooms")
			}
			return jsonResponse(http.StatusOK, `[{"id":"room-1","title":"alpha","participants":["u-admin","u-alice"],"messages":[{"id":"msg-1"}]}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "room", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "room-1", "alpha", "2", "1")
}

func TestExecuteRoomListFeishuUsesChannelRoute(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/rooms" {
				t.Fatalf("url = %q, want feishu rooms route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"fsroom-1","title":"alpha","participants":["fsu-admin"],"messages":[]}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "room", "list", "--channel", "feishu"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "fsroom-1", "alpha", "1", "0")
}

func TestExecuteUserCreateFeishuUsesChannelRoute(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/users" {
				t.Fatalf("url = %q, want feishu users route", req.URL.String())
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["name"] != "Alice" || payload["handle"] != "alice" {
				t.Fatalf("payload = %#v, want Alice/alice", payload)
			}
			return jsonResponse(http.StatusCreated, `{"id":"fsu-alice","name":"Alice","handle":"alice","role":"worker","is_online":true}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "user", "create", "--channel", "feishu", "--name", "Alice", "--handle", "alice", "--role", "worker"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "fsu-alice", "Alice", "alice", "worker", "true")
}

func TestExecuteMemberCreateFeishuUsesChannelRoomMembersRoute(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/rooms/oc_alpha/members" {
				t.Fatalf("url = %q, want feishu room members route", req.URL.String())
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			userIDs, ok := payload["user_ids"].([]any)
			if !ok || len(userIDs) != 1 || userIDs[0] != "ou_alice" {
				t.Fatalf("payload[user_ids] = %#v, want [ou_alice]", payload["user_ids"])
			}
			if payload["inviter_id"] != "u-manager" {
				t.Fatalf("payload[inviter_id] = %#v, want u-manager", payload["inviter_id"])
			}
			return jsonResponse(http.StatusOK, `{"id":"oc_alpha","title":"alpha","participants":["u-manager","ou_alice"],"messages":[]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "member", "create", "--channel", "feishu", "--room-id", "oc_alpha", "--user-id", "ou_alice", "--inviter-id", "u-manager"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "oc_alpha", "alpha", "2", "0")
}

func TestExecuteMemberListFeishuUsesChannelRoomMembersRoute(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/rooms/oc_alpha/members" {
				t.Fatalf("url = %q, want feishu room members route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"ou_alice","name":"Alice","handle":"alice","role":"worker","is_online":true}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "member", "list", "--channel", "feishu", "--room-id", "oc_alpha"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "ou_alice", "Alice", "alice", "worker", "true")
}

func TestExecuteMemberListCsgclawUnsupported(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}),
	}

	err := app.Execute(context.Background(), []string{"member", "list", "--channel", "csgclaw", "--room-id", "room-1"})
	if err == nil {
		t.Fatal("Execute() error = nil, want unsupported channel")
	}
	if !strings.Contains(err.Error(), "currently supports --channel feishu") {
		t.Fatalf("Execute() error = %v, want feishu-only error", err)
	}
}

func TestExecuteMemberCreateCsgclawUnsupported(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}),
	}

	err := app.Execute(context.Background(), []string{"member", "create", "--channel", "csgclaw", "--room-id", "room-1", "--user-id", "u-alice"})
	if err == nil {
		t.Fatal("Execute() error = nil, want unsupported channel")
	}
	if !strings.Contains(err.Error(), "currently supports --channel feishu") {
		t.Fatalf("Execute() error = %v, want feishu-only error", err)
	}
}

func TestExecuteRoomCreateUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/rooms" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/rooms")
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["title"] != "alpha" {
				t.Fatalf("payload[title] = %#v, want %q", payload["title"], "alpha")
			}
			if payload["creator_id"] != "u-admin" {
				t.Fatalf("payload[creator_id] = %#v, want %q", payload["creator_id"], "u-admin")
			}
			participantIDs, ok := payload["participant_ids"].([]any)
			if !ok || len(participantIDs) != 2 || participantIDs[0] != "u-alice" || participantIDs[1] != "u-bob" {
				t.Fatalf("payload[participant_ids] = %#v, want [u-alice u-bob]", payload["participant_ids"])
			}

			return jsonResponse(http.StatusCreated, `{"id":"room-1","title":"alpha","participants":["u-admin","u-alice","u-bob"],"messages":[{"id":"msg-1"}]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "room", "create", "--title", "alpha", "--creator-id", "u-admin", "--participant-ids", "u-alice, u-bob"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"title": "alpha"`) {
		t.Fatalf("stdout = %q, want JSON room payload", stdout.String())
	}
}

func TestExecuteRoomDeleteUsesHTTPClient(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			if req.URL.String() != "http://example.test/api/v1/rooms/room-1" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/rooms/room-1")
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Status:     http.StatusText(http.StatusNoContent),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "room", "delete", "room-1"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteUserListUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/users" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/users")
			}
			return jsonResponse(http.StatusOK, `[{"id":"u-alice","name":"Alice","handle":"alice","role":"Worker","is_online":true}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "user", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "Alice", "alice", "Worker", "true")
}

func TestExecuteUserKickUsesHTTPClient(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			if req.URL.String() != "http://example.test/api/v1/users/u-alice" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/users/u-alice")
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Status:     http.StatusText(http.StatusNoContent),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "user", "kick", "u-alice"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestUsageIncludesTopLevelCommandIndex(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), nil)
	if err != flag.ErrHelp {
		t.Fatalf("Execute() error = %v, want %v", err, flag.ErrHelp)
	}

	got := stderr.String()
	for _, want := range []string{
		"Available Commands:",
		"agent    Manage agents",
		"bot      Manage bots",
		"room     Manage IM rooms",
		"member   Manage IM room members",
		"user     Manage IM users",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage = %q, want substring %q", got, want)
		}
	}
}

func TestRootHelpIncludesAvailableCommands(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"-h"})
	if err != flag.ErrHelp {
		t.Fatalf("Execute() error = %v, want %v", err, flag.ErrHelp)
	}

	got := stderr.String()
	for _, want := range []string{
		"Available Commands:",
		"agent    Manage agents",
		"bot      Manage bots",
		"room     Manage IM rooms",
		"member   Manage IM room members",
		"user     Manage IM users",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help = %q, want substring %q", got, want)
		}
	}
}

func TestExecuteVersionFlagPrintsVersion(t *testing.T) {
	originalVersion := appversion.Version
	appversion.Version = "1.2.3-test"
	t.Cleanup(func() { appversion.Version = originalVersion })

	var stdout bytes.Buffer
	app := &App{
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	if err := app.Execute(context.Background(), []string{"--version"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := strings.TrimSpace(stdout.String()), "csgclaw version 1.2.3-test"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestExecuteVersionFlagShortFormPrintsVersion(t *testing.T) {
	originalVersion := appversion.Version
	appversion.Version = "1.2.3-test"
	t.Cleanup(func() { appversion.Version = originalVersion })

	var stdout bytes.Buffer
	app := &App{
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	if err := app.Execute(context.Background(), []string{"-V"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := strings.TrimSpace(stdout.String()), "csgclaw version 1.2.3-test"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestAgentHelpIncludesSubcommands(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"agent", "-h"})
	if err != flag.ErrHelp {
		t.Fatalf("Execute() error = %v, want %v", err, flag.ErrHelp)
	}

	got := stderr.String()
	for _, want := range []string{
		"Manage agents.",
		"csgclaw agent <subcommand> [flags]",
		"list               List agents",
		"create             Create an agent",
		"delete <id>        Delete an agent",
		"status [id]        Show one agent or list all agents",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help = %q, want substring %q", got, want)
		}
	}
}

func TestBotHelpIncludesSubcommands(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"bot", "-h"})
	if err != flag.ErrHelp {
		t.Fatalf("Execute() error = %v, want %v", err, flag.ErrHelp)
	}

	got := stderr.String()
	for _, want := range []string{
		"Manage bots.",
		"csgclaw bot <subcommand> [flags]",
		"list               List bots",
		"create             Create a bot",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help = %q, want substring %q", got, want)
		}
	}
}

func TestAgentSubcommandHelpIncludesUsageAndFlags(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"agent", "list", "-h"})
	if err != flag.ErrHelp {
		t.Fatalf("Execute() error = %v, want %v", err, flag.ErrHelp)
	}

	got := stderr.String()
	for _, want := range []string{
		"List agents.",
		"csgclaw agent list [flags]",
		"Flags:",
		"-filter string",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help = %q, want substring %q", got, want)
		}
	}
}

func TestExecuteStartIsRejected(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"--config", "/tmp/test.toml", "start"})
	if err == nil {
		t.Fatal("Execute() error = nil, want unknown command")
	}
	if !strings.Contains(err.Error(), `unknown command "start"`) {
		t.Fatalf("Execute() error = %v, want unknown command start", err)
	}
	if !strings.Contains(stderr.String(), "  serve    Start the local HTTP server") {
		t.Fatalf("stderr = %q, want serve command in usage", stderr.String())
	}
}

func TestExecutePluralRoomAndUserCommandsAreRejected(t *testing.T) {
	for _, command := range []string{"rooms", "users"} {
		t.Run(command, func(t *testing.T) {
			app := &App{
				stdout:     &bytes.Buffer{},
				stderr:     &bytes.Buffer{},
				httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
			}

			err := app.Execute(context.Background(), []string{command, "list"})
			if err == nil {
				t.Fatal("Execute() error = nil, want unknown command")
			}
			if !strings.Contains(err.Error(), `unknown command "`+command+`"`) {
				t.Fatalf("Execute() error = %v, want unknown command %q", err, command)
			}
		})
	}
}

func TestRunStopRejectsInvalidPIDFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := &App{
		stdout:     &stdout,
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	pidPath := filepath.Join(t.TempDir(), "server.pid")
	if err := os.WriteFile(pidPath, []byte("abc\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	err := app.Execute(context.Background(), []string{"stop", "--pid", pidPath})
	if err == nil {
		t.Fatal("Execute() error = nil, want parse failure")
	}
	if !strings.Contains(err.Error(), "parse pid file") {
		t.Fatalf("Execute() error = %q, want parse pid file", err)
	}
}

func TestExecuteAgentStatusErrorOmitsHTTPStatusCode(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusNotFound, "agent not found\n"), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "status", "missing"})
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != "agent not found" {
		t.Fatalf("Execute() error = %q, want %q", err.Error(), "agent not found")
	}
	if strings.Contains(strings.ToLower(err.Error()), "http") || strings.Contains(err.Error(), "404") {
		t.Fatalf("Execute() error = %q, should not include HTTP status details", err.Error())
	}
}

func TestExecuteAgentStatusErrorUsesJSONMessageWithoutHTTPStatusCode(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusBadRequest, `{"error":"invalid agent id"}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "status", "bad/id"})
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if err.Error() != "invalid agent id" {
		t.Fatalf("Execute() error = %q, want %q", err.Error(), "invalid agent id")
	}
	if strings.Contains(strings.ToLower(err.Error()), "http") || strings.Contains(err.Error(), "400") {
		t.Fatalf("Execute() error = %q, should not include HTTP status details", err.Error())
	}
}

func assertTableHasRow(t *testing.T, table string, want ...string) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(table), "\n") {
		if slicesEqual(strings.Fields(line), want) {
			return
		}
	}
	t.Fatalf("table = %q, want row %v", table, want)
}

func slicesEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func fieldStartIndexes(line string) []int {
	var starts []int
	inField := false
	for i, r := range line {
		if r != ' ' && !inField {
			starts = append(starts, i)
			inField = true
			continue
		}
		if r == ' ' {
			inField = false
		}
	}
	return starts
}

func slicesEqualInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
