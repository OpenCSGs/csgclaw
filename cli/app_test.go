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

	"csgclaw/internal/apitypes"
	"csgclaw/internal/bot"
	"csgclaw/internal/channel"
	appversion "csgclaw/internal/version"
)

func TestExecuteAgentListUsesHTTPClientJSON(t *testing.T) {
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

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "agent", "list"})
	if err != nil {
		t.Fatalf("Execute() error = %v; stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id": "u-alice"`) {
		t.Fatalf("stdout = %q, want JSON agent payload", stdout.String())
	}
}

func TestExecuteDefaultsToJSONOutputForNonTerminalStdout(t *testing.T) {
	stdout, err := os.CreateTemp(t.TempDir(), "stdout-*")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	defer stdout.Close()

	app := &App{
		stdout: stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
			}
			return jsonResponse(http.StatusOK, `[{"id":"u-alice","name":"alice","role":"worker","status":"running","created_at":"2026-04-01T12:00:00Z"}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatalf("ReadFile(stdout) error = %v", err)
	}
	if !strings.Contains(string(got), `"id": "u-alice"`) {
		t.Fatalf("stdout = %q, want JSON agent payload", string(got))
	}
}

func TestExecuteUsesEnvironmentForEndpointAndToken(t *testing.T) {
	t.Setenv(envBaseURL, "http://env.example.test")
	t.Setenv(envAccessToken, "env-secret-token")

	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://env.example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://env.example.test/api/v1/agents")
			}
			if got := req.Header.Get("Authorization"); got != "Bearer env-secret-token" {
				t.Fatalf("Authorization = %q, want %q", got, "Bearer env-secret-token")
			}
			return jsonResponse(http.StatusOK, `[]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"agent", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteFlagsOverrideEnvironmentForEndpointAndToken(t *testing.T) {
	t.Setenv(envBaseURL, "http://env.example.test")
	t.Setenv(envAccessToken, "env-secret-token")

	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://flag.example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://flag.example.test/api/v1/agents")
			}
			if got := req.Header.Get("Authorization"); got != "Bearer flag-secret-token" {
				t.Fatalf("Authorization = %q, want %q", got, "Bearer flag-secret-token")
			}
			return jsonResponse(http.StatusOK, `[]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{
		"--endpoint", "http://flag.example.test",
		"--token", "flag-secret-token",
		"agent", "list",
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
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
			return jsonResponse(http.StatusOK, `[{"id":"u-alice","name":"alice","role":"worker","status":"running","image":"ghcr.io/opencsg/csgclaw-agent:2026.4.28","created_at":"2026-04-01T12:00:00Z","profile":"codex-main"}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "worker", "running", "codex-main", "ghcr.io/opencsg/csgclaw-agent:2026.4.28")
}

func TestExecuteAgentStartUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/agents/u-alice/start" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents/u-alice/start")
			}
			return jsonResponse(http.StatusOK, `{"id":"u-alice","name":"alice","role":"worker","status":"running","image":"ghcr.io/opencsg/csgclaw-agent:2026.4.28","created_at":"2026-04-01T12:00:00Z","profile":"codex-main"}`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "start", "u-alice"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "worker", "running", "codex-main", "ghcr.io/opencsg/csgclaw-agent:2026.4.28")
}

func TestExecuteAgentStopUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/agents/u-alice/stop" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents/u-alice/stop")
			}
			return jsonResponse(http.StatusOK, `{"id":"u-alice","name":"alice","role":"worker","status":"stopped","image":"ghcr.io/opencsg/csgclaw-agent:2026.4.28","created_at":"2026-04-01T12:00:00Z","profile":"codex-main"}`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "stop", "u-alice"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "worker", "stopped", "codex-main", "ghcr.io/opencsg/csgclaw-agent:2026.4.28")
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
			return jsonResponse(http.StatusOK, `[{"id":"bot-alice","name":"alice","role":"worker","channel":"csgclaw","agent_id":"u-alice","user_id":"u-alice","available":true,"created_at":"2026-04-12T09:00:00Z"}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "bot", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "bot-alice", "alice", "-", "worker", "csgclaw", "u-alice", "u-alice", "true")
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

func TestExecuteBotListUsesRoleQuery(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/bots?channel=csgclaw&role=worker" {
				t.Fatalf("url = %q, want role-filtered bot list route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"bot-alice","name":"alice","description":"abcdefghijklmnopqrstuvwxyz1234567890ABCDE","role":"worker","channel":"csgclaw","agent_id":"u-alice","user_id":"u-alice","available":true,"created_at":"2026-04-12T09:00:00Z"}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "bot", "list", "--role", "worker"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "bot-alice", "alice", "abcdefghijklmnopqrstuvwxyz1234567890ABCD...", "worker", "csgclaw", "u-alice", "u-alice", "true")
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
			return jsonResponse(http.StatusCreated, `{"id":"u-alice","name":"alice","description":"test-lead","role":"worker","channel":"csgclaw","agent_id":"u-alice","user_id":"u-alice","available":true,"created_at":"2026-04-12T09:00:00Z"}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "bot", "create", "--name", "alice", "--description", "test lead", "--role", "worker", "--model-id", "gpt-test"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "test-lead", "worker", "csgclaw", "u-alice", "u-alice", "true")
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

func TestExecuteBotDeleteUsesDefaultChannel(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			if req.URL.String() != "http://example.test/api/v1/bots/u-alice?channel=csgclaw" {
				t.Fatalf("url = %q, want csgclaw bot delete route", req.URL.String())
			}
			return jsonResponse(http.StatusNoContent, ``), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "bot", "delete", "u-alice"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteBotDeleteFeishuUsesChannelQuery(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			if req.URL.String() != "http://example.test/api/v1/bots/u-alice?channel=feishu" {
				t.Fatalf("url = %q, want feishu bot delete route", req.URL.String())
			}
			return jsonResponse(http.StatusNoContent, ``), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "bot", "delete", "--channel", "feishu", "u-alice"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteBotDeleteSupportsJSONOutput(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			return jsonResponse(http.StatusNoContent, ``), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "bot", "delete", "--channel", "feishu", "u-alice"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{`"command": "bot"`, `"action": "delete"`, `"status": "deleted"`, `"id": "u-alice"`, `"channel": "feishu"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %s", stdout.String(), want)
		}
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

func TestExecuteMessageCreateSendsToDefaultChannel(t *testing.T) {
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

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "message", "create", "--room-id", "room-1", "--sender-id", "u-admin", "--content", "hello"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "msg-1", "u-admin", "message", "hello")
}

func TestExecuteMessageCreateSendsMentionIDToDefaultChannel(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/messages" {
				t.Fatalf("url = %q, want default messages route", req.URL.String())
			}
			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["room_id"] != "room-1" || payload["sender_id"] != "u-admin" || payload["content"] != "hi" || payload["mention_id"] != "u-dev" {
				t.Fatalf("payload = %#v, want room/sender/content/mention_id", payload)
			}
			return jsonResponse(http.StatusCreated, `{"id":"msg-1","sender_id":"u-admin","kind":"message","content":"<at user_id=\"u-dev\">dev</at> hi","created_at":"2026-04-12T09:00:00Z","mentions":["u-dev"]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "message", "create", "--channel", "csgclaw", "--room-id", "room-1", "--sender-id", "u-admin", "--content", "hi", "--mention-id", "u-dev"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "msg-1") || !strings.Contains(stdout.String(), `<at user_id="u-dev">dev</at> hi`) {
		t.Fatalf("stdout = %q, want message with mention-prefixed content", stdout.String())
	}
}

func TestExecuteMessageCreateSendsToFeishuChannel(t *testing.T) {
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

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "message", "create", "--channel", "feishu", "--room-id", "oc_alpha", "--sender-id", "u-manager", "--content", "hello", "--mention-id", "u-dev"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "om_1"`) || !strings.Contains(stdout.String(), `"sender_id": "u-manager"`) {
		t.Fatalf("stdout = %q, want JSON message payload", stdout.String())
	}
}

func TestExecuteMessageListReadsFromDefaultChannel(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/messages?room_id=room-1" {
				t.Fatalf("url = %q, want csgclaw message list route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"msg-1","room_id":"room-1","sender_id":"u-admin","kind":"message","content":"hello","created_at":"2026-04-12T09:00:00Z","mentions":[]}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "message", "list", "--room-id", "room-1"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "msg-1", "u-admin", "message", "hello")
}

func TestExecuteMessageListReadsFromFeishuChannel(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/messages?room_id=oc_alpha" {
				t.Fatalf("url = %q, want feishu message list route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"om_1","room_id":"oc_alpha","sender_id":"ou_manager","kind":"message","content":"hello","created_at":"2026-04-12T09:00:00Z","mentions":[]}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "message", "list", "--channel", "feishu", "--room-id", "oc_alpha"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "om_1"`) || !strings.Contains(stdout.String(), `"sender_id": "ou_manager"`) {
		t.Fatalf("stdout = %q, want JSON message list payload", stdout.String())
	}
}

func TestRenderAgentsTableAlignsLongColumns(t *testing.T) {
	var buf bytes.Buffer
	agents := []apitypes.Agent{
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

	re := regexp.MustCompile(`^(\S+)(\s{2,})(\S+)(\s{2,})(\S+)(\s{2,})(\S+)(\s{2,})(\S+)(\s{2,})(\S+)$`)
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

	if err := renderAgentsTable(&buf, []apitypes.Agent{{ID: "u-alice", Name: "alice", Role: "worker", Status: "running"}}); err != nil {
		t.Fatalf("renderAgentsTable() error = %v", err)
	}

	assertTableHasRow(t, buf.String(), "u-alice", "alice", "worker", "running", "-", "-")
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
			if payload["image"] != "ghcr.io/opencsg/csgclaw-agent:2026.4.28" {
				t.Fatalf("payload[image] = %#v, want %q", payload["image"], "ghcr.io/opencsg/csgclaw-agent:2026.4.28")
			}
			if payload["profile"] != "cliproxy-codex" {
				t.Fatalf("payload[profile] = %#v, want %q", payload["profile"], "cliproxy-codex")
			}

			return jsonResponse(http.StatusCreated, `{"id":"u-alice","name":"alice","role":"worker","status":"running","image":"ghcr.io/opencsg/csgclaw-agent:2026.4.28","created_at":"2026-04-01T12:00:00Z","profile":"codex-main"}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{
		"--endpoint", "http://example.test",
		"--token", "secret-token",
		"agent", "create",
		"--name", "alice",
		"--description", "worker",
		"--image", "ghcr.io/opencsg/csgclaw-agent:2026.4.28",
		"--profile", "cliproxy-codex",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "worker", "running", "codex-main", "ghcr.io/opencsg/csgclaw-agent:2026.4.28")
}

func TestExecuteAgentCreateReplaceKeepsExistingFieldsWhenNoOverrides(t *testing.T) {
	var stdout bytes.Buffer
	requests := 0
	app := &App{
		stdin:  strings.NewReader("y\n"),
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["id"] != "u-alice" {
				t.Fatalf("payload[id] = %#v, want %q", payload["id"], "u-alice")
			}
			if _, ok := payload["description"]; ok {
				t.Fatalf("payload should omit description when not explicitly set, got %#v", payload["description"])
			}
			if _, ok := payload["image"]; ok {
				t.Fatalf("payload should omit image when not explicitly set, got %#v", payload["image"])
			}
			if _, ok := payload["profile"]; ok {
				t.Fatalf("payload should omit profile when not explicitly set, got %#v", payload["profile"])
			}
			assertStringSliceValue(t, payload["field_mask"], []string{"id"})

			return jsonResponse(http.StatusCreated, `{"id":"u-alice","name":"alice","description":"worker","image":"ghcr.io/opencsg/csgclaw-agent:2026.4.20","role":"worker","status":"running","created_at":"2026-04-01T12:00:00Z","profile":"codex-main"}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "create", "-r", "--id", "u-alice"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "This will recreate agent u-alice in place. Are you sure? [y/N] ") {
		t.Fatalf("stdout = %q, want confirmation prompt", stdout.String())
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "worker", "running", "codex-main", "ghcr.io/opencsg/csgclaw-agent:2026.4.20")
}

func TestExecuteAgentCreateReplaceOverridesExplicitFields(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdin:  strings.NewReader("y\n"),
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["id"] != "u-alice" {
				t.Fatalf("payload[id] = %#v, want %q", payload["id"], "u-alice")
			}
			if payload["name"] != "alice-v2" {
				t.Fatalf("payload[name] = %#v, want %q", payload["name"], "alice-v2")
			}
			if _, ok := payload["description"]; ok {
				t.Fatalf("payload should omit description when explicitly cleared, got %#v", payload["description"])
			}
			if payload["image"] != "ghcr.io/opencsg/csgclaw-agent:2026.4.28" {
				t.Fatalf("payload[image] = %#v, want %q", payload["image"], "ghcr.io/opencsg/csgclaw-agent:2026.4.28")
			}
			if payload["replace"] != true {
				t.Fatalf("payload[replace] = %#v, want true", payload["replace"])
			}
			if payload["profile"] != "codex-fast" {
				t.Fatalf("payload[profile] = %#v, want %q", payload["profile"], "codex-fast")
			}
			assertStringSliceValue(t, payload["field_mask"], []string{"id", "name", "description", "image", "profile"})

			return jsonResponse(http.StatusCreated, `{"id":"u-alice","name":"alice-v2","role":"worker","status":"running","image":"ghcr.io/opencsg/csgclaw-agent:2026.4.28","created_at":"2026-04-01T12:00:00Z","profile":"codex-fast"}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{
		"--endpoint", "http://example.test",
		"agent", "create",
		"--replace",
		"--id", "u-alice",
		"--name", "alice-v2",
		"--description", "",
		"--image", "ghcr.io/opencsg/csgclaw-agent:2026.4.28",
		"--profile", "codex-fast",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice-v2", "worker", "running", "codex-fast", "ghcr.io/opencsg/csgclaw-agent:2026.4.28")
}

func TestExecuteAgentCreateReplaceCancelledWithoutForce(t *testing.T) {
	var stdout bytes.Buffer
	requests := 0
	app := &App{
		stdin:  strings.NewReader("n\n"),
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			t.Fatalf("unexpected request #%d: %s %s", requests, req.Method, req.URL.String())
			return nil, nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "create", "--replace", "--id", "u-alice"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
	if !strings.Contains(stdout.String(), "cancelled replacing agent u-alice") {
		t.Fatalf("stdout = %q, want cancellation message", stdout.String())
	}
}

func TestExecuteAgentCreateReplaceForceSkipsConfirmation(t *testing.T) {
	var stdout bytes.Buffer
	requests := 0
	app := &App{
		stdin:  strings.NewReader("n\n"),
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
			}
			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["replace"] != true {
				t.Fatalf("payload[replace] = %#v, want true", payload["replace"])
			}
			assertStringSliceValue(t, payload["field_mask"], []string{"id"})
			return jsonResponse(http.StatusCreated, `{"id":"u-alice","name":"alice","description":"worker","image":"ghcr.io/opencsg/csgclaw-agent:2026.4.20","role":"worker","status":"running","created_at":"2026-04-01T12:00:00Z","profile":"codex-main"}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "create", "--replace", "--force", "--id", "u-alice"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(stdout.String(), "Are you sure?") {
		t.Fatalf("stdout = %q, want no confirmation prompt", stdout.String())
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "worker", "running", "codex-main", "ghcr.io/opencsg/csgclaw-agent:2026.4.20")
}

func TestExecuteAgentCreateReplaceRequiresExistingAgent(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
			}
			return jsonResponse(http.StatusNotFound, "agent \"u-missing\" not found\n"), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "create", "--replace", "--force", "--id", "u-missing"})
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `agent "u-missing" not found`) {
		t.Fatalf("Execute() error = %v, want missing agent error", err)
	}
}

func TestExecuteAgentDeleteUsesHTTPClient(t *testing.T) {
	requests := 0
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
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
	if requests != 1 {
		t.Fatalf("requests = %d, want one delete-only request", requests)
	}
}

func TestExecuteAgentDeleteManagerAliasUsesManagerID(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			if req.URL.String() != "http://example.test/api/v1/agents/u-manager" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents/u-manager")
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Status:     http.StatusText(http.StatusNoContent),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "delete", "manager"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteAgentCreateReplaceManagerAliasUsesManagerID(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdin:  strings.NewReader("y\n"),
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["id"] != "u-manager" {
				t.Fatalf("payload[id] = %#v, want %q", payload["id"], "u-manager")
			}
			if payload["replace"] != true {
				t.Fatalf("payload[replace] = %#v, want true", payload["replace"])
			}
			assertStringSliceValue(t, payload["field_mask"], []string{"id"})
			return jsonResponse(http.StatusCreated, `{"id":"u-manager","name":"manager","role":"manager","status":"running","created_at":"2026-04-01T12:00:00Z","profile":"codex-main"}`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "create", "--replace", "--id", "manager"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "This will recreate agent u-manager in place. Are you sure? [y/N] ") {
		t.Fatalf("stdout = %q, want manager confirmation prompt", stdout.String())
	}
}

func TestExecuteAgentCreateReplaceManagerSendsImageOverride(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/agents" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["id"] != "u-manager" {
				t.Fatalf("payload[id] = %#v, want %q", payload["id"], "u-manager")
			}
			if payload["image"] != "manager-image:2" {
				t.Fatalf("payload[image] = %#v, want %q", payload["image"], "manager-image:2")
			}
			assertStringSliceValue(t, payload["field_mask"], []string{"id", "image"})
			return jsonResponse(http.StatusCreated, `{"id":"u-manager","name":"manager","role":"manager","status":"running","image":"manager-image:2","created_at":"2026-04-01T12:00:00Z","profile":"codex-main"}`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "create", "--replace", "--force", "--id", "manager", "--image", "manager-image:2"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteAgentDeleteAllUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	requests := 0
	app := &App{
		stdin:  strings.NewReader("y\n"),
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			switch requests {
			case 1:
				if req.Method != http.MethodGet {
					t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
				}
				if req.URL.String() != "http://example.test/api/v1/agents" {
					t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
				}
				return jsonResponse(http.StatusOK, `[{"id":"u-alice","name":"alice","role":"worker","status":"running"},{"id":"u-bob","name":"bob","role":"worker","status":"stopped"}]`), nil
			case 2:
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
			case 3:
				if req.Method != http.MethodDelete {
					t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
				}
				if req.URL.String() != "http://example.test/api/v1/agents/u-bob" {
					t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents/u-bob")
				}
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Status:     http.StatusText(http.StatusNoContent),
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			default:
				t.Fatalf("unexpected request #%d: %s %s", requests, req.Method, req.URL.String())
				return nil, nil
			}
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "delete", "--all"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "This will remove all agents. Are you sure? [y/N] ") {
		t.Fatalf("stdout = %q, want confirmation prompt", got)
	}
	if got := stdout.String(); !strings.Contains(got, "deleted 2 agents") {
		t.Fatalf("stdout = %q, want message about deleting 2 agents", got)
	}
}

func TestExecuteAgentDeleteAllCancelledWithoutConfirmation(t *testing.T) {
	var stdout bytes.Buffer
	requests := 0
	app := &App{
		stdin:  strings.NewReader("n\n"),
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "delete", "--all"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
	if got := stdout.String(); !strings.Contains(got, "This will remove all agents. Are you sure? [y/N] ") {
		t.Fatalf("stdout = %q, want confirmation prompt", got)
	}
	if got := stdout.String(); !strings.Contains(got, "cancelled deleting all agents") {
		t.Fatalf("stdout = %q, want cancellation message", got)
	}
}

func TestExecuteAgentDeleteAllForceSkipsConfirmation(t *testing.T) {
	var stdout bytes.Buffer
	requests := 0
	app := &App{
		stdin:  strings.NewReader("n\n"),
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			switch requests {
			case 1:
				if req.Method != http.MethodGet {
					t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
				}
				if req.URL.String() != "http://example.test/api/v1/agents" {
					t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/agents")
				}
				return jsonResponse(http.StatusOK, `[{"id":"u-alice","name":"alice","role":"worker","status":"running"}]`), nil
			case 2:
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
			default:
				t.Fatalf("unexpected request #%d: %s %s", requests, req.Method, req.URL.String())
				return nil, nil
			}
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "delete", "--all", "--force"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := stdout.String(); strings.Contains(got, "Are you sure?") {
		t.Fatalf("stdout = %q, want no confirmation prompt", got)
	}
	if got := stdout.String(); !strings.Contains(got, "deleted 1 agents") {
		t.Fatalf("stdout = %q, want delete message", got)
	}
}

func TestExecuteAgentDeleteAllRejectsID(t *testing.T) {
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "delete", "--all", "u-alice"})
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "does not accept an id") {
		t.Fatalf("error = %q, want id rejection", err)
	}
}

func TestExecuteAgentStatusIsRejected(t *testing.T) {
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "status", "u-alice"})
	if err == nil {
		t.Fatal("Execute() error = nil, want unknown subcommand")
	}
	if !strings.Contains(err.Error(), `unknown agent subcommand "status"`) {
		t.Fatalf("Execute() error = %v, want unknown status subcommand", err)
	}
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

func TestExecuteAgentLogsDefaultsToStreamForNonTerminalStdout(t *testing.T) {
	stdout, err := os.CreateTemp(t.TempDir(), "stdout-*")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	defer stdout.Close()

	app := &App{
		stdout: stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://example.test/api/v1/agents/u-alice/logs?lines=2" {
				t.Fatalf("url = %q, want logs route", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("line-1\nline-2\n")),
			}, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "agent", "logs", "u-alice", "-n", "2"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatalf("ReadFile(stdout) error = %v", err)
	}
	if string(got) != "line-1\nline-2\n" {
		t.Fatalf("stdout = %q, want streamed logs", string(got))
	}
}

func TestExecuteAgentLogsSupportsJSONOutputWithoutFollow(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://example.test/api/v1/agents/u-alice/logs?lines=2" {
				t.Fatalf("url = %q, want logs route", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("line-1\nline-2\n")),
			}, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "agent", "logs", "u-alice", "-n", "2"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{`"command": "agent"`, `"action": "logs"`, `"id": "u-alice"`, `"logs": "line-1\nline-2\n"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %s", stdout.String(), want)
		}
	}
}

func TestExecuteAgentLogsRejectsJSONFollow(t *testing.T) {
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "agent", "logs", "u-alice", "--follow"})
	if err == nil || !strings.Contains(err.Error(), "does not support --output json with --follow") {
		t.Fatalf("Execute() error = %v, want json follow error", err)
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
			return jsonResponse(http.StatusOK, `[{"id":"room-1","title":"alpha","members":["u-admin","u-alice"],"messages":[{"id":"msg-1"}]}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "room", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "room-1", "alpha", "false", "2", "1")
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
			return jsonResponse(http.StatusOK, `[{"id":"fsroom-1","title":"alpha","members":["fsu-admin"],"messages":[]}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "room", "list", "--channel", "feishu"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "fsroom-1", "alpha", "false", "1", "0")
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
			return jsonResponse(http.StatusOK, `{"id":"oc_alpha","title":"alpha","members":["u-manager","ou_alice"],"messages":[]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "member", "create", "--channel", "feishu", "--room-id", "oc_alpha", "--user-id", "ou_alice", "--inviter-id", "u-manager"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "oc_alpha", "alpha", "false", "2", "0")
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

func TestExecuteMemberListCsgclawUsesRoomMembersRoute(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/rooms/room-1/members" {
				t.Fatalf("url = %q, want csgclaw room members route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"u-alice","name":"Alice","handle":"alice","role":"worker","is_online":true}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "member", "list", "--channel", "csgclaw", "--room-id", "room-1"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "Alice", "alice", "worker", "true")
}

func TestExecuteMemberCreateCsgclawUsesRoomMembersRoute(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/rooms/room-1/members" {
				t.Fatalf("url = %q, want csgclaw room members route", req.URL.String())
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["room_id"] != "room-1" {
				t.Fatalf("payload[room_id] = %#v, want room-1", payload["room_id"])
			}
			userIDs, ok := payload["user_ids"].([]any)
			if !ok || len(userIDs) != 1 || userIDs[0] != "u-alice" {
				t.Fatalf("payload[user_ids] = %#v, want [u-alice]", payload["user_ids"])
			}
			if payload["inviter_id"] != "u-admin" {
				t.Fatalf("payload[inviter_id] = %#v, want u-admin", payload["inviter_id"])
			}
			return jsonResponse(http.StatusOK, `{"id":"room-1","title":"Ops","members":["u-admin","u-alice"],"messages":[]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "member", "create", "--channel", "csgclaw", "--room-id", "room-1", "--user-id", "u-alice", "--inviter-id", "u-admin"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "room-1", "Ops", "false", "2", "0")
}

func TestExecuteMemberCreateUsesCsgclawDefault(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/rooms/room-1/members" {
				t.Fatalf("url = %q, want csgclaw room members route", req.URL.String())
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["room_id"] != "room-1" {
				t.Fatalf("payload[room_id] = %#v, want room-1", payload["room_id"])
			}
			userIDs, ok := payload["user_ids"].([]any)
			if !ok || len(userIDs) != 1 || userIDs[0] != "u-alice" {
				t.Fatalf("payload[user_ids] = %#v, want [u-alice]", payload["user_ids"])
			}
			return jsonResponse(http.StatusOK, `{"id":"room-1","title":"Ops","members":["u-admin","u-alice"],"messages":[]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "member", "create", "--room-id", "room-1", "--user-id", "u-alice", "--inviter-id", "u-admin"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "room-1", "Ops", "false", "2", "0")
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
			memberIDs, ok := payload["member_ids"].([]any)
			if !ok || len(memberIDs) != 2 || memberIDs[0] != "u-alice" || memberIDs[1] != "u-bob" {
				t.Fatalf("payload[member_ids] = %#v, want [u-alice u-bob]", payload["member_ids"])
			}

			return jsonResponse(http.StatusCreated, `{"id":"room-1","title":"alpha","members":["u-admin","u-alice","u-bob"],"messages":[{"id":"msg-1"}]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "room", "create", "--title", "alpha", "--creator-id", "u-admin", "--member-ids", "u-alice, u-bob"})
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

func TestExecuteRoomDeleteUsesFeishuChannelRoute(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/rooms/oc_alpha" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/channels/feishu/rooms/oc_alpha")
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Status:     http.StatusText(http.StatusNoContent),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "room", "delete", "--channel", "feishu", "oc_alpha"}); err != nil {
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

func TestExecuteUserCreateUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/users" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/users")
			}
			var body apitypes.CreateUserRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if body.ID != "u-alice" || body.Name != "Alice" || body.Handle != "alice" || body.Role != "worker" {
				t.Fatalf("body = %+v, want csgclaw create user payload", body)
			}
			return jsonResponse(http.StatusCreated, `{"id":"u-alice","name":"alice","handle":"alice","role":"worker","is_online":true}`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "user", "create", "--id", "u-alice", "--name", "Alice", "--handle", "alice", "--role", "worker"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u-alice", "alice", "alice", "worker", "true")
}

func TestExecuteUserCreateUsesFeishuChannelRoute(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/users" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/channels/feishu/users")
			}
			var body channel.FeishuCreateUserRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if body.ID != "ou-alice" || body.Name != "Alice" || body.Handle != "alice" || body.Role != "worker" || body.Avatar != "AL" {
				t.Fatalf("body = %+v, want feishu create user payload", body)
			}
			return jsonResponse(http.StatusCreated, `{"id":"ou-alice","name":"Alice","handle":"alice","role":"worker","avatar":"AL","is_online":true}`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "user", "create", "--channel", "feishu", "--id", "ou-alice", "--name", "Alice", "--handle", "alice", "--role", "worker", "--avatar", "AL"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteUserDeleteUsesHTTPClient(t *testing.T) {
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

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "user", "delete", "u-alice"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteUserDeleteUsesChannelFlag(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/users/ou-alice" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/channels/feishu/users/ou-alice")
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Status:     http.StatusText(http.StatusNoContent),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "user", "delete", "--channel", "feishu", "ou-alice"}); err != nil {
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
		"model    Manage model providers.",
		"bot      Manage bots",
		"room     Manage IM rooms",
		"member   Manage IM room members",
		"user     Manage IM users",
		"completion Generate shell completion scripts.",
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
		"model    Manage model providers.",
		"bot      Manage bots",
		"room     Manage IM rooms",
		"member   Manage IM room members",
		"user     Manage IM users",
		"completion Generate shell completion scripts.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help = %q, want substring %q", got, want)
		}
	}
}

func TestExecuteDoesNotRegisterTopLevelAuthAliases(t *testing.T) {
	for _, args := range [][]string{{"auth", "login", "codex"}, {"codex-login"}, {"claude-login"}} {
		var stderr bytes.Buffer
		app := &App{
			stdout:     &bytes.Buffer{},
			stderr:     &stderr,
			httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
		}
		err := app.Execute(context.Background(), args)
		if err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Fatalf("Execute(%v) error = %v, want unknown command", args, err)
		}
	}
}

func TestExecuteCompletionBash(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := &App{
		stdout:     &stdout,
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	if err := app.Execute(context.Background(), []string{"completion", "bash"}); err != nil {
		t.Fatalf("Execute() error = %v; stderr=%s", err, stderr.String())
	}
	for _, want := range []string{`"${COMP_WORDS[0]}" __complete`, "complete -F _csgclaw_completion csgclaw"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestExecuteHiddenCompleteUsesFullCommandSet(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	if err := app.Execute(context.Background(), []string{"__complete", "csgclaw", ""}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := stdout.String()
	for _, want := range []string{"agent\n", "model\n", "completion\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout = %q, want substring %q", got, want)
		}
	}
	for _, notWant := range []string{"_serve\n", "__complete\n"} {
		if strings.Contains(got, notWant) {
			t.Fatalf("stdout = %q, should not include %q", got, notWant)
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

func TestExecuteVersionFlagSupportsJSONOutput(t *testing.T) {
	originalVersion := appversion.Version
	appversion.Version = "1.2.3-test"
	t.Cleanup(func() { appversion.Version = originalVersion })

	var stdout bytes.Buffer
	app := &App{
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	if err := app.Execute(context.Background(), []string{"--output", "json", "--version"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{`"program": "csgclaw"`, `"version": "1.2.3-test"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %s", stdout.String(), want)
		}
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
		"delete <id>        Delete one agent",
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
		"delete <id>        Delete a bot",
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

func TestAgentDeleteSubcommandHelpShowsShortAndLongFlags(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"agent", "delete", "-h"})
	if err != flag.ErrHelp {
		t.Fatalf("Execute() error = %v, want %v", err, flag.ErrHelp)
	}

	got := stderr.String()
	for _, want := range []string{
		"Delete one agent or all agents.",
		"csgclaw agent delete <id> [flags]",
		"csgclaw agent delete --all [flags]",
		"-a, --all     delete all agents",
		"-f, --force   delete all agents without confirmation",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help = %q, want substring %q", got, want)
		}
	}
}

func TestAgentCreateSubcommandHelpShowsReplaceAndForceFlags(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		stdout:     &bytes.Buffer{},
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	err := app.Execute(context.Background(), []string{"agent", "create", "-h"})
	if err != flag.ErrHelp {
		t.Fatalf("Execute() error = %v, want %v", err, flag.ErrHelp)
	}

	got := stderr.String()
	for _, want := range []string{
		"Create an agent.",
		"csgclaw agent create [flags]",
		"csgclaw agent create --replace --id <id> [flags]",
		"-r, --replace           replace an existing agent in place",
		"-f, --force             replace an existing agent without confirmation",
		"--id string             agent id",
		"--name string           agent name",
		"--description string    agent description",
		"--image string          agent image",
		"--profile string        agent llm profile",
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

func assertStringSliceValue(t *testing.T, got any, want []string) {
	t.Helper()
	raw, ok := got.([]any)
	if !ok {
		t.Fatalf("value = %#v, want string slice", got)
	}
	items := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok {
			t.Fatalf("value = %#v, want string slice", got)
		}
		items = append(items, s)
	}
	if !slicesEqual(items, want) {
		t.Fatalf("value = %#v, want %v", items, want)
	}
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
