package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	agentruntime "csgclaw/internal/runtime"
)

// Exercise the real app-server history and tool loop with a deterministic model
// fixture that replays the observed mistake when turn scope is unspecified.
func TestFailedAttachmentTurnScopeBundledCodexE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY to the bundled Codex binary")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "host"))

	const attachment = "sample.exe"
	const greeting = "你好"
	const continuation = "继续分析刚才的文件"
	var available atomic.Bool
	var toolCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []struct {
				Type    string
				Role    string
				Content []struct{ Text string }
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var current string
		var oldAttachment, turnPolicy, toolResult bool
		for _, item := range body.Input {
			for _, part := range item.Content {
				switch item.Role {
				case "developer":
					turnPolicy = turnPolicy || strings.Contains(part.Text, "Do not resume unfinished work from earlier turns unless")
				case "user":
					oldAttachment = oldAttachment || strings.Contains(part.Text, attachment)
					if strings.HasPrefix(part.Text, "Current message:\n") {
						current = strings.TrimPrefix(part.Text, "Current message:\n")
					}
				}
			}
			toolResult = toolResult || item.Type == "function_call_output"
		}
		if !oldAttachment {
			t.Error("model request lost the original attachment context")
		}
		if !available.Load() {
			http.Error(w, "model temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(event string, data map[string]any) {
			data["type"] = event
			raw, _ := json.Marshal(data)
			fmt.Fprintf(w, "data: %s\n\n", raw)
		}
		response := map[string]any{"id": "response", "status": "completed"}
		emit("response.created", map[string]any{"response": response})
		if !toolResult && (current == continuation || current == greeting && !turnPolicy) {
			toolCalls.Add(1)
			emit("response.output_item.done", map[string]any{"item": map[string]any{"type": "function_call", "call_id": "inspect", "name": "exec_command", "arguments": `{"cmd":"echo inspect-sample-exe"}`}})
		} else {
			answer := "你好！"
			if toolResult {
				answer = "已检查旧附件。"
			}
			emit("response.output_item.done", map[string]any{"item": map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": answer}}}})
		}
		emit("response.completed", map[string]any{"response": response})
	}))
	defer upstream.Close()

	profile := agentruntime.Profile{Provider: "api", ModelID: "fixture-model", BaseURL: upstream.URL + "/v1"}
	rt := New(Dependencies{BinaryProvider: fakeBinaryProvider{path: binary}, EventSink: NewEventSink(), AgentHome: func(string) (string, error) {
		return filepath.Join(home, "agent"), nil
	}, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "alice", Name: "alice", Profile: profile, RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
	}})
	defer rt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", AgentName: "alice", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	run := func(id, message string, extra ...contract.InputPart) contract.TurnResult {
		return rt.Conversation(handle.RuntimeID).Run(ctx, contract.TurnRequest{
			ID: contract.TurnID(id), ConversationKey: "room:attachment",
			Input: append([]contract.InputPart{{Kind: contract.InputPartText, Text: "Current message:\n" + message}}, extra...),
		}, nil)
	}

	const content = "fixture attachment"
	file, err := contract.NewOutputFile(ctx, contract.OutputFileMetadata{
		Name: attachment, MediaType: "application/octet-stream", SizeBytes: int64(len(content)),
	}, strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Cleanup()
	failed := run("attachment", "介绍这个文件", contract.InputPart{Kind: contract.InputPartFile, File: &contract.InputFile{
		ID: file.ID, Resolved: file,
	}})
	if failed.Status != contract.TurnFailed || failed.Error == nil || !strings.Contains(failed.Error.Message, "503") {
		t.Fatalf("attachment turn did not fail with HTTP 503: %+v", failed)
	}
	// Reopen the persisted thread as in the Windows report.
	if _, err := rt.Stop(ctx, handle); err != nil {
		t.Fatal(err)
	}
	available.Store(true)
	reply := run("greeting", greeting)
	if reply.Status != contract.TurnSucceeded || reply.Output != "你好！" || toolCalls.Load() != 0 {
		t.Fatalf("greeting after HTTP 503: status=%s output=%q tools=%d error=%+v", reply.Status, reply.Output, toolCalls.Load(), reply.Error)
	}
	reply = run("continue", continuation)
	if reply.Status != contract.TurnSucceeded || reply.Output != "已检查旧附件。" || toolCalls.Load() != 1 {
		t.Fatalf("explicit continuation: status=%s output=%q tools=%d error=%+v", reply.Status, reply.Output, toolCalls.Load(), reply.Error)
	}
}
