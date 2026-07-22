package agent

import (
	"strings"
	"testing"
)

func TestAgentsInstructionsBlockMarkers(t *testing.T) {
	start, end := AgentsInstructionsBlockMarkers()
	if start != agentsInstructionsBlockStart {
		t.Fatalf("AgentsInstructionsBlockMarkers() start = %q, want %q", start, agentsInstructionsBlockStart)
	}
	if end != agentsInstructionsBlockEnd {
		t.Fatalf("AgentsInstructionsBlockMarkers() end = %q, want %q", end, agentsInstructionsBlockEnd)
	}
}

func TestRenderAgentsInstructionsBlockIncludesEmbeddedRules(t *testing.T) {
	got := RenderAgentsInstructionsBlock("")
	for _, want := range []string{
		"### Scope",
		"### Workflow",
		"### Room And Participant Rules",
		"### Worker Notification Rules",
		"### Operating Rules",
		"`mention_only`",
		"`agent-teams`",
		"`manager`",
		"`u-manager`",
		"`participant list`",
		"`member list`",
		"`message list`",
		"`<at user_id=\"...\">`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderAgentsInstructionsBlock() missing excerpt %q in %q", want, got)
		}
	}
	if strings.Contains(got, "basics/SKILL.md") {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want embedded rules instead of template reference", got)
	}
	if strings.Contains(got, "```") {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want summarized rules instead of full command examples", got)
	}
}

func TestExtractUserInstructionsFromAgentsDocument(t *testing.T) {
	document := "# Template Base\n\nKeep this base.\n\n" + RenderAgentsInstructionsBlock("answer concisely")
	if got, want := ExtractUserInstructionsFromAgentsDocument(document), "answer concisely"; got != want {
		t.Fatalf("ExtractUserInstructionsFromAgentsDocument() = %q, want %q", got, want)
	}
	if got := ExtractUserInstructionsFromAgentsDocument("# Template Base\n"); got != "" {
		t.Fatalf("ExtractUserInstructionsFromAgentsDocument(without block) = %q, want empty", got)
	}
}

func TestAgentsInstructionsTemplateUsesMarkers(t *testing.T) {
	got := RenderAgentsInstructionsBlock("")
	if !strings.Contains(got, agentsInstructionsBlockStart) {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want start marker from Go constant", got)
	}
	if !strings.Contains(got, agentsInstructionsBlockEnd) {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want end marker from Go constant", got)
	}
}

func TestRenderAgentsInstructionsBlockWithoutInstructions(t *testing.T) {
	got := RenderAgentsInstructionsBlock("  ")

	startIdx := strings.Index(got, agentsInstructionsBlockStart)
	rulesIdx := strings.Index(got, "# CSGClaw Rules")
	if startIdx < 0 || rulesIdx < 0 || startIdx >= rulesIdx {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want rules heading after start marker", got)
	}
	if strings.Contains(got, "# Agent Instructions") {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want no agent instructions section", got)
	}
	if !strings.Contains(got, "# CSGClaw Rules\n\n### Scope") {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want embedded rules section", got)
	}
	if !strings.HasSuffix(got, agentsInstructionsBlockEnd+"\n") {
		t.Fatalf("RenderAgentsInstructionsBlock() suffix = %q", got)
	}
}

func TestRenderAgentsInstructionsBlockWithInstructions(t *testing.T) {
	instructions := "Use Chinese for teammate-facing docs.\nKeep code identifiers unchanged."
	got := RenderAgentsInstructionsBlock(instructions)

	agentInstructionsIdx := strings.Index(got, "# Agent Instructions")
	if agentInstructionsIdx < 0 {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want agent instructions section", got)
	}
	instructionsIdx := strings.Index(got, instructions)
	if instructionsIdx < 0 {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want instructions body", got)
	}
	rulesIdx := strings.Index(got, "# CSGClaw Rules")
	if rulesIdx < 0 {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want CSGClaw rules section", got)
	}
	if !(agentInstructionsIdx < instructionsIdx && instructionsIdx < rulesIdx) {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want instructions section before rules", got)
	}
	if strings.Count(got, agentsInstructionsBlockStart) != 1 {
		t.Fatalf("RenderAgentsInstructionsBlock() start marker count = %d, want 1", strings.Count(got, agentsInstructionsBlockStart))
	}
	if strings.Count(got, agentsInstructionsBlockEnd) != 1 {
		t.Fatalf("RenderAgentsInstructionsBlock() end marker count = %d, want 1", strings.Count(got, agentsInstructionsBlockEnd))
	}
}

func TestRenderRuntimeAgentsInstructionsBlockAddsManagerConnectorRulesOnlyForManager(t *testing.T) {
	manager := RenderRuntimeAgentsInstructionsBlock(ManagerUserID, "Stay concise.")
	if !strings.Contains(manager, "# Managed Runtime Instructions") {
		t.Fatalf("manager runtime instructions missing managed section: %q", manager)
	}
	for _, want := range []string{
		"GitHub Connector Access",
		"/api/v1/agents/agent-manager/connectors/github/credential",
		"GitLab Connector Access",
		"/api/v1/agents/agent-manager/connectors/gitlab/credential",
		"X-CSGClaw-Connector-Capability: $CSGCLAW_CONNECTOR_CAPABILITY",
		"`access_token`",
		"Do not rely on connector tokens from environment variables",
		"Do not treat an empty result from an external Codex GitHub app connector as proof",
		"reconnect the CSGClaw GitHub OAuth connector",
		"Historical Attachment Recovery",
		"csgclaw-cli message list --channel <current_channel> --room-id <target_room_id>",
		"jq '[.[] as $message | ($message.attachments // [])[]",
		"runtime-local cache copies, not as the durable attachment index",
		"GET $CSGCLAW_BASE_URL/api/v1/attachments/<attachment-id>",
		"curl -fsS -H \"Authorization: Bearer ${CSGCLAW_ACCESS_TOKEN:?}\"",
		"Use the stable attachment ID for authenticated downloads",
		"until durable CSGClaw history has been checked",
	} {
		if !strings.Contains(manager, want) {
			t.Fatalf("manager runtime instructions missing %q in %q", want, manager)
		}
	}
	if strings.Contains(manager, "skills/gitlab/SKILL.md") {
		t.Fatalf("manager runtime instructions hard-code optional GitLab skill path: %q", manager)
	}

	worker := RenderRuntimeAgentsInstructionsBlock("agent-worker", "Stay concise.")
	if strings.Contains(worker, "GitHub Connector Access") ||
		strings.Contains(worker, "GitLab Connector Access") ||
		strings.Contains(worker, "Historical Attachment Recovery") ||
		strings.Contains(worker, "`GITHUB_TOKEN`") {
		t.Fatalf("worker runtime instructions include manager connector guidance: %q", worker)
	}
}
