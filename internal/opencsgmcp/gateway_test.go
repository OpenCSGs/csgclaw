package opencsgmcp

import "testing"

func TestRuntimeServersRewritesGatewayToTrustedProxy(t *testing.T) {
	servers := map[string]any{
		"file-parser": map[string]any{
			"url":       "https://attacker.example/mcp",
			"transport": "streamable-http",
			"headers": map[string]any{
				"Authorization": "Bearer publisher-token",
				"X-Trace":       "keep",
			},
			ManagedMetaKey: map[string]any{
				ManagedMetaNamespace: map[string]any{
					"type":      GatewayMCPType,
					"auth_type": CSGHubAuthType,
				},
			},
		},
	}

	got, err := RuntimeServers(servers, "http://host.docker.internal:18080/", "internal-token")
	if err != nil {
		t.Fatal(err)
	}
	entry := got["file-parser"].(map[string]any)
	if want := "http://host.docker.internal:18080/api/v1/opencsg-mcp-gateway/mcp"; entry["url"] != want {
		t.Fatalf("url = %#v, want %q", entry["url"], want)
	}
	headers := entry["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer internal-token" || headers["X-Trace"] != "keep" {
		t.Fatalf("headers = %#v", headers)
	}
	if _, exists := entry[ManagedMetaKey]; exists {
		t.Fatalf("runtime config leaked managed metadata: %#v", entry)
	}

	original := servers["file-parser"].(map[string]any)
	if original["url"] != "https://attacker.example/mcp" {
		t.Fatalf("source config was mutated: %#v", original)
	}
}

func TestIsGatewayServerRequiresTypeAndAuth(t *testing.T) {
	config := map[string]any{
		ManagedMetaKey: map[string]any{
			ManagedMetaNamespace: map[string]any{
				"type":      GatewayMCPType,
				"auth_type": CSGHubAuthType,
			},
		},
	}
	if !IsGatewayServer(config) {
		t.Fatal("expected managed Gateway config")
	}
	config[ManagedMetaKey].(map[string]any)[ManagedMetaNamespace].(map[string]any)["auth_type"] = "other"
	if IsGatewayServer(config) {
		t.Fatal("unexpected match for unsupported auth type")
	}
}
