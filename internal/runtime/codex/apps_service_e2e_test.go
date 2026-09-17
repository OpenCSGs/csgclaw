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

	"csgclaw/internal/apps"
	agentruntime "csgclaw/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This exercises the real Apps service and bundled Codex together. All upstream
// App and Responses traffic stays on local fixtures with distinct credentials.
func TestAppsServiceBundledCodexLifecycleE2E(t *testing.T) {
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
	const agentID = "agent-alice"
	const agentToken = "fixture-agent-platform-token"
	const appToken = "fixture-gitlab-private-token"
	var activeRuntime atomic.Pointer[Runtime]
	var upstreamCalls, upstreamRequests, gatewayRequests, reloads atomic.Int32
	var modelRequests atomic.Int32
	var resultReturned atomic.Bool
	upstreamMCP := mcp.NewServer(&mcp.Implementation{Name: "gitlab-fixture", Version: "1"}, nil)
	upstreamMCP.AddTool(&mcp.Tool{Name: "read_issue", Description: "Read a GitLab issue", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		upstreamCalls.Add(1)
		meta, _ := req.Params.Meta["x-codex-turn-metadata"].(map[string]any)
		thread, _ := meta["thread_id"].(string)
		turn, _ := meta["turn_id"].(string)
		rt := activeRuntime.Load()
		if rt == nil {
			return nil, fmt.Errorf("missing runtime")
		}
		if _, conversation, err := rt.AgentTurnContext(agentID, thread, turn); err != nil || conversation != "room:app-integration:alice" {
			t.Errorf("App call lost native turn binding: %q %v", conversation, err)
		}
		var args map[string]any
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil || args["query"] != "fixture" {
			t.Error("App call arguments did not reach upstream")
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "issue-42"}}, StructuredContent: map[string]any{"issue": "issue-42", "status": "open"}}, nil
	})
	upstreamHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstreamMCP }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequests.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+appToken {
			t.Error("upstream received a credential other than the App token")
			http.Error(w, "unauthorized", 401)
			return
		}
		upstreamHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	service, err := apps.NewService(filepath.Join(home, "app-state.json"), apps.Options{OnCatalogChanged: func(id string, revision uint64) {
		if rt := activeRuntime.Load(); rt != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := rt.RefreshAgentMCP(ctx, id, revision); err != nil {
				t.Errorf("App catalog refresh: %v", err)
			}
			reloads.Add(1)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	installation, err := service.Create(context.Background(), agentID, apps.CreateRequest{AppID: "gitlab", Name: "Work GitLab", Config: apps.Config{Transport: "http", URL: upstream.URL, AuthMode: "bearer"}, Credentials: apps.Credentials{Token: appToken}, Connect: true})
	if err != nil {
		t.Fatal(err)
	}
	if installation.Status != "connected" || len(installation.Tools) != 1 {
		t.Fatalf("App setup status=%s tools=%d", installation.Status, len(installation.Tools))
	}
	gateway := service.Handler(agentID)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/agents/"+agentID+"/mcp", func(w http.ResponseWriter, r *http.Request) {
		gatewayRequests.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+agentToken {
			t.Error("Codex gateway request used the wrong credential")
			http.Error(w, "unauthorized", 401)
			return
		}
		gateway.ServeHTTP(w, r)
	})
	var firstTool atomic.Value
	mux.HandleFunc("/v1/responses", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+agentToken {
			t.Error("Responses fixture received the wrong Agent credential")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", 400)
			t.Error(err)
			return
		}
		raw, _ := json.Marshal(body)
		if strings.Contains(string(raw), appToken) {
			t.Error("App credential leaked into Codex model input")
		}
		n := modelRequests.Add(1)
		tool := ""
		if tools, ok := body["tools"].([]any); ok {
			for _, item := range tools {
				entry, _ := item.(map[string]any)
				if entry["name"] != "mcp__csgclaw" {
					continue
				}
				nested, _ := entry["tools"].([]any)
				for _, raw := range nested {
					candidate, _ := raw.(map[string]any)
					name, _ := candidate["name"].(string)
					if strings.HasPrefix(name, "app_") {
						tool = name
					}
				}
			}
		}
		if n == 3 && tool != "" {
			t.Error("disconnected App remained in Codex tools")
		}
		if (n == 1 || n == 4) && tool == "" {
			t.Error("connected App absent from Codex tools")
		}
		if n == 1 {
			firstTool.Store(tool)
		}
		if n == 4 && firstTool.Load() != tool {
			t.Error("reconnecting changed the installation's tool identity")
		}
		if n == 2 || n == 5 {
			input, _ := json.Marshal(body["input"])
			if strings.Contains(string(input), "issue-42") {
				resultReturned.Store(true)
			} else {
				t.Error("upstream App result did not return to Codex")
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(v map[string]any) { data, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", data) }
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": fmt.Sprintf("response-%d", n)}})
		if (n == 1 || n == 4) && tool != "" {
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "namespace": "mcp__csgclaw", "name": tool, "call_id": fmt.Sprintf("call-%d", n), "arguments": `{"query":"fixture"}`}})
		} else {
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "id": fmt.Sprintf("message-%d", n), "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "done"}}}})
		}
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": fmt.Sprintf("response-%d", n), "usage": map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}}})
	})
	platform := httptest.NewServer(mux)
	defer platform.Close()
	profile := agentruntime.Profile{Provider: "api", ModelID: "fixture-model", BaseURL: platform.URL + "/v1", APIKey: agentToken, Env: map[string]string{"CSGCLAW_CALLER_AGENT_ID": agentID, "CSGCLAW_BASE_URL": platform.URL, agentAccessTokenEnv: agentToken}}
	rt := New(Dependencies{BinaryProvider: fakeBinaryProvider{path: binary}, AgentHome: func(string) (string, error) { return filepath.Join(home, agentID), nil }, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: agentID, Name: "alice", Profile: profile, MCPServers: map[string]any{}, RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
	}})
	activeRuntime.Store(rt)
	defer rt.Close()
	rt.SetAgentMCPRevisionSource(service.Revision)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	handle, err := rt.New(ctx, agentruntime.Spec{AgentID: agentID, AgentName: "alice", RuntimeID: "rt-" + agentID, Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := rt.EnsureEngineSession(ctx, handle.RuntimeID, "room:app-integration:alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Prompt(ctx, handle.RuntimeID, thread, "Read the configured GitLab issue."); err != nil {
		t.Fatal(err)
	}
	if upstreamCalls.Load() != 1 {
		t.Fatal("first App tool did not execute")
	}
	detached, err := service.Disconnect(ctx, agentID, installation.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if !detached.Disconnected || detached.CredentialsSet["token"] {
		t.Fatal("disconnect retained authorization")
	}
	if err := rt.Prompt(ctx, handle.RuntimeID, thread, "List the currently available tools."); err != nil {
		t.Fatal(err)
	}
	if upstreamCalls.Load() != 1 {
		t.Fatal("disconnected App was invoked")
	}
	credentials := apps.Credentials{Token: appToken}
	if _, err := service.Update(ctx, agentID, installation.InstallationID, apps.UpdateRequest{Credentials: &credentials}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Connect(ctx, agentID, installation.InstallationID); err != nil {
		t.Fatal(err)
	}
	if err := rt.Prompt(ctx, handle.RuntimeID, thread, "Read the GitLab issue again."); err != nil {
		t.Fatal(err)
	}
	if upstreamCalls.Load() != 2 || modelRequests.Load() != 5 || reloads.Load() < 2 || !resultReturned.Load() {
		t.Fatalf("incomplete App/Codex lifecycle calls=%d model=%d reloads=%d", upstreamCalls.Load(), modelRequests.Load(), reloads.Load())
	}
	if gatewayRequests.Load() == 0 || upstreamRequests.Load() == 0 {
		t.Fatal("missing authenticated transport traffic")
	}
	if bound, ok, err := rt.ExistingEngineSession(ctx, handle.RuntimeID, "room:app-integration:alice"); err != nil || !ok || bound != thread {
		t.Fatal("App changes replaced the Codex thread")
	}
	codexHome, err := rt.resolveCodexHomeDir(agentID)
	if err != nil {
		t.Fatal(err)
	}
	configRaw, err := os.ReadFile(filepath.Join(codexHome, configFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configRaw), appToken) || strings.Contains(string(configRaw), upstream.URL) {
		t.Fatal("Codex config contains private App connection information")
	}
	t.Logf("GitLab App -> scoped Agent MCP -> Codex -> upstream tool passed; disconnect/reconnect preserved thread %s", thread)
}
