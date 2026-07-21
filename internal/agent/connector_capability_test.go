package agent

import (
	"testing"

	"csgclaw/internal/config"
)

func TestConnectorCapabilityIsBoundToManagerIdentity(t *testing.T) {
	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	managerCapability := svc.connectorCapability(ManagerUserID)
	if managerCapability == "" {
		t.Fatal("manager connector capability is empty")
	}
	if !svc.AuthorizesConnectorCapability(ManagerUserID, managerCapability) {
		t.Fatal("manager connector capability was rejected")
	}
	if svc.AuthorizesConnectorCapability("agent-worker", managerCapability) {
		t.Fatal("manager connector capability authorized a worker identity")
	}
	if svc.AuthorizesConnectorCapability(ManagerUserID, "wrong-capability") {
		t.Fatal("invalid manager connector capability was accepted")
	}
}
