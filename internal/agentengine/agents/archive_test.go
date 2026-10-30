package agents

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

func TestArchivePersistsDisablesStartupAndPreservesWorkspaceOnDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	state := filepath.Join(t.TempDir(), "agents.json")
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", state, WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}))
	if err != nil {
		t.Fatal(err)
	}
	svc.agents["agent-worker"] = Agent{ID: "agent-worker", Name: "worker", Role: RoleWorker, RuntimeKind: RuntimeKindCodex, RuntimeID: "rt-worker", Status: string(agentruntime.StateRunning), DesiredState: DesiredStateRunning, ProfileComplete: true, AgentProfile: AgentProfile{Name: "worker", Provider: ProviderAPI, BaseURL: "https://example.com/v1", APIKey: "secret", ModelID: "model", ProfileComplete: true}}
	if err := svc.saveLocked(); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(context.Background(), "agent-worker"); err == nil {
		t.Fatal("deleted active agent")
	}
	token := svc.agentAccessToken("agent-worker")
	if !svc.AuthorizesAgentAccessToken("agent-worker", token) {
		t.Fatal("active agent token rejected")
	}
	if _, err := svc.SetArchived(context.Background(), "agent-worker", true); err != nil {
		t.Fatal(err)
	}
	item, _ := svc.Agent("agent-worker")
	if svc.AuthorizesAgentAccessToken(item.ID, token) {
		t.Fatal("archived agent token still authorized")
	}
	if item.ArchivedAt == nil || item.DesiredState != DesiredStateStopped || isConfiguredAgentStartupCandidate(item) {
		t.Fatalf("archive = %+v", item)
	}
	if repeated, err := svc.SetArchived(context.Background(), item.ID, true); err != nil || repeated.ArchivedAt == nil || !repeated.ArchivedAt.Equal(*item.ArchivedAt) {
		t.Fatalf("repeat archive changed its transition identity: %+v %v", repeated, err)
	}
	if _, err := svc.Start(context.Background(), item.ID); err == nil {
		t.Fatal("started archived agent")
	}
	if _, err := svc.Recreate(context.Background(), item.ID, contract.AgentRecreateOptions{}); err == nil {
		t.Fatal("recreated archived agent")
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	item, _ = svc.Agent(item.ID)
	if item.ArchivedAt == nil {
		t.Fatal("archive lost after reload")
	}
	normal, err := svc.List(context.Background(), contract.AgentListOptions{})
	if err != nil || len(normal) != 0 {
		t.Fatalf("normal list = %v, %v", normal, err)
	}
	all, err := svc.List(context.Background(), contract.AgentListOptions{IncludeArchived: true})
	if err != nil || len(all) != 1 || all[0].ArchivedAt == nil {
		t.Fatalf("archive list = %v, %v", all, err)
	}

	if restored, err := svc.SetArchived(context.Background(), item.ID, false); err != nil || restored.ArchivedAt != nil || restored.Spec.DesiredState != contract.AgentDesiredStateRunning {
		t.Fatalf("restore=%+v %v", restored, err)
	}
	if !svc.AuthorizesAgentAccessToken(item.ID, token) {
		t.Fatal("restored agent token rejected")
	}
	if _, err := svc.SetArchived(context.Background(), item.ID, true); err != nil {
		t.Fatal(err)
	}
	root, err := svc.WorkspaceRootByID(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "result.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	home, err := svc.agentHomeDir(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(home, "private-token"), []byte("remove"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = svc.Delete(context.Background(), item.ID); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "result.txt")); err != nil || string(data) != "keep" {
		t.Fatalf("working file: %q %v", data, err)
	}
	if _, err = os.Stat(filepath.Join(home, "private-token")); !os.IsNotExist(err) {
		t.Fatalf("private runtime state remains: %v", err)
	}
	if _, ok := svc.Agent(item.ID); ok {
		t.Fatal("agent record remains")
	}
	if svc.AuthorizesAgentAccessToken(item.ID, token) {
		t.Fatal("deleted agent token still authorized")
	}
	svc.agents[ManagerUserID] = Agent{ID: ManagerUserID, Name: ManagerName, Role: RoleManager, RuntimeKind: RuntimeKindCodex}
	if _, err := svc.SetArchived(context.Background(), ManagerUserID, true); err == nil {
		t.Fatal("archived protected manager")
	}
}
