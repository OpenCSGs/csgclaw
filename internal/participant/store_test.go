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
