package agents

import (
	"testing"

	"csgclaw/internal/config"
)

func TestAgentAccessTokenIsScopedAndRevokedWithAgent(t *testing.T) {
	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{AccessToken: "admin-secret"}, "manager:test", "")
	if err != nil {
		t.Fatal(err)
	}
	svc.agents["agent-alice"] = Agent{ID: "agent-alice", Name: "alice"}
	svc.agents["agent-bob"] = Agent{ID: "agent-bob", Name: "bob"}
	token := svc.agentAccessToken("agent-alice")
	if !svc.AuthorizesAgentAccessToken("agent-alice", token) {
		t.Fatal("own token rejected")
	}
	if svc.AuthorizesAgentAccessToken("agent-bob", token) || svc.AuthorizesAgentAccessToken("agent-alice", "admin-secret") {
		t.Fatal("foreign credential accepted")
	}
	if got, ok := svc.AgentIDForAccessToken(token); !ok || got != "agent-alice" {
		t.Fatalf("identity = %q, %v", got, ok)
	}
	for _, invalid := range []string{"", "agent.invalid.value", token + "x"} {
		if _, ok := svc.AgentIDForAccessToken(invalid); ok {
			t.Fatal("malformed token accepted")
		}
	}
	delete(svc.agents, "agent-alice")
	if svc.AuthorizesAgentAccessToken("agent-alice", token) {
		t.Fatal("deleted Agent remains authorized")
	}
}
