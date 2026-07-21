package agent

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

func newConnectorCapabilityKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

func (s *Service) connectorCapability(agentID string) string {
	if s == nil || len(s.connectorCapabilityKey) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, s.connectorCapabilityKey)
	_, _ = mac.Write([]byte("connector-credential\x00" + canonicalAgentID(agentID)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) AuthorizesConnectorCapability(agentID, capability string) bool {
	capability = strings.TrimSpace(capability)
	if canonicalAgentID(agentID) != ManagerUserID || capability == "" {
		return false
	}
	expected := s.connectorCapability(agentID)
	return subtle.ConstantTimeCompare([]byte(capability), []byte(expected)) == 1
}
