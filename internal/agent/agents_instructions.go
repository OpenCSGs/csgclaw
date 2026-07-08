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
- If the credential API returns ` + "`400`" + `, ` + "`401`" + `, or ` + "`403`" + `, tell the user to reconnect the CSGClaw GitHub OAuth connector or check connector access policy.`

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
