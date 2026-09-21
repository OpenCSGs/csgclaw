package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/cli/command"
)

func TestAppCommandsUseSharedAuthenticatedAPI(t *testing.T) {
	t.Setenv("CSGCLAW_CALLER_AGENT_ID", "")
	for _, tc := range []struct {
		action, method, path string
		withID, withFile     bool
	}{
		{"catalog", "GET", "/api/v1/apps", false, false},
		{"catalog", "GET", "/api/v1/apps/install-1", true, false},
		{"list", "GET", "/api/v1/agents/agent-dev/apps", false, false},
		{"get", "GET", "/api/v1/agents/agent-dev/apps/install-1", true, false},
		{"add", "POST", "/api/v1/agents/agent-dev/apps", false, true},
		{"update", "PATCH", "/api/v1/agents/agent-dev/apps/install-1", true, true},
		{"probe", "POST", "/api/v1/agents/agent-dev/apps:probe", true, true},
		{"connect", "POST", "/api/v1/agents/agent-dev/apps/install-1/connect", true, false},
		{"disconnect", "POST", "/api/v1/agents/agent-dev/apps/install-1/disconnect", true, false},
		{"remove", "DELETE", "/api/v1/agents/agent-dev/apps/install-1", true, false},
	} {
		t.Run(tc.action+tc.path, func(t *testing.T) {
			var calls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != tc.method || r.URL.Path != tc.path {
					t.Errorf("request = %s %s, want %s %s", r.Method, r.URL.Path, tc.method, tc.path)
				}
				if r.Header.Get("Authorization") != "Bearer management-fixture" {
					t.Error("missing management authentication")
				}
				if tc.withFile {
					var input map[string]any
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
					}
					creds, _ := input["credentials"].(map[string]any)
					if creds["token"] != "private-fixture-value" {
						t.Error("JSON input was not forwarded")
					}
					if tc.action == "probe" && input["installation_id"] != "install-1" {
						t.Error("probe ID was not forwarded")
					}
				}
				if tc.action == "remove" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"installation_id":"install-1","app_id":"gitlab","name":"Work","status":"connected","credentials_set":{"token":true}}`)
			}))
			defer server.Close()
			var stdout, stderr bytes.Buffer
			run := &command.Context{Program: "csgclaw", Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader(`{"app_id":"gitlab","name":"Work","credentials":{"token":"private-fixture-value"}}`)}
			args := []string{tc.action, "--agent", "agent-dev"}
			if tc.withID {
				args = append(args, "--id", "install-1")
			}
			if tc.withFile {
				args = append(args, "--file", "-")
			}
			if err := NewCmd().Run(context.Background(), run, args, command.GlobalOptions{Endpoint: server.URL, Token: "management-fixture", Output: "json"}); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("requests = %d", calls)
			}
			for _, secret := range []string{"private-fixture-value", "management-fixture"} {
				if strings.Contains(stdout.String()+stderr.String(), secret) {
					t.Fatal("CLI echoed credentials")
				}
			}
			if !strings.Contains(stdout.String(), "install-1") {
				t.Fatal("missing API result")
			}
		})
	}
}

func TestAppRequestFileAndInvalidInputDoNotEchoSecrets(t *testing.T) {
	t.Setenv("CSGCLAW_CALLER_AGENT_ID", "")
	file := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(file, []byte(`{"credentials":{"token":"file-private-value"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	input, err := readInput(&command.Context{}, file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(input["credentials"], []byte("file-private-value")) {
		t.Fatal("file was not read")
	}
	for _, body := range []string{`{"credentials":{"token":"do-not-echo"},`, strings.Repeat("do-not-echo", 1<<18), `null`, `{} {"token":"do-not-echo"}`} {
		_, err := readInput(&command.Context{Stdin: strings.NewReader(body)}, "-")
		if err == nil || strings.Contains(err.Error(), "do-not-echo") {
			t.Fatalf("unsafe input error: %v", err)
		}
	}
}

func TestAgentCLIOnlyReadsAndReturnsSettingsLink(t *testing.T) {
	t.Setenv("CSGCLAW_CALLER_AGENT_ID", "agent-dev")
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/api/v1/agents/agent-dev/apps" || r.Header.Get("X-CSGClaw-Caller-Agent") != "agent-dev" {
			t.Errorf("unexpected Agent request %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"items":[{"installation_id":"install-1","app_id":"gitlab","name":"Work","status":"connected"}]}`)
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	run := &command.Context{Program: "csgclaw-cli", Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader("never read")}
	globals := command.GlobalOptions{Endpoint: server.URL, Token: "agent-scoped-fixture", Output: "json"}
	if err := NewCmd().Run(context.Background(), run, []string{"get", "--id", "install-1"}, globals); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "settings_url") || !strings.Contains(stdout.String(), "tab=apps") {
		t.Fatalf("missing settings link: %s", stdout.String())
	}
	err := NewCmd().Run(context.Background(), run, []string{"add", "--file", "-"}, globals)
	if err == nil || !strings.Contains(err.Error(), "Change App settings in the UI") || calls != 1 {
		t.Fatalf("Agent mutation was not rejected: %v calls=%d", err, calls)
	}
}
