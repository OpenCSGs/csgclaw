package apps

import (
	"slices"
	"testing"
)

func TestCatalogInstallationDefaults(t *testing.T) {
	service := newTestService(t, Options{})
	for _, tc := range []struct{ app, url, auth string }{
		{"gitlab", "https://u-agentichub-gitlab-mcp-1qq.public.opencsg.com/mcp", "bearer"},
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
		if definition.Kind != KindMCP || slices.Contains(definition.AuthMethods, "feishu") {
			t.Fatalf("unexpected MCP definition: %+v", definition)
		}
		for _, key := range []string{"token", "app_id", "app_secret"} {
			if field, ok := properties[key].(map[string]any); ok && field["default"] != nil {
				t.Fatal("secret in catalog defaults")
			}
		}
	}
}

func TestFeishuCatalogUsesCLI(t *testing.T) {
	s := newTestService(t, Options{})
	pkg := s.packages["feishu"]
	pkg.Manifest.Description = "Custom Feishu description"
	pkg.Manifest.Interface.DisplayName = "Custom Feishu name"
	pkg.Manifest.Version = "manifest-version"
	s.packages["feishu"] = pkg
	d, err := s.Definition("feishu")
	if err != nil || d.Kind != KindCLI || d.Version != pkg.Manifest.Version {
		t.Fatalf("definition=%+v error=%v", d, err)
	}
	if d.Name != pkg.Manifest.Interface.DisplayName || d.Description != pkg.Manifest.Description {
		t.Fatalf("catalog does not use manifest display metadata: %+v", d)
	}
	if !d.OAuthSupported || !slices.Equal(d.AuthMethods, []string{"command"}) {
		t.Fatalf("unexpected CLI authorization: %+v", d)
	}
	if len(d.ConfigSchema["properties"].(map[string]any)) != 0 {
		t.Fatal("CLI setup must use official authorization")
	}
}
