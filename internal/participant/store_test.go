package participant

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
)

func TestNewStoreMigratesLegacyBotsAndDeletesSource(t *testing.T) {
	dir := t.TempDir()
	participantsPath := filepath.Join(dir, "participants.json")
	botsPath := filepath.Join(dir, "bots.json")

	createdAt := time.Date(2026, 6, 4, 14, 0, 7, 0, time.UTC)
	writeJSONFile(t, participantsPath, persistedState{Participants: []apitypes.Participant{
		{
			ID:              "dev",
			Channel:         ChannelCSGClaw,
			Type:            TypeAgent,
			Name:            "dev",
			ChannelUserRef:  "u-dev",
			ChannelUserKind: ChannelUserKindLocalUserID,
			AgentID:         "u-dev",
			LifecycleStatus: LifecycleStatusActive,
			Mentionable:     true,
			CreatedAt:       createdAt.Add(time.Minute),
			UpdatedAt:       createdAt.Add(time.Minute),
		},
	}})
	writeJSONFile(t, botsPath, legacyBotState{Bots: []apitypes.LegacyBot{
		{
			ID:        "u-manager",
			Name:      "manager",
			Type:      "normal",
			Role:      "manager",
			Channel:   ChannelCSGClaw,
			AgentID:   "u-manager",
			UserID:    "u-manager",
			Available: true,
			CreatedAt: createdAt,
		},
		{
			ID:      "n-alerts",
			Name:    "alerts",
			Type:    TypeNotification,
			Role:    "worker",
			Channel: ChannelCSGClaw,
			UserID:  "n-alerts",
			RuntimeOptions: map[string]any{
				"delivery_mode": "webhook",
				"webhook_token": "secret-token",
			},
			CreatedAt: createdAt.Add(2 * time.Minute),
		},
		{
			ID:        "u-alice",
			Name:      "alice",
			Type:      "normal",
			Role:      "worker",
			Channel:   ChannelCSGClaw,
			AgentID:   "u-alice",
			UserID:    "u-alice",
			Available: true,
			CreatedAt: createdAt.Add(3 * time.Minute),
		},
	}})

	store, err := NewStore(participantsPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	manager, ok := store.Get(ChannelCSGClaw, agent.ManagerParticipantID)
	if !ok {
		t.Fatal("manager participant was not migrated from legacy bots.json")
	}
	if manager.Type != TypeAgent || manager.AgentID != agent.ManagerUserID || manager.ChannelUserRef != agent.ManagerParticipantID {
		t.Fatalf("manager participant = %+v, want agent %q and channel user %q", manager, agent.ManagerUserID, agent.ManagerParticipantID)
	}
	if _, ok := store.Get(ChannelCSGClaw, "u-manager"); ok {
		t.Fatal("manager participant was migrated under agent ID u-manager")
	}
	if manager.ChannelUserKind != ChannelUserKindLocalUserID || !manager.Mentionable || manager.LifecycleStatus != LifecycleStatusActive {
		t.Fatalf("manager identity fields = %+v, want active mentionable local user", manager)
	}

	notify, ok := store.Get(ChannelCSGClaw, "n-alerts")
	if !ok {
		t.Fatal("notification participant was not migrated from legacy bots.json")
	}
	if notify.Type != TypeNotification || notify.ChannelUserRef != "n-alerts" {
		t.Fatalf("notification participant = %+v, want dedicated notification identity", notify)
	}
	if notify.Metadata["delivery_mode"] != "webhook" || notify.Metadata["webhook_token"] != "secret-token" {
		t.Fatalf("notification metadata = %#v, want legacy runtime_options preserved", notify.Metadata)
	}

	if _, err := os.Stat(botsPath); !os.IsNotExist(err) {
		t.Fatalf("bots.json still exists after successful migration; stat err=%v", err)
	}
	if _, ok := store.Get(ChannelCSGClaw, "dev"); !ok {
		t.Fatal("existing participant was not preserved during migration")
	}
	worker, ok := store.Get(ChannelCSGClaw, "alice")
	if !ok {
		t.Fatal("worker participant was not migrated to stripped participant ID")
	}
	if worker.Type != TypeAgent || worker.AgentID != "u-alice" || worker.ChannelUserRef != "u-alice" {
		t.Fatalf("worker participant = %+v, want participant alice bound to channel/agent u-alice", worker)
	}
	if _, ok := store.Get(ChannelCSGClaw, "u-alice"); ok {
		t.Fatal("worker participant was left under legacy agent ID u-alice")
	}
}

func TestNewStoreRepairsLegacyPrefixedAgentParticipants(t *testing.T) {
	dir := t.TempDir()
	participantsPath := filepath.Join(dir, "participants.json")
	createdAt := time.Date(2026, 6, 4, 14, 0, 7, 0, time.UTC)
	writeJSONFile(t, participantsPath, persistedState{Participants: []apitypes.Participant{
		{
			ID:              "u-alice",
			Channel:         ChannelCSGClaw,
			Type:            TypeAgent,
			Name:            "alice",
			ChannelUserRef:  "u-alice",
			ChannelUserKind: ChannelUserKindLocalUserID,
			AgentID:         "u-alice",
			LifecycleStatus: LifecycleStatusActive,
			Mentionable:     true,
			CreatedAt:       createdAt,
			UpdatedAt:       createdAt,
		},
	}})

	store, err := NewStore(participantsPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	worker, ok := store.Get(ChannelCSGClaw, "alice")
	if !ok {
		t.Fatal("worker participant was not repaired to stripped participant ID")
	}
	if worker.AgentID != "u-alice" || worker.ChannelUserRef != "u-alice" {
		t.Fatalf("worker participant = %+v, want participant alice bound to channel/agent u-alice", worker)
	}
	if _, ok := store.Get(ChannelCSGClaw, "u-alice"); ok {
		t.Fatal("legacy prefixed participant u-alice still exists after repair")
	}

	reloaded, err := NewStore(participantsPath)
	if err != nil {
		t.Fatalf("reload NewStore() error = %v", err)
	}
	if _, ok := reloaded.Get(ChannelCSGClaw, "alice"); !ok {
		t.Fatal("reloaded store missing repaired participant alice")
	}
	if _, ok := reloaded.Get(ChannelCSGClaw, "u-alice"); ok {
		t.Fatal("reloaded store still has legacy participant u-alice")
	}
}

func TestNewStoreRepairsLegacyAdminParticipant(t *testing.T) {
	dir := t.TempDir()
	participantsPath := filepath.Join(dir, "participants.json")
	createdAt := time.Date(2026, 6, 9, 11, 30, 0, 0, time.UTC)
	writeJSONFile(t, participantsPath, persistedState{Participants: []apitypes.Participant{
		{
			ID:              "u-admin",
			Channel:         ChannelCSGClaw,
			Type:            TypeAgent,
			Name:            "Admin",
			Avatar:          "avatar.png",
			ChannelUserRef:  "u-admin",
			ChannelUserKind: ChannelUserKindLocalUserID,
			AgentID:         "u-admin",
			LifecycleStatus: LifecycleStatusActive,
			Mentionable:     true,
			Metadata:        map[string]any{"legacy": "kept"},
			CreatedAt:       createdAt,
			UpdatedAt:       createdAt.Add(time.Minute),
		},
	}})

	store, err := NewStore(participantsPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	admin, ok := store.Get(ChannelCSGClaw, "admin")
	if !ok {
		t.Fatal("admin participant was not repaired from legacy u-admin")
	}
	if admin.Type != TypeHuman {
		t.Fatalf("admin type = %q, want %q", admin.Type, TypeHuman)
	}
	if admin.AgentID != "" {
		t.Fatalf("admin agent_id = %q, want empty for human participant", admin.AgentID)
	}
	if admin.ChannelUserRef != "admin" || admin.ChannelUserKind != ChannelUserKindLocalUserID {
		t.Fatalf("admin channel identity = %+v, want local user admin", admin)
	}
	if !admin.Mentionable || admin.LifecycleStatus != LifecycleStatusActive {
		t.Fatalf("admin lifecycle fields = %+v, want active mentionable participant", admin)
	}
	if !admin.CreatedAt.Equal(createdAt) || !admin.UpdatedAt.Equal(createdAt.Add(time.Minute)) || admin.Avatar != "avatar.png" || admin.Metadata["legacy"] != "kept" {
		t.Fatalf("admin preserved fields = %+v, want legacy fields preserved", admin)
	}
	if _, ok := store.Get(ChannelCSGClaw, "u-admin"); ok {
		t.Fatal("legacy admin participant u-admin still exists after repair")
	}

	reloaded, err := NewStore(participantsPath)
	if err != nil {
		t.Fatalf("reload NewStore() error = %v", err)
	}
	if _, ok := reloaded.Get(ChannelCSGClaw, "admin"); !ok {
		t.Fatal("reloaded store missing repaired admin participant")
	}
	if _, ok := reloaded.Get(ChannelCSGClaw, "u-admin"); ok {
		t.Fatal("reloaded store still has legacy participant u-admin")
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
