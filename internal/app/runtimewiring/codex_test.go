package runtimewiring

import (
	"strings"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
)

func TestWithCodexRuntimeRejectsInvalidExecutionMode(t *testing.T) {
	t.Setenv("CSGCLAW_EXECUTION_MODE", "invalid")
	err := WithCodexRuntime()(&agent.Controller{})
	if err == nil || !strings.Contains(err.Error(), "CSGCLAW_EXECUTION_MODE") {
		t.Fatalf("WithCodexRuntime() error = %v, want execution mode error", err)
	}
}
