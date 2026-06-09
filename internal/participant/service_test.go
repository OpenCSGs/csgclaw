package participant

import (
	"context"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/sandboxtest"
)

func TestCreateAgentParticipantUsesStableParticipantIDForDefaultAgentID(t *testing.T) {
	agentSvc := mustNewAgentService(t)
	imSvc := im.NewService()
	store := NewMemoryStore(nil)
	svc := NewService(store, WithAgentService(agentSvc), WithIMService(imSvc))

	created, err := svc.Create(context.Background(), CreateRequest{
		ID:      "qa",
		Channel: ChannelCSGClaw,
		Type:    TypeAgent,
		Name:    "QA Display Name",
		ChannelUser: ChannelUserSpec{
			Ref:  "u-qa",
			Kind: ChannelUserKindLocalUserID,
		},
		AgentBinding: AgentBindingSpec{
			Mode: BindingModeCreate,
			Agent: &agent.CreateAgentSpec{
				Name:        "QA Display Name",
				Role:        agent.RoleWorker,
				RuntimeKind: agent.RuntimeKindPicoClawSandbox,
				Image:       "agent-image:test",
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if created.ID != "qa" {
		t.Fatalf("participant ID = %q, want qa", created.ID)
	}
	if created.AgentID != "u-qa" {
		t.Fatalf("agent ID = %q, want u-qa", created.AgentID)
	}
	if _, ok := agentSvc.Agent("u-qa"); !ok {
		t.Fatal("agent u-qa was not created")
	}
	if _, ok := agentSvc.Agent("u-qa-display-name"); ok {
		t.Fatal("agent ID was derived from editable display name")
	}
	if user, ok := imSvc.User("u-qa"); !ok || !strings.EqualFold(user.Name, "QA Display Name") {
		t.Fatalf("channel user = %+v, ok=%v; want u-qa display user", user, ok)
	}
}

func TestCreateAgentParticipantCanReuseExistingAgentWithDifferentParticipantID(t *testing.T) {
	agentSvc := mustNewAgentService(t)
	imSvc := im.NewService()
	store := NewMemoryStore(nil)
	svc := NewService(store, WithAgentService(agentSvc), WithIMService(imSvc))

	if _, err := agentSvc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:          "u-qa",
			Name:        "QA Runtime",
			Role:        agent.RoleWorker,
			RuntimeKind: agent.RuntimeKindPicoClawSandbox,
			Image:       "agent-image:test",
		},
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	created, err := svc.Create(context.Background(), CreateRequest{
		ID:            "test",
		Channel:       ChannelFeishu,
		Type:          TypeAgent,
		Name:          "QA Feishu",
		ChannelAppRef: "cli_xxx",
		ChannelUser: ChannelUserSpec{
			Ref:  "ou_xxx",
			Kind: ChannelUserKindOpenID,
		},
		AgentBinding: AgentBindingSpec{
			Mode:    BindingModeReuse,
			AgentID: "u-qa",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if created.ID != "test" || created.AgentID != "u-qa" {
		t.Fatalf("created participant = %+v, want id test bound to u-qa", created)
	}
	if created.ChannelUserRef != "ou_xxx" || created.ChannelAppRef != "cli_xxx" {
		t.Fatalf("created participant channel identity = %+v, want Feishu app/open_id scope", created)
	}
}

func TestEnsureBootstrapAdminCreatesHumanParticipantWithoutAgent(t *testing.T) {
	imSvc := im.NewService()
	store := NewMemoryStore(nil)
	svc := NewService(store, WithIMService(imSvc))

	created, err := svc.EnsureBootstrapAdmin(context.Background())
	if err != nil {
		t.Fatalf("EnsureBootstrapAdmin() error = %v", err)
	}

	if created.ID != im.AdminUserID {
		t.Fatalf("participant ID = %q, want %q", created.ID, im.AdminUserID)
	}
	if created.Type != TypeHuman {
		t.Fatalf("participant type = %q, want %q", created.Type, TypeHuman)
	}
	if created.AgentID != "" {
		t.Fatalf("agent ID = %q, want empty for human admin", created.AgentID)
	}
	if created.ChannelUserRef != im.AdminUserID {
		t.Fatalf("channel user ref = %q, want %q", created.ChannelUserRef, im.AdminUserID)
	}
	if created.ChannelUserKind != ChannelUserKindLocalUserID {
		t.Fatalf("channel user kind = %q, want %q", created.ChannelUserKind, ChannelUserKindLocalUserID)
	}
	if !created.Mentionable {
		t.Fatal("admin participant Mentionable = false, want true")
	}
	if _, ok := store.Get(ChannelCSGClaw, im.AdminUserID); !ok {
		t.Fatal("store missing admin participant")
	}
	if user, ok := imSvc.User(im.AdminUserID); !ok || user.ID != im.AdminUserID || user.Handle != "admin" || user.Role != "admin" {
		t.Fatalf("admin channel user = %+v, ok=%v; want local admin user", user, ok)
	}
}

func TestEnsureBootstrapAdminRenamesLegacyAdminParticipant(t *testing.T) {
	imSvc := im.NewService()
	createdAt := time.Date(2026, 6, 9, 10, 5, 0, 0, time.UTC)
	store := NewMemoryStore([]Participant{{
		ID:              "u-admin",
		Channel:         ChannelCSGClaw,
		Type:            TypeHuman,
		Name:            "Local Admin",
		Avatar:          "avatar.png",
		ChannelUserRef:  "u-admin",
		ChannelUserKind: ChannelUserKindLocalUserID,
		AgentID:         "u-admin",
		LifecycleStatus: LifecycleStatusActive,
		Mentionable:     true,
		Metadata:        map[string]any{"legacy": "kept"},
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}})
	svc := NewService(store, WithIMService(imSvc))

	created, err := svc.EnsureBootstrapAdmin(context.Background())
	if err != nil {
		t.Fatalf("EnsureBootstrapAdmin() error = %v", err)
	}

	if created.ID != im.AdminUserID || created.Type != TypeHuman {
		t.Fatalf("admin participant = %+v, want human participant %q", created, im.AdminUserID)
	}
	if created.AgentID != "" {
		t.Fatalf("agent ID = %q, want empty after admin migration", created.AgentID)
	}
	if created.ChannelUserRef != im.AdminUserID {
		t.Fatalf("channel user ref = %q, want %q", created.ChannelUserRef, im.AdminUserID)
	}
	if !created.CreatedAt.Equal(createdAt) || created.Avatar != "avatar.png" || created.Metadata["legacy"] != "kept" {
		t.Fatalf("admin participant did not preserve legacy fields: %+v", created)
	}
	if _, ok := store.Get(ChannelCSGClaw, "u-admin"); ok {
		t.Fatal("legacy admin participant u-admin was not deleted")
	}
}

func TestEnsureBootstrapManagerUsesDefaultParticipantIDSeparateFromAgentID(t *testing.T) {
	agentSvc := mustNewManagerAgentService(t)
	imSvc := im.NewService()
	store := NewMemoryStore(nil)
	svc := NewService(store, WithAgentService(agentSvc), WithIMService(imSvc))

	created, err := svc.EnsureBootstrapManager(context.Background())
	if err != nil {
		t.Fatalf("EnsureBootstrapManager() error = %v", err)
	}

	if created.ID != agent.ManagerParticipantID {
		t.Fatalf("participant ID = %q, want %q", created.ID, agent.ManagerParticipantID)
	}
	if created.AgentID != agent.ManagerUserID {
		t.Fatalf("agent ID = %q, want %q", created.AgentID, agent.ManagerUserID)
	}
	if created.ChannelUserRef != agent.ManagerParticipantID {
		t.Fatalf("channel user ref = %q, want %q", created.ChannelUserRef, agent.ManagerParticipantID)
	}
	if user, ok := imSvc.User(agent.ManagerParticipantID); !ok || user.ID != agent.ManagerParticipantID {
		t.Fatalf("manager channel user = %+v, ok=%v; want local user %q", user, ok, agent.ManagerParticipantID)
	}
	if _, ok := store.Get(ChannelCSGClaw, agent.ManagerParticipantID); !ok {
		t.Fatalf("store missing manager participant %q", agent.ManagerParticipantID)
	}
	if _, ok := store.Get(ChannelCSGClaw, agent.ManagerUserID); ok {
		t.Fatalf("store still has manager participant under agent ID %q", agent.ManagerUserID)
	}
}

func TestEnsureBootstrapManagerRenamesLegacyManagerParticipant(t *testing.T) {
	agentSvc := mustNewManagerAgentService(t)
	imSvc := im.NewService()
	createdAt := time.Date(2026, 6, 4, 14, 0, 7, 0, time.UTC)
	store := NewMemoryStore([]Participant{{
		ID:              agent.ManagerUserID,
		Channel:         ChannelCSGClaw,
		Type:            TypeAgent,
		Name:            "manager",
		Avatar:          "avatar.png",
		ChannelUserRef:  agent.ManagerUserID,
		ChannelUserKind: ChannelUserKindLocalUserID,
		AgentID:         agent.ManagerUserID,
		LifecycleStatus: LifecycleStatusActive,
		Mentionable:     true,
		Metadata:        map[string]any{"legacy": "kept"},
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}})
	svc := NewService(store, WithAgentService(agentSvc), WithIMService(imSvc))

	created, err := svc.EnsureBootstrapManager(context.Background())
	if err != nil {
		t.Fatalf("EnsureBootstrapManager() error = %v", err)
	}

	if created.ID != agent.ManagerParticipantID || created.AgentID != agent.ManagerUserID {
		t.Fatalf("manager participant = %+v, want id %q bound to agent %q", created, agent.ManagerParticipantID, agent.ManagerUserID)
	}
	if !created.CreatedAt.Equal(createdAt) || created.Avatar != "avatar.png" || created.Metadata["legacy"] != "kept" {
		t.Fatalf("manager participant did not preserve legacy fields: %+v", created)
	}
	if _, ok := store.Get(ChannelCSGClaw, agent.ManagerUserID); ok {
		t.Fatalf("legacy manager participant %q was not deleted", agent.ManagerUserID)
	}
}

func TestEnsureBootstrapManagerDeletesMisspelledManagerParticipant(t *testing.T) {
	agentSvc := mustNewManagerAgentService(t)
	imSvc := im.NewService()
	createdAt := time.Date(2026, 6, 4, 14, 0, 7, 0, time.UTC)
	legacyID := "man" + "ger"
	store := NewMemoryStore([]Participant{{
		ID:              legacyID,
		Channel:         ChannelCSGClaw,
		Type:            TypeAgent,
		Name:            "manager",
		Avatar:          "avatar.png",
		ChannelUserRef:  legacyID,
		ChannelUserKind: ChannelUserKindLocalUserID,
		AgentID:         agent.ManagerUserID,
		LifecycleStatus: LifecycleStatusActive,
		Mentionable:     true,
		Metadata:        map[string]any{"legacy": "kept"},
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}})
	svc := NewService(store, WithAgentService(agentSvc), WithIMService(imSvc))

	created, err := svc.EnsureBootstrapManager(context.Background())
	if err != nil {
		t.Fatalf("EnsureBootstrapManager() error = %v", err)
	}

	if created.ID != agent.ManagerParticipantID || created.ChannelUserRef != agent.ManagerParticipantID || created.AgentID != agent.ManagerUserID {
		t.Fatalf("manager participant = %+v, want manager participant bound to %q", created, agent.ManagerUserID)
	}
	if !created.CreatedAt.Equal(createdAt) || created.Avatar != "avatar.png" || created.Metadata["legacy"] != "kept" {
		t.Fatalf("manager participant did not preserve legacy fields: %+v", created)
	}
	if _, ok := store.Get(ChannelCSGClaw, legacyID); ok {
		t.Fatalf("legacy manager participant %q was not deleted", legacyID)
	}
}

func TestDeleteParticipantDoesNotDeleteAgentByDefault(t *testing.T) {
	agentSvc := mustNewAgentService(t)
	svc := NewService(NewMemoryStore(nil), WithAgentService(agentSvc), WithIMService(im.NewService()))
	if _, err := agentSvc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:          "u-qa",
			Name:        "QA Runtime",
			Role:        agent.RoleWorker,
			RuntimeKind: agent.RuntimeKindPicoClawSandbox,
			Image:       "agent-image:test",
		},
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := svc.Create(context.Background(), CreateRequest{
		ID:      "qa",
		Channel: ChannelCSGClaw,
		Type:    TypeAgent,
		Name:    "QA",
		ChannelUser: ChannelUserSpec{
			Ref:  "u-qa",
			Kind: ChannelUserKindLocalUserID,
		},
		AgentBinding: AgentBindingSpec{
			Mode:    BindingModeReuse,
			AgentID: "u-qa",
		},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, ok, err := svc.Delete(context.Background(), ChannelCSGClaw, "qa", DeleteOptions{}); err != nil || !ok {
		t.Fatalf("Delete() ok=%v error=%v, want ok", ok, err)
	}
	if _, ok := svc.Get(ChannelCSGClaw, "qa"); ok {
		t.Fatal("participant csgclaw:qa still exists after delete")
	}
	if _, ok := agentSvc.Agent("u-qa"); !ok {
		t.Fatal("agent u-qa was deleted by default participant delete")
	}
}

func TestDeleteParticipantRejectsAgentCleanupWhenStillReferenced(t *testing.T) {
	agentSvc := mustNewAgentService(t)
	svc := NewService(NewMemoryStore(nil), WithAgentService(agentSvc), WithIMService(im.NewService()))
	if _, err := agentSvc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:          "u-qa",
			Name:        "QA Runtime",
			Role:        agent.RoleWorker,
			RuntimeKind: agent.RuntimeKindPicoClawSandbox,
			Image:       "agent-image:test",
		},
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	for _, req := range []CreateRequest{
		{
			ID:      "qa",
			Channel: ChannelCSGClaw,
			Type:    TypeAgent,
			Name:    "QA",
			ChannelUser: ChannelUserSpec{
				Ref:  "u-qa",
				Kind: ChannelUserKindLocalUserID,
			},
			AgentBinding: AgentBindingSpec{
				Mode:    BindingModeReuse,
				AgentID: "u-qa",
			},
		},
		{
			ID:            "test",
			Channel:       ChannelFeishu,
			Type:          TypeAgent,
			Name:          "QA Feishu",
			ChannelAppRef: "cli_xxx",
			ChannelUser: ChannelUserSpec{
				Ref:  "ou_xxx",
				Kind: ChannelUserKindOpenID,
			},
			AgentBinding: AgentBindingSpec{
				Mode:    BindingModeReuse,
				AgentID: "u-qa",
			},
		},
	} {
		if _, err := svc.Create(context.Background(), req); err != nil {
			t.Fatalf("Create(%s:%s) error = %v", req.Channel, req.ID, err)
		}
	}

	_, ok, err := svc.Delete(context.Background(), ChannelCSGClaw, "qa", DeleteOptions{DeleteAgent: DeleteAgentIfUnreferenced})
	if err == nil {
		t.Fatal("Delete(delete_agent=if_unreferenced) error = nil, want referenced-agent error")
	}
	if ok {
		t.Fatal("Delete(delete_agent=if_unreferenced) ok = true, want false when cleanup is rejected")
	}
	if _, exists := svc.Get(ChannelCSGClaw, "qa"); !exists {
		t.Fatal("participant csgclaw:qa was deleted despite referenced-agent rejection")
	}
	if _, exists := agentSvc.Agent("u-qa"); !exists {
		t.Fatal("agent u-qa was deleted despite referenced-agent rejection")
	}
}

func TestDeleteParticipantAgentCleanupKeepsSharedCSGClawUser(t *testing.T) {
	agentSvc := mustNewAgentService(t)
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-qa", Name: "QA", Handle: "qa"},
		},
	})
	svc := NewService(NewMemoryStore(nil), WithAgentService(agentSvc), WithIMService(imSvc))
	if _, err := agentSvc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:          "u-qa",
			Name:        "QA Runtime",
			Role:        agent.RoleWorker,
			RuntimeKind: agent.RuntimeKindPicoClawSandbox,
			Image:       "agent-image:test",
		},
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	for _, req := range []CreateRequest{
		{
			ID:      "qa",
			Channel: ChannelCSGClaw,
			Type:    TypeAgent,
			Name:    "QA",
			ChannelUser: ChannelUserSpec{
				Ref:  "u-qa",
				Kind: ChannelUserKindLocalUserID,
			},
			AgentBinding: AgentBindingSpec{
				Mode:    BindingModeReuse,
				AgentID: "u-qa",
			},
		},
		{
			ID:      "qa-human-ref",
			Channel: ChannelCSGClaw,
			Type:    TypeHuman,
			Name:    "QA Human Ref",
			ChannelUser: ChannelUserSpec{
				Ref:  "u-qa",
				Kind: ChannelUserKindLocalUserID,
			},
		},
	} {
		if _, err := svc.Create(context.Background(), req); err != nil {
			t.Fatalf("Create(%s) error = %v", req.ID, err)
		}
	}

	if _, ok, err := svc.Delete(context.Background(), ChannelCSGClaw, "qa", DeleteOptions{DeleteAgent: DeleteAgentIfUnreferenced}); err != nil || !ok {
		t.Fatalf("Delete() ok=%v error=%v, want ok", ok, err)
	}
	if _, exists := agentSvc.Agent("u-qa"); exists {
		t.Fatal("agent u-qa still exists after cleanup")
	}
	if _, exists := imSvc.User("u-qa"); !exists {
		t.Fatal("shared user u-qa was deleted despite another participant reference")
	}
}

func TestCreateHumanParticipantRejectsCreateAgentBinding(t *testing.T) {
	svc := NewService(NewMemoryStore(nil))

	_, err := svc.Create(context.Background(), CreateRequest{
		ID:      "alice",
		Channel: ChannelCSGClaw,
		Type:    TypeHuman,
		Name:    "Alice",
		AgentBinding: AgentBindingSpec{
			Mode: BindingModeCreate,
		},
	})
	if err == nil {
		t.Fatal("Create() error = nil, want validation error")
	}
}

func mustNewAgentService(t *testing.T) *agent.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc, err := agent.NewService(
		config.ModelConfig{
			Provider: config.ProviderLLMAPI,
			BaseURL:  "http://127.0.0.1:4000",
			APIKey:   "sk-test",
			ModelID:  "model-1",
		},
		config.ServerConfig{},
		"manager-image:test",
		"",
		agent.WithRuntime(testRuntime{kind: agent.RuntimeKindPicoClawSandbox}),
	)
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	return svc
}

func mustNewManagerAgentService(t *testing.T) *agent.Service {
	t.Helper()
	svc := mustNewAgentService(t)
	agent.SetTestHooks(
		func(_ *agent.Service, _ string) (sandbox.Runtime, error) {
			return sandboxtest.NewRuntime(), nil
		},
		func(_ *agent.Service, _ context.Context, _ sandbox.Runtime, _, name, _ string, _ agent.AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			info := sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 6, 4, 14, 0, 7, 0, time.UTC),
			}
			return sandboxtest.NewInstance(info), info, nil
		},
	)
	t.Cleanup(agent.ResetTestHooks)
	return svc
}

type testRuntime struct {
	kind string
}

func (r testRuntime) Kind() string {
	return r.kind
}

func (testRuntime) WorkspaceRoot(agentHome string) string {
	return agentHome
}

func (testRuntime) New(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
	return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: spec.AgentID}, nil
}

func (testRuntime) Start(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateRunning, nil
}

func (testRuntime) Stop(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateStopped, nil
}

func (testRuntime) Delete(context.Context, agentruntime.Handle) error {
	return nil
}

func (testRuntime) State(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateRunning, nil
}

func (testRuntime) Info(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
	return agentruntime.Info{State: agentruntime.StateRunning, CreatedAt: time.Now().UTC()}, nil
}
