package dsh

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/dshcli"
	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"
)

func TestOutputLimitNativeDSHE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_DSH_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_DSH_BINARY to exercise native output limits")
	}
	for _, tc := range []struct {
		name, model string
		advertised  int64
		wantBudget  int
		partial     bool
	}{
		{name: "QA Flash alias", model: "deepseek-v4.1-flash", wantBudget: 65536},
		{name: "official Flash alias", model: "deepseek-flash", wantBudget: 65536},
		{name: "1017 snapshot", model: "deepseek-v4.1-flash-1017", wantBudget: 65536},
		{name: "smaller deployment limit", model: "deepseek-v4.1-flash", advertised: 4096, wantBudget: 4096},
		{name: "unknown model", model: "custom-model", wantBudget: 8192},
		{name: "partial answer", model: "deepseek-flash", wantBudget: 65536, partial: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var limits []int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" {
					http.NotFound(w, r)
					return
				}
				var body struct {
					Model     string `json:"model"`
					MaxTokens int    `json:"max_tokens"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if body.Model != tc.model {
					t.Errorf("model=%s want=%s", body.Model, tc.model)
				}
				mu.Lock()
				limits = append(limits, body.MaxTokens)
				mu.Unlock()
				w.Header().Set("Content-Type", "text/event-stream")
				emit := func(delta map[string]string, finish any) {
					data, _ := json.Marshal(map[string]any{"id": "fixture", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
					fmt.Fprintf(w, "data: %s\n\n", data)
				}
				emit(map[string]string{"role": "assistant", "reasoning_content": "Working on the task."}, nil)
				if tc.partial {
					emit(map[string]string{"content": "Partial answer"}, nil)
				}
				emit(map[string]string{}, "length")
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			home := filepath.Join(t.TempDir(), "agent")
			profile := agentruntime.Profile{BaseURL: server.URL + "/v1", APIKey: "fixture-key", ModelID: tc.model, ReasoningEffort: "auto",
				ModelMetadata: modelcap.Resolve("api", server.URL, tc.model, modelcap.Metadata{MaxOutputTokens: tc.advertised}, modelcap.Metadata{}),
			}
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
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := rt.Provision(ctx, agentruntime.ProvisionRequest{RuntimeID: "rt-alice", AgentID: "alice", Profile: profile}); err != nil {
				t.Fatal(err)
			}
			handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", Profile: profile})
			if err != nil {
				t.Fatal(err)
			}
			result := rt.Conversation(handle.RuntimeID).Run(ctx, contract.TurnRequest{
				ID: "turn", ConversationKey: "room:test", Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "Make a knowledge card."}},
			}, nil)
			if tc.partial {
				if result.Status != contract.TurnSucceeded || result.Output != "Partial answer" {
					t.Fatalf("partial answer was lost: %+v", result)
				}
			} else if result.Status != contract.TurnFailed || contract.ErrorCodeOf(result.Error) != "output_token_limit" {
				t.Fatalf("missing output-limit failure: %+v", result)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(limits) != 1 || limits[0] != tc.wantBudget {
				t.Fatalf("request limits=%v want one request with max_tokens=%d", limits, tc.wantBudget)
			}
		})
	}
}
