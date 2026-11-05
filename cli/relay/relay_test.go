package relay

import (
	"bytes"
	"context"
	relayapi "csgclaw/internal/relay"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"csgclaw/cli/command"
)

func TestCallUsesRelayCredentialsAndContinuation(t *testing.T) {
	t.Setenv("CSGCLAW_RELAY_TOKEN", "relay-only-secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer relay-only-secret" || r.Header.Get("Idempotency-Key") != "stable-call" || r.URL.Path != "/v1/responses" {
			t.Error("invalid relay request headers/path")
		}
		if r.Header.Get("X-CSG-Client-ID") != "" || r.Header.Get("X-CSG-User-UUID") != "" {
			t.Error("standalone token mode sent forwarding identity headers")
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req["previous_response_id"] != "resp_before" || req["model"] != "agt_remote" || req["input"] != "hello" {
			t.Errorf("request: %v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_now","object":"response","status":"completed","output":[]}`))
	}))
	defer server.Close()
	t.Setenv("CSGCLAW_RELAY_URL", server.URL)
	var stdout, stderr bytes.Buffer
	run := &command.Context{Program: "csgclaw", Stdout: &stdout, Stderr: &stderr}
	err := NewCmd().Run(context.Background(), run, []string{"call", "--agent", "agt_remote", "--input", "hello", "--previous-response", "resp_before", "--key", "stable-call"}, command.GlobalOptions{Token: "local-only-secret"})
	if err != nil || !strings.Contains(stdout.String(), "resp_now") {
		t.Fatalf("%s %v", stdout.String(), err)
	}
	if strings.Contains(stdout.String()+stderr.String(), "secret") {
		t.Fatal("credential leaked")
	}
}

func TestInvalidArgumentsAndMissingToken(t *testing.T) {
	t.Setenv("CSGCLAW_RELAY_TOKEN", "")
	var stdout, stderr bytes.Buffer
	run := &command.Context{Program: "csgclaw", Stdout: &stdout, Stderr: &stderr}
	for _, args := range [][]string{{"call", "--agent", "a", "--input", "x"}, {"unknown"}, {"list", "unexpected"}, {"mock"}} {
		if err := NewCmd().Run(context.Background(), run, args, command.GlobalOptions{}); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestFailedRemoteResponseIsPrintedAndReturnsError(t *testing.T) {
	t.Setenv("CSGCLAW_RELAY_TOKEN", "test-only")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"resp_failed","status":"failed","error":{"code":"execution_failed","message":"failure"}}`))
	}))
	defer server.Close()
	t.Setenv("CSGCLAW_RELAY_URL", server.URL)
	var stdout, stderr bytes.Buffer
	run := &command.Context{Program: "csgclaw", Stdout: &stdout, Stderr: &stderr}
	err := NewCmd().Run(context.Background(), run, []string{"call", "--agent", "a", "--input", "x"}, command.GlobalOptions{})
	if err == nil || !strings.Contains(stdout.String(), "resp_failed") {
		t.Fatalf("%s %v", stdout.String(), err)
	}
}

func TestMockCommandRemoved(t *testing.T) {
	var stdout, stderr bytes.Buffer
	run := &command.Context{Program: "csgclaw", Stdout: &stdout, Stderr: &stderr}
	err := NewCmd().Run(context.Background(), run, []string{"mock"}, command.GlobalOptions{})
	if err == nil || !strings.Contains(err.Error(), `unknown relay subcommand "mock"`) {
		t.Fatalf("unexpected removed command result: %v", err)
	}
	_ = NewCmd().Run(context.Background(), run, []string{"--help"}, command.GlobalOptions{})
	if strings.Contains(stdout.String()+stderr.String(), "mock") {
		t.Fatal("removed mock service is still advertised")
	}
}

func TestLiveStandaloneTokenCommands(t *testing.T) {
	endpoint, token := os.Getenv("CSGCLAW_TEST_RELAY_URL"), os.Getenv("CSGCLAW_TEST_RELAY_TOKEN")
	if endpoint == "" || token == "" {
		t.Skip("live standalone relay not configured")
	}
	t.Setenv("CSGCLAW_RELAY_TOKEN", token)
	t.Setenv("CSGCLAW_RELAY_URL", endpoint)
	client, err := relayapi.NewClient(endpoint, token, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	publication, err := client.Publish(ctx, uuid.NewString(), relayapi.PublishRequest{Name: "diagnostic-token", SourceAgentID: uuid.NewString(), Visibility: "private"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Request(context.Background(), http.MethodDelete, "/v1/agents/"+publication.Agent.ID, "", nil, nil)
	for _, args := range [][]string{{"list", "--scope", "mine"}, {"get", "--id", publication.Agent.ID}, {"unpublish", "--id", publication.Agent.ID}} {
		var stdout, stderr bytes.Buffer
		run := &command.Context{Program: "csgclaw", Stdout: &stdout, Stderr: &stderr}
		if err := NewCmd().Run(ctx, run, args, command.GlobalOptions{}); err != nil {
			t.Fatalf("standalone diagnostic %s failed: %v", args[0], err)
		}
	}
}
