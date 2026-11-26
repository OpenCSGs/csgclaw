package dsh

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
)

func TestConversationRequiresVisibleOutput(t *testing.T) {
	for _, tc := range []struct {
		name, stop, text, update string
		present                  bool
		wantCode                 contract.ErrorCode
	}{
		{name: "thought only", stop: "end_turn", wantCode: "empty_agent_response"},
		{name: "thought reaches output limit", stop: "max_tokens", wantCode: "output_token_limit"},
		{name: "native output limit masked by end turn", stop: "end_turn", update: "native_max_tokens", wantCode: "output_token_limit"},
		{name: "native failure masked by end turn", stop: "end_turn", update: "native_error", wantCode: contract.ErrorRuntimeFailed},
		{name: "whitespace is not an answer", stop: "end_turn", text: " \n", wantCode: "empty_agent_response"},
		{name: "context error masked by end turn", stop: "end_turn", update: "context_error", wantCode: "context_length_exceeded"},
		{name: "answer", stop: "end_turn", text: "Here is the answer."},
		{name: "partial answer", stop: "max_tokens", text: "Partial answer"},
		{name: "file without text", stop: "end_turn", present: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host, peer := net.Pipe()
			t.Cleanup(func() {
				_ = host.Close()
				_ = peer.Close()
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			deadline, _ := ctx.Deadline()
			_ = host.SetDeadline(deadline)
			_ = peer.SetDeadline(deadline)
			client := newACPClient(host, host)
			workspace := t.TempDir()
			if tc.present {
				if err := os.WriteFile(filepath.Join(workspace, "index.html"), []byte("<p>Answer</p>"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			proc := &process{
				client: client, workspace: workspace,
				meta: runtimeMetadata{Sessions: map[string]string{"room-1": "session-1"}},
				done: make(chan struct{}), active: make(map[string]*activeTurn), ready: map[string]bool{"session-1": true},
			}
			rt := &Runtime{processes: map[string]*process{"rt-1": proc}}
			client.setHandlers(nil, func(note notification) { rt.handleNotification(proc, note) })

			peerDone := make(chan struct{})
			go func() {
				defer close(peerDone)
				var frame testACPFrame
				if err := json.NewDecoder(peer).Decode(&frame); err != nil {
					t.Error(err)
					return
				}
				if frame.Method != "session/prompt" {
					t.Errorf("ACP request = %q, want session/prompt", frame.Method)
					return
				}
				encoder := json.NewEncoder(peer)
				send := func(frame map[string]any) {
					frame["jsonrpc"] = "2.0"
					if err := encoder.Encode(frame); err != nil {
						t.Error(err)
					}
				}
				update := func(method string, data map[string]any) {
					send(map[string]any{"method": method, "params": map[string]any{"sessionId": "session-1", "update": data}})
				}
				chunk := func(kind, text string) {
					update("session/update", map[string]any{"sessionUpdate": kind, "content": map[string]string{"type": "text", "text": text}})
				}
				chunk("agent_thought_chunk", "Thinking about the task.")
				if tc.text != "" {
					chunk("agent_message_chunk", tc.text)
				}
				if tc.update != "" {
					switch tc.update {
					case "native_max_tokens":
						update("csgclaw/context", map[string]any{"sessionUpdate": "turn_end", "reason": map[string]any{"kind": "max-tokens"}})
					case "native_error":
						update("csgclaw/context", map[string]any{"sessionUpdate": "turn_end", "reason": map[string]any{"kind": "error", "error": map[string]any{"code": "INVALID_REQUEST", "message": "fixture output cap rejected"}}})
					default:
						update("csgclaw/context", map[string]any{"sessionUpdate": tc.update})
					}
				}
				if tc.present {
					update("session/update", map[string]any{
						"sessionUpdate": "tool_call_update", "toolCallId": "file-1", "kind": "present", "status": "completed",
						"rawInput": map[string]any{"files": []map[string]string{{"path": "index.html"}}},
					})
				}
				send(map[string]any{"id": frame.ID, "result": map[string]string{"stopReason": tc.stop}})
			}()

			var thoughts, text strings.Builder
			result := rt.Conversation("rt-1").Run(ctx, contract.TurnRequest{
				ID: "turn-1", ConversationKey: "room-1",
				Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "Make a knowledge card."}},
			}, contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error {
				switch event.Kind {
				case contract.TurnEventThoughtDelta:
					thoughts.WriteString(event.Thought)
				case contract.TurnEventTextDelta:
					text.WriteString(event.Text)
				}
				return nil
			}))
			<-peerDone
			t.Cleanup(func() { cleanupPresentedFiles(result.RuntimeFiles) })
			wantStatus := contract.TurnSucceeded
			if tc.wantCode != "" {
				wantStatus = contract.TurnFailed
			}
			if result.Status != wantStatus || contract.ErrorCodeOf(result.Error) != tc.wantCode || !result.Dispatched || result.Output != tc.text {
				t.Fatalf("turn = %+v, want status %s, code %q and output %q", result, wantStatus, tc.wantCode, tc.text)
			}
			if thoughts.String() != "Thinking about the task." || text.String() != tc.text {
				t.Fatalf("streamed thought = %q, text = %q", thoughts.String(), text.String())
			}
			if tc.present && (len(result.RuntimeFiles) != 1 || result.RuntimeFiles[0].Name != "index.html") {
				t.Fatalf("missing file output: %+v", result.RuntimeFiles)
			}
		})
	}
}
