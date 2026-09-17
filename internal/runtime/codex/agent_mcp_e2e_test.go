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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentruntime "csgclaw/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// An opt-in integration test exercises the actual bundled app-server against
// local MCP and Responses fixtures, without external model calls or secrets.
func TestAgentMCPBundledCodexE2E(t *testing.T) {
	t.Run("full_tools", func(t *testing.T) { testAgentMCPBundledCodex(t, false) })
	t.Run("native_tool_search", func(t *testing.T) { testAgentMCPBundledCodex(t, true) })
}

func testAgentMCPBundledCodex(t *testing.T, search bool) {
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
	t.Setenv("CODEX_HOME", filepath.Join(home, "host-codex"))
	var revision atomic.Uint64
	var calls atomic.Uint64
	var sawSearch, sawSearchOutput atomic.Bool
	var bodyMu sync.Mutex
	var lastBody map[string]any
	var rt *Runtime
	makeServer := func(name string) *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: name, Version: "1"}, nil)
		server.AddTool(&mcp.Tool{Name: name, Description: "Read the current catalog revision", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			calls.Add(1)
			metadata, _ := req.Params.Meta["x-codex-turn-metadata"].(map[string]any)
			threadID, _ := metadata["thread_id"].(string)
			turnID, _ := metadata["turn_id"].(string)
			if _, conversation, err := rt.AgentTurnContext("agent-alice", threadID, turnID); err != nil || conversation != "room:test:participant:alice" {
				t.Errorf("MCP turn binding: %q %v", conversation, err)
			}
			t.Logf("MCP tool=%s thread=%s turn=%s", name, threadID, turnID)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: name}}}, nil
		})
		return server
	}
	servers := []*mcp.Server{makeServer("revision_zero"), makeServer("revision_one")}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return servers[min(revision.Load(), 1)] }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/agents/agent-alice/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer agent-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	})
	var requests atomic.Uint64
	mux.HandleFunc("/v1/responses", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, err.Error(), 400)
			return
		}
		bodyMu.Lock()
		lastBody = body
		bodyMu.Unlock()
		n := requests.Add(1)
		tools, _ := json.Marshal(body["tools"])
		input, _ := json.Marshal(body["input"])
		if strings.Contains(string(tools), `"type":"tool_search"`) {
			sawSearch.Store(true)
		}
		if strings.Contains(string(input), `"type":"tool_search_output"`) {
			sawSearchOutput.Store(true)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(event map[string]any) { raw, _ := json.Marshal(event); fmt.Fprintf(w, "data: %s\n\n", raw) }
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": fmt.Sprintf("response-%d", n)}})
		step := n % 2
		if search {
			step = n % 3
		}
		if search && step == 1 {
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "tool_search_call", "call_id": fmt.Sprintf("search-%d", n), "execution": "client", "arguments": map[string]any{"query": "revision", "limit": 5}}})
		} else if (!search && step == 1) || (search && step == 2) {
			name := "revision_zero"
			if revision.Load() > 0 {
				name = "revision_one"
			}
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "call_id": fmt.Sprintf("call-%d", n), "namespace": "mcp__csgclaw", "name": name, "arguments": "{}"}})
		} else {
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": fmt.Sprintf("message-%d", n), "content": []map[string]any{{"type": "output_text", "text": "done"}}}})
		}
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": fmt.Sprintf("response-%d", n), "usage": map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	profile := testAgentMCPProfile()
	profile.Provider = "api"
	profile.ModelID = "fixture-model"
	if search {
		profile.Provider = "codex"
		profile.ModelID = "gpt-5.5"
	}
	profile.BaseURL = server.URL + "/v1"
	profile.Env["CSGCLAW_BASE_URL"] = server.URL
	newRuntime := func() *Runtime {
		return New(Dependencies{BinaryProvider: fakeBinaryProvider{path: binary}, AgentHome: func(string) (string, error) { return filepath.Join(home, "agent-alice"), nil }, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "agent-alice", Name: "alice", Profile: profile, MCPServers: map[string]any{}, RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
		}})
	}
	rt = newRuntime()
	rt.SetAgentMCPRevisionSource(func(string) uint64 { return revision.Load() })
	defer func() { _ = rt.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-agent-alice", AgentID: "agent-alice", AgentName: "alice", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := rt.EnsureEngineSession(ctx, handle.RuntimeID, "room:test:participant:alice")
	if err != nil {
		t.Fatal(err)
	}
	if err = rt.Prompt(ctx, handle.RuntimeID, thread, "Call the revision tool."); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		bodyMu.Lock()
		raw, _ := json.Marshal(lastBody["tools"])
		input := lastBody["model"]
		bodyMu.Unlock()
		t.Fatalf("first tool not called; tools=%s model=%s", raw, input)
	}
	revision.Store(1)
	if err = rt.RefreshAgentMCP(ctx, "agent-alice", 1); err != nil {
		t.Fatal(err)
	}
	if err = rt.Prompt(ctx, handle.RuntimeID, thread, "Call the new revision tool."); err != nil {
		t.Fatal(err)
	}
	bodyMu.Lock()
	raw, _ := json.Marshal(lastBody["tools"])
	if search {
		for _, item := range lastBody["input"].([]any) {
			value, _ := item.(map[string]any)
			if value["type"] == "tool_search_output" {
				raw, _ = json.Marshal(value)
			}
		}
	}
	bodyMu.Unlock()
	if calls.Load() != 2 || !strings.Contains(string(raw), "revision_one") || strings.Contains(string(raw), "revision_zero") {
		t.Fatalf("refresh calls=%d tools=%s", calls.Load(), raw)
	}
	if got, ok, err := rt.ExistingEngineSession(ctx, handle.RuntimeID, "room:test:participant:alice"); err != nil || !ok || got != thread {
		t.Fatalf("thread changed: %s %v %v", got, ok, err)
	}
	if sawSearch.Load() != search || sawSearchOutput.Load() != search {
		t.Fatalf("tool search support=%v output=%v expected=%v", sawSearch.Load(), sawSearchOutput.Load(), search)
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	revision.Store(2)
	rt = newRuntime()
	rt.SetAgentMCPRevisionSource(func(string) uint64 { return revision.Load() })
	resumed, err := rt.EnsureEngineSession(ctx, handle.RuntimeID, "room:test:participant:alice")
	if err != nil || resumed != thread {
		t.Fatalf("cold resume thread=%q err=%v", resumed, err)
	}
	if err = rt.Prompt(ctx, handle.RuntimeID, resumed, "Call the current tool after recovery."); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("cold recovery did not call tool, calls=%d", calls.Load())
	}
	codexHome, err := rt.resolveCodexHomeDir("agent-alice")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := os.ReadFile(filepath.Join(codexHome, configFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "catalog_revision=2") {
		t.Fatal("cold recovery did not use latest revision")
	}
	t.Logf("same-thread MCP refresh, cold recovery and three real tool calls succeeded: %s", thread)
}
