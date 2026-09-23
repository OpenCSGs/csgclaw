package dsh

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/dshcli"
	agentruntime "csgclaw/internal/runtime"
)

func TestProjectAgentMCPPreservesManualServers(t *testing.T) {
	profile := agentMCPTestProfile()
	manual := map[string]any{"manual": map[string]any{"url": "https://example.test/mcp"}}
	projected, err := projectAgentMCP(profile, manual, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(manual) != 1 || projected["manual"] == nil {
		t.Fatalf("manual MCP configuration changed: %#v", manual)
	}
	builtin := projected[agentMCPServerName].(map[string]any)
	if builtin["url"] != "http://127.0.0.1:18080/api/v1/agents/agent-alice/mcp?catalog_revision=7" {
		t.Fatalf("App MCP URL = %v", builtin["url"])
	}
	if builtin["headers"].(map[string]any)["Authorization"] != "Bearer scoped-token" {
		t.Fatal("App MCP did not use the scoped Agent token")
	}
	if err := New(Dependencies{}).ValidateMCPServers(context.Background(), agentruntime.MCPServersSnapshot{Servers: projected}); err == nil {
		t.Fatal("reserved App MCP server was accepted as manual configuration")
	}
}

func TestDSHSelectedWorkspaceAndAppCatalogRefresh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX launcher")
	}
	root := t.TempDir()
	project := filepath.Join(root, "project")
	other := filepath.Join(root, "other")
	for _, path := range []string{project, other} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "index.html"), []byte("<!doctype html>"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	projectInstructions := "Project-specific instructions\n"
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte(projectInstructions), 0o600); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(root, "dsh-test")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexec \"$DSH_TEST_BINARY\" -test.run=TestDSHHelperProcess\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(root, "session.json")
	t.Setenv("GO_WANT_DSH_HELPER_PROCESS", "1")
	t.Setenv("DSH_TEST_BINARY", os.Args[0])
	t.Setenv("DSH_TEST_SESSION_RECORD", recordPath)
	profile := agentMCPTestProfile()
	ref := AgentRef{ID: "agent-alice", RuntimeID: "rt-agent-alice", Instructions: "Follow the Agent instructions.", Profile: profile, RuntimeOptions: map[string]any{
		localWorkspaceDirOptionKey: project, PermissionModeOptionKey: PermissionModeDangerFullAccess,
	}}
	rt := New(Dependencies{
		ResolveBinary: func(context.Context) (dshcli.Info, error) { return dshcli.Info{Path: launcher}, nil },
		ResolveAgent:  func(agentruntime.Handle) (AgentRef, error) { return ref, nil },
		AgentHome:     func(string) (string, error) { return filepath.Join(root, "agent"), nil },
	})
	t.Cleanup(func() { _ = rt.Close() })
	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{RuntimeID: ref.RuntimeID, AgentID: ref.ID, Profile: profile, Instructions: ref.Instructions}); err != nil {
		t.Fatal(err)
	}
	handle, err := rt.New(context.Background(), agentruntime.Spec{RuntimeID: ref.RuntimeID, AgentID: ref.ID, Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	proc, err := rt.process(ref.RuntimeID)
	if err != nil || environmentMap(proc.environment)[permissionModeEnvName] != PermissionModeDangerFullAccess {
		t.Fatalf("DSH permission mode was not applied: %v", err)
	}
	run := func(id string) map[string]any {
		t.Helper()
		result := rt.Conversation(ref.RuntimeID).Run(context.Background(), contract.TurnRequest{
			ID: contract.TurnID(id), ConversationKey: "room-1", Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "hello"}},
		}, nil)
		if result.Status != contract.TurnSucceeded {
			t.Fatalf("DSH turn = %+v", result)
		}
		data, err := os.ReadFile(recordPath)
		if err != nil {
			t.Fatal(err)
		}
		var record map[string]any
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		return record
	}
	first := run("turn-1")
	assertDSHSessionWorkspaceAndApp(t, first, project, "catalog_revision=0")
	if first["method"] != "session/new" {
		t.Fatalf("first session method = %v", first["method"])
	}
	if contents, err := os.ReadFile(filepath.Join(project, "AGENTS.md")); err != nil || string(contents) != projectInstructions {
		t.Fatalf("selected project's AGENTS.md changed: %q, %v", contents, err)
	}
	global := filepath.Join(root, "agent", hostStateDirName, homeDirName, "AGENTS.md")
	if contents, err := os.ReadFile(global); err != nil || !strings.Contains(string(contents), "Follow the Agent instructions.") {
		t.Fatalf("DSH global instructions = %q, %v", contents, err)
	}
	if err := rt.RefreshAgentMCP(context.Background(), ref.ID, 2); err != nil {
		t.Fatal(err)
	}
	second := run("turn-2")
	assertDSHSessionWorkspaceAndApp(t, second, project, "catalog_revision=2")
	if second["method"] != "session/resume" {
		t.Fatalf("refreshed session method = %v", second["method"])
	}
	if _, err := rt.Stop(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	ref.RuntimeOptions[localWorkspaceDirOptionKey] = other
	if _, err := rt.Start(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	third := run("turn-3")
	assertDSHSessionWorkspaceAndApp(t, third, other, "catalog_revision=2")
	if third["method"] != "session/new" {
		t.Fatalf("changed workspace resumed an old session: %v", third["method"])
	}
	if _, err := rt.Stop(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	ref.RuntimeOptions[localWorkspaceDirOptionKey] = ""
	if _, err := rt.Start(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	if contents, err := os.ReadFile(global); err != nil || !strings.Contains(string(contents), "Follow the Agent instructions.") {
		t.Fatalf("DSH home instructions changed after restoring default workspace: %q, %v", contents, err)
	}
	if _, err := os.Stat(filepath.Join(root, "agent", hostStateDirName, workspaceDirName, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("managed workspace contains a duplicate AGENTS.md: %v", err)
	}
}

func agentMCPTestProfile() agentruntime.Profile {
	return agentruntime.Profile{BaseURL: "https://gateway.example/v1", APIKey: "model-token", ModelID: "test-model", Env: map[string]string{
		"CSGCLAW_CALLER_AGENT_ID": "agent-alice",
		"CSGCLAW_BASE_URL":        "http://127.0.0.1:18080",
		"CSGCLAW_ACCESS_TOKEN":    "scoped-token",
	}}
}

func assertDSHSessionWorkspaceAndApp(t *testing.T, record map[string]any, workspace, revision string) {
	t.Helper()
	params, _ := record["params"].(map[string]any)
	physicalWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if record["cwd"] != physicalWorkspace || params["cwd"] != workspace {
		t.Fatalf("DSH cwd = %v, session cwd = %v; want %s", record["cwd"], params["cwd"], workspace)
	}
	servers, _ := params["mcpServers"].([]any)
	for _, raw := range servers {
		server, _ := raw.(map[string]any)
		if server["name"] != agentMCPServerName {
			continue
		}
		if server["type"] != "http" || !strings.Contains(server["url"].(string), revision) {
			t.Fatalf("App MCP server = %#v", server)
		}
		headers, _ := server["headers"].([]any)
		for _, rawHeader := range headers {
			header, _ := rawHeader.(map[string]any)
			if header["name"] == "Authorization" && header["value"] == "Bearer scoped-token" {
				return
			}
		}
		t.Fatalf("App MCP server lacks scoped authorization: %#v", server)
	}
	t.Fatalf("App MCP server missing from DSH session: %#v", servers)
}
