package picoclawsandbox

import (
	"strings"
	"testing"
)

func TestGatewayRunCommandWritesGatewayOutputToSingleLog(t *testing.T) {
	cmd := GatewayRunCommand()

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
}
