package team

import (
	"fmt"
	"strings"

	"csgclaw/internal/agent"
)

func cleanParticipantID(id string) string {
	id = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(id), "@"))
	return id
}

func requireCanonicalParticipantID(field, id string) (string, error) {
	id = cleanParticipantID(id)
	if id == "" {
		return "", nil
	}
	if strings.ContainsAny(id, " \t\r\n") {
		return "", invalidParticipantIDError(field, id)
	}
	if strings.HasPrefix(id, "u-") {
		return "", legacyUserIDAsParticipantIDError(field, id)
	}
	return id, nil
}

func requireAgentID(field, id string) (string, error) {
	id = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(id), "@"))
	if id == "" {
		return "", nil
	}
	if strings.ContainsAny(id, " \t\r\n") {
		return "", fmt.Errorf("%s must be a stable agent id without whitespace: %s", field, id)
	}
	return id, nil
}

func invalidParticipantIDError(field, id string) error {
	return fmt.Errorf("%s must be a stable participant id without whitespace: %s", field, id)
}

func legacyUserIDAsParticipantIDError(field, id string) error {
	return fmt.Errorf("%s must be a participant id, not CSGClaw user/agent id %q", field, id)
}

// ParticipantIDsMatch reports whether two participant ids refer to the same team participant.
func ParticipantIDsMatch(left, right string) bool {
	left = cleanParticipantID(left)
	right = cleanParticipantID(right)
	return left != "" && left == right
}

type agentParticipantResolver interface {
	ParticipantIDForAgentID(agentID string) string
}

func participantIDForAgentID(resolver any, agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	if r, ok := resolver.(agentParticipantResolver); ok {
		if participantID := strings.TrimSpace(r.ParticipantIDForAgentID(agentID)); participantID != "" {
			return participantID
		}
	}
	return defaultParticipantIDForAgentID(agentID)
}

func defaultParticipantIDForAgentID(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	if agentID == agent.ManagerUserID {
		return agent.ManagerParticipantID
	}
	if strings.HasPrefix(agentID, "u-") && len(agentID) > len("u-") {
		return strings.TrimPrefix(agentID, "u-")
	}
	return agentID
}
