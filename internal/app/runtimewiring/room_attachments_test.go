package runtimewiring

import "testing"

func TestOpenClawRoomCLICallerIdentity(t *testing.T) {
	env := openClawBoxEnvVars("http://host:18080", "test-token", "pt-worker", "agent-worker", "http://host:18080/llm", "model", nil)
	if env["CSGCLAW_CALLER_AGENT_ID"] != "agent-worker" || env["CSGCLAW_BOT_ID"] != "pt-worker" {
		t.Fatalf("OpenClaw CLI caller identity missing")
	}
}
