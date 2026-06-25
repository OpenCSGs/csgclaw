package command

import (
	"bytes"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
)

func TestRenderAgentsTableShowsParticipantNamesAndIDs(t *testing.T) {
	var buf bytes.Buffer

	err := RenderAgentsTable(&buf, []apitypes.Agent{{
		ID:               "agent-dev",
		Name:             "dev",
		Role:             "worker",
		Status:           "running",
		RuntimeKind:      "picoclaw_sandbox",
		Profile:          "openai/gpt-4.1",
		ParticipantIDs:   []string{"pt-dev"},
		ParticipantNames: []string{"Dev Bot"},
	}})
	if err != nil {
		t.Fatalf("RenderAgentsTable() error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "PARTICIPANTS") || !strings.Contains(out, "Dev Bot(pt-dev)") {
		t.Fatalf("RenderAgentsTable() = %q, want participant display", out)
	}
}

func TestRenderParticipantsTableShowsRelatedNamesAndIDs(t *testing.T) {
	var buf bytes.Buffer

	err := RenderParticipantsTable(&buf, []apitypes.Participant{{
		ID:              "pt-dev",
		Name:            "Dev Bot",
		Type:            "agent",
		Channel:         "csgclaw",
		AgentID:         "agent-dev",
		AgentName:       "dev",
		UserID:          "user-dev",
		UserName:        "Dev User",
		LifecycleStatus: "active",
	}})
	if err != nil {
		t.Fatalf("RenderParticipantsTable() error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "dev(agent-dev)") || !strings.Contains(out, "Dev User(user-dev)") {
		t.Fatalf("RenderParticipantsTable() = %q, want related display names", out)
	}
}

func TestRenderRoomsAndTeamsTablesShowDisplayNamesWhenAvailable(t *testing.T) {
	var rooms bytes.Buffer
	if err := RenderRoomsTable(&rooms, []apitypes.Room{{
		ID:          "room-dev",
		Title:       "dev",
		Members:     []string{"pt-admin", "pt-dev"},
		MemberNames: []string{"admin", "Dev Bot"},
	}}); err != nil {
		t.Fatalf("RenderRoomsTable() error = %v", err)
	}
	if out := rooms.String(); !strings.Contains(out, "MEMBER_NAMES") || !strings.Contains(out, "admin,Dev Bot") {
		t.Fatalf("RenderRoomsTable() = %q, want member display names", out)
	}

	var teams bytes.Buffer
	if err := RenderTeamsTable(&teams, []apitypes.Team{{
		ID:            "team-dev",
		RoomID:        "room-dev",
		Channel:       "csgclaw",
		LeadAgentID:   "agent-manager",
		LeadAgentName: "manager",
		Status:        "active",
		Title:         "dev",
	}}); err != nil {
		t.Fatalf("RenderTeamsTable() error = %v", err)
	}
	if out := teams.String(); !strings.Contains(out, "manager(agent-manager)") {
		t.Fatalf("RenderTeamsTable() = %q, want lead display name", out)
	}
}
