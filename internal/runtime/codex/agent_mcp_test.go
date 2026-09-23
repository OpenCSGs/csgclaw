package codex

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/opencsgmcp"
	agentruntime "csgclaw/internal/runtime"
)

func testAgentMCPProfile() agentruntime.Profile {
	return agentruntime.Profile{Provider: "codex", ModelID: "gpt-5.5", APIKey: "agent-token", Env: map[string]string{
		"CSGCLAW_CALLER_AGENT_ID": "agent-alice", "CSGCLAW_BASE_URL": "http://127.0.0.1:18080", agentAccessTokenEnv: "agent-token",
	}}
}

func TestAgentMCPProjectionPreservesManualServersAndScopedCredential(t *testing.T) {
	r := New(Dependencies{})
	r.SetAgentMCPRevisionSource(func(string) uint64 { return 7 })
	profile := testAgentMCPProfile()
	servers, err := r.projectAgentMCP(profile, nil, "[mcp_servers.manual]\nurl = \"https://manual.example/mcp\"\n[mcp_servers.csgclaw]\nurl = \"https://stale.example\"\n")
	if err != nil {
		t.Fatal(err)
	}
	builtin := servers[AgentMCPServerName].(map[string]any)
	if builtin["required"] != true {
		t.Fatal("platform catalog must be ready before an Agent turn")
	}
	if builtin["url"] != "http://127.0.0.1:18080/api/v1/agents/agent-alice/mcp?catalog_revision=7" || builtin["bearer_token_env_var"] != agentAccessTokenEnv {
		t.Fatalf("builtin = %#v", builtin)
	}
	if servers["manual"] == nil {
		t.Fatal("unmanaged manual server lost")
	}
	explicit := map[string]any{}
	projected, err := r.projectAgentMCP(profile, explicit, "")
	if err != nil || len(projected) != 1 || len(explicit) != 0 {
		t.Fatalf("projection mutated desired state: %#v %v", explicit, err)
	}
	if err := r.ValidateMCPServers(context.Background(), agentruntime.MCPServersSnapshot{Servers: projected}); err == nil {
		t.Fatal("reserved server was accepted as manual")
	}
}

func TestAgentMCPReadbackExcludesReservedServer(t *testing.T) {
	home := t.TempDir()
	r := New(Dependencies{AgentHome: func(string) (string, error) { return home, nil }, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) { return AgentRef{ID: "agent-alice", Name: "alice"}, nil }})
	codexHome, err := r.resolveCodexHomeDir("agent-alice")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(codexHome, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(codexHome, configFileName), []byte("[mcp_servers.csgclaw]\nurl = \"http://localhost/mcp\"\n[mcp_servers.manual]\nurl = \"https://manual.example/mcp\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := r.ListMCPServers(context.Background(), agentruntime.Handle{RuntimeID: "rt-agent-alice"}, agentruntime.MCPServersSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Servers) != 1 || got.Servers["manual"] == nil {
		t.Fatalf("readback = %#v", got.Servers)
	}
}

func TestAgentMCPReloadChangesURLAndRetriesBeforePrompt(t *testing.T) {
	home := t.TempDir()
	writer := &lockedStringWriter{}
	client := newAppServerClient(writer, nil)
	profile := testAgentMCPProfile()
	manager := newAppServerManager(managerDeps{})
	live := &liveSession{spec: SessionSpec{RuntimeID: "rt-alice", AgentID: "agent-alice", CodexHomeDir: home, WorkspaceDir: home, Profile: profile}, appClient: client}
	manager.sessions["rt-alice"] = live
	r := New(Dependencies{Manager: manager, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "agent-alice", Name: "alice", Profile: profile, MCPServers: map[string]any{"parser": map[string]any{"url": "https://managed.example/mcp"}}}, nil
	}, MaterializeMCPServers: func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{"parser": map[string]any{"url": "http://127.0.0.1:18080/api/v1/opencsg-mcp-gateway/mcp", "headers": map[string]any{"Authorization": "Bearer bridge-root"}, opencsgmcp.RuntimeFileBindingsKey: map[string]any{"read": map[string]any{"encoding": "base64"}}}}, nil
	}})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.RefreshAgentMCP(ctx, "agent-alice", 2) }()
	req := waitForJSONRPCLine(t, writer)
	if req["method"] != "config/mcpServer/reload" {
		t.Fatalf("method = %#v", req)
	}
	raw, err := os.ReadFile(filepath.Join(home, configFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "/api/v1/agents/agent-alice/mcp-file-bridge/parser") || !strings.Contains(string(raw), opencsgmcp.FileBridgeToken("bridge-root", "agent-alice", "parser")) || strings.Contains(string(raw), "https://managed.example") {
		t.Fatal("App reload lost the managed MCP file bridge")
	}
	if !strings.Contains(string(raw), "catalog_revision=2") {
		t.Fatalf("catalog revision absent: %s", raw)
	}
	client.handleLine(`{"id":1,"error":{"code":-1,"message":"temporary failure"}}`)
	if err = <-done; err == nil {
		t.Fatal("reload failure hidden")
	}
	if live.mcpCatalogRevision != 0 {
		t.Fatal("failed revision marked applied")
	}
	writer.mu.Lock()
	writer.b.Reset()
	writer.mu.Unlock()
	go func() { done <- r.refreshSessionMCP(ctx, SessionHandle{RuntimeID: "rt-alice"}) }()
	req = waitForJSONRPCLine(t, writer)
	if req["method"] != "config/mcpServer/reload" {
		t.Fatalf("retry method = %#v", req)
	}
	client.handleLine(`{"id":2,"result":{}}`)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if live.mcpCatalogRevision != 2 {
		t.Fatal("reload revision not acknowledged")
	}
}

func TestAgentMCPEnvironmentNeverInheritsAdminToken(t *testing.T) {
	t.Setenv(agentAccessTokenEnv, "admin-secret")
	t.Setenv("CSGCLAW_CONNECTOR_CAPABILITY", "admin-capability")
	t.Setenv("OPENAI_API_KEY", "host-key")
	for _, mode := range []string{ExecutionModeStandard, ExecutionModeReadOnly} {
		env := strings.Join(buildSessionEnv(SessionSpec{ExecutionMode: mode, Profile: testAgentMCPProfile()}), "\n")
		for _, secret := range []string{"admin-secret", "admin-capability", "host-key"} {
			if strings.Contains(env, secret) {
				t.Fatalf("inherited secret in %s", mode)
			}
		}
		if !strings.Contains(env, agentAccessTokenEnv+"=agent-token") {
			t.Fatalf("scoped token absent in %s", mode)
		}
	}
}

func TestAgentMCPFileToolRequiresExactActiveTurn(t *testing.T) {
	sink := &recordingSink{}
	manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
	spec := testAppServerSessionSpec(t.TempDir())
	spec.AgentID = "agent-alice"
	live := &liveSession{spec: spec, filePublishingThreads: map[string]bool{"thread-a": true, "thread-b": true}, conversationSessions: map[string]string{"room-a": "thread-a", "room-b": "thread-b"}}
	manager.sessions[spec.RuntimeID] = live
	r := New(Dependencies{Manager: manager})
	for _, thread := range []string{"thread-a", "thread-b"} {
		w, err := live.registerAppServerTurnWaiter(thread)
		if err != nil {
			t.Fatal(err)
		}
		w.setTurnID("turn-" + thread)
		live.setAppServerTurnContext(thread, w, context.Background())
	}
	args := json.RawMessage(`{"path":"result.txt"}`)
	if _, err := r.CallPlatformFileTool(context.Background(), "agent-bob", appServerPublishFileToolName, "thread-a", "turn-thread-a", args); err == nil {
		t.Fatal("other agent accepted")
	}
	if _, err := r.CallPlatformFileTool(context.Background(), "agent-alice", appServerPublishFileToolName, "thread-a", "turn-thread-b", args); err == nil {
		t.Fatal("mixed thread/turn accepted")
	}
	result, err := r.CallPlatformFileTool(context.Background(), "agent-alice", appServerPublishFileToolName, "thread-a", "turn-thread-a", args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), `"isError":false`) {
		t.Fatalf("result = %s", result)
	}
	events := sink.snapshot()
	if len(events) != 1 || events[0].SessionID != "thread-a" || events[0].TurnID != "turn-thread-a" {
		t.Fatalf("events = %#v", events)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	w := live.appServerTurnWaiter("thread-a")
	live.setAppServerTurnContext("thread-a", w, cancelled)
	if _, err := r.ActiveTurnContext(spec.RuntimeID, "thread-a", "turn-thread-a"); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled turn accepted")
	}
}
