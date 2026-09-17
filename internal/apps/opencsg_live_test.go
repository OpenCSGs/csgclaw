package apps

import (
	"context"
	"os"
	"testing"
	"time"
)

// Opt-in live discovery uses the current login through the production resolver.
// No token is supplied in test arguments, source, or output; no business tool runs.
func TestOpenCSGLoginLiveMCPDiscovery(t *testing.T) {
	endpoint := os.Getenv("CSGCLAW_TEST_OPENCSG_MCP_URL")
	if endpoint == "" {
		t.Skip("set CSGCLAW_TEST_OPENCSG_MCP_URL for live authenticated discovery")
	}
	service, err := NewService("", Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	for _, appID := range []string{"gitlab", "feishu", "llm-wiki"} {
		t.Run(appID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := service.Probe(ctx, "live-probe", ProbeRequest{AppID: appID, Config: Config{URL: endpoint, AuthMode: "none"}})
			if err != nil {
				t.Fatal(err)
			}
			if !result.Connected || len(result.Tools) == 0 {
				t.Fatal("live service returned no tools")
			}
			t.Logf("Automatic OpenCSG login connected successfully; %d tools discovered", len(result.Tools))
		})
	}
}
