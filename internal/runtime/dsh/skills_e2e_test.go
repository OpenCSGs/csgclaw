package dsh

import (
	"context"
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
	skill "csgclaw/internal/skill/state"
)

func TestSkillEnablementNativeDSHE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_DSH_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_DSH_BINARY to run native DSH skill discovery")
	}
	var mu sync.Mutex
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			http.Error(w, "read request", 400)
			return
		}
		mu.Lock()
		requestBody = string(body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(v any) { b, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", b) }
		emit(map[string]any{"id": "r1", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "done"}, "finish_reason": nil}}})
		emit(map[string]any{"id": "r1", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 1, "total_tokens": 101}})
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	home := filepath.Join(t.TempDir(), "agent")
	profile := agentruntime.Profile{BaseURL: server.URL + "/v1", APIKey: "fixture-key", ModelID: "fixture-model"}
	states := map[string]skill.State{}
	rt := New(Dependencies{ResolveBinary: func(context.Context) (dshcli.Info, error) {
		return dshcli.Info{Path: binary, Version: "0.1.5-rc.2"}, nil
	}, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "alice", RuntimeID: "rt-alice", Profile: profile, SkillStates: states}, nil
	}, AgentHome: func(string) (string, error) { return home, nil }})
	defer rt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := rt.Provision(ctx, agentruntime.ProvisionRequest{RuntimeID: "rt-alice", AgentID: "alice", Profile: profile}); err != nil {
		t.Fatal(err)
	}
	const skillName = "csgclaw-native-enablement-probe"
	skillPath := filepath.Join(rt.Layout(home).SkillsRoot, skillName, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("---\nname: "+skillName+"\ndescription: Review local code changes.\n---\nReview the supplied code.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		name    string
		enabled bool
	}{{"enabled", true}, {"disabled", false}, {"reenabled", true}} {
		t.Run(step.name, func(t *testing.T) {
			if _, err := rt.Stop(ctx, handle); err != nil {
				t.Fatal(err)
			}
			states[skillName] = skill.State{Enabled: step.enabled}
			if _, err := rt.Start(ctx, handle); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			requestBody = ""
			mu.Unlock()
			result := rt.Conversation(handle.RuntimeID).Run(ctx, contract.TurnRequest{ID: contract.TurnID(step.name), ConversationKey: contract.ConversationKey("room:" + step.name), Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "Reply done."}}}, contract.EventSinkFunc(func(context.Context, contract.TurnEvent) error { return nil }))
			if result.Status != contract.TurnSucceeded {
				log, _ := os.ReadFile(filepath.Join(home, hostStateDirName, stderrFileName))
				t.Fatalf("turn failed: %+v\n%s", result, log)
			}
			mu.Lock()
			body := requestBody
			mu.Unlock()
			if body == "" {
				t.Fatal("DSH did not send a model request")
			}
			if found := strings.Contains(body, skillName); found != step.enabled {
				t.Fatalf("model skill catalog includes probe = %v, want %v", found, step.enabled)
			}
			if _, err := os.Stat(skillPath); err != nil {
				t.Fatalf("original skill file unavailable: %v", err)
			}
		})
	}
}
