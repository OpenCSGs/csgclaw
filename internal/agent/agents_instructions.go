package agent

import (
	"bytes"
	_ "embed"
	"strings"
	"text/template"
)

const (
	agentsInstructionsBlockStart = "<!-- BEGIN CSGCLAW-INSTRUCTIONS (auto-generated; do not edit) -->"
	agentsInstructionsBlockEnd   = "<!-- END CSGCLAW-INSTRUCTIONS -->"
)

//go:embed embed/agents_instructions.md.tmpl
var agentsInstructionsBlockTemplate string

var parsedAgentsInstructionsBlockTemplate = template.Must(
	template.New("agents_instructions_block").Parse(agentsInstructionsBlockTemplate),
)

func AgentsInstructionsBlockMarkers() (start string, end string) {
	return agentsInstructionsBlockStart, agentsInstructionsBlockEnd
}

func RenderAgentsInstructionsBlock(instructions string) string {
	return renderAgentsInstructionsBlock(instructions, "")
}

func RenderRuntimeAgentsInstructionsBlock(agentID, instructions string) string {
	managedInstructions := ""
	if strings.TrimSpace(agentID) == ManagerUserID {
		managedInstructions = strings.TrimSpace(managerRuntimeConnectorInstructions)
	}
	return renderAgentsInstructionsBlock(instructions, managedInstructions)
}

const managerRuntimeConnectorInstructions = `### GitHub Connector Access

- The Manager can request CSGClaw-managed connector credentials dynamically through the local CSGClaw API.
- For GitHub repository, pull request, issue, or review workflows, request a fresh lease with ` + "`POST $CSGCLAW_BASE_URL/api/v1/agents/agent-manager/connectors/github/credential`" + ` using ` + "`Authorization: Bearer $CSGCLAW_ACCESS_TOKEN`" + `.
- Use the returned ` + "`access_token`" + ` only in process memory for GitHub API or GitHub CLI calls.
- Never print, echo, log, write, persist, or include the token value in prompts, messages, UI text, state files, snapshots, or ` + "`AGENTS.md`" + ` edits.
- Do not rely on connector tokens from environment variables such as ` + "`GITHUB_TOKEN`" + `; connector credentials are intentionally fetched on demand so reconnects and refreshes work without restarting the Manager.
- Do not treat an empty result from an external Codex GitHub app connector as proof that the CSGClaw GitHub connector has no repository access.
- If the credential API returns ` + "`400`" + `, ` + "`401`" + `, or ` + "`403`" + `, tell the user to reconnect the CSGClaw GitHub OAuth connector or check connector access policy.

### Historical Attachment Recovery

- Treat files under ` + "`.csgclaw/attachments/`" + ` as runtime-local cache copies, not as the durable attachment index.
- When the user refers to a previously uploaded file that is absent from the current workspace, query CSGClaw message history before claiming the file is unavailable or asking the user to upload it again.
- Use the current ` + "`channel`" + ` and ` + "`room_id`" + ` from the hidden channel context with ` + "`csgclaw-cli message list --channel <current_channel> --room-id <target_room_id>`" + `.
- Filter the JSON locally to attachment-bearing messages and retain ` + "`id`" + `, ` + "`name`" + `, ` + "`media_type`" + `, ` + "`size_bytes`" + `, ` + "`sha256`" + `, ` + "`created_at`" + `, the originating message ID, and the originating message text.
- Use a structured pipeline that excludes capability-bearing download URLs, such as ` + "`csgclaw-cli message list --channel <current_channel> --room-id <target_room_id> | jq '[.[] as $message | ($message.attachments // [])[] | {id, name, kind, media_type, size_bytes, sha256, created_at, message_id: $message.id, message_text: $message.content}]'`" + `.
- Match candidates using the filename, the originating message text, and recency.
- If exactly one candidate matches, download it by stable attachment ID into ` + "`.csgclaw/retrieved/<attachment-id>-<safe-name>`" + ` with ` + "`GET $CSGCLAW_BASE_URL/api/v1/attachments/<attachment-id>`" + ` and ` + "`Authorization: Bearer $CSGCLAW_ACCESS_TOKEN`" + `.
- A safe download command is ` + "`curl -fsS -H \"Authorization: Bearer ${CSGCLAW_ACCESS_TOKEN:?}\" \"$CSGCLAW_BASE_URL/api/v1/attachments/<attachment-id>\" --output \".csgclaw/retrieved/<attachment-id>-<safe-name>\"`" + `.
- Use the stable attachment ID for authenticated downloads instead of copying a capability-bearing ` + "`download_url`" + ` into commands, logs, or responses.
- Verify the downloaded file against its ` + "`sha256`" + ` before reading it.
- If multiple candidates plausibly match, show the user a concise candidate list instead of guessing.
- If the current room has no match and the user clearly refers to an upload from another conversation, list rooms and inspect only the relevant candidate rooms.
- Do not search the web for a referenced upload, rely only on ` + "`find`" + ` in the current workspace, or request a re-upload until durable CSGClaw history has been checked.
- Never print, echo, or include ` + "`CSGCLAW_ACCESS_TOKEN`" + ` or a capability token in tool output, logs, prompts, or responses.`

func renderAgentsInstructionsBlock(instructions, managedInstructions string) string {
	instructions = strings.TrimSpace(instructions)
	managedInstructions = strings.TrimSpace(managedInstructions)
	data := struct {
		StartMarker            string
		EndMarker              string
		Instructions           string
		HasInstructions        bool
		ManagedInstructions    string
		HasManagedInstructions bool
	}{
		StartMarker:            agentsInstructionsBlockStart,
		EndMarker:              agentsInstructionsBlockEnd,
		Instructions:           instructions,
		HasInstructions:        instructions != "",
		ManagedInstructions:    managedInstructions,
		HasManagedInstructions: managedInstructions != "",
	}
	var b bytes.Buffer
	if err := parsedAgentsInstructionsBlockTemplate.Execute(&b, data); err != nil {
		panic("render agents instructions block: " + err.Error())
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}
