package apps

import (
	"slices"
	"testing"
)

func TestCatalogInstallationDefaults(t *testing.T) {
	service := newTestService(t, Options{})
	for _, tc := range []struct{ app, url, auth string }{
		{"gitlab", "https://u-wanghj-gitlab-mcp-161.public.opencsg-stg.com/mcp", "bearer"},
		{"feishu", "https://u-ryandraco-lark-mcp-passthrough-15s.public.opencsg-stg.com/mcp", "feishu"},
		{"llm-wiki", "http://127.0.0.1:19093/mcp", "bearer"},
	} {
		definition, err := service.Definition(tc.app)
		if err != nil {
			t.Fatal(err)
		}
		properties := definition.ConfigSchema["properties"].(map[string]any)
		if properties["url"].(map[string]any)["default"] != tc.url || properties["auth_mode"].(map[string]any)["default"] != tc.auth {
			t.Fatalf("missing connection defaults for %s", tc.app)
		}
		if slices.Contains(definition.AuthMethods, "feishu") != (tc.app == "feishu") {
			t.Fatal("Feishu auth advertised for another service")
		}
		for _, key := range []string{"token", "app_id", "app_secret"} {
			if field, ok := properties[key].(map[string]any); ok && field["default"] != nil {
				t.Fatal("secret in catalog defaults")
			}
		}
	}
}
