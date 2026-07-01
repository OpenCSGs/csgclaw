package csgclawcli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"csgclaw/internal/participant"
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
		"participant  Manage channel participants.",
		"pt           Manage channel participants.",
		"template     Discover agent templates.",
		"room         Manage IM rooms",
		"member       Manage IM room members",
		"message      Manage IM messages.",
		"team         Manage agent teams.",
		"skill        Discover and install ClawHub skills.",
		"completion   Generate shell completion scripts.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage = %q, want substring %q", got, want)
		}
	}
	for _, notWant := range []string{"  agent", "  serve", "  onboard", "  user", "\n  bot ", "\n  channel "} {
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

func TestExecuteTemplateListAcceptsOutputShorthand(t *testing.T) {
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

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "-o", "json", "template", "list"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "builtin.gitlab-worker"`) {
		t.Fatalf("stdout = %q, want JSON template payload", stdout.String())
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
	for _, want := range []string{"participant\n", "pt\n", "template\n", "room\n", "member\n", "message\n", "team\n", "completion\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout = %q, want substring %q", got, want)
		}
	}
	for _, notWant := range []string{"agent\n", "serve\n", "onboard\n", "user\n", "bot\n", "channel\n", "__complete\n"} {
		if strings.Contains(got, notWant) {
			t.Fatalf("stdout = %q, should not include %q", got, notWant)
		}
	}
}

func TestExecuteRejectsUnavailableCommands(t *testing.T) {
	for _, command := range []string{"agent", "serve", "onboard", "user", "bot", "channel"} {
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

func TestExecuteCollaborationIdentityHelpUsesParticipantSemantics(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "room create",
			args: []string{"room", "create", "--help"},
			want: []string{"creator participant id", "comma-separated member participant ids"},
		},
		{
			name: "member create",
			args: []string{"member", "create", "--help"},
			want: []string{"participant id to add", "inviter participant id"},
		},
		{
			name: "message create",
			args: []string{"message", "create", "--help"},
			want: []string{"sender participant id", "mentioned participant id"},
		},
		{
			name: "team create",
			args: []string{"team", "create", "--help"},
			want: []string{"lead agent id", "worker agent ids"},
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
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if payload["lead_agent_id"] != "u-manager" {
				t.Fatalf("lead_agent_id = %v, want u-manager; payload=%v", payload["lead_agent_id"], payload)
			}
			if _, ok := payload["lead_participant_id"]; ok {
				t.Fatalf("payload includes legacy lead_participant_id: %v", payload)
			}
			return jsonResponse(http.StatusCreated, `{"id":"room-1","room_id":"room-1","channel":"csgclaw","title":"release","lead_agent_id":"u-manager","status":"active","created_at":"2026-05-30T00:00:00Z","updated_at":"2026-05-30T00:00:00Z"}`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "team", "create", "--lead-agent-id", "u-manager", "--title", "release"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "room-1") || !strings.Contains(stdout.String(), "u-manager") {
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

func TestExecuteCollaborationIdentityRequiredErrorsUseParticipantSemantics(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "member create missing user id",
			args: []string{"member", "create", "--room-id", "room-1", "--inviter-id", "u-manager"},
			want: "--user-id participant id is required",
		},
		{
			name: "message create missing sender id",
			args: []string{"message", "create", "--room-id", "room-1", "--content", "hello"},
			want: "--sender-id participant id is required",
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

func TestExecuteParticipantListUsesAPIClient(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/participants" {
				t.Fatalf("url = %q, want feishu participant list route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"bot-feishu","name":"feishu","type":"agent","channel":"feishu","agent_id":"u-manager","channel_user_ref":"fsu-manager","lifecycle_status":"active","created_at":"2026-04-12T09:00:00Z"}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "participant", "list", "--channel", "feishu"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "bot-feishu"`) || !strings.Contains(stdout.String(), `"channel": "feishu"`) {
		t.Fatalf("stdout = %q, want JSON participant payload", stdout.String())
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
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/participants" {
				t.Fatalf("url = %q, want feishu participant list route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"bot-feishu","name":"feishu","type":"agent","channel":"feishu","agent_id":"u-manager","channel_user_ref":"fsu-manager","lifecycle_status":"active","created_at":"2026-04-12T09:00:00Z"}]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "participant", "list", "--channel", "feishu"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatalf("ReadFile(stdout) error = %v", err)
	}
	if !strings.Contains(string(got), `"id": "bot-feishu"`) || !strings.Contains(string(got), `"channel": "feishu"`) {
		t.Fatalf("stdout = %q, want JSON participant payload", string(got))
	}
}

func TestExecuteParticipantListUsesTypeQuery(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodGet)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/participants?type=agent" {
				t.Fatalf("url = %q, want type-filtered participant list route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"bot-feishu","name":"feishu","type":"agent","channel":"feishu","agent_id":"u-manager","channel_user_ref":"fsu-manager","lifecycle_status":"active","created_at":"2026-04-12T09:00:00Z"}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "participant", "list", "--channel", "feishu", "--type", "agent"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "bot-feishu"`) || !strings.Contains(stdout.String(), `"type": "agent"`) {
		t.Fatalf("stdout = %q, want JSON participant payload", stdout.String())
	}
}

func TestExecuteParticipantBindAdminHumanUsesParticipantAPI(t *testing.T) {
	var stdout bytes.Buffer
	call := 0
	app := &App{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				if req.Method != http.MethodGet || req.URL.String() != "http://example.test/api/v1/channels/feishu/participants" {
					t.Fatalf("request %d = %s %s, want participant list", call, req.Method, req.URL.String())
				}
				return jsonResponse(http.StatusOK, `[]`), nil
			case 2:
				if req.Method != http.MethodPost || req.URL.String() != "http://example.test/api/v1/channels/feishu/participants" {
					t.Fatalf("request %d = %s %s, want participant create", call, req.Method, req.URL.String())
				}
				var payload map[string]any
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				channelUser, _ := payload["channel_user"].(map[string]any)
				if payload["id"] != "admin" || payload["type"] != "human" || payload["name"] != "admin" || channelUser["ref"] != "ou_admin" || channelUser["kind"] != "open_id" {
					t.Fatalf("payload = %#v, want feishu admin human open_id", payload)
				}
				return jsonResponse(http.StatusCreated, `{"id":"admin","name":"admin","type":"human","channel":"feishu","channel_user_ref":"ou_admin","channel_user_kind":"open_id","lifecycle_status":"active","created_at":"2026-04-12T09:00:00Z"}`), nil
			default:
				t.Fatalf("unexpected request %d: %s %s", call, req.Method, req.URL.String())
				return nil, nil
			}
		}),
	}

	err := app.Execute(context.Background(), []string{
		"--endpoint", "http://example.test",
		"--output", "json",
		"pt", "bind",
		"--channel", "feishu",
		"--feishu-kind", "human",
		"--admin",
		"--open-id", "ou_admin",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if call != 2 {
		t.Fatalf("request count = %d, want 2", call)
	}
	if !strings.Contains(stdout.String(), `"participant_id": "admin"`) || !strings.Contains(stdout.String(), `"participant_type": "human"`) {
		t.Fatalf("stdout = %q, want admin bind result", stdout.String())
	}
}

func TestExecuteParticipantBindBotWritesConfigAndRecreatesWorkerWhenRestartFlagSet(t *testing.T) {
	var stdout, stderr bytes.Buffer
	call := 0
	app := &App{
		stdin:  strings.NewReader("secret-value\n"),
		stdout: &stdout,
		stderr: &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				if req.Method != http.MethodGet || req.URL.String() != "http://example.test/api/v1/agents/u-dev" {
					t.Fatalf("request %d = %s %s, want agent get", call, req.Method, req.URL.String())
				}
				return jsonResponse(http.StatusOK, `{"id":"u-dev","name":"dev","role":"worker","status":"running","created_at":"2026-04-12T09:00:00Z"}`), nil
			case 2:
				if req.Method != http.MethodGet || req.URL.String() != "http://example.test/api/v1/channels/feishu/participants" {
					t.Fatalf("request %d = %s %s, want participant list", call, req.Method, req.URL.String())
				}
				return jsonResponse(http.StatusOK, `[]`), nil
			case 3:
				if req.Method != http.MethodPost || req.URL.String() != "http://example.test/api/v1/channels/feishu/participants" {
					t.Fatalf("request %d = %s %s, want participant create", call, req.Method, req.URL.String())
				}
				var payload map[string]any
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				channelUser, _ := payload["channel_user"].(map[string]any)
				appConfig, _ := payload["channel_app_config"].(map[string]any)
				agentBinding, _ := payload["agent_binding"].(map[string]any)
				if payload["id"] != "pt-dev" || payload["type"] != "agent" || payload["name"] != "dev" {
					t.Fatalf("payload = %#v, want canonical dev agent participant", payload)
				}
				if channelUser["kind"] != "app_id" || channelUser["ref"] != nil {
					t.Fatalf("channel_user = %#v, want app_id without open_id ref", channelUser)
				}
				if appConfig["app_id"] != "cli_dev" || appConfig["app_secret"] != "secret-value" {
					t.Fatalf("channel_app_config = %#v, want app credentials", appConfig)
				}
				if agentBinding["mode"] != "reuse" || agentBinding["agent_id"] != "agent-dev" {
					t.Fatalf("agent_binding = %#v, want reuse agent-dev", agentBinding)
				}
				return jsonResponse(http.StatusCreated, feishuBotParticipantResponse("pt-dev", "dev", "agent-dev", "cli_dev")), nil
			case 4:
				if req.Method != http.MethodPost || req.URL.String() != "http://example.test/api/v1/agents/agent-dev/recreate" {
					t.Fatalf("request %d = %s %s, want worker recreate", call, req.Method, req.URL.String())
				}
				return jsonResponse(http.StatusCreated, `{"id":"u-dev","name":"dev","role":"worker","status":"running","created_at":"2026-04-12T09:00:00Z"}`), nil
			default:
				t.Fatalf("unexpected request %d: %s %s", call, req.Method, req.URL.String())
				return nil, nil
			}
		}),
	}

	err := app.Execute(context.Background(), []string{
		"--endpoint", "http://example.test",
		"--output", "json",
		"pt", "bind",
		"--channel", "feishu",
		"--feishu-kind", "bot",
		"--agent", "u-dev",
		"--app-id", "cli_dev",
		"--app-secret-stdin",
		"--restart",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v; stderr=%s", err, stderr.String())
	}
	if call != 4 {
		t.Fatalf("request count = %d, want 4", call)
	}
	if strings.Contains(stdout.String(), "secret-value") || strings.Contains(stderr.String(), "secret-value") {
		t.Fatalf("output leaked app secret: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	for _, want := range []string{`"participant_id": "pt-dev"`, `"agent_id": "agent-dev"`, `"restart_status": "worker_recreated"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %s", stdout.String(), want)
		}
	}
}

func TestExecuteParticipantBindBotReportsPartialResultWhenWorkerRecreateFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	call := 0
	app := &App{
		stdin:  strings.NewReader("secret-value\n"),
		stdout: &stdout,
		stderr: &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				if req.Method != http.MethodGet || req.URL.String() != "http://example.test/api/v1/agents/u-dev" {
					t.Fatalf("request %d = %s %s, want agent get", call, req.Method, req.URL.String())
				}
				return jsonResponse(http.StatusOK, `{"id":"u-dev","name":"dev","role":"worker","status":"running","created_at":"2026-04-12T09:00:00Z"}`), nil
			case 2:
				if req.Method != http.MethodGet || req.URL.String() != "http://example.test/api/v1/channels/feishu/participants" {
					t.Fatalf("request %d = %s %s, want participant list", call, req.Method, req.URL.String())
				}
				return jsonResponse(http.StatusOK, `[]`), nil
			case 3:
				if req.Method != http.MethodPost || req.URL.String() != "http://example.test/api/v1/channels/feishu/participants" {
					t.Fatalf("request %d = %s %s, want participant create", call, req.Method, req.URL.String())
				}
				return jsonResponse(http.StatusCreated, feishuBotParticipantResponse("pt-dev", "dev", "agent-dev", "cli_dev")), nil
			case 4:
				if req.Method != http.MethodPost || req.URL.String() != "http://example.test/api/v1/agents/agent-dev/recreate" {
					t.Fatalf("request %d = %s %s, want worker recreate", call, req.Method, req.URL.String())
				}
				return jsonResponse(http.StatusInternalServerError, `recreate failed`), nil
			default:
				t.Fatalf("unexpected request %d: %s %s", call, req.Method, req.URL.String())
				return nil, nil
			}
		}),
	}

	err := app.Execute(context.Background(), []string{
		"--endpoint", "http://example.test",
		"--output", "json",
		"pt", "bind",
		"--channel", "feishu",
		"--feishu-kind", "bot",
		"--agent", "u-dev",
		"--app-id", "cli_dev",
		"--app-secret-stdin",
		"--restart",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v; stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	if call != 4 {
		t.Fatalf("request count = %d, want 4", call)
	}
	if strings.Contains(stdout.String(), "secret-value") || strings.Contains(stderr.String(), "secret-value") {
		t.Fatalf("output leaked app secret: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	for _, want := range []string{
		`"status": "partial"`,
		`"participant_id": "pt-dev"`,
		`"agent_id": "agent-dev"`,
		`"config_saved": true`,
		`"restart_status": "recreate_failed"`,
		`"restart_error":`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %s", stdout.String(), want)
		}
	}
	if !strings.Contains(stderr.String(), "pt bind failed at recreate: agent_id=agent-dev participant_id=pt-dev") {
		t.Fatalf("stderr = %q, want recreate failure context", stderr.String())
	}
}

func TestExecuteParticipantBindBotRecreatesManagerWhenRestartFlagSet(t *testing.T) {
	var stdout, stderr bytes.Buffer
	call := 0
	app := &App{
		stdin:  strings.NewReader("secret-value\n"),
		stdout: &stdout,
		stderr: &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				if req.Method != http.MethodGet || req.URL.String() != "http://example.test/api/v1/agents/u-manager" {
					t.Fatalf("request %d = %s %s, want agent get", call, req.Method, req.URL.String())
				}
				return jsonResponse(http.StatusOK, `{"id":"u-manager","name":"manager","role":"manager","status":"running","created_at":"2026-04-12T09:00:00Z"}`), nil
			case 2:
				if req.Method != http.MethodGet || req.URL.String() != "http://example.test/api/v1/channels/feishu/participants" {
					t.Fatalf("request %d = %s %s, want participant list", call, req.Method, req.URL.String())
				}
				return jsonResponse(http.StatusOK, `[]`), nil
			case 3:
				if req.Method != http.MethodPost || req.URL.String() != "http://example.test/api/v1/channels/feishu/participants" {
					t.Fatalf("request %d = %s %s, want participant create", call, req.Method, req.URL.String())
				}
				var payload map[string]any
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				channelUser, _ := payload["channel_user"].(map[string]any)
				appConfig, _ := payload["channel_app_config"].(map[string]any)
				agentBinding, _ := payload["agent_binding"].(map[string]any)
				if payload["id"] != "pt-manager" || payload["type"] != "agent" || payload["name"] != "manager" {
					t.Fatalf("payload = %#v, want canonical manager agent participant", payload)
				}
				if channelUser["kind"] != "app_id" || channelUser["ref"] != nil {
					t.Fatalf("channel_user = %#v, want app_id without open_id ref", channelUser)
				}
				if appConfig["app_id"] != "cli_manager" || appConfig["app_secret"] != "secret-value" {
					t.Fatalf("channel_app_config = %#v, want app credentials", appConfig)
				}
				if agentBinding["mode"] != "reuse" || agentBinding["agent_id"] != "agent-manager" {
					t.Fatalf("agent_binding = %#v, want reuse agent-manager", agentBinding)
				}
				return jsonResponse(http.StatusCreated, feishuBotParticipantResponse("pt-manager", "manager", "agent-manager", "cli_manager")), nil
			case 4:
				if req.Method != http.MethodPost || req.URL.String() != "http://example.test/api/v1/agents/agent-manager/recreate" {
					t.Fatalf("request %d = %s %s, want manager recreate", call, req.Method, req.URL.String())
				}
				return jsonResponse(http.StatusCreated, `{"id":"u-manager","name":"manager","role":"manager","status":"running","created_at":"2026-04-12T09:00:00Z"}`), nil
			default:
				t.Fatalf("unexpected request %d: %s %s", call, req.Method, req.URL.String())
				return nil, nil
			}
		}),
	}

	err := app.Execute(context.Background(), []string{
		"--endpoint", "http://example.test",
		"--output", "json",
		"pt", "bind",
		"--channel", "feishu",
		"--feishu-kind", "bot",
		"--agent", "u-manager",
		"--app-id", "cli_manager",
		"--app-secret-stdin",
		"--restart",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v; stderr=%s", err, stderr.String())
	}
	if call != 4 {
		t.Fatalf("request count = %d, want 4", call)
	}
	if strings.Contains(stdout.String(), "secret-value") || strings.Contains(stderr.String(), "secret-value") {
		t.Fatalf("output leaked app secret: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	for _, want := range []string{`"participant_id": "pt-manager"`, `"agent_id": "agent-manager"`, `"restart_status": "manager_recreated"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %s", stdout.String(), want)
		}
	}
}

func TestExecuteParticipantBindBotDefaultsToNoRestart(t *testing.T) {
	var stdout, stderr bytes.Buffer
	call := 0
	app := &App{
		stdin:  strings.NewReader("secret-value\n"),
		stdout: &stdout,
		stderr: &stderr,
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				if req.Method != http.MethodGet || req.URL.String() != "http://example.test/api/v1/agents/u-dev" {
					t.Fatalf("request %d = %s %s, want agent get", call, req.Method, req.URL.String())
				}
				return jsonResponse(http.StatusOK, `{"id":"u-dev","name":"dev","role":"worker","status":"running","created_at":"2026-04-12T09:00:00Z"}`), nil
			case 2:
				if req.Method != http.MethodGet || req.URL.String() != "http://example.test/api/v1/channels/feishu/participants" {
					t.Fatalf("request %d = %s %s, want participant list", call, req.Method, req.URL.String())
				}
				return jsonResponse(http.StatusOK, `[]`), nil
			case 3:
				if req.Method != http.MethodPost || req.URL.String() != "http://example.test/api/v1/channels/feishu/participants" {
					t.Fatalf("request %d = %s %s, want participant create", call, req.Method, req.URL.String())
				}
				var payload map[string]any
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				channelUser, _ := payload["channel_user"].(map[string]any)
				appConfig, _ := payload["channel_app_config"].(map[string]any)
				agentBinding, _ := payload["agent_binding"].(map[string]any)
				if payload["id"] != "pt-dev" || payload["type"] != "agent" || payload["name"] != "dev" {
					t.Fatalf("payload = %#v, want canonical dev agent participant", payload)
				}
				if channelUser["kind"] != "app_id" || channelUser["ref"] != nil {
					t.Fatalf("channel_user = %#v, want app_id without open_id ref", channelUser)
				}
				if appConfig["app_id"] != "cli_dev" || appConfig["app_secret"] != "secret-value" {
					t.Fatalf("channel_app_config = %#v, want app credentials", appConfig)
				}
				if agentBinding["mode"] != "reuse" || agentBinding["agent_id"] != "agent-dev" {
					t.Fatalf("agent_binding = %#v, want reuse agent-dev", agentBinding)
				}
				return jsonResponse(http.StatusCreated, feishuBotParticipantResponse("pt-dev", "dev", "agent-dev", "cli_dev")), nil
			default:
				t.Fatalf("unexpected request %d: %s %s", call, req.Method, req.URL.String())
				return nil, nil
			}
		}),
	}

	err := app.Execute(context.Background(), []string{
		"--endpoint", "http://example.test",
		"--output", "json",
		"pt", "bind",
		"--channel", "feishu",
		"--feishu-kind", "bot",
		"--agent", "u-dev",
		"--app-id", "cli_dev",
		"--app-secret-stdin",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v; stderr=%s", err, stderr.String())
	}
	if call != 3 {
		t.Fatalf("request count = %d, want 3", call)
	}
	if strings.Contains(stdout.String(), "secret-value") || strings.Contains(stderr.String(), "secret-value") {
		t.Fatalf("output leaked app secret: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	for _, want := range []string{"\"participant_id\": \"pt-dev\"", "\"agent_id\": \"agent-dev\"", "\"restart_status\": \"restart_skipped\""} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %s", stdout.String(), want)
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
			if req.URL.String() != "http://env.example.test/api/v1/channels/feishu/participants" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://env.example.test/api/v1/channels/feishu/participants")
			}
			if got := req.Header.Get("Authorization"); got != "Bearer env-secret-token" {
				t.Fatalf("Authorization = %q, want %q", got, "Bearer env-secret-token")
			}
			return jsonResponse(http.StatusOK, `[]`), nil
		}),
	}

	if err := app.Execute(context.Background(), []string{"participant", "list", "--channel", "feishu"}); err != nil {
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
			if req.URL.String() != "http://flag.example.test/api/v1/channels/feishu/participants" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "http://flag.example.test/api/v1/channels/feishu/participants")
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
		"participant", "list", "--channel", "feishu",
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteParticipantDeleteUsesAPIClient(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		httpClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodDelete)
			}
			if req.URL.String() != "http://example.test/api/v1/channels/feishu/participants/u-alice" {
				t.Fatalf("url = %q, want feishu participant delete route", req.URL.String())
			}
			return jsonResponse(http.StatusNoContent, ``), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "participant", "delete", "--channel", "feishu", "u-alice"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteParticipantDeleteSupportsJSONOutput(t *testing.T) {
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

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "--output", "json", "participant", "delete", "--channel", "feishu", "u-alice"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{`"command": "participant"`, `"action": "delete"`, `"status": "deleted"`, `"id": "u-alice"`, `"channel": "feishu"`} {
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
			if req.URL.String() != "http://example.test/api/v1/channels/csgclaw/rooms/oc_alpha/members" {
				t.Fatalf("url = %q, want csgclaw room members route", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `[{"id":"u_alice","name":"Alice","role":"worker","is_online":true}]`), nil
		}),
	}

	err := app.Execute(context.Background(), []string{"--endpoint", "http://example.test", "member", "list", "--room-id", "oc_alpha"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertTableHasRow(t, stdout.String(), "u_alice", "Alice", "worker", "true")
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

func feishuBotParticipantResponse(id, name, agentID, appID string) string {
	return fmt.Sprintf(
		`{"id":%q,"name":%q,"type":"agent","channel":"feishu","agent_id":%q,"channel_user_kind":"app_id","channel_app_config":{"app_id":%q,"app_secret":%q},"lifecycle_status":"active","created_at":"2026-04-12T09:00:00Z"}`,
		id,
		name,
		agentID,
		appID,
		participant.RedactedSecretValue,
	)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
