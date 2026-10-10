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
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/dshcli"
	agentruntime "csgclaw/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestResourcesNativeDSHE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_DSH_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_DSH_BINARY")
	}
	var connectorCalls, knowledgeCalls, modelCalls, materializations atomic.Int32
	var sawRefreshedCatalog atomic.Bool
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "resource-fixture", Version: "1"}, nil)
	for _, toolName := range []string{"connector_lookup", "kb_search"} {
		mcpServer.AddTool(&mcp.Tool{Name: toolName, Description: "Read fixture data", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if req.Params.Name == "kb_search" {
				knowledgeCalls.Add(1)
			} else {
				connectorCalls.Add(1)
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "resource-proof-42"}}}, nil
		})
	}
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	resources := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "Bearer agent-scoped-token"
		if r.URL.Path == "/wiki" {
			want = "Bearer hydrated-wiki-token"
		}
		if r.Header.Get("Authorization") != want {
			t.Errorf("resource %s received incorrect authentication", r.URL.Path)
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.URL.Query().Get("catalog_revision") == "2" {
			sawRefreshedCatalog.Store(true)
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	defer resources.Close()
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		first := modelCalls.Add(1) == 1
		if strings.Contains(string(body), "mcp__disabled__") {
			t.Error("disabled MCP tools reached the model")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(delta any, reason any) {
			raw, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})
			fmt.Fprintf(w, "data: %s\n\n", raw)
		}
		if first {
			var calls []any
			for i, name := range []string{"mcp__csgclaw__connector_lookup", "mcp__wiki__kb_search"} {
				if !strings.Contains(string(body), name) {
					t.Errorf("resource tool %s is absent from model catalog", name)
				}
				calls = append(calls, map[string]any{"index": i, "id": fmt.Sprintf("resource-%d", i), "type": "function", "function": map[string]any{"name": name, "arguments": "{}"}})
			}
			emit(map[string]any{"role": "assistant", "tool_calls": calls}, nil)
			emit(map[string]any{}, "tool_calls")
		} else {
			if !strings.Contains(string(body), "resource-proof-42") {
				t.Error("MCP tool result did not return to model history")
			}
			emit(map[string]any{"role": "assistant", "content": "done"}, nil)
			emit(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer model.Close()
	home := filepath.Join(t.TempDir(), "agent")
	profile := agentruntime.Profile{BaseURL: model.URL + "/v1", APIKey: "fixture-key", ModelID: "fixture-model", Env: map[string]string{"CSGCLAW_CALLER_AGENT_ID": "alice", "CSGCLAW_BASE_URL": resources.URL, "CSGCLAW_ACCESS_TOKEN": "agent-scoped-token"}}
	manual := map[string]any{"wiki": map[string]any{"url": resources.URL + "/wiki"}, "disabled": map[string]any{"url": resources.URL + "/wiki", "enabled": false}}
	rt := New(Dependencies{ResolveBinary: func(context.Context) (dshcli.Info, error) { return dshcli.Info{Path: binary}, nil }, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "alice", RuntimeID: "rt-alice", Profile: profile, MCPServers: manual}, nil
	}, AgentHome: func(string) (string, error) { return home, nil }, MaterializeMCPServers: func(_ context.Context, raw map[string]any) (map[string]any, error) {
		materializations.Add(1)
		return map[string]any{"wiki": map[string]any{"url": resources.URL + "/wiki", "headers": map[string]any{"Authorization": "Bearer hydrated-wiki-token"}}, "disabled": raw["disabled"]}, nil
	}})
	defer rt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := rt.Provision(ctx, agentruntime.ProvisionRequest{AgentID: "alice", RuntimeID: "rt-alice", Profile: profile}); err != nil {
		t.Fatal(err)
	}
	handle, err := rt.New(ctx, agentruntime.Spec{AgentID: "alice", RuntimeID: "rt-alice", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	run := func(id string) {
		t.Helper()
		result := rt.Conversation(handle.RuntimeID).Run(ctx, contract.TurnRequest{ID: contract.TurnID(id), ConversationKey: "resource-room", Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "Read the connector and knowledge base."}}}, nil)
		if result.Status != contract.TurnSucceeded {
			log, _ := os.ReadFile(filepath.Join(home, hostStateDirName, stderrFileName))
			t.Fatalf("turn = %+v\n%s", result, log)
		}
	}
	run("first")
	if connectorCalls.Load() != 1 || knowledgeCalls.Load() != 1 {
		t.Fatalf("resource calls: connector=%d knowledge=%d", connectorCalls.Load(), knowledgeCalls.Load())
	}
	proc, _ := rt.process(handle.RuntimeID)
	session := proc.meta.Sessions["resource-room"]
	if err := rt.RefreshAgentMCP(ctx, "alice", 2); err != nil {
		t.Fatal(err)
	}
	run("after-catalog-refresh")
	proc, _ = rt.process(handle.RuntimeID)
	if proc.meta.Sessions["resource-room"] != session || !sawRefreshedCatalog.Load() || materializations.Load() != 2 {
		t.Fatal("catalog refresh lost the session or did not rematerialize resources")
	}
	if manual["wiki"].(map[string]any)["headers"] != nil {
		t.Fatal("runtime credentials leaked into stored MCP configuration")
	}
}
