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
		"hub      Discover agent templates.",
		"room     Manage IM rooms",
		"member   Manage IM room members",
		"message  Manage IM messages.",
		"team     Manage agent teams.",
		"skill    Discover and install ClawHub skills.",
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

func TestExecuteHubListAcceptsOutputShorthand(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/hub/templates" {
				t.Fatalf("url = %q, want hub templates route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"builtin.gitlab-worker","name":"gitlab-worker","source":{"name":"builtin","kind":"builtin"}}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "-o", "json", "hub", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "builtin.gitlab-worker"`) {
		t.Fatalf("stdout = %q, want JSON hub template payload", stdout.String())
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
	for _, want := range []string{"bot\n", "hub\n", "room\n", "member\n", "message\n", "team\n", "completion\n"} {
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

func TestExecuteBotIdentityHelpUsesBotIDSemantics(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "room create",
			args: []string{"room", "create", "--help"},
			want: []string{"creator bot id", "comma-separated member bot ids"},
		},
		{
			name: "member create",
			args: []string{"member", "create", "--help"},
			want: []string{"bot id to add", "inviter bot id"},
		},
		{
			name: "message create",
			args: []string{"message", "create", "--help"},
			want: []string{"sender bot id", "mentioned bot id"},
		},
		{
			name: "team create",
			args: []string{"team", "create", "--help"},
			want: []string{"lead bot id", "worker bot ids"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			app := &App{
				stdout: &bytes.Buffer{},
				stderr: &stderr,
				httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
					return nil, nil
				}),
			}

			err := app.Execute(context.Background(), tt.args)
			if err != flag.ErrHelp {
				t.Fatalf("Execute() error = %v, want %v", err, flag.ErrHelp)
			}
			for _, want := range tt.want {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("help = %q, want substring %q", stderr.String(), want)
				}
			}
		})
	}
}

func TestExecuteTeamCreateUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/teams" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/teams")
			}
			return jsonResponse(http.StatusCreated, `{"id":"room-1","room_id":"room-1","channel":"csgclaw","title":"release","lead_bot_id":"bot-manager","status":"active","created_at":"2026-05-30T00:00:00Z","updated_at":"2026-05-30T00:00:00Z"}`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "team", "create", "--lead-bot-id", "bot-manager", "--title", "release"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "room-1") || !strings.Contains(stdout.String(), "bot-manager") {
		t.Fatalf("stdout = %q, want rendered team row", stdout.String())
	}
}

func TestExecuteTeamTaskListUsesHTTPClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/teams/team-1/tasks" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://example.test/api/v1/teams/team-1/tasks")
			}
			return jsonResponse(http.StatusOK, `[{"id":"task-1","team_id":"team-1","room_id":"room-1","title":"Run tests","status":"blocked","created_by":"bot-manager","assigned_to":"bot-worker","claimed_by":"bot-worker","error":"need approval","created_at":"2026-05-30T00:00:00Z","updated_at":"2026-05-30T00:01:00Z"}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "team", "task", "list", "--team", "team-1"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "task-1") || !strings.Contains(stdout.String(), "blocked") {
		t.Fatalf("stdout = %q, want rendered task row", stdout.String())
	}
}

func TestExecuteBotIdentityRequiredErrorsUseBotIDSemantics(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "member create missing user id",
			args: []string{"member", "create", "--room-id", "room-1", "--inviter-id", "u-manager"},
			want: "--user-id bot id is required",
		},
		{
			name: "message create missing sender id",
			args: []string{"message", "create", "--room-id", "room-1", "--content", "hello"},
			want: "--sender-id bot id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{
				stdout: &bytes.Buffer{},
				stderr: &bytes.Buffer{},
				httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
					return nil, nil
				}),
			}

			err := app.Execute(context.Background(), tt.args)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("Execute() error = %v, want %q", err, tt.want)
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
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/bots" {
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
	for _, unexpected := range []string{`"agent_id"`, `"user_id"`, `"created_at"`} {
		if strings.Contains(stdout.String(), unexpected) {
			t.Fatalf("stdout = %q, want compact csgclaw-cli bot list without %s", stdout.String(), unexpected)
		}
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
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/bots" {
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
	for _, unexpected := range []string{`"agent_id"`, `"user_id"`, `"created_at"`} {
		if strings.Contains(string(got), unexpected) {
			t.Fatalf("stdout = %q, want compact csgclaw-cli bot list without %s", string(got), unexpected)
		}
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
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/bots?role=manager" {
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
	for _, unexpected := range []string{`"agent_id"`, `"user_id"`, `"created_at"`} {
		if strings.Contains(stdout.String(), unexpected) {
			t.Fatalf("stdout = %q, want compact csgclaw-cli bot list without %s", stdout.String(), unexpected)
		}
	}
}

func TestExecuteUsesEnvironmentForEndpointAndToken(t *testing.T) {
	t.Setenv(envBaseURL, "http://env.example.test")
	t.Setenv(envAccessToken, "env-secret-token")

	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "http://env.example.test/api/v1/channels/feishu/bots" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://env.example.test/api/v1/channels/feishu/bots")
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
			if req.URL.String() != "http://flag.example.test/api/v1/channels/feishu/bots" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://flag.example.test/api/v1/channels/feishu/bots")
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
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/bots/u-alice" {
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

func TestExecuteBotConfigSetUsesFeishuConfigRoute(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdin:  strings.NewReader("stdin-secret\n"),
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPut {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPut)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/config" {
				t.Fatalf("url = %q, want feishu config route", req.URL.String())
			}
			if got := req.Header.Get("Authorization"); got != "Bearer token" {
				t.Fatalf("Authorization = %q, want bearer token", got)
			}
			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			for key, want := range map[string]string{
				"bot_id":        "u-dev",
				"app_id":        "cli_dev",
				"app_secret":    "stdin-secret",
				"admin_open_id": "ou_admin",
			} {
				if got := payload[key]; got != want {
					t.Fatalf("payload[%s] = %#v, want %q; payload=%#v", key, got, want, payload)
				}
			}
			return jsonResponse(http.StatusOK, `{"bot_id":"u-dev","configured":true,"app_id":"cli_dev","app_secret":"present","admin_open_id":"ou_admin","reloaded":true}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--token", "token", "--output", "json", "bot", "config", "--channel", "feishu", "--set", "--bot-id", "u-dev", "--app-id", "cli_dev", "--admin-open-id", "ou_admin", "--app-secret-stdin"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(stdout.String(), "stdin-secret") {
		t.Fatalf("stdout leaked secret: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"app_secret": "present"`) {
		t.Fatalf("stdout = %q, want masked secret", stdout.String())
	}
}

func TestExecuteBotConfigGetUsesFeishuConfigRoute(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/config?bot_id=u-dev" {
				t.Fatalf("url = %q, want feishu config get route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `{"bot_id":"u-dev","configured":true,"app_id":"cli_dev","app_secret":"present"}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "bot", "config", "--channel", "feishu", "--get", "--bot-id", "u-dev"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "u-dev") || !strings.Contains(stdout.String(), "present") {
		t.Fatalf("stdout = %s, want bot and masked secret", stdout.String())
	}
}

func TestExecuteBotConfigReloadUsesFeishuConfigRoute(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/config" {
				t.Fatalf("url = %q, want feishu config reload route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `{"status":"reloaded","feishu_bots":["u-dev"]}`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "bot", "config", "--channel", "feishu", "--reload"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "reloaded") || !strings.Contains(stdout.String(), "u-dev") {
		t.Fatalf("stdout = %s, want reload result", stdout.String())
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
			if req.URL.String() != "http://example.test/api/v1/channels/csgclaw/rooms/oc_alpha/members" {
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
