package runtime

import (
	"math"
	"strings"
	"testing"
)

func TestNormalizeMCPServersAcceptsTimeoutFields(t *testing.T) {
	got, err := NormalizeMCPServers(map[string]any{
		"grafana": map[string]any{
			"command":             "uvx",
			"args":                []any{"mcp-grafana"},
			"startup_timeout_sec": float64(90),
			"tool_timeout_sec":    120,
		},
	})
	if err != nil {
		t.Fatalf("NormalizeMCPServers() error = %v", err)
	}
	grafana := got["grafana"].(map[string]any)
	if got, want := grafana["startup_timeout_sec"], float64(90); got != want {
		t.Fatalf("startup_timeout_sec = %#v, want %#v", got, want)
	}
	if got, want := grafana["tool_timeout_sec"], 120; got != want {
		t.Fatalf("tool_timeout_sec = %#v, want %#v", got, want)
	}
}

func TestNormalizeMCPServersRejectsInvalidTimeoutField(t *testing.T) {
	for _, value := range []any{30.5, float64(math.MaxInt64)} {
		_, err := NormalizeMCPServers(map[string]any{
			"grafana": map[string]any{
				"command":             "uvx",
				"startup_timeout_sec": value,
			},
		})
		if err == nil {
			t.Fatalf("NormalizeMCPServers(startup_timeout_sec=%#v) error = nil, want error", value)
		}
		if !strings.Contains(err.Error(), "startup_timeout_sec must be a positive integer") {
			t.Fatalf("NormalizeMCPServers(startup_timeout_sec=%#v) error = %v", value, err)
		}
	}
}
