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
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The real ACP process uses an accelerated idle window and a deterministic model;
// normal conversation replies never call a memory tool.
func TestBackgroundMemoryNativeDSHE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_DSH_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_DSH_BINARY")
	}
	previousTiming := defaultMemoryMaintenanceTiming
	defaultMemoryMaintenanceTiming = memoryMaintenanceTiming{
		CheckpointInterval: 2 * time.Second,
		CheckpointIdle:     time.Second,
		RetryDelay:         100 * time.Millisecond,
		RequestTimeout:     10 * time.Second,
	}
	t.Cleanup(func() { defaultMemoryMaintenanceTiming = previousTiming })
	const summary = "The user prefers Chinese replies for work documents."
	const combinedSummary = summary + " Lead with the conclusion."
	const verification = "verified-build-proof: 62 tests passed"
	const evidencePassword = "fixture-password-native-680"
	const evidenceToken = "fixture-token-native-680"
	const handbook = "# Work documents\nScope: this Agent's work documents.\nThe user explicitly requested Chinese.\n\n# Verification\n" + verification
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "verification-fixture", Version: "1"}, nil)
	mcpServer.AddTool(&mcp.Tool{Name: "verify_build", Description: "Read a deterministic build verification result", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: verification + ` {"access_token":"` + evidenceToken + `"}`}}}, nil
	})
	resources := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer }, &mcp.StreamableHTTPOptions{JSONResponse: true}))
	defer resources.Close()
	var mu sync.Mutex
	var extractionCalls, consolidationCalls int
	var mainBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		body := string(data)
		content := "正常对话已完成。"
		var toolCall map[string]any
		mu.Lock()
		switch {
		case strings.Contains(body, "CSGClaw DSH memory extraction"):
			extractionCalls++
			if !strings.Contains(body, "工作文档") && !strings.Contains(body, "先给结论") {
				t.Error("background extraction did not receive the ordinary conversation")
			}
			if extractionCalls == 1 && (!strings.Contains(body, verification) || !strings.Contains(body, "tool result mcp__verify__verify_build")) {
				t.Error("background extraction did not receive structured tool verification")
			}
			if strings.Contains(body, evidencePassword) || strings.Contains(body, evidenceToken) {
				t.Error("JSON credentials reached the background extraction provider")
			}
			memory := summary
			if strings.Contains(body, "先给结论") {
				memory = "Lead with the conclusion."
			}
			content = `{"memory":"` + memory + `","rollout_summary":"The user discussed reply preferences for work."}`
		case strings.Contains(body, "CSGClaw DSH memory consolidation"):
			consolidationCalls++
			documents := map[string]string{"summary": summary, "handbook": handbook}
			if strings.Contains(body, "Lead with the conclusion.") {
				documents["summary"] = combinedSummary
				documents["handbook"] += "\nLead with the conclusion."
			}
			raw, _ := json.Marshal(documents)
			content = string(raw)
		default:
			mainBodies = append(mainBodies, body)
			if len(mainBodies) == 1 {
				toolCall = map[string]any{"name": "mcp__verify__verify_build", "arguments": `{"password":"` + evidencePassword + `"}`}
			} else if strings.Contains(body, "请读取详细手册") && !strings.Contains(body, `"role":"tool"`) {
				toolCall = map[string]any{"name": "memory_read", "arguments": `{"document":"handbook"}`}
			}
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(delta any, reason any) {
			raw, _ := json.Marshal(map[string]any{"id": "memory-fixture", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})
			fmt.Fprintf(w, "data: %s\n\n", raw)
		}
		if toolCall != nil {
			emit(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "verification-read", "type": "function", "function": toolCall}}}, nil)
			emit(map[string]any{}, "tool_calls")
		} else {
			emit(map[string]any{"role": "assistant", "content": content}, nil)
			emit(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	home := filepath.Join(t.TempDir(), "agent")
	profile := agentruntime.Profile{BaseURL: server.URL + "/v1", APIKey: "fixture-key", ModelID: "fixture-model"}
	opts := map[string]any{}
	rt := New(Dependencies{
		ResolveBinary: func(context.Context) (dshcli.Info, error) {
			return dshcli.Info{Path: binary, Version: "0.1.5-rc.3"}, nil
		},
		ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "alice", RuntimeID: "rt-alice", Profile: profile, RuntimeOptions: opts, MCPServers: map[string]any{"verify": map[string]any{"url": resources.URL}}}, nil
		},
		AgentHome: func(string) (string, error) { return home, nil },
	})
	defer rt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := rt.Provision(ctx, agentruntime.ProvisionRequest{AgentID: "alice", RuntimeID: "rt-alice", Profile: profile}); err != nil {
		t.Fatal(err)
	}
	handle, err := rt.New(ctx, agentruntime.Spec{AgentID: "alice", RuntimeID: "rt-alice", Profile: profile})
	if err != nil {
		log, _ := os.ReadFile(filepath.Join(home, hostStateDirName, stderrFileName))
		t.Fatalf("start: %v\n%s", err, log)
	}
	run := func(key, text string) {
		t.Helper()
		result := rt.Conversation(handle.RuntimeID).Run(ctx, contract.TurnRequest{
			ID: contract.TurnID(key), ConversationKey: contract.ConversationKey(key),
			Input: []contract.InputPart{{Kind: contract.InputPartText, Text: text}},
		}, nil)
		if result.Status != contract.TurnSucceeded || result.Output != "正常对话已完成。" {
			log, _ := os.ReadFile(filepath.Join(home, hostStateDirName, stderrFileName))
			t.Fatalf("ordinary turn: %+v\n%s", result, log)
		}
	}
	run("ordinary-chat", "我的工作文档都使用中文，回复用中文就可以。")
	run("ordinary-chat", "我还喜欢回答先给结论。")
	statePath := filepath.Join(home, hostStateDirName, homeDirName, "memories", "maintenance.json")
	if state, err := os.ReadFile(statePath); err != nil || !strings.Contains(string(state), "ordinary") && !strings.Contains(string(state), "工作文档") {
		t.Fatalf("ordinary conversation checkpoint not saved: %v", err)
	} else if strings.Contains(string(state), evidencePassword) || strings.Contains(string(state), evidenceToken) {
		t.Fatal("JSON credentials were persisted in the memory queue")
	}
	if _, err := rt.Stop(ctx, handle); err != nil {
		t.Fatal(err)
	}
	opts, err = rt.ConfigureMemory(opts, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Start(ctx, handle); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	mu.Lock()
	count := extractionCalls + consolidationCalls
	mu.Unlock()
	if count != 0 {
		t.Fatal("disabled memory processed a pending snapshot")
	}
	if _, err := rt.Stop(ctx, handle); err != nil {
		t.Fatal(err)
	}
	opts, err = rt.ConfigureMemory(opts, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Start(ctx, handle); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		document, err := rt.ReadMemoryDocument(ctx, home, opts)
		if err != nil {
			t.Fatal(err)
		}
		if document.Content == combinedSummary {
			break
		}
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(filepath.Join(home, hostStateDirName, stderrFileName))
			t.Fatalf("background memory was not generated after restart\n%s", log)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if content, err := os.ReadFile(filepath.Join(home, hostStateDirName, homeDirName, "memories", "MEMORY.md")); err != nil || !strings.Contains(string(content), verification) {
		t.Fatalf("detailed handbook was not saved: %v", err)
	}
	run("new-conversation", "请读取详细手册，帮我写一个工作说明。")
	mu.Lock()
	defer mu.Unlock()
	if extractionCalls != 2 || consolidationCalls != 2 {
		t.Fatalf("unexpected pipeline calls: extract=%d consolidate=%d", extractionCalls, consolidationCalls)
	}
	if len(mainBodies) != 5 || !strings.Contains(mainBodies[3], combinedSummary) || !strings.Contains(mainBodies[4], verification) {
		t.Fatal("new ordinary conversation did not receive the generated memory")
	}
	reports, err := os.ReadDir(filepath.Join(home, hostStateDirName, homeDirName, "memories", "rollout_summaries"))
	if err != nil || len(reports) != 2 {
		t.Fatalf("memory evidence record missing: %v", err)
	}
}
