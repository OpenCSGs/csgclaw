package participant

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"csgclaw/cli/command"
)

func TestCreateParticipantAutoPublishFlag(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				value, _ := body["auto_publish"].(bool)
				if value != enabled {
					t.Error("lost publication intent")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"pt-startup","agent_id":"startup","type":"agent"}`))
			}))
			defer server.Close()
			args := []string{"create", "--name", "Startup", "--type", "agent", "--bind", "create", "--agent-id", "startup"}
			if enabled {
				args = append(args, "--auto-publish=true")
			}
			run := &command.Context{Program: "csgclaw-cli", Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
			if err := NewCmd().Run(context.Background(), run, args, command.GlobalOptions{Endpoint: server.URL, Output: "json"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestAutoPublishRejectsUnboundParticipants(t *testing.T) {
	run := &command.Context{Program: "csgclaw-cli", Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	if err := NewCmd().Run(context.Background(), run, []string{"create", "--name", "Human", "--type", "human", "--auto-publish"}, command.GlobalOptions{}); err == nil {
		t.Fatal("accepted unbound publication")
	}
}
