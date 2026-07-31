package runtime

import "testing"

func TestMCPServersNeedsRestartNormalizesBeforeComparing(t *testing.T) {
	restart, err := MCPServersNeedsRestart(
		map[string]any{"server": map[string]any{"command": " npx "}},
		map[string]any{"server": map[string]any{"command": "npx"}},
	)
	if err != nil {
		t.Fatalf("MCPServersNeedsRestart() error = %v", err)
	}
	if restart {
		t.Fatal("MCPServersNeedsRestart() = true, want false for equivalent normalized configuration")
	}
}
