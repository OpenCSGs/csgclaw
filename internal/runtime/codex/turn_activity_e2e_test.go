package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Uses the bundled process and local Responses/MCP servers. Activity is checked
// while native work is in flight, followed by completion/cancellation and reuse.
func TestTurnActivityBundledCodexE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY to the bundled Codex binary")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"reasoning_text", "reasoning_summary", "mcp_progress", "compaction", "silent", "cancel_silent"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("CODEX_HOME", filepath.Join(home, "host"))
			release := make(chan struct{})
			entered := make(chan struct{}, 1)
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			waitForRelease := func(ctx context.Context) bool {
				select {
				case entered <- struct{}{}:
				default:
				}
				select {
				case <-release:
					return true
				case <-ctx.Done():
					return false
				}
			}
			mcpServer := mcp.NewServer(&mcp.Implementation{Name: "activity-fixture", Version: "1"}, nil)
			mcpServer.AddTool(&mcp.Tool{Name: "slow_operation", Description: "Report progress while working", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				token := req.Params.GetProgressToken()
				if token == nil {
					return nil, fmt.Errorf("native MCP call did not supply a progress token")
				}
				if err := req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{ProgressToken: token, Progress: 1, Total: 2, Message: "still working"}); err != nil {
					return nil, err
				}
				if !waitForRelease(ctx) {
					return nil, ctx.Err()
				}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "done"}}}, nil
			})
			mux := http.NewServeMux()
			mux.Handle("/api/v1/agents/alice/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer }, nil))
			var requests atomic.Int64
			mux.HandleFunc("/v1/responses", func(w http.ResponseWriter, r *http.Request) {
				n := requests.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				emit := func(v any) { raw, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", raw); w.(http.Flusher).Flush() }
				emit(map[string]any{"type": "response.created", "response": map[string]any{"id": fmt.Sprint(n)}})
				tokens := 1000
				if mode == "compaction" && n == 1 {
					tokens = 26000
				}
				if mode == "mcp_progress" && n == 1 {
					emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "call_id": "slow-call", "namespace": "mcp__csgclaw", "name": "slow_operation", "arguments": "{}"}})
				} else {
					if n == 1 && (mode == "reasoning_text" || mode == "reasoning_summary") {
						item := map[string]any{"id": "reasoning-1", "type": "reasoning", "summary": []any{}}
						emit(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item})
						deltaType := "response.reasoning_text.delta"
						if mode == "reasoning_summary" {
							deltaType = "response.reasoning_summary_text.delta"
						}
						emit(map[string]any{"type": deltaType, "item_id": "reasoning-1", "output_index": 0, "content_index": 0, "summary_index": 0, "delta": "thinking"})
						if !waitForRelease(r.Context()) {
							return
						}
						emit(map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item})
					}
					if (mode == "compaction" && n == 2) || (n == 1 && (mode == "silent" || mode == "cancel_silent")) {
						if !waitForRelease(r.Context()) {
							return
						}
					}
					emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": fmt.Sprint(n), "content": []any{map[string]any{"type": "output_text", "text": "done"}}}})
				}
				emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": fmt.Sprint(n), "usage": map[string]any{"input_tokens": tokens, "output_tokens": 20, "total_tokens": tokens + 20}}})
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			profile := testAgentMCPProfile()
			profile.Env["CSGCLAW_CALLER_AGENT_ID"] = "alice"
			profile.Provider, profile.ModelID, profile.BaseURL = "api", "fixture-model", server.URL+"/v1"
			profile.ModelMetadata = modelcap.Resolved{ContextWindow: 32768, ContextSource: "user"}
			delete(profile.Env, "CSGCLAW_BASE_URL")
			if mode == "mcp_progress" {
				profile.Env["CSGCLAW_BASE_URL"] = server.URL
			}
			rt := New(Dependencies{EventSink: NewEventSink(), BinaryProvider: fakeBinaryProvider{path: binary}, AgentHome: func(string) (string, error) { return filepath.Join(home, "agent"), nil }, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
				return AgentRef{ID: "alice", Name: "alice", Profile: profile, RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
			}})
			defer func() { unblock(); _ = rt.Close() }()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			h, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "activity-fixture", AgentID: "alice", AgentName: "alice", Profile: profile})
			if err != nil {
				t.Fatal(err)
			}
			thread, err := rt.EnsureEngineSession(ctx, h.RuntimeID, "room:activity")
			if err != nil {
				t.Fatal(err)
			}
			if mode == "compaction" {
				if err := rt.Prompt(ctx, h.RuntimeID, thread, "seed context usage"); err != nil {
					t.Fatal(err)
				}
			}
			live := rt.SessionManager().(*appServerManager).liveSession(h.RuntimeID)
			pid := live.session.ProcessID
			promptCtx, cancelPrompt := context.WithCancel(ctx)
			defer cancelPrompt()
			done := make(chan error, 1)
			go func() { done <- rt.Prompt(promptCtx, h.RuntimeID, thread, "continue working") }()
			select {
			case <-entered:
			case err := <-done:
				t.Fatalf("turn ended before fixture reached progress: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			wantPrefix := ""
			switch mode {
			case "reasoning_text":
				wantPrefix = "reasoning/textDelta:"
			case "reasoning_summary":
				wantPrefix = "reasoning/summaryTextDelta:"
			case "mcp_progress":
				// Bundled Codex 0.149.1 logs remote MCP progress but does not
				// forward it as an app-server notification. Check its real tool
				// lifecycle here; the progress wire contract is tested separately.
				wantPrefix = "started:mcpToolCall:"
			case "compaction":
				wantPrefix = "started:contextCompaction:"
			}
			if wantPrefix != "" {
				waitForRuntime(t, func() bool {
					waiter := live.appServerTurnWaiter(thread)
					return waiter != nil && strings.HasPrefix(waiter.currentLastActivity(), wantPrefix)
				})
			}
			select {
			case err := <-done:
				t.Fatalf("in-flight turn ended: %v", err)
			default:
			}
			if mode == "cancel_silent" {
				cancelPrompt()
			} else {
				unblock()
			}
			select {
			case err := <-done:
				if mode == "cancel_silent" {
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("cancellation=%v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			unblock()
			if live.appServerTurnWaiter(thread) != nil {
				t.Fatal("turn retained its waiter")
			}
			if err := rt.Prompt(ctx, h.RuntimeID, thread, "follow up after previous turn"); err != nil {
				t.Fatalf("follow-up: %v", err)
			}
			if live.session.ProcessID != pid || !processAlive(pid) {
				t.Fatal("worker was replaced or stopped")
			}
			t.Logf("mode=%s activity=%s requests=%d follow-up completed on same worker", mode, wantPrefix, requests.Load())
		})
	}
}
