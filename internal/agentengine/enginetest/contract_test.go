package enginetest

import (
	"testing"

	"csgclaw/internal/agentengine"
)

func TestMemoryClientInterfaceContract(t *testing.T) {
	RunInterfaceContract(t, func(_ testing.TB, agents []agentengine.Agent, behavior TurnBehavior) agentengine.Interface {
		client := NewMemoryClient(agents...)
		client.SetTurnBehavior(behavior)
		return client
	})
}
