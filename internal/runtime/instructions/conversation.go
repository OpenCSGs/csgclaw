package instructions

import (
	"csgclaw/internal/identity"
	"csgclaw/internal/roomtask"
	"strings"
)

// StandingInstructions combines platform identity with the conversation
// policies selected by the server's current room context.
func StandingInstructions(agentID, name, cliPath string) string {
	role := roomtask.TurnRoleWorker
	if identity.CanonicalAgentID(agentID) == identity.ManagerAgentID {
		role = roomtask.TurnRoleManager
	}
	return AgentIdentity(agentID, name) + "\n\n" + roomtask.ConversationInstructions(role, companionCLICommand(cliPath))
}

func companionCLICommand(path string) string {
	if path = strings.TrimSpace(path); path != "" {
		return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
	}
	return `"$CSGCLAW_CLI"`
}
