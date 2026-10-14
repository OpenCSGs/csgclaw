package dsh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/dshcli"
	agentruntime "csgclaw/internal/runtime"
)

func TestMemoryNativeDSHE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_DSH_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_DSH_BINARY")
	}
	const summary = "The user prefers concise Chinese replies."
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			http.Error(w, "read request", 400)
			return
		}
		mu.Lock()
		bodies = append(bodies, string(data))
		first := len(bodies) == 1
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(delta any, reason any) {
			raw, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})
			fmt.Fprintf(w, "data: %s\n\n", raw)
		}
		if first {
			empty := sha256.Sum256(nil)
			args, _ := json.Marshal(map[string]any{"content": summary, "expected_revision": hex.EncodeToString(empty[:])})
			emit(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "save-memory", "type": "function", "function": map[string]any{"name": "memory_update", "arguments": string(args)}}}}, nil)
			emit(map[string]any{}, "tool_calls")
		} else {
			emit(map[string]any{"role": "assistant", "content": "done"}, nil)
			emit(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	home := filepath.Join(t.TempDir(), "agent")
	profile := agentruntime.Profile{BaseURL: server.URL + "/v1", APIKey: "fixture-key", ModelID: "fixture-model"}
	opts := map[string]any{}
	rt := New(Dependencies{ResolveBinary: func(context.Context) (dshcli.Info, error) { return dshcli.Info{Path: binary}, nil }, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "alice", RuntimeID: "rt-alice", Profile: profile, RuntimeOptions: opts}, nil
	}, AgentHome: func(string) (string, error) { return home, nil }})
	defer rt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := rt.Provision(ctx, agentruntime.ProvisionRequest{AgentID: "alice", RuntimeID: "rt-alice", Profile: profile}); err != nil {
		t.Fatal(err)
	}
	handle, err := rt.New(ctx, agentruntime.Spec{AgentID: "alice", RuntimeID: "rt-alice", Profile: profile})
	if err != nil {
		log, _ := os.ReadFile(filepath.Join(home, hostStateDirName, stderrFileName))
		t.Fatalf("start: %v\n%s", err, log)
	}
	run := func(key string) string {
		t.Helper()
		result := rt.Conversation(handle.RuntimeID).Run(ctx, contract.TurnRequest{ID: contract.TurnID(key), ConversationKey: contract.ConversationKey(key), Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "Remember my preference, then reply done."}}}, nil)
		if result.Status != contract.TurnSucceeded {
			log, _ := os.ReadFile(filepath.Join(home, hostStateDirName, stderrFileName))
			t.Fatalf("turn: %+v\n%s", result, log)
		}
		mu.Lock()
		defer mu.Unlock()
		return bodies[len(bodies)-1]
	}
	run("learn")
	document, err := rt.ReadMemoryDocument(ctx, home, opts)
	if err != nil || document.Content != summary {
		t.Fatalf("learned memory = %+v, %v", document, err)
	}
	for _, step := range []struct {
		name    string
		enabled bool
	}{{"resumed-process", true}, {"disabled", false}, {"reenabled", true}} {
		if _, err := rt.Stop(ctx, handle); err != nil {
			t.Fatal(err)
		}
		opts, err = rt.ConfigureMemory(opts, step.enabled)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rt.Start(ctx, handle); err != nil {
			t.Fatal(err)
		}
		body := run(step.name)
		if strings.Contains(body, summary) != step.enabled || strings.Contains(body, `"memory_update"`) != step.enabled {
			t.Fatalf("%s: memory context/tools visibility incorrect: %s", step.name, body)
		}
	}
}
