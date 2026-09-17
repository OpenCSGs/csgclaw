package agents

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strings"
)

const (
	ConnectorCapabilityEnv    = "CSGCLAW_CONNECTOR_CAPABILITY"
	ConnectorCapabilityHeader = "X-CSGClaw-Connector-Capability"
)

// agentAccessToken is a scoped platform credential, never the administrator
// credential. The signature key belongs to this Controller's lifetime.
func (s *Controller) agentAccessToken(agentID string) string {
	agentID = canonicalAgentID(agentID)
	if s == nil || len(s.connectorCapabilityKey) == 0 || agentID == "" {
		return ""
	}
	subject := base64.RawURLEncoding.EncodeToString([]byte(agentID))
	mac := hmac.New(sha256.New, s.connectorCapabilityKey)
	_, _ = mac.Write([]byte("agent-platform\x00" + agentID))
	return "agent." + subject + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// AuthorizesAgentAccessToken checks both the signature and the still-existing
// Agent. A credential cannot authorize another Agent's MCP or REST endpoint.
func (s *Controller) AuthorizesAgentAccessToken(agentID, token string) bool {
	agentID = canonicalAgentID(agentID)
	if agentID == "" || s == nil {
		return false
	}
	expected := s.agentAccessToken(agentID)
	if expected == "" || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(expected)) != 1 {
		return false
	}
	_, ok := s.agentSnapshot(agentID)
	return ok
}

// AgentIDForAccessToken authenticates a runtime's platform credential.
func (s *Controller) AgentIDForAccessToken(token string) (string, bool) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 || parts[0] != "agent" {
		return "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(decoded) == 0 {
		return "", false
	}
	id := string(decoded)
	if !s.AuthorizesAgentAccessToken(id, token) {
		return "", false
	}
	return canonicalAgentID(id), true
}

func newConnectorCapabilityKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

func (s *Controller) connectorCapability(agentID string) string {
	if s == nil || len(s.connectorCapabilityKey) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, s.connectorCapabilityKey)
	_, _ = mac.Write([]byte("connector-credential\x00" + canonicalAgentID(agentID)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Controller) AuthorizesConnectorCapability(agentID, capability string) bool {
	capability = strings.TrimSpace(capability)
	if canonicalAgentID(agentID) != ManagerUserID || capability == "" {
		return false
	}
	expected := s.connectorCapability(agentID)
	return subtle.ConstantTimeCompare([]byte(capability), []byte(expected)) == 1
}
