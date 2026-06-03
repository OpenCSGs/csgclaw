package openclawsandbox

import (
	"strings"
	"testing"
)

func TestGatewayRunCommandWritesGatewayOutputToSingleLog(t *testing.T) {
	cmd := GatewayRunCommand()

	if !strings.Contains(cmd, "exec node /app/openclaw.mjs gateway ") {
		t.Fatalf("GatewayRunCommand() = %q, want Node to launch OpenClaw", cmd)
	}
	if !strings.Contains(cmd, "1>"+BoxGatewayLogPath) {
		t.Fatalf("GatewayRunCommand() = %q, want stdout written to gateway log", cmd)
	}
	if !strings.Contains(cmd, " 2>&1") {
		t.Fatalf("GatewayRunCommand() = %q, want stderr written to gateway log", cmd)
	}
	if strings.Contains(cmd, "gateway.err.pipe") || strings.Contains(cmd, "mkfifo") || strings.Contains(cmd, "tee ") {
		t.Fatalf("GatewayRunCommand() = %q, want direct logging without stderr pipe", cmd)
	}
	if strings.Contains(cmd, "mkdir -p ") {
		t.Fatalf("GatewayRunCommand() = %q, want directory creation handled during provisioning", cmd)
	}
	if strings.Contains(cmd, "gateway stop") || strings.Contains(cmd, "sleep ") {
		t.Fatalf("GatewayRunCommand() = %q, want first-start command without pre-stop delay", cmd)
	}
}
