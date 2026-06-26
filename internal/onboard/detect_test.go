package onboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/localstore"
	"csgclaw/internal/participant"
)

func TestDetectStateFreshHomeReportsIncompleteBootstrap(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	result, err := DetectState(DetectStateOptions{})
	if err != nil {
		t.Fatalf("DetectState() error = %v", err)
	}

	if result.ConfigExists {
		t.Fatal("ConfigExists = true, want false")
	}
	if result.ConfigComplete {
		t.Fatal("ConfigComplete = true, want false")
	}
	if result.IMBootstrapComplete {
		t.Fatal("IMBootstrapComplete = true, want false")
	}
	if result.ManagerAgentComplete {
		t.Fatal("ManagerAgentComplete = true, want false")
	}
	if result.AdminParticipantComplete {
		t.Fatal("AdminParticipantComplete = true, want false")
	}
	if result.ManagerParticipantComplete {
		t.Fatal("ManagerParticipantComplete = true, want false")
	}
	if result.Complete() {
		t.Fatal("Complete() = true, want false")
	}
}

func TestDetectStateCompleteBootstrapReportsComplete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath, err := config.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	if err := defaultConfig().Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	imStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		t.Fatalf("DefaultIMStatePath() error = %v", err)
	}
	if err := im.SaveBootstrap(imStatePath, im.Bootstrap{
		CurrentUserID: im.AdminUserID,
		Users: []im.User{
			{ID: im.AdminUserID, Name: "admin", Role: "admin"},
			{ID: im.ManagerUserID, Name: "manager", Role: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-bootstrap",
				Title:    "admin & manager",
				IsDirect: true,
				Members:  []string{participant.BootstrapAdminParticipantID, agent.ManagerParticipantID},
			},
		},
	}); err != nil {
		t.Fatalf("SaveBootstrap() error = %v", err)
	}

	if err := writeManagerAgentState(t); err != nil {
		t.Fatalf("writeManagerAgentState() error = %v", err)
	}
	if err := writeManagerBotState(t, apitypes.LegacyBot{
		ID:        agent.ManagerUserID,
		Name:      "manager",
		Role:      agent.RoleManager,
		Channel:   participant.ChannelCSGClaw,
		AgentID:   agent.ManagerUserID,
		UserID:    agent.ManagerUserID,
		Available: true,
		CreatedAt: time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("writeManagerBotState() error = %v", err)
	}
	if err := writeParticipantsState(t, []apitypes.Participant{{
		ID:              participant.BootstrapAdminParticipantID,
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeHuman,
		Name:            "admin",
		ChannelUserRef:  im.AdminUserID,
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
		CreatedAt:       time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatalf("writeParticipantsState() error = %v", err)
	}

	result, err := DetectState(DetectStateOptions{})
	if err != nil {
		t.Fatalf("DetectState() error = %v", err)
	}

	if !result.ConfigExists || !result.ConfigComplete || !result.IMBootstrapComplete || !result.ManagerAgentComplete || !result.AdminParticipantComplete || !result.ManagerParticipantComplete {
		t.Fatalf("DetectState() completeness = %+v, want all true", result)
	}
	if !result.Complete() {
		t.Fatal("Complete() = false, want true")
	}
	assertLegacyBotsMigrated(t)
}

func TestDetectStateFlagsMissingManagerBotWhenOtherBootstrapStateExists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath, err := config.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	if err := defaultConfig().Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	imStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		t.Fatalf("DefaultIMStatePath() error = %v", err)
	}
	if err := im.EnsureBootstrapState(imStatePath); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if err := writeManagerAgentState(t); err != nil {
		t.Fatalf("writeManagerAgentState() error = %v", err)
	}

	result, err := DetectState(DetectStateOptions{})
	if err != nil {
		t.Fatalf("DetectState() error = %v", err)
	}

	if !result.ConfigExists || !result.ConfigComplete || !result.IMBootstrapComplete || !result.ManagerAgentComplete {
		t.Fatalf("DetectState() = %+v, want config/im/agent complete", result)
	}
	if result.AdminParticipantComplete {
		t.Fatal("AdminParticipantComplete = true, want false")
	}
	if result.ManagerParticipantComplete {
		t.Fatal("ManagerParticipantComplete = true, want false")
	}
	if result.Complete() {
		t.Fatal("Complete() = true, want false")
	}
}

func TestDetectStateFlagsMissingAdminParticipantWhenManagerParticipantExists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath, err := config.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	if err := defaultConfig().Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	imStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		t.Fatalf("DefaultIMStatePath() error = %v", err)
	}
	if err := im.EnsureBootstrapState(imStatePath); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if err := writeManagerAgentState(t); err != nil {
		t.Fatalf("writeManagerAgentState() error = %v", err)
	}
	if err := writeParticipantsState(t, []apitypes.Participant{{
		ID:              agent.ManagerParticipantID,
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            "manager",
		ChannelUserRef:  im.ManagerUserID,
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         agent.ManagerUserID,
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
		CreatedAt:       time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatalf("writeParticipantsState() error = %v", err)
	}

	result, err := DetectState(DetectStateOptions{})
	if err != nil {
		t.Fatalf("DetectState() error = %v", err)
	}

	if result.AdminParticipantComplete {
		t.Fatal("AdminParticipantComplete = true for manager-only participant state, want false")
	}
	if !result.ManagerParticipantComplete {
		t.Fatal("ManagerParticipantComplete = false, want true for manager participant fixture")
	}
	if result.Complete() {
		t.Fatalf("Complete() = true for manager-only participant state: %+v", result)
	}
}

func writeManagerAgentState(t *testing.T) error {
	t.Helper()

	agentsPath, err := config.DefaultAgentsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(agentsPath), 0o755); err != nil {
		return err
	}

	state := map[string]any{
		"version": 1,
		"agents": map[string]any{
			"items": []map[string]any{
				{
					"id":           agent.ManagerUserID,
					"name":         agent.ManagerName,
					"role":         agent.RoleManager,
					"runtime_kind": agent.RuntimeKindPicoClawSandbox,
					"image":        "manager-image:test",
					"status":       "running",
					"created_at":   time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
				},
			},
		},
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(agentsPath, append(data, '\n'), 0o600)
}

func writeManagerBotState(t *testing.T, manager apitypes.LegacyBot) error {
	t.Helper()

	imStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(imStatePath), "bots.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(map[string]any{
		"bots": []apitypes.LegacyBot{manager},
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func writeParticipantsState(t *testing.T, participants []apitypes.Participant) error {
	t.Helper()

	path, err := config.DefaultStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return localstore.WriteSection(path, "participants", map[string]any{"items": participants})
}

func assertLegacyBotsMigrated(t *testing.T) {
	t.Helper()

	imStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		t.Fatalf("DefaultIMStatePath() error = %v", err)
	}
	botsPath := filepath.Join(filepath.Dir(imStatePath), "bots.json")
	if _, err := os.Stat(botsPath); !os.IsNotExist(err) {
		t.Fatalf("bots.json still exists after participant migration; stat err=%v", err)
	}

	participantsPath, err := config.DefaultStatePath()
	if err != nil {
		t.Fatalf("DefaultStatePath() error = %v", err)
	}
	store, err := participant.NewStore(participantsPath)
	if err != nil {
		t.Fatalf("participant.NewStore() error = %v", err)
	}
	got, ok := store.Get(participant.ChannelCSGClaw, agent.ManagerParticipantID)
	if !ok {
		t.Fatal("manager participant was not created from legacy bots.json")
	}
	if got.AgentID != agent.ManagerUserID || got.ChannelUserRef != im.ManagerUserID {
		t.Fatalf("manager participant = %+v, want agent %q and channel user %q", got, agent.ManagerUserID, im.ManagerUserID)
	}
	if _, ok := store.Get(participant.ChannelCSGClaw, agent.ManagerUserID); ok {
		t.Fatalf("manager participant was migrated under old agent id %q", agent.ManagerUserID)
	}
}
