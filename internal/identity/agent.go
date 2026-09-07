package identity

import "strings"

const AgentIDPrefix = "agent-"
const ManagerAgentID = "agent-manager"

func CanonicalAgentID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if id == "manager" || id == "u-manager" {
		return ManagerAgentID
	}
	if strings.HasPrefix(id, AgentIDPrefix) {
		return id
	}
	if suffix, ok := strings.CutPrefix(id, "u-"); ok && suffix != "" {
		if strings.HasPrefix(suffix, AgentIDPrefix) {
			return suffix
		}
		return AgentIDPrefix + suffix
	}
	return AgentIDPrefix + id
}
