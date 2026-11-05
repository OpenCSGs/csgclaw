package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentengine/enginetest"
	"csgclaw/internal/auth"
	"csgclaw/internal/config"
	"csgclaw/internal/mcp"
	agentruntime "csgclaw/internal/runtime"
	hub "csgclaw/internal/template"
)

func TestAgentRecreateUsesCurrentConfigurationWithoutHubAuthentication(t *testing.T) {
	previous := appAuthStatus
	authCalls := 0
	appAuthStatus = func(*http.Request) (auth.Status, error) {
		authCalls++
		return auth.Status{}, errors.New("read Hub credential cache")
	}
	t.Cleanup(func() { appAuthStatus = previous })

	for _, test := range []struct {
		name, body, description string
	}{
		{name: "current configuration", body: `{}`, description: "original"},
		{name: "selected replacement", body: `{"configuration":{"description":"updated"},"field_mask":["description"]}`, description: "updated"},
	} {
		t.Run(test.name, func(t *testing.T) {
			seed := agentengine.Agent{ID: "agent-worker", ResourceVersion: "1", Spec: agentengine.AgentSpec{
				Name: "worker", Description: "original", Role: agentengine.AgentRoleWorker,
				Runtime: agentengine.RuntimeSpec{Adapter: "codex"},
			}}
			client := enginetest.NewMemoryClient(seed)
			handler := &Handler{agentEngine: client, configPath: filepath.Join(t.TempDir(), "config.toml")}
			recorder := httptest.NewRecorder()
			handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-worker/recreate", strings.NewReader(test.body)))
			if recorder.Code != http.StatusOK {
				t.Fatalf("recreate = %d: %s", recorder.Code, recorder.Body.String())
			}
			got, err := client.Agents().Get(context.Background(), seed.ID, agentengine.AgentGetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Spec.Name != seed.Spec.Name || got.Spec.Description != test.description {
				t.Fatalf("unexpected configuration: %+v", got.Spec)
			}
		})
	}
	if authCalls != 0 {
		t.Fatalf("Hub authentication calls = %d, want 0", authCalls)
	}
}

func TestAgentRecreateRuntimeChangeFiltersTemplateMCPWithoutHubAuthentication(t *testing.T) {
	previous := appAuthStatus
	authCalls := 0
	appAuthStatus = func(*http.Request) (auth.Status, error) {
		authCalls++
		return auth.Status{}, errors.New("read Hub credential cache")
	}
	t.Cleanup(func() { appAuthStatus = previous })

	registryRoot, workspaceRoot := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "USER.md"), []byte("template user\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := hub.NewLocalStore(registryRoot)
	if _, err := store.Publish(context.Background(), hub.PublishSpec{
		ID: "dsh-worker", Name: "dsh-worker", RuntimeKind: agent.RuntimeKindDSH,
		WorkspaceRef: hub.WorkspaceRef{Kind: hub.WorkspaceKindDir, Path: workspaceRoot},
		MCPServers: map[string]any{
			"manual": map[string]any{"command": "manual-mcp"},
			"remote": mcp.RemoteServer{ID: "remote-denied", Name: "Remote Docs", URL: "https://remote.example.test/mcp"}.Config(),
		},
	}); err != nil {
		t.Fatal(err)
	}
	hubSvc, err := hub.NewService(config.HubConfig{
		DefaultRegistry: "local",
		Registries:      []config.HubRegistryConfig{{Name: "local", Kind: hub.RegistryKindLocal, Path: registryRoot, Enabled: true}},
	}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatal(err)
	}
	runtimeCreates := 0
	svc := mustNewSeededServiceWithOptions(t, []agent.Agent{{
		ID: "agent-worker", Name: "worker", Role: agent.RoleWorker,
		RuntimeKind: agent.RuntimeKindCodex, RuntimeName: agent.RuntimeNameCodex,
		RuntimeID: "runtime-worker", BoxID: "box-worker", Status: "stopped",
		AgentProfile: agent.AgentProfile{
			Name: "worker", Provider: agent.ProviderAPI, BaseURL: "https://provider.example.test/v1",
			APIKey: "test-key", ModelID: "model-1", ProfileComplete: true,
		},
	}}, agent.WithHubService(hubSvc),
		agent.WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local.dsh-worker"}),
		agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}),
		agent.WithRuntime(templateCodexTestRuntime{fakeCompatRuntime{kind: agent.RuntimeKindDSH, new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
			runtimeCreates++
			return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "dsh-worker"}, nil
		}}}))
	before, ok := svc.Agent("agent-worker")
	if !ok || before.MCPServers != nil {
		t.Fatalf("initial Agent MCP configuration = %#v, found=%t", before.MCPServers, ok)
	}
	handler := &Handler{
		svc: svc, agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc,
		configPath: filepath.Join(t.TempDir(), "config.toml"),
		mcp:        mcp.NewService(mcp.WithServerProber(failingMCPServerProber{})),
	}
	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-worker/recreate",
		strings.NewReader(`{"configuration":{"runtime":{"name":"dsh"}},"field_mask":["runtime"]}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("recreate = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		MCPServers       map[string]any                `json:"mcp_servers"`
		SkippedResources []mcp.SkippedTemplateResource `json:"skipped_resources"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.SkippedResources) != 1 || response.SkippedResources[0].Type != mcp.TemplateResourceMCP || response.SkippedResources[0].Name != "Remote Docs" {
		t.Fatalf("skipped resources = %#v", response.SkippedResources)
	}
	after, ok := svc.Agent(before.ID)
	if !ok || after.RuntimeKind != agent.RuntimeKindDSH || runtimeCreates != 1 {
		t.Fatalf("recreated Agent = %+v, found=%t, runtime creations=%d", after, ok, runtimeCreates)
	}
	for name, servers := range map[string]map[string]any{"response": response.MCPServers, "saved Agent": after.MCPServers} {
		if _, retained := servers["remote"]; retained {
			t.Fatalf("%s retained unavailable MCP: %#v", name, servers)
		}
		manual, ok := servers["manual"].(map[string]any)
		if !ok || manual["command"] != "manual-mcp" {
			t.Fatalf("%s lost manual MCP: %#v", name, servers)
		}
	}
	if authCalls != 0 {
		t.Fatalf("Hub authentication calls = %d, want 0", authCalls)
	}
}
