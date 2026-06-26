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
	if manager.Type != TypeAgent || manager.AgentID != agent.ManagerUserID || manager.ChannelUserRef != "user-manager" {
		t.Fatalf("manager participant = %+v, want agent %q and channel user %q", manager, agent.ManagerUserID, "user-manager")
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
	worker, ok := store.Get(ChannelCSGClaw, "pt-alice")
	if !ok {
		t.Fatal("worker participant was not migrated to typed participant ID")
	}
	if worker.Type != TypeAgent || worker.AgentID != "agent-alice" || worker.ChannelUserRef != "user-alice" {
		t.Fatalf("worker participant = %+v, want participant pt-alice bound to channel user-alice and agent-alice", worker)
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

	worker, ok := store.Get(ChannelCSGClaw, "pt-alice")
	if !ok {
		t.Fatal("worker participant was not repaired to typed participant ID")
	}
	if worker.AgentID != "agent-alice" || worker.ChannelUserRef != "user-alice" {
		t.Fatalf("worker participant = %+v, want participant pt-alice bound to channel user-alice and agent-alice", worker)
	}
	if _, ok := store.Get(ChannelCSGClaw, "u-alice"); ok {
		t.Fatal("legacy prefixed participant u-alice still exists after repair")
	}

	reloaded, err := NewStore(participantsPath)
	if err != nil {
		t.Fatalf("reload NewStore() error = %v", err)
	}
	if _, ok := reloaded.Get(ChannelCSGClaw, "pt-alice"); !ok {
		t.Fatal("reloaded store missing repaired participant pt-alice")
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

	admin, ok := store.Get(ChannelCSGClaw, "pt-admin")
	if !ok {
		t.Fatal("admin participant was not repaired from legacy u-admin")
	}
	if admin.Type != TypeHuman {
		t.Fatalf("admin type = %q, want %q", admin.Type, TypeHuman)
	}
	if admin.AgentID != "" {
		t.Fatalf("admin agent_id = %q, want empty for human participant", admin.AgentID)
	}
	if admin.ChannelUserRef != "user-admin" || admin.ChannelUserKind != ChannelUserKindLocalUserID {
		t.Fatalf("admin channel identity = %+v, want local user user-admin", admin)
	}
	if !admin.Mentionable || admin.LifecycleStatus != LifecycleStatusActive {
		t.Fatalf("admin lifecycle fields = %+v, want active mentionable participant", admin)
	}
	if !admin.CreatedAt.Equal(createdAt) || !admin.UpdatedAt.Equal(createdAt.Add(time.Minute)) || admin.Avatar != "" || admin.Metadata["legacy"] != "kept" {
		t.Fatalf("admin preserved fields = %+v, want legacy fields preserved", admin)
	}
	if _, ok := store.Get(ChannelCSGClaw, "u-admin"); ok {
		t.Fatal("legacy admin participant u-admin still exists after repair")
	}

	reloaded, err := NewStore(participantsPath)
	if err != nil {
		t.Fatalf("reload NewStore() error = %v", err)
	}
	if _, ok := reloaded.Get(ChannelCSGClaw, "pt-admin"); !ok {
		t.Fatal("reloaded store missing repaired admin participant")
	}
	if _, ok := reloaded.Get(ChannelCSGClaw, "u-admin"); ok {
		t.Fatal("reloaded store still has legacy participant u-admin")
	}
}

func TestNewStoreReadsAndWritesRootParticipantsSection(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".csgclaw")
	statePath := filepath.Join(root, "state.json")
	writeJSONFile(t, statePath, map[string]any{
		"version":         1,
		"model_providers": map[string]any{"items": map[string]any{"openai": map[string]any{}}},
		"participants": map[string]any{
			"items": []apitypes.Participant{{
				ID:              "pt-manager",
				Channel:         ChannelCSGClaw,
				Type:            TypeAgent,
				Name:            "manager",
				ChannelUserRef:  "user-manager",
				ChannelUserKind: ChannelUserKindLocalUserID,
				AgentID:         "agent-manager",
				LifecycleStatus: LifecycleStatusActive,
				Mentionable:     true,
			}},
		},
	})

	store, err := NewStore(statePath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if got, ok := store.Get(ChannelCSGClaw, "pt-manager"); !ok || got.AgentID != "agent-manager" || got.ChannelUserRef != "user-manager" {
		t.Fatalf("root participant = %+v, ok=%v", got, ok)
	}
	if err := store.Save(apitypes.Participant{
		ID:              "pt-admin",
		Channel:         ChannelCSGClaw,
		Type:            TypeHuman,
		Name:            "Admin",
		ChannelUserRef:  "user-admin",
		ChannelUserKind: ChannelUserKindLocalUserID,
		LifecycleStatus: LifecycleStatusActive,
		Mentionable:     true,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var state map[string]any
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if _, ok := state["model_providers"]; !ok {
		t.Fatalf("Save removed model_providers section: %s", data)
	}
	participants := state["participants"].(map[string]any)
	items := participants["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("participants items len = %d, want 2: %s", len(items), data)
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
