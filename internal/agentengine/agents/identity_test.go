package agents

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"path/filepath"
	"testing"
)

func TestNamingProvenanceSurvivesRenameAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", path)
	if err != nil {
		t.Fatal(err)
	}
	// Legacy creation sources are unknown and must not be backfilled.
	svc.agents["agent-identity"] = Agent{ID: "agent-identity", Name: "Original", Role: RoleWorker, RuntimeID: "rt-identity", RuntimeKind: RuntimeKindCodex, Status: string(agentruntime.StateStopped), DesiredState: string(agentruntime.StateStopped)}
	updated, err := svc.Update(context.Background(), "agent-identity", contract.AgentUpdateRequest{ActorID: "user-owner", Spec: contract.AgentSpec{Name: "Reviewer"}, FieldMask: []string{"name"}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CreatedBy != "" || updated.NamedBy != "user-owner" {
		t.Fatalf("wrong provenance: %+v", updated)
	}
	if _, err := svc.Update(context.Background(), "agent-identity", contract.AgentUpdateRequest{ActorID: "another", Spec: contract.AgentSpec{Name: "Reviewer"}, FieldMask: []string{"name"}}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewController(testModelConfig(), config.ServerConfig{}, "", path)
	if err != nil {
		t.Fatal(err)
	}
	record, ok := reloaded.Agent("agent-identity")
	if !ok || record.NamedBy != "user-owner" || record.CreatedBy != "" || record.Name != "Reviewer" {
		t.Fatalf("provenance changed on reload: %+v", record)
	}
}

func TestCreationRecordsActualIdentityActor(t *testing.T) {
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Create(withIdentityActor(context.Background(), "ignored"), contract.AgentCreateRequest{ActorID: "user-owner", Spec: contract.AgentSpec{Name: "NewReviewer", Role: contract.AgentRoleWorker, Runtime: contract.RuntimeSpec{Adapter: RuntimeNameCodex}, DesiredState: contract.AgentDesiredStateStopped}})
	if err != nil {
		t.Fatal(err)
	}
	if created.CreatedBy != "user-owner" || created.NamedBy != "user-owner" {
		t.Fatalf("creation actor lost: %+v", created)
	}
}
