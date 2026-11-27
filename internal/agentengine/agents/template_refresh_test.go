package agents

import "testing"

func TestTemplateMCPAuthenticationOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, from, to string
		keep           bool
	}{
		{"path and query", "https://tools.example/one", "https://tools.example/two?v=3", true},
		{"default HTTPS port", "https://TOOLS.example:443", "https://tools.example/mcp", true},
		{"default HTTP port", "http://tools.example", "http://tools.example:80/mcp", true},
		{"IPv6", "https://[::1]/one", "https://[::1]:443/two", true},
		{"host changed", "https://tools.example/mcp", "https://other.example/mcp", false},
		{"port changed", "https://tools.example", "https://tools.example:8443", false},
		{"scheme changed", "https://tools.example:443", "http://tools.example:443", false},
		{"unknown old origin", "", "https://tools.example/mcp", false},
		{"invalid new endpoint", "https://tools.example", "/relative", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := map[string]any{"url": tc.from, "oauth_client_id": "original-client", "oauth_resource": "original-resource"}
			stored := map[string]any{"tools": old}
			merged := mergeTemplateMCPServers(stored, map[string]any{"tools": map[string]any{"url": tc.to}})
			server := merged["tools"].(map[string]any)
			for _, key := range []string{"oauth_client_id", "oauth_resource"} {
				if _, ok := server[key]; ok != tc.keep {
					t.Errorf("%s retained = %v, want %v", key, ok, tc.keep)
				}
				if _, ok := old[key]; !ok {
					t.Errorf("merge modified the original %s", key)
				}
			}
		})
	}
}
