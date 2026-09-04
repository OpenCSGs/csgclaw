package codex

import (
	runtimeinstructions "csgclaw/internal/runtime/instructions"
	"strings"
	"testing"
)

func TestRenderRuntimeAgentsInstructionsBlockAddsFeishuLarkCLIWhenEnabled(t *testing.T) {
	plain := runtimeinstructions.RenderRuntimeAgentsInstructionsBlock("agent-worker", "Stay concise.")
	if strings.Contains(plain, "Feishu lark-cli Access") {
		t.Fatalf("plain worker instructions unexpectedly include lark-cli guidance: %q", plain)
	}

	got := runtimeinstructions.RenderRuntimeAgentsInstructionsBlockWithOptions("agent-worker", "Stay concise.", runtimeinstructions.RuntimeManagedInstructionsOptions{
		Extensions: []string{feishuLarkCLIManagedInstructions},
	})
	for _, want := range []string{
		"Feishu lark-cli Access",
		"`LARK_CHANNEL_CONFIG`",
		"`LARKSUITE_CLI_CONFIG_DIR`",
		"Run every lark-cli command directly through `command_execution`",
		"Do not invoke lark-cli through `mcp_tool_call`",
		"Node.js or Python subprocesses",
		"Treat a `not_configured` result from any non-`command_execution` environment as invalid",
		"lark-cli docs +fetch --api-version v2",
		"Feishu Historical Attachment Recovery",
		"lark-cli im +chat-messages-list --as bot --chat-id <current_chat_id>",
		"Do not search, list, or inspect other chats",
		"Do not use `--download-resources` during discovery",
		"lark-cli im +messages-resources-download --as bot --message-id <message_id>",
		"must come from the same message",
		"do not silently retry as a user",
		"lark-cli auth login --no-wait --json --recommend",
		"Do not background the device-code wait",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("lark-cli managed instructions missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "GitHub Connector Access") {
		t.Fatalf("worker lark-cli instructions should not include manager connector guidance: %q", got)
	}
	for _, unwanted := range []string{
		".csgclaw/attachments",
		"csgclaw-cli message list",
		"/api/v1/attachments/",
	} {
		if strings.Contains(feishuLarkCLIManagedInstructions, unwanted) {
			t.Fatalf("Feishu lark-cli instructions include CSGClaw attachment mechanism %q", unwanted)
		}
	}
}
