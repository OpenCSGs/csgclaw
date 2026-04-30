package csgclawcli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	appversion "csgclaw/internal/version"
)

func TestExecuteExposesOnlyLiteCommands(t *testing.T) {
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
		"bot      Manage bots",
		"room     Manage IM rooms",
		"member   Manage IM room members",
		"message  Manage IM messages.",
		"completion Generate shell completion scripts.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage = %q, want substring %q", got, want)
		}
	}
	for _, notWant := range []string{"  agent", "  serve", "  onboard", "  user"} {
		if strings.Contains(got, notWant) {
			t.Fatalf("usage = %q, should not include %q", got, notWant)
		}
	}
}

func TestExecuteCompletionFish(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := &App{
		stdout:     &stdout,
		stderr:     &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	if err := app.Execute(context.Background(), []string{"completion", "fish"}); err != nil {
		t.Fatalf("Execute() error = %v; stderr=%s", err, stderr.String())
	}
	for _, want := range []string{"command csgclaw-cli __complete", "complete -c csgclaw-cli"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestExecuteHiddenCompleteUsesLiteCommandSet(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout:     &stdout,
		stderr:     &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, nil }),
	}

	if err := app.Execute(context.Background(), []string{"__complete", "csgclaw-cli", ""}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := stdout.String()
	for _, want := range []string{"bot\n", "room\n", "member\n", "message\n", "completion\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout = %q, want substring %q", got, want)
		}
	}
	for _, notWant := range []string{"agent\n", "serve\n", "onboard\n", "user\n", "__complete\n"} {
		if strings.Contains(got, notWant) {
			t.Fatalf("stdout = %q, should not include %q", got, notWant)
		}
	}
}

func TestExecuteRejectsFullCsgclawCommands(t *testing.T) {
	for _, command := range []string{"agent", "serve", "onboard", "user"} {
		t.Run(command, func(t *testing.T) {
			app := &App{
				stdout: &bytes.Buffer{},
				stderr: &bytes.Buffer{},
				httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
					return nil, nil
				}),
			}

			err := app.Execute(context.Background(), []string{command})
			if err == nil || !strings.Contains(err.Error(), `unknown command "`+command+`"`) {
				t.Fatalf("Execute(%q) error = %v, want unknown command", command, err)
			}
		})
	}
}

func TestExecuteBotListUsesAPIClient(t *testing.T) {
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

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "bot", "list", "--channel", "feishu"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "bot-feishu"`) || !strings.Contains(stdout.String(), `"channel": "feishu"`) {
		t.Fatalf("stdout = %q, want JSON bot payload", stdout.String())
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
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/bots?channel=feishu" {
				t.Fatalf("url = %q, want feishu bot list route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"bot-feishu","name":"feishu","role":"manager","channel":"feishu","agent_id":"u-manager","user_id":"fsu-manager","created_at":"2026-04-12T09:00:00Z"}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "bot", "list", "--channel", "feishu"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatalf("ReadFile(stdout) error = %v", err)
	}
	if !strings.Contains(string(got), `"id": "bot-feishu"`) || !strings.Contains(string(got), `"channel": "feishu"`) {
		t.Fatalf("stdout = %q, want JSON bot payload", string(got))
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
			if req.URL.String() != "http://example.test/api/v1/bots?channel=feishu&role=manager" {
				t.Fatalf("url = %q, want role-filtered bot list route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"bot-feishu","name":"feishu","role":"manager","channel":"feishu","agent_id":"u-manager","user_id":"fsu-manager","created_at":"2026-04-12T09:00:00Z"}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "bot", "list", "--channel", "feishu", "--role", "manager"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "bot-feishu"`) || !strings.Contains(stdout.String(), `"role": "manager"`) {
		t.Fatalf("stdout = %q, want JSON bot payload", stdout.String())
	}
}

func TestExecuteUsesEnvironmentForEndpointAndToken(t *testing.T) {
	t.Setenv(envBaseURL, "http://env.example.test")
	t.Setenv(envAccessToken, "env-secret-token")

	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://env.example.test/api/v1/bots?channel=feishu" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://env.example.test/api/v1/bots?channel=feishu")
			}
			if got := req.Header.Get("Authorization"); got != "Bearer env-secret-token" {
				t.Fatalf("Authorization = %q, want %q", got, "Bearer env-secret-token")
			}
			return jsonResponse(http.StatusOK, `[]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"bot", "list", "--channel", "feishu"}); err != nil {
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
			if req.URL.String() != "http://flag.example.test/api/v1/bots?channel=feishu" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://flag.example.test/api/v1/bots?channel=feishu")
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
		"bot", "list", "--channel", "feishu",
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteBotDeleteUsesAPIClient(t *testing.T) {
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

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "bot", "delete", "--channel", "feishu", "u-alice"})
	if err != nil {
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

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "bot", "delete", "--channel", "feishu", "u-alice"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{`"command": "bot"`, `"action": "delete"`, `"status": "deleted"`, `"id": "u-alice"`, `"channel": "feishu"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %s", stdout.String(), want)
		}
	}
}

func TestExecuteRoomCreateUsesChannelRoute(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/rooms" {
				t.Fatalf("url = %q, want feishu room create route", req.URL.String())
			}
			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["title"] != "alpha" || payload["creator_id"] != "ou_admin" {
				t.Fatalf("payload = %#v, want title and creator", payload)
			}
			return jsonResponse(http.StatusCreated, `{"id":"oc_alpha","title":"alpha","is_direct":false,"members":["ou_admin"],"messages":[]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "room", "create", "--channel", "feishu", "--title", "alpha", "--creator-id", "ou_admin"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "oc_alpha", "alpha", "false", "1", "0")
}

func TestExecuteRoomListRendersDirectColumn(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/rooms" {
				t.Fatalf("url = %q, want feishu room list route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"oc_dm","title":"alice","is_direct":true,"members":["ou_admin","ou_alice"],"messages":[]},{"id":"oc_group","title":"ops","is_direct":false,"members":["ou_admin","ou_alice","ou_bob"],"messages":[]}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "room", "list", "--channel", "feishu"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "DIRECT") {
		t.Fatalf("stdout = %q, want DIRECT column", stdout.String())
	}
	assertTableHasRow(t, stdout.String(), "oc_dm", "alice", "true", "2", "0")
	assertTableHasRow(t, stdout.String(), "oc_group", "ops", "false", "3", "0")
}

func TestExecuteRoomDeleteUsesChannelRoute(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/rooms/oc_alpha" {
				t.Fatalf("url = %q, want feishu room delete route", req.URL.String())
			}
			return jsonResponse(http.StatusNoContent, ``), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "room", "delete", "--channel", "feishu", "oc_alpha"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{`"command": "room"`, `"action": "delete"`, `"status": "deleted"`, `"id": "oc_alpha"`, `"channel": "feishu"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %s", stdout.String(), want)
		}
	}
}

func TestExecuteMessageCreateUsesChannelRoute(t *testing.T) {
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
			if payload["room_id"] != "oc_alpha" || payload["sender_id"] != "u-manager" || payload["content"] != "hello" {
				t.Fatalf("payload = %#v, want room/sender/content", payload)
			}
			return jsonResponse(http.StatusCreated, `{"id":"om_1","sender_id":"u-manager","kind":"message","content":"hello","created_at":"2026-04-12T09:00:00Z","mentions":[]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "message", "create", "--channel", "feishu", "--room-id", "oc_alpha", "--sender-id", "u-manager", "--content", "hello"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "om_1", "u-manager", "message", "hello")
}

func TestExecuteMessageListUsesChannelRoute(t *testing.T) {
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

func TestExecuteMemberListUsesCSGClawDefault(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/rooms/oc_alpha/members" {
				t.Fatalf("url = %q, want csgclaw room members route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"u_alice","name":"Alice","handle":"alice","role":"worker","is_online":true}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "member", "list", "--room-id", "oc_alpha"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u_alice", "Alice", "alice", "worker", "true")
}

func TestExecuteVersionFlagPrintsCsgclawCLIVersion(t *testing.T) {
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
	if got, want := strings.TrimSpace(stdout.String()), "csgclaw-cli version 1.2.3-test"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
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
