package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/mcpschema"
	agentruntime "csgclaw/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Real Codex process and MCP tool calls, with a local model fixture. Verify
// Unicode presentation survives rename and cold recovery without tool-ID churn.
func TestManualMCPIdentityBundledCodexE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	binary, _ = filepath.Abs(binary)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "host"))
	var calls atomic.Int32
	upstream := mcp.NewServer(&mcp.Implementation{Name: "中文 MCP 服务", Version: "1"}, nil)
	upstream.AddTool(&mcp.Tool{Name: "echo", Description: "Echo", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls.Add(1)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "identity-ok"}}}, nil
	})
	mcpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstream }, &mcp.StreamableHTTPOptions{JSONResponse: true}))
	defer mcpServer.Close()
	persisted, err := mcpschema.WithServerIdentities(map[string]any{"必应搜索中文": map[string]any{"url": mcpServer.URL}, "Web Fetch": map[string]any{"url": mcpServer.URL}})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(persisted))
	for id := range persisted {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var requests atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		n := requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(v any) { raw, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", raw) }
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": fmt.Sprintf("r%d", n)}})
		if n%2 == 1 {
			id := ids[((n-1)/2)%2]
			tools, _ := json.Marshal(body["tools"])
			if !strings.Contains(string(tools), id) {
				t.Errorf("runtime tool identity missing: %s", id)
			}
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "call_id": fmt.Sprintf("c%d", n), "namespace": "mcp__" + id, "name": "echo", "arguments": "{}"}})
		} else {
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": fmt.Sprintf("m%d", n), "content": []any{map[string]any{"type": "output_text", "text": "done"}}}})
		}
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": fmt.Sprintf("r%d", n), "usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}})
	}))
	defer model.Close()
	profile := testAgentMCPProfile()
	profile.Provider = "api"
	profile.ModelID = "fixture-model"
	profile.BaseURL = model.URL + "/v1"
	delete(profile.Env, "CSGCLAW_BASE_URL")
	materialize := func() map[string]any {
		servers, err := mcpschema.NormalizeMCPServers(persisted)
		if err != nil {
			t.Fatal(err)
		}
		mcpschema.StripPresentation(servers)
		return servers
	}
	newRuntime := func() *Runtime {
		return New(Dependencies{BinaryProvider: fakeBinaryProvider{path: binary}, AgentHome: func(string) (string, error) { return filepath.Join(home, "agent"), nil }, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "agent-alice", Name: "alice", Profile: profile, MCPServers: materialize(), RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
		}})
	}
	rt := newRuntime()
	defer func() { _ = rt.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-agent-alice", AgentID: "agent-alice", AgentName: "alice", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := rt.EnsureEngineSession(ctx, handle.RuntimeID, "room:identity")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = rt.Prompt(ctx, handle.RuntimeID, thread, "Call echo."); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("initial calls=%d", calls.Load())
	}
	before := materialize()
	for _, raw := range persisted {
		raw.(map[string]any)[mcpschema.DisplayNameKey] = "改名之后 中文"
	}
	change := agentruntime.MCPServersChange{Previous: agentruntime.MCPServersSnapshot{Servers: before}, Current: agentruntime.MCPServersSnapshot{Servers: materialize()}}
	if restart, err := rt.MCPServersRestartRequired(change); err != nil || restart {
		t.Fatalf("rename restart=%v err=%v", restart, err)
	}
	if err = rt.ReconcileMCPServers(ctx, handle, change); err != nil {
		t.Fatal(err)
	}
	if err = rt.Close(); err != nil {
		t.Fatal(err)
	}
	rt = newRuntime()
	resumed, err := rt.EnsureEngineSession(ctx, handle.RuntimeID, "room:identity")
	if err != nil || resumed != thread {
		t.Fatalf("cold resume=%q err=%v", resumed, err)
	}
	for i := 0; i < 2; i++ {
		if err = rt.Prompt(ctx, handle.RuntimeID, resumed, "Call echo after rename."); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 4 {
		t.Fatalf("calls after rename and recovery=%d", calls.Load())
	}
	readback, err := rt.ListMCPServers(ctx, handle, agentruntime.MCPServersSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(readback.Servers) != 2 {
		t.Fatalf("readback entries=%d", len(readback.Servers))
	}
	for _, id := range ids {
		if readback.Servers[id] == nil {
			t.Fatalf("lost fixed ID %s", id)
		}
	}
	t.Log("two MCP identities survived rename, cold recovery, readback and four real tool calls")
}
