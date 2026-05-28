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

func TestNewDefaultsReadinessProbeToHealthEndpoint(t *testing.T) {
	rt := New(Dependencies{})
	if rt == nil {
		t.Fatal("New() = nil")
	}
	probe := rt.deps.ReadinessProbe
	if probe.Name != "wget" {
		t.Fatalf("ReadinessProbe.Name = %q, want wget", probe.Name)
	}
	if got := strings.Join(probe.Args, " "); !strings.Contains(got, "http://127.0.0.1:18790/health") {
		t.Fatalf("ReadinessProbe.Args = %q, want /health endpoint", got)
	}
	if got := strings.Join(probe.Args, " "); !strings.Contains(got, "--spider") {
		t.Fatalf("ReadinessProbe.Args = %q, want wget spider probe", got)
	}
	if got := strings.Join(probe.Args, " "); strings.Contains(got, "/ready") {
		t.Fatalf("ReadinessProbe.Args = %q, want docker health endpoint rather than /ready", got)
	}
}
