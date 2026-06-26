package runtime

import "strings"

const SandboxNamePrefix = "csgclaw-"

func SandboxNameForAgentID(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	return SandboxNamePrefix + agentID
}
