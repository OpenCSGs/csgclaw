package participant

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/assets"
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
		Name:    "QA-Display-Name",
		ChannelUser: ChannelUserSpec{
			Ref:  "u-qa",
			Kind: ChannelUserKindLocalUserID,
		},
		AgentBinding: AgentBindingSpec{
			Mode: BindingModeCreate,
			Agent: &agent.CreateAgentSpec{
				Name:        "QA-Display-Name",
				Role:        agent.RoleWorker,
				RuntimeKind: agent.RuntimeKindPicoClawSandbox,
				Image:       "agent-image:test",
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if created.ID != "pt-qa" {
		t.Fatalf("participant ID = %q, want pt-qa", created.ID)
	}
	if created.AgentID != "agent-qa" {
		t.Fatalf("agent ID = %q, want agent-qa", created.AgentID)
	}
	if _, ok := agentSvc.Agent("u-qa"); !ok {
		t.Fatal("agent u-qa was not created")
	}
	if _, ok := agentSvc.Agent("u-qa-display-name"); ok {
		t.Fatal("agent ID was derived from editable display name")
	}
	if user, ok := imSvc.User("user-qa"); !ok || !strings.EqualFold(user.Name, "QA-Display-Name") {
		t.Fatalf("channel user = %+v, ok=%v; want user-qa display user", user, ok)
	}
}

func TestCreateParticipantKeepsAvatarOnIMUserOnly(t *testing.T) {
	agentSvc := mustNewAgentService(t)
	imSvc := im.NewService()
	store := NewMemoryStore(nil)
	svc := NewService(store, WithAgentService(agentSvc), WithIMService(imSvc))

	created, err := svc.Create(context.Background(), CreateRequest{
		ID:      "qa",
		Channel: ChannelCSGClaw,
		Type:    TypeAgent,
		Name:    "QA",
		ChannelUser: ChannelUserSpec{
			Ref:  "u-qa",
			Kind: ChannelUserKindLocalUserID,
		},
		AgentBinding: AgentBindingSpec{
			Mode: BindingModeCreate,
			Agent: &agent.CreateAgentSpec{
				Name:        "QA",
				Role:        agent.RoleWorker,
				RuntimeKind: agent.RuntimeKindPicoClawSandbox,
				Image:       "agent-image:test",
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if created.Avatar != "" {
		t.Fatalf("participant avatar = %q, want empty", created.Avatar)
	}
	if user, ok := imSvc.User("user-qa"); !ok || user.Avatar == "" {
		t.Fatalf("channel user = %+v, ok=%v; want user-owned avatar", user, ok)
	}
	if runtimeAgent, ok := agentSvc.Agent("u-qa"); !ok || runtimeAgent.Avatar != "" {
		t.Fatalf("agent = %+v, ok=%v; want empty avatar", runtimeAgent, ok)
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
			Name:        "QA-Runtime",
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

	if created.ID != "pt-test" || created.AgentID != "agent-qa" {
		t.Fatalf("created participant = %+v, want id pt-test bound to agent-qa", created)
	}
	if created.ChannelUserRef != "ou_xxx" || created.ChannelAppRef != "cli_xxx" {
		t.Fatalf("created participant channel identity = %+v, want Feishu app/open_id scope", created)
	}
}

func TestCreateCSGClawAgentParticipantRequiresAgentBinding(t *testing.T) {
	imSvc := im.NewService()
	store := NewMemoryStore(nil)
	svc := NewService(store, WithIMService(imSvc))

	_, err := svc.Create(context.Background(), CreateRequest{
		ID:      "gitlab-worker",
		Channel: ChannelCSGClaw,
		Type:    TypeAgent,
		Name:    "gitlab-worker",
	})
	if err == nil {
		t.Fatal("Create() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "agent_binding.mode") {
		t.Fatalf("Create() error = %v, want agent_binding.mode validation", err)
	}
	if got := store.List(ListOptions{Channel: ChannelCSGClaw}); len(got) != 0 {
		t.Fatalf("participants = %+v, want none after missing agent binding", got)
	}
	if _, ok := store.Get(ChannelCSGClaw, "pt-gitlab-worker"); ok {
		t.Fatal("participant was saved despite missing agent binding")
	}
	if _, ok := imSvc.User("user-gitlab-worker"); ok {
		t.Fatal("channel user was created despite missing agent binding")
	}
}

func TestRepairDanglingCSGClawAgentParticipantsRemovesParticipantShells(t *testing.T) {
	agentSvc := mustNewAgentService(t)
	if _, err := agentSvc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:          "agent-qa",
			Name:        "qa",
			Role:        agent.RoleWorker,
			RuntimeKind: agent.RuntimeKindPicoClawSandbox,
			Image:       "agent-image:test",
		},
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: im.AdminUserID,
		Users: []im.User{
			{ID: im.AdminUserID, Name: "admin", Role: "admin"},
			{ID: "user-gitlab-worker", Name: "gitlab-worker", Role: "worker"},
			{ID: "user-stale-worker", Name: "stale-worker", Role: "worker"},
			{ID: "user-qa", Name: "qa", Role: "worker"},
			{ID: im.ManagerUserID, Name: agent.ManagerName, Role: agent.RoleManager},
		},
	})
	store := NewMemoryStore([]Participant{
		{
			ID:              "pt-gitlab-worker",
			Channel:         ChannelCSGClaw,
			Type:            TypeAgent,
			Name:            "gitlab-worker",
			ChannelUserRef:  "user-gitlab-worker",
			ChannelUserKind: ChannelUserKindLocalUserID,
		},
		{
			ID:              "pt-stale-worker",
			Channel:         ChannelCSGClaw,
			Type:            TypeAgent,
			Name:            "stale-worker",
			ChannelUserRef:  "user-stale-worker",
			ChannelUserKind: ChannelUserKindLocalUserID,
			AgentID:         "agent-stale-worker",
		},
		{
			ID:              "pt-qa",
			Channel:         ChannelCSGClaw,
			Type:            TypeAgent,
			Name:            "qa",
			ChannelUserRef:  "user-qa",
			ChannelUserKind: ChannelUserKindLocalUserID,
			AgentID:         "agent-qa",
		},
		{
			ID:              agent.ManagerParticipantID,
			Channel:         ChannelCSGClaw,
			Type:            TypeAgent,
			Name:            agent.ManagerName,
			ChannelUserRef:  im.ManagerUserID,
			ChannelUserKind: ChannelUserKindLocalUserID,
		},
	})
	svc := NewService(store, WithAgentService(agentSvc), WithIMService(imSvc))

	deleted, err := svc.RepairDanglingCSGClawAgentParticipants()
	if err != nil {
		t.Fatalf("RepairDanglingCSGClawAgentParticipants() error = %v", err)
	}
	if len(deleted) != 2 {
		t.Fatalf("deleted = %+v, want two dangling worker shells", deleted)
	}
	for _, id := range []string{"pt-gitlab-worker", "pt-stale-worker"} {
		if _, ok := store.Get(ChannelCSGClaw, id); ok {
			t.Fatalf("participant %q still exists after repair", id)
		}
	}
	if _, ok := store.Get(ChannelCSGClaw, "pt-qa"); !ok {
		t.Fatal("valid worker participant was deleted")
	}
	if _, ok := store.Get(ChannelCSGClaw, agent.ManagerParticipantID); !ok {
		t.Fatal("manager participant was deleted")
	}
	for _, id := range []string{"user-gitlab-worker", "user-stale-worker"} {
		if _, ok := imSvc.User(id); ok {
			t.Fatalf("dangling user %q still exists after repair", id)
		}
	}
	if _, ok := imSvc.User("user-qa"); !ok {
		t.Fatal("valid worker user was deleted")
	}
	if _, ok := imSvc.User(im.ManagerUserID); !ok {
		t.Fatal("manager user was deleted")
	}
}

func TestCreateFeishuBotParticipantStoresChannelAppConfigWithoutOpenID(t *testing.T) {
	svc := NewService(NewMemoryStore(nil))

	created, err := svc.Create(context.Background(), CreateRequest{
		ID:      "dev",
		Channel: ChannelFeishu,
		Type:    TypeAgent,
		Name:    "Dev",
		ChannelUser: ChannelUserSpec{
			Kind: ChannelUserKindAppID,
		},
		ChannelAppConfig: map[string]any{
			"app_id":     "cli_dev",
			"app_secret": "dev-secret",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if created.ChannelUserRef != "" || created.ChannelUserKind != ChannelUserKindAppID {
		t.Fatalf("created channel user = %+v, want app_id identity without open_id", created)
	}
	if got, want := created.ChannelAppConfig["app_id"], "cli_dev"; got != want {
		t.Fatalf("channel_app_config.app_id = %#v, want %q", got, want)
	}
	if got, want := created.ChannelAppConfig["app_secret"], "dev-secret"; got != want {
		t.Fatalf("channel_app_config.app_secret = %#v, want %q", got, want)
	}
}

func TestUpdateFeishuBotParticipantChannelAppConfig(t *testing.T) {
	svc := NewService(NewMemoryStore(nil))
	if _, err := svc.Create(context.Background(), CreateRequest{
		ID:      "dev",
		Channel: ChannelFeishu,
		Type:    TypeAgent,
		Name:    "Dev",
		ChannelUser: ChannelUserSpec{
			Kind: ChannelUserKindAppID,
		},
		ChannelAppConfig: map[string]any{
			"app_id":     "cli_old",
			"app_secret": "old-secret",
		},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, ok, err := svc.Update(context.Background(), ChannelFeishu, "dev", UpdateRequest{
		ChannelAppConfig: map[string]any{
			"app_id":     "cli_new",
			"app_secret": "new-secret",
		},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !ok {
		t.Fatal("Update() ok = false, want true")
	}
	if got, want := updated.ChannelAppConfig["app_id"], "cli_new"; got != want {
		t.Fatalf("updated channel_app_config.app_id = %#v, want %q", got, want)
	}
	if got, want := updated.ChannelAppConfig["app_secret"], "new-secret"; got != want {
		t.Fatalf("updated channel_app_config.app_secret = %#v, want %q", got, want)
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

	if created.ID != bootstrapAdminParticipantID {
		t.Fatalf("participant ID = %q, want %q", created.ID, bootstrapAdminParticipantID)
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
	if created.Avatar != "" {
		t.Fatalf("admin participant avatar = %q, want empty", created.Avatar)
	}
	if _, ok := store.Get(ChannelCSGClaw, bootstrapAdminParticipantID); !ok {
		t.Fatal("store missing admin participant")
	}
	if user, ok := imSvc.User(im.AdminUserID); !ok || user.ID != im.AdminUserID || user.Name != "admin" || user.Role != "admin" || user.Avatar == "" {
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
		Name:            "Local-Admin",
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

	if created.ID != bootstrapAdminParticipantID || created.Type != TypeHuman {
		t.Fatalf("admin participant = %+v, want human participant %q", created, bootstrapAdminParticipantID)
	}
	if created.AgentID != "" {
		t.Fatalf("agent ID = %q, want empty after admin migration", created.AgentID)
	}
	if created.ChannelUserRef != im.AdminUserID {
		t.Fatalf("channel user ref = %q, want %q", created.ChannelUserRef, im.AdminUserID)
	}
	if !created.CreatedAt.Equal(createdAt) || created.Avatar != "" || created.Metadata["legacy"] != "kept" {
		t.Fatalf("admin participant did not preserve legacy fields: %+v", created)
	}
	if _, ok := store.Get(ChannelCSGClaw, "u-admin"); ok {
		t.Fatal("legacy admin participant u-admin was not deleted")
	}
}

func TestUpdateHumanParticipantAvatarIgnored(t *testing.T) {
	imSvc := im.NewService()
	store := NewMemoryStore(nil)
	svc := NewService(store, WithIMService(imSvc))
	if _, err := svc.EnsureBootstrapAdmin(context.Background()); err != nil {
		t.Fatalf("EnsureBootstrapAdmin() error = %v", err)
	}

	avatar := "avatar/cartoon-2.png"
	updated, ok, err := svc.Update(context.Background(), ChannelCSGClaw, im.AdminUserID, UpdateRequest{
		Avatar: &avatar,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !ok {
		t.Fatal("Update() ok = false, want true")
	}
	if updated.Avatar != "" {
		t.Fatalf("participant avatar = %q, want empty", updated.Avatar)
	}
	if user, ok := imSvc.User(im.AdminUserID); !ok || user.Avatar == avatar {
		t.Fatalf("admin channel user = %+v, ok=%v; want participant avatar update ignored", user, ok)
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
	if created.ChannelUserRef != im.ManagerUserID {
		t.Fatalf("channel user ref = %q, want %q", created.ChannelUserRef, im.ManagerUserID)
	}
	if created.Avatar != "" {
		t.Fatalf("manager participant avatar = %q, want empty", created.Avatar)
	}
	if user, ok := imSvc.User(im.ManagerUserID); !ok || user.ID != im.ManagerUserID || user.Avatar == "" {
		t.Fatalf("manager channel user = %+v, ok=%v; want local user %q", user, ok, im.ManagerUserID)
	}
	if manager, ok := agentSvc.Agent(agent.ManagerUserID); !ok || manager.Avatar != assets.DefaultManagerAvatar {
		t.Fatalf("manager agent = %+v, ok=%v; want default avatar %q", manager, ok, assets.DefaultManagerAvatar)
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
	if !created.CreatedAt.Equal(createdAt) || created.Avatar != "" || created.Metadata["legacy"] != "kept" {
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

	if created.ID != agent.ManagerParticipantID || created.ChannelUserRef != im.ManagerUserID || created.AgentID != agent.ManagerUserID {
		t.Fatalf("manager participant = %+v, want manager participant bound to %q", created, agent.ManagerUserID)
	}
	if !created.CreatedAt.Equal(createdAt) || created.Avatar != "" || created.Metadata["legacy"] != "kept" {
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
			Name:        "QA-Runtime",
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
			Name:        "QA-Runtime",
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
			{ID: "u-admin", Name: "admin"},
			{ID: "u-qa", Name: "QA"},
		},
	})
	svc := NewService(NewMemoryStore(nil), WithAgentService(agentSvc), WithIMService(imSvc))
	if _, err := agentSvc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:          "u-qa",
			Name:        "QA-Runtime",
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
			Name:    "QA-Human-Ref",
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

func (testRuntime) Layout(agentHome string) agentruntime.Layout {
	return agentruntime.Layout{
		WorkspaceRoot: agentHome,
		SkillsRoot:    filepath.Join(agentHome, "skills"),
	}
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
