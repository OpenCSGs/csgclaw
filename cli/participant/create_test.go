package participant

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/cli/command"
	"csgclaw/internal/participant"
)

func TestCreateVerifiesExplicitParticipantID(t *testing.T) {
	for _, tc := range []struct {
		input, returned string
		wantError       bool
	}{
		{"pt-İpek", "pt-ipek", false},
		{"pt-e\u0301quipe", "pt-e-quipe", false},
		{"pt-a" + strings.Repeat("界", 16), participant.CanonicalID("pt-a" + strings.Repeat("界", 16)), false},
		{"pt-İpek", "pt-other", true},
		{"pt-İpek", "", true},
	} {
		t.Run(tc.input+"/"+tc.returned, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(participant.Participant{ID: tc.returned, AgentID: "agent-bound"})
			}))
			defer server.Close()
			out := &bytes.Buffer{}
			run := &command.Context{Program: "csgclaw", Stdout: out, Stderr: &bytes.Buffer{}}
			err := NewCmd().Run(t.Context(), run, []string{"create", "--id", tc.input, "--name", "Template", "--channel", "CSGClaw", "--bind", "CREATE"}, command.GlobalOptions{Endpoint: server.URL, Output: "json"})
			if (err != nil) != tc.wantError {
				t.Fatalf("create error: %v", err)
			}
			if tc.wantError {
				if out.Len() != 0 {
					t.Fatalf("unexpected success output: %s", out)
				}
				return
			}
			var rows []participant.Participant
			if err := json.Unmarshal(out.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0].AgentID != "agent-bound" {
				t.Fatalf("create output: %s, error: %v", out, err)
			}
		})
	}
}
