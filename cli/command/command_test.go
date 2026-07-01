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
	if !strings.Contains(out, "MODEL") || strings.Contains(out, "PROFILE") {
		t.Fatalf("RenderAgentsTable() = %q, want MODEL header without PROFILE", out)
	}
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

func TestRenderParticipantsTableAlignsUTF8Cells(t *testing.T) {
	var buf bytes.Buffer

	err := RenderParticipantsTable(&buf, []apitypes.Participant{
		{
			ID:              "pt-admin",
			Name:            "admin",
			Type:            "human",
			Channel:         "csgclaw",
			UserID:          "user-admin",
			UserName:        "admin",
			LifecycleStatus: "active",
		},
		{
			ID:              "pt-manager",
			Name:            "强人",
			Type:            "agent",
			Channel:         "csgclaw",
			AgentID:         "agent-manager",
			AgentName:       "强人",
			UserID:          "user-manager",
			UserName:        "强人",
			LifecycleStatus: "active",
		},
	})
	if err != nil {
		t.Fatalf("RenderParticipantsTable() error = %v", err)
	}

	lines := nonEmptyLines(buf.String())
	if len(lines) != 3 {
		t.Fatalf("RenderParticipantsTable() lines = %q, want header plus 2 rows", lines)
	}
	adminChannel := displayColumn(lines[1], "csgclaw")
	managerChannel := displayColumn(lines[2], "csgclaw")
	if adminChannel != managerChannel {
		t.Fatalf("CHANNEL display columns = admin %d manager %d in %q", adminChannel, managerChannel, buf.String())
	}
}

func TestRenderAgentsTablePreservesWideFields(t *testing.T) {
	var buf bytes.Buffer
	longImage := "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.6.23"

	err := RenderAgentsTable(&buf, []apitypes.Agent{{
		ID:               "agent-yupvpb",
		Name:             "测试工程师",
		Role:             "worker",
		Status:           "running",
		RuntimeKind:      "picoclaw_sandbox",
		Profile:          "codex.gpt-5.5",
		ParticipantIDs:   []string{"pt-yupvpb", "pt-yupvpb-5905c292"},
		ParticipantNames: []string{"测试工程师", "测试工程师"},
		Image:            longImage,
	}})
	if err != nil {
		t.Fatalf("RenderAgentsTable() error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, longImage) {
		t.Fatalf("RenderAgentsTable() = %q, want full image value", out)
	}
	lines := nonEmptyLines(out)
	if len(lines) != 2 {
		t.Fatalf("RenderAgentsTable() lines = %q, want header plus row", lines)
	}
	if strings.Contains(lines[1], "...") {
		t.Fatalf("RenderAgentsTable() row = %q, want no truncation marker", lines[1])
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
		LeadAgentID:   "agent-manager",
		LeadAgentName: "manager",
		MemberAgentIDs: []string{
			"agent-dev",
		},
		Status: "active",
		Title:  "dev",
	}}); err != nil {
		t.Fatalf("RenderTeamsTable() error = %v", err)
	}
	if out := teams.String(); !strings.Contains(out, "manager(agent-manager)") || !strings.Contains(out, "agent-dev") {
		t.Fatalf("RenderTeamsTable() = %q, want lead and member display values", out)
	}
}

func nonEmptyLines(out string) []string {
	raw := strings.Split(strings.TrimSpace(out), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func displayColumn(line, marker string) int {
	idx := strings.Index(line, marker)
	if idx < 0 {
		return -1
	}
	return displayWidthForTest(line[:idx])
}

func displayWidthForTest(value string) int {
	width := 0
	for _, r := range value {
		switch {
		case r >= 0x1100 && r <= 0x115f:
			width += 2
		case r >= 0x2e80 && r <= 0xa4cf:
			width += 2
		case r >= 0xac00 && r <= 0xd7a3:
			width += 2
		case r >= 0xf900 && r <= 0xfaff:
			width += 2
		case r >= 0xff01 && r <= 0xff60:
			width += 2
		case r >= 0xffe0 && r <= 0xffe6:
			width += 2
		default:
			width++
		}
	}
	return width
}
