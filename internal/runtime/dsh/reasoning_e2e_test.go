package dsh

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/dshcli"
	agentruntime "csgclaw/internal/runtime"
)

func TestReasoningEffortNativeDSHE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_DSH_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_DSH_BINARY to run native DSH reasoning selection")
	}
	for _, test := range []struct{ effort, want, thinking string }{
		{effort: "minimal", want: "low", thinking: "enabled"},
		{effort: "low", want: "low", thinking: "enabled"},
		{effort: "medium", want: "high", thinking: "enabled"},
		{effort: "high", want: "high", thinking: "enabled"},
		{effort: "xhigh", want: "max", thinking: "enabled"},
		{effort: "none", thinking: "disabled"},
		{effort: "auto", want: "high", thinking: "enabled"},
	} {
		t.Run(test.effort, func(t *testing.T) {
			type modelRequest struct {
				Model           string `json:"model"`
				ReasoningEffort string `json:"reasoning_effort"`
				Thinking        struct {
					Type string `json:"type"`
				} `json:"thinking"`
			}
			requests := make(chan modelRequest, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" {
					http.NotFound(w, r)
					return
				}
				var body modelRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				requests <- body
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"id\":\"r1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"done\"},\"finish_reason\":null}]}\n\n")
				fmt.Fprint(w, "data: {\"id\":\"r1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":1,\"total_tokens\":101}}\n\n")
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			home := filepath.Join(t.TempDir(), "agent")
			profile := agentruntime.Profile{BaseURL: server.URL + "/v1", APIKey: "fixture-key", ModelID: "fixture-model", ReasoningEffort: test.effort}
			rt := New(Dependencies{
				ResolveBinary: func(context.Context) (dshcli.Info, error) {
					return dshcli.Info{Path: binary, Version: dshcli.DefaultVersion}, nil
				},
				ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
					return AgentRef{ID: "alice", RuntimeID: "rt-alice", Profile: profile}, nil
				},
				AgentHome: func(string) (string, error) { return home, nil },
			})
			defer rt.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			if err := rt.Provision(ctx, agentruntime.ProvisionRequest{RuntimeID: "rt-alice", AgentID: "alice", Profile: profile}); err != nil {
				t.Fatal(err)
			}
			handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", Profile: profile})
			if err != nil {
				t.Fatal(err)
			}
			for _, phase := range []string{"new", "resume"} {
				if phase == "resume" {
					if _, err := rt.Stop(ctx, handle); err != nil {
						t.Fatal(err)
					}
					if _, err := rt.Start(ctx, handle); err != nil {
						t.Fatal(err)
					}
				}
				result := rt.Conversation(handle.RuntimeID).Run(ctx, contract.TurnRequest{
					ID: contract.TurnID(phase), ConversationKey: "room:test",
					Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "Reply done."}},
				}, nil)
				if result.Status != contract.TurnSucceeded || result.Output != "done" || !result.Dispatched {
					log, _ := os.ReadFile(filepath.Join(home, hostStateDirName, stderrFileName))
					t.Fatalf("%s turn = %+v\n%s", phase, result, log)
				}
				select {
				case body := <-requests:
					if body.Model != profile.ModelID || body.ReasoningEffort != test.want || body.Thinking.Type != test.thinking {
						t.Fatalf("%s model request = %+v; want effort %q, thinking %q", phase, body, test.want, test.thinking)
					}
				default:
					t.Fatalf("%s turn sent no model request", phase)
				}
			}
		})
	}
}
