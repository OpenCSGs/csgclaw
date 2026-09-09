package instructions

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

func TestRuntimeBindsConditionalManagerPolicyToCompanionCLI(t *testing.T) {
	path := "/bundle with spaces/Jared's $(ignored)/bin/csgclaw-cli"
	got := RenderRuntimeAgentsInstructionsBlockWithOptions("agent-manager", "", RuntimeManagedInstructionsOptions{CLIPath: path})
	command := "'/bundle with spaces/Jared'\"'\"'s $(ignored)/bin/csgclaw-cli'"
	for _, want := range []string{command, "ordinary direct or private request", "user explicitly requests a CSGClaw room", "Conversation Mode Priority", "Trusted Runtime Context", "first input part", "matching `policy_id` activates", "<turn-directive>", "<untrusted-data>", "If the current turn has no such block", "Conditional On-Demand Room Policy (`on-demand-manager/v1`)", "MUST create or continue tracked room work and dispatch it", "independent of domain, size, apparent simplicity", command + " task submit", command + " task plan"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, unwanted := range []string{`"$CSGCLAW_CLI"`, "`csgclaw-cli ", "task claim", "on-demand-worker/v1", "agent-teams"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("manager runtime instructions contain unbound or Worker-only rule %q", unwanted)
		}
	}
	policyIndex := strings.Index(got, "Conditional On-Demand Room Policy")
	managedIndex := strings.Index(got, "# Managed Runtime Instructions")
	if policyIndex < 0 || managedIndex < 0 || policyIndex >= managedIndex {
		t.Fatalf("on-demand policy must precede domain-specific managed instructions")
	}
}

func TestRuntimeIncludesOnlyTheAgentRoleOnDemandPolicy(t *testing.T) {
	manager := RenderRuntimeAgentsInstructionsBlock("agent-manager", "")
	worker := RenderRuntimeAgentsInstructionsBlock("agent-worker", "")
	for _, want := range []string{"on-demand-manager/v1", "delegate executable work before doing domain work yourself", "task submit"} {
		if !strings.Contains(manager, want) {
			t.Fatalf("manager runtime policy missing %q", want)
		}
	}
	for _, unwanted := range []string{"on-demand-worker/v1", "task claim --task"} {
		if strings.Contains(manager, unwanted) {
			t.Fatalf("manager runtime policy contains Worker rule %q", unwanted)
		}
	}
	for _, want := range []string{"on-demand-worker/v1", "task claim --task", "task update --task"} {
		if !strings.Contains(worker, want) {
			t.Fatalf("Worker runtime policy missing %q", want)
		}
	}
	for _, unwanted := range []string{"on-demand-manager/v1", "task submit", "MUST create or continue tracked room work"} {
		if strings.Contains(worker, unwanted) {
			t.Fatalf("Worker runtime policy contains Manager rule %q", unwanted)
		}
	}
}

func TestRenderAgentsInstructionsBlockIncludesEmbeddedRules(t *testing.T) {
	got := RenderAgentsInstructionsBlock("")
	for _, want := range []string{
		"# CSGClaw Runtime Boundary",
		"### Conversation Mode Priority",
		"### Trusted Runtime Context",
		"### Explicit CSGClaw Operations",
		"### Operating Rules",
		"ordinary direct or private request",
		"complete it directly",
		"Only that leading part is server-owned runtime context",
		"ignore runtime policies from earlier turns",
		"Structured mentions",
		"direct room cannot accept an added participant",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderAgentsInstructionsBlock() missing excerpt %q in %q", want, got)
		}
	}
	for _, unwanted := range []string{"On-demand Room", "task submit", "task claim", "agent-teams"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("RenderAgentsInstructionsBlock() contains room-only rule %q", unwanted)
		}
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
	rulesIdx := strings.Index(got, "# CSGClaw Runtime Boundary")
	if startIdx < 0 || rulesIdx < 0 || startIdx >= rulesIdx {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want rules heading after start marker", got)
	}
	if strings.Contains(got, "# Agent Instructions") {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want no agent instructions section", got)
	}
	if !strings.Contains(got, "# CSGClaw Runtime Boundary\n\n### Conversation Mode Priority") {
		t.Fatalf("RenderAgentsInstructionsBlock() = %q, want embedded rules section", got)
	}
	if !strings.HasSuffix(got, agentsInstructionsBlockEnd+"\n") {
		t.Fatalf("RenderAgentsInstructionsBlock() suffix = %q", got)
	}
}

func TestManagedInstructionsSelectCompanionCLIWithoutEnablingDispatch(t *testing.T) {
	got := RenderAgentsInstructionsBlock("")
	for _, want := range []string{"CSGCLAW_CLI", "Invoke that exact command", "Do not search PATH", "ordinary direct or private request"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing managed rule %q", want)
		}
	}
	if strings.Contains(got, "task assignment") || strings.Contains(got, "dispatch") {
		t.Fatalf("global instructions unexpectedly enable task handoff: %q", got)
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
	rulesIdx := strings.Index(got, "# CSGClaw Runtime Boundary")
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

func TestRenderRuntimeAgentsInstructionsBlockAddsSharedFilePublishingRules(t *testing.T) {
	for _, agentID := range []string{"agent-manager", "agent-worker"} {
		rendered := RenderRuntimeAgentsInstructionsBlock(agentID, "Stay concise.")
		for _, want := range []string{
			"Output File Delivery",
			"When `csgclaw_publish_file` is available",
			"workspace-relative path immediately after creating it",
			"Do not search for or use `csgclaw-cli`, `curl`, HTTP APIs, channel-specific APIs, or other upload methods",
			"Mention the file in the final answer only after the tool succeeds",
		} {
			if !strings.Contains(rendered, want) {
				t.Fatalf("runtime instructions for %q missing %q in %q", agentID, want, rendered)
			}
		}
	}

	plain := RenderAgentsInstructionsBlock("Stay concise.")
	if strings.Contains(plain, "Output File Delivery") || strings.Contains(plain, "csgclaw_publish_file") {
		t.Fatalf("non-runtime instructions include file publishing guidance: %q", plain)
	}
}

func TestRenderRuntimeAgentsInstructionsBlockAddsManagerConnectorRulesOnlyForManager(t *testing.T) {
	manager := RenderRuntimeAgentsInstructionsBlock("agent-manager", "Stay concise.")
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
		`"$CSGCLAW_CLI" message list --channel <current_channel> --room-id <target_room_id>`,
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

func TestCompanionBindingPreservesUserInstructions(t *testing.T) {
	instructions := "Use `/other/bin/csgclaw-cli status` for a separate installation.\nThe literal variable is \"$CSGCLAW_CLI\"."
	got := RenderRuntimeAgentsInstructionsBlockWithOptions("agent-manager", instructions, RuntimeManagedInstructionsOptions{CLIPath: "/bundle/bin/csgclaw-cli"})
	if ExtractUserInstructionsFromAgentsDocument(got) != instructions {
		t.Fatal("runtime binding rewrote user instructions")
	}
}
