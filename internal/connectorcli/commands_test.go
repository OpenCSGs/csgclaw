package connectorcli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStatusConditionsAndSelectedFields(t *testing.T) {
	spec := Spec{StatusMatchJSON: JSONMatch{{"/identities/user/verified": true}}, StatusFields: []string{"/appId", "/identities/user/verified"}}
	ready, fields, err := spec.ReadStatus([]byte("notice\n" + `{"appId":"app","identities":{"user":{"verified":true,"access_token":"private"}},"device_code":"secret"}`))
	if err != nil || !ready || string(fields) != `{"appId":"app","identities":{"user":{"verified":true}}}` {
		t.Fatalf("ready=%v fields=%s err=%v", ready, fields, err)
	}
	for _, data := range []string{`{"identities":{"user":{"verified":false}}}`, `{"identities":{"user":{"verified":"true"}}}`, `{"identities":{"user":{}}}`} {
		ready, _, err = spec.ReadStatus([]byte(data))
		if err != nil || ready {
			t.Fatalf("accepted unverified status %s: %v", data, err)
		}
	}
	if _, _, err = spec.ReadStatus([]byte("invalid JSON")); err == nil {
		t.Fatal("invalid status accepted")
	}
}
func TestCommandGuardsPreserveFailures(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		err          error
		calls        int
		fails        bool
	}{
		{"invalid response", "unexpected output", nil, 1, true},
		{"already configured", `{"ready":true}`, nil, 1, false},
		{"unconfigured", `{"error":{"subtype":"not_configured"}}`, errors.New("exit 3"), 2, false},
		{"storage error", `{"error":{"subtype":"storage_error"}}`, errors.New("exit 3"), 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			workflow := Workflow{Client: Client{Runner: func(_ context.Context, _ Client, _ io.Reader, args ...string) ([]byte, error) {
				calls++
				if args[0] == "status" {
					return []byte(tc.output), tc.err
				}
				return nil, nil
			}}}
			err := workflow.Run(context.Background(), Commands{{Args: []string{"init"}, Unless: &Check{Args: []string{"status"}, MatchJSON: JSONMatch{{"/ready": true}}, UnavailableJSON: JSONMatch{{"/error/subtype": "not_configured"}}}}})
			if (err != nil) != tc.fails || calls != tc.calls {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
func testDeviceCommand() Command {
	return Command{Args: []string{"login", "--scope", "${REQUESTED_SCOPES}"}, DeviceFlow: &DeviceFlow{CodeField: "/device_code", URLField: "/verification_url", ExpiresField: "/expires_in", PollArgs: []string{"poll", "${DEVICE_CODE}"}, AcceptErrorJSON: JSONMatch{{"/event": "authorization_complete", "/warning/type": "missing_scope"}}}}
}
func TestDeviceAuthorizationResumeAndScopeChange(t *testing.T) {
	command := testDeviceCommand()
	requests := 0
	var saved *DeviceAuthorization
	runner := func(ctx context.Context, c Client, _ io.Reader, args ...string) ([]byte, error) {
		if args[0] == "login" {
			requests++
			return []byte(`{"device_code":"private","verification_url":"https://example.com/device","expires_in":600}`), nil
		}
		if args[1] != "private" {
			t.Fatal("poll did not use device code")
		}
		return nil, context.Canceled
	}
	workflow := Workflow{Client: Client{Runner: runner, AuthorizationDomains: []string{"example.com"}}, RequestedScopes: "docs:read", SaveDevice: func(value *DeviceAuthorization) error { saved = value; return nil }}
	if err := workflow.Run(context.Background(), Commands{command}); err == nil || saved == nil {
		t.Fatalf("pending authorization not saved: %v", err)
	}
	for _, scopes := range []string{"docs:read", "docs:write"} {
		resumed := Workflow{Client: workflow.Client, RequestedScopes: scopes, Device: saved, SaveDevice: workflow.SaveDevice}
		_ = resumed.Run(context.Background(), Commands{command})
	}
	if requests != 2 {
		t.Fatalf("authorization requests=%d", requests)
	}
	workflow.Device = saved
	workflow.RequestedScopes = "docs:write"
	workflow.Client.Runner = func(context.Context, Client, io.Reader, ...string) ([]byte, error) {
		return []byte(`{"event":"authorization_complete","warning":{"type":"missing_scope"}}`), errors.New("exit 3")
	}
	if err := workflow.Run(context.Background(), Commands{command}); err != nil {
		t.Fatal(err)
	}
	if saved != nil {
		t.Fatal("completed device authorization retained")
	}
}
func TestDeviceAuthorizationRejectsUnexpectedResults(t *testing.T) {
	for _, output := range []string{`{"device_code":"private","verification_url":"https://attacker.example/device","expires_in":600}`, `{"device_code":"private","verification_url":"https://example.com/device","expires_in":0}`, `{"verification_url":"https://example.com/device","expires_in":600}`} {
		workflow := Workflow{Client: Client{AuthorizationDomains: []string{"example.com"}, Runner: func(context.Context, Client, io.Reader, ...string) ([]byte, error) { return []byte(output), nil }}}
		if err := workflow.Run(context.Background(), Commands{testDeviceCommand()}); err == nil {
			t.Fatal("invalid device response accepted")
		}
	}
	for _, output := range []string{`{"error":{"type":"missing_scope"}}`, `{"event":"authorization_complete","warning":{"type":"storage_error"}}`} {
		calls := 0
		workflow := Workflow{Client: Client{AuthorizationDomains: []string{"example.com"}, Runner: func(context.Context, Client, io.Reader, ...string) ([]byte, error) {
			calls++
			if calls == 1 {
				return []byte(`{"device_code":"private","verification_url":"https://example.com/device","expires_in":600}`), nil
			}
			return []byte(output), errors.New("exit 3")
		}}}
		if err := workflow.Run(context.Background(), Commands{testDeviceCommand()}); err == nil {
			t.Fatal("failed authorization accepted")
		}
	}
}
func TestDeviceAuthorizationUsesActualProcessOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	root := t.TempDir()
	executable := filepath.Join(root, "fixture")
	script := `#!/bin/sh
case "$1" in
login) printf '%s\n' '{"device_code":"private","verification_url":"https://example.com/device","expires_in":60}';;
poll) printf '%s\n' '{"event":"authorization_complete","warning":{"type":"missing_scope"}}'; exit 3;;
esac
`
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	var shown string
	workflow := Workflow{Client: Client{Executable: executable, ConfigDir: root, AuthorizationDomains: []string{"example.com"}, URL: func(value string) { shown = value }}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := workflow.Run(ctx, Commands{testDeviceCommand()}); err != nil {
		t.Fatal(err)
	}
	if shown != "https://example.com/device" {
		t.Fatalf("URL=%s", shown)
	}
}
func TestCommandArgumentsAndEnvironment(t *testing.T) {
	var commands Commands
	if err := json.Unmarshal([]byte(`["auth","login"]`), &commands); err != nil || !reflect.DeepEqual(commands, Commands{{Args: []string{"auth", "login"}}}) {
		t.Fatalf("commands=%v err=%v", commands, err)
	}
	t.Setenv("FIXTURE_TOKEN", "host-secret")
	t.Setenv("FIXTURE_CONFIG", "host-config")
	spec := Spec{Environment: map[string]string{"FIXTURE_CONFIG": "${CONFIG_DIR}"}, ExcludeEnvironment: []string{"FIXTURE_*"}}
	environment := strings.Join((Client{Env: spec.Env("resource-config"), ExcludeEnv: spec.ExcludesEnvironment}).Environment(), "\n")
	if strings.Contains(environment, "host-secret") || strings.Contains(environment, "host-config") || !strings.Contains(environment, "FIXTURE_CONFIG=resource-config") {
		t.Fatal("package environment was not isolated")
	}
}

func TestCommandAcceptsOnlyConfiguredErrors(t *testing.T) {
	for _, state := range []string{"not_configured", "storage_error", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			workflow := Workflow{Client: Client{Runner: func(context.Context, Client, io.Reader, ...string) ([]byte, error) {
				if state == "cancelled" {
					cancel()
					return []byte(`{"error":{"subtype":"not_configured"}}`), context.Canceled
				}
				return []byte(`{"error":{"subtype":"` + state + `"}}`), errors.New("exit 3")
			}}}
			err := workflow.Run(ctx, Commands{{Args: []string{"config", "remove"}, AcceptErrorJSON: JSONMatch{{"/error/subtype": "not_configured"}}}})
			if (err == nil) != (state == "not_configured") {
				t.Fatalf("state=%s error=%v", state, err)
			}
		})
	}
}

func TestWorkflowStageCompletion(t *testing.T) {
	for _, mode := range []string{"success", "already authorized", "failed", "cancelled", "unnamed", "callback failed"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			command := Command{Args: []string{"login"}, Stage: "authorization"}
			if mode == "already authorized" {
				command.Unless = &Check{Args: []string{"status"}, MatchJSON: JSONMatch{{"/ready": true}}}
			}
			if mode == "unnamed" {
				command.Stage = ""
			}
			workflow := Workflow{Client: Client{Runner: func(_ context.Context, _ Client, _ io.Reader, args ...string) ([]byte, error) {
				if mode == "failed" {
					return nil, errors.New("authorization failed")
				}
				if mode == "cancelled" {
					cancel()
				}
				if mode == "already authorized" && args[0] != "status" {
					t.Fatal("repeated authorization")
				}
				return []byte(`{"ready":true}`), nil
			}}, StageCompleted: func(context.Context) error {
				calls++
				if mode == "callback failed" {
					return errors.New("save failed")
				}
				return nil
			}}
			err := workflow.Run(ctx, Commands{command})
			wantCalls := 0
			if mode == "success" || mode == "already authorized" || mode == "callback failed" {
				wantCalls = 1
			}
			wantError := mode == "failed" || mode == "cancelled" || mode == "callback failed"
			if calls != wantCalls || (err != nil) != wantError {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
