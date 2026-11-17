package dsh

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/dshcli"
	agentruntime "csgclaw/internal/runtime"
)

func TestThoughtStreamReconcilesOnlyMatchingCommittedMessage(t *testing.T) {
	var thoughts []string
	var output string
	rt := &Runtime{}
	proc := &process{active: map[string]*activeTurn{"s": {
		sink: contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error {
			switch event.Kind {
			case contract.TurnEventThoughtDelta:
				thoughts = append(thoughts, event.Thought)
			case contract.TurnEventTextDelta:
				output += event.Text
			}
			return nil
		}),
	}}}
	send := func(method, session string, update map[string]any) {
		t.Helper()
		params, err := json.Marshal(map[string]any{"sessionId": session, "update": update})
		if err != nil {
			t.Fatal(err)
		}
		rt.handleNotification(proc, notification{Method: method, Params: params})
	}
	chunk := func(kind, id, text string) {
		send("session/update", "s", map[string]any{
			"sessionUpdate": kind, "messageId": id, "content": map[string]string{"type": "text", "text": text},
		})
	}
	send("csgclaw/context", "background", map[string]any{"sessionUpdate": "reasoning_delta", "text": "Other session"})
	send("csgclaw/context", "s", map[string]any{"sessionUpdate": "reasoning_delta", "text": "思考"})
	send("csgclaw/context", "s", map[string]any{"sessionUpdate": "reasoning_delta", "text": "一步"})
	send("csgclaw/context", "s", map[string]any{
		"sessionUpdate": "reasoning_commit", "messageId": "m1", "prefixBytes": []int{len("思考"), len("一步"), 0},
	})
	chunk("agent_thought_chunk", "other", "原样保留")
	chunk("agent_thought_chunk", "m1", "思考")
	chunk("agent_thought_chunk", "m1", "一步之后")
	chunk("agent_thought_chunk", "m1", "补充")
	chunk("agent_message_chunk", "m1", "最终回答")
	if want := []string{"思考", "一步", "原样保留", "之后", "补充"}; !reflect.DeepEqual(thoughts, want) {
		t.Fatalf("thoughts = %q, want %q", thoughts, want)
	}
	if output != "最终回答" || proc.active["s"].output.String() != output {
		t.Fatalf("visible answer = %q, want 最终回答", output)
	}
	if len(proc.active["s"].thoughtPrefixes) != 0 {
		t.Fatal("consumed thought prefixes were retained")
	}
}

func TestThoughtStreamNativeDSHE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_DSH_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_DSH_BINARY to test native DSH reasoning")
	}
	received := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta map[string]string, finish any) {
			data, _ := json.Marshal(map[string]any{"id": "r1", "object": "chat.completion.chunk", "choices": []any{
				map[string]any{"index": 0, "delta": delta, "finish_reason": finish},
			}})
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.(http.Flusher).Flush()
		}
		waitFor := func(want string) {
			select {
			case got := <-received:
				if got != want {
					t.Errorf("live thought = %q, want %q", got, want)
				}
			case <-req.Context().Done():
				t.Error("model request closed before thought arrived")
			case <-time.After(4 * time.Second):
				t.Errorf("thought %q was not delivered before the next model event", want)
			}
		}
		send(map[string]string{"role": "assistant", "reasoning_content": "先看"}, nil)
		waitFor("先看")
		send(map[string]string{"reasoning_content": "目录。"}, nil)
		waitFor("目录。")
		send(map[string]string{"content": "Done."}, nil)
		send(map[string]string{}, "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	home := filepath.Join(t.TempDir(), "agent")
	profile := agentruntime.Profile{BaseURL: server.URL + "/v1", APIKey: "fixture-key", ModelID: "fixture-model", ReasoningEffort: "auto"}
	ref := AgentRef{ID: "alice", RuntimeID: "rt-alice", Profile: profile}
	rt := New(Dependencies{
		ResolveBinary: func(context.Context) (dshcli.Info, error) {
			return dshcli.Info{Path: binary, Version: dshcli.DefaultVersion}, nil
		},
		ResolveAgent: func(agentruntime.Handle) (AgentRef, error) { return ref, nil },
		AgentHome:    func(string) (string, error) { return home, nil },
	})
	defer rt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := rt.Provision(ctx, agentruntime.ProvisionRequest{
		RuntimeID: ref.RuntimeID, AgentID: ref.ID, Profile: profile,
	}); err != nil {
		t.Fatal(err)
	}
	handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: ref.RuntimeID, AgentID: ref.ID, Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	var thought strings.Builder
	result := rt.Conversation(handle.RuntimeID).Run(ctx, contract.TurnRequest{
		ID: "turn", ConversationKey: "room:test",
		Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "Reply Done."}},
	}, contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error {
		if event.Kind == contract.TurnEventThoughtDelta {
			thought.WriteString(event.Thought)
			received <- event.Thought
		}
		return nil
	}))
	if result.Status != contract.TurnSucceeded || result.Output != "Done." || thought.String() != "先看目录。" {
		t.Fatalf("turn = %+v, thoughts = %q", result, thought.String())
	}
}
