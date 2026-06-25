package agent

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
)

const AgentIDPrefix = "agent-"

func newAgentID() (string, error) {
	var raw [10]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate agent id: %w", err)
	}
	encoded := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:]))
	return AgentIDPrefix + encoded, nil
}

func normalizeExplicitAgentID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", nil
	}
	if strings.ContainsAny(id, " \t\r\n/\\") {
		return "", fmt.Errorf("agent id must be path-safe: %s", id)
	}
	return canonicalAgentID(id), nil
}

func canonicalAgentID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	switch id {
	case ManagerName, "u-manager":
		return ManagerUserID
	}
	if strings.HasPrefix(id, AgentIDPrefix) {
		return id
	}
	if strings.HasPrefix(id, "u-") {
		suffix := strings.TrimPrefix(id, "u-")
		if suffix != "" {
			if strings.HasPrefix(suffix, AgentIDPrefix) {
				return suffix
			}
			return AgentIDPrefix + suffix
		}
	}
	return AgentIDPrefix + id
}

func CanonicalID(id string) string {
	return canonicalAgentID(id)
}

func agentIDAliases(id string) []string {
	typed := canonicalAgentID(id)
	if typed == "" {
		return nil
	}
	aliases := []string{typed}
	suffix := strings.TrimPrefix(typed, AgentIDPrefix)
	if suffix == "" || suffix == typed {
		return aliases
	}
	aliases = append(aliases, "u-"+suffix, suffix)
	if suffix == ManagerName {
		aliases = append(aliases, ManagerName)
	}
	return aliases
}
