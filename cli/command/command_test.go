package command

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/apitypes"
)

func TestRenderCompactBotListJSONOmitsOperationalFields(t *testing.T) {
	bots := []apitypes.Bot{{
		ID:          "bot-feishu",
		Name:        "feishu",
		Description: "manager bot",
		Role:        "manager",
		Channel:     "feishu",
		AgentID:     "u-manager",
		UserID:      "ou_manager",
		Available:   true,
		RuntimeKind: "codex",
		CreatedAt:   time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
	}}

	var stdout bytes.Buffer
	if err := RenderCompactBotList("json", &stdout, bots); err != nil {
		t.Fatalf("RenderCompactBotList() error = %v", err)
	}

	out := stdout.String()
	for _, want := range []string{`"id": "bot-feishu"`, `"description": "manager bot"`, `"role": "manager"`, `"channel": "feishu"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("compact JSON = %q, want %s", out, want)
		}
	}
	for _, unexpected := range []string{`"agent_id"`, `"user_id"`, `"available"`, `"runtime_kind"`, `"created_at"`} {
		if strings.Contains(out, unexpected) {
			t.Fatalf("compact JSON = %q, should omit %s", out, unexpected)
		}
	}
}

func TestRenderCompactBotListTableUsesCompactColumns(t *testing.T) {
	bots := []apitypes.Bot{{
		ID:          "bot-feishu",
		Name:        "feishu",
		Role:        "manager",
		Channel:     "feishu",
		AgentID:     "u-manager",
		UserID:      "ou_manager",
		Available:   true,
		RuntimeKind: "codex",
		CreatedAt:   time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
	}}

	var stdout bytes.Buffer
	if err := RenderCompactBotList("table", &stdout, bots); err != nil {
		t.Fatalf("RenderCompactBotList() error = %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "CHANNEL") || !strings.Contains(out, "bot-feishu") {
		t.Fatalf("compact table = %q, want compact bot columns", out)
	}
	for _, unexpected := range []string{"AGENT_ID", "USER_ID", "AVAILABLE", "RUNTIME_KIND", "CREATED_AT", "u-manager", "ou_manager", "codex"} {
		if strings.Contains(out, unexpected) {
			t.Fatalf("compact table = %q, should omit %s", out, unexpected)
		}
	}
}

func TestRenderFullBotListTableIncludesRuntime(t *testing.T) {
	bots := []apitypes.Bot{{
		ID:          "bot-alice",
		Name:        "alice",
		Role:        "worker",
		Channel:     "csgclaw",
		AgentID:     "u-alice",
		UserID:      "u-alice",
		Available:   true,
		RuntimeKind: "codex",
		CreatedAt:   time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
	}}

	var stdout bytes.Buffer
	if err := RenderFullBotList("table", &stdout, bots); err != nil {
		t.Fatalf("RenderFullBotList() error = %v", err)
	}

	out := stdout.String()
	for _, want := range []string{"AGENT_ID", "USER_ID", "AVAILABLE", "RUNTIME_KIND", "CREATED_AT", "bot-alice", "u-alice", "true", "codex", "2026-04-12T09:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Fatalf("full table = %q, want %s", out, want)
		}
	}
	if strings.Contains(out, "%!s(MISSING)") {
		t.Fatalf("full table = %q, contains missing fmt argument", out)
	}
}
