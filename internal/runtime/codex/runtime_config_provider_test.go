package codex

import (
	"reflect"
	"testing"

	agentruntime "csgclaw/internal/runtime"

	toml "github.com/pelletier/go-toml/v2"
)

func TestConfigureCodexHomeConfigManagedProviderDropsInheritedProviders(t *testing.T) {
	const settings = `model_reasoning_effort = "high"
developer_instructions = "Keep the example [model_providers.dongfang] in this text."
`
	const tables = `
[projects."/workspace/support/打包资料"]
trust_level = "trusted"

[history]
persistence = "save-all"
`
	fixtures := map[string]string{
		"provider tables and nested settings": `
[model_providers.dongfang]
name = "Host provider"
base_url = "https://host.example/v1"
api_key = "test-only-invalid-host-key"
[model_providers.dongfang.http_headers]
"X-Test" = "host-header"
[model_providers.dongfang.auth]
command = "host-auth"
args = [
  "--test",
  "--provider=dongfang",
]
[model_providers.another]
name = "Another host provider"
env_key = "HOST_API_KEY"
`,
		"quoted table keys": `
[ "model_providers" . 'dongfang.custom' ] # custom host provider
name = "Host provider"
api_key = "test-only-invalid-host-key"
[ 'model_providers' . "dongfang.custom" . 'http_headers' ]
"X-Test" = "host-header"
[model_providers."proxy"]
name = "Stale proxy"
api_key = "test-only-invalid-proxy-key"
`,
		"provider root table with inline entries": `
[model_providers]
dongfang = { name = "Host provider", api_key = "test-only-invalid-host-key", http_headers = { "X-Test" = "host-header" } }
proxy = { name = "Stale proxy", api_key = "test-only-invalid-proxy-key" }
`,
		"root inline providers": `model_providers = { dongfang = { name = "Host provider", api_key = "test-only-invalid-host-key" }, proxy = { name = "Stale proxy", api_key = "test-only-invalid-proxy-key" } }
`,
		"root dotted provider keys": `model_providers.dongfang.name = "Host provider"
"model_providers" . "dongfang" . api_key = "test-only-invalid-host-key"
'model_providers'.dongfang.http_headers."X-Test" = "host-header"
model_providers."proxy".name = "Stale proxy"
`,
	}
	profile := agentruntime.Profile{
		ModelID: "managed-model",
		BaseURL: "https://managed.example/v1",
		APIKey:  "test-only-managed-key",
	}
	for name, providerConfig := range fixtures {
		for _, mode := range []string{ExecutionModeStandard, ExecutionModeReadOnly} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				existing := settings + providerConfig + tables
				before := parseProviderTestConfig(t, existing)
				generated := configureCodexHomeConfigWithWorkspaceForPlatformAndExecutionMode(existing, profile, nil, "", false, mode)
				after := assertManagedProviderTestConfig(t, generated, profile)
				for _, key := range []string{"model_reasoning_effort", "developer_instructions", "projects", "history"} {
					if !reflect.DeepEqual(after[key], before[key]) {
						t.Errorf("unrelated setting %q changed: got %#v, want %#v", key, after[key], before[key])
					}
				}
				repeated := configureCodexHomeConfigWithWorkspaceForPlatformAndExecutionMode(generated, profile, nil, "", false, mode)
				if !reflect.DeepEqual(parseProviderTestConfig(t, repeated), after) {
					t.Error("repeated configuration changed the generated config semantics")
				}
			})
		}
	}
}

func TestConfigureCodexHomeConfigManagedProviderCleansExistingAgentConfig(t *testing.T) {
	profile := agentruntime.Profile{ModelID: "old-model", BaseURL: "https://old.example/v1"}
	existing := configureCodexHomeConfig(`
[projects."/workspace/support"]
trust_level = "trusted"
`, profile, nil)
	// An existing Agent can still contain providers copied before isolation.
	existing += `
[model_providers.dongfang]
api_key = "test-only-invalid-host-key"
[model_providers.dongfang.http_headers]
"X-Test" = "host-header"
`
	profile = agentruntime.Profile{ModelID: "new-model", BaseURL: "https://new.example/v1", APIKey: "test-only-managed-key"}
	for _, mode := range []string{ExecutionModeReadOnly, ExecutionModeStandard} {
		existing = configureCodexHomeConfigWithWorkspaceForPlatformAndExecutionMode(existing, profile, nil, "", false, mode)
		parsed := assertManagedProviderTestConfig(t, existing, profile)
		projects, ok := parsed["projects"].(map[string]any)
		if !ok || !reflect.DeepEqual(projects["/workspace/support"], map[string]any{"trust_level": "trusted"}) {
			t.Fatalf("project settings were changed during %s reconfiguration: %#v", mode, parsed["projects"])
		}
	}
}

func TestConfigureCodexHomeConfigUnmanagedProviderPreservesHostProviders(t *testing.T) {
	const existing = `
[model_providers.dongfang]
name = "Host provider"
base_url = "https://host.example/v1"
env_key = "HOST_API_KEY"
[model_providers.dongfang.http_headers]
"X-Test" = "host-header"
[projects."/workspace/support"]
trust_level = "trusted"
`
	before := parseProviderTestConfig(t, existing)
	for name, profile := range map[string]agentruntime.Profile{
		"empty profile":    {},
		"missing model":    {BaseURL: "https://managed.example/v1"},
		"missing endpoint": {ModelID: "managed-model"},
		"blank model":      {BaseURL: "https://managed.example/v1", ModelID: " "},
		"blank endpoint":   {BaseURL: " ", ModelID: "managed-model"},
	} {
		t.Run(name, func(t *testing.T) {
			for _, mode := range []string{ExecutionModeStandard, ExecutionModeReadOnly} {
				generated := configureCodexHomeConfigWithWorkspaceForPlatformAndExecutionMode(existing, profile, nil, "", false, mode)
				after := parseProviderTestConfig(t, generated)
				if !reflect.DeepEqual(after["model_providers"], before["model_providers"]) {
					t.Errorf("%s mode changed unmanaged providers: got %#v, want %#v", mode, after["model_providers"], before["model_providers"])
				}
			}
		})
	}
}

func parseProviderTestConfig(t *testing.T, config string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := toml.Unmarshal([]byte(config), &parsed); err != nil {
		t.Fatalf("generated configuration is not valid TOML: %v\n%s", err, config)
	}
	return parsed
}

func assertManagedProviderTestConfig(t *testing.T, config string, profile agentruntime.Profile) map[string]any {
	t.Helper()
	parsed := parseProviderTestConfig(t, config)
	providers, ok := parsed["model_providers"].(map[string]any)
	if !ok || len(providers) != 1 {
		t.Fatalf("managed configuration should contain exactly one provider, got %#v", parsed["model_providers"])
	}
	proxy, ok := providers["proxy"].(map[string]any)
	if !ok {
		t.Fatalf("managed proxy missing from providers: %#v", providers)
	}
	for key, want := range map[string]any{
		"name":                "OpenAI using LLM proxy",
		"base_url":            profile.BaseURL,
		"wire_api":            "responses",
		"supports_websockets": false,
	} {
		if !reflect.DeepEqual(proxy[key], want) {
			t.Errorf("proxy %s = %#v, want %#v", key, proxy[key], want)
		}
	}
	for _, key := range []string{"api_key", "http_headers", "auth"} {
		if value, ok := proxy[key]; ok {
			t.Errorf("inherited proxy setting %q survived: %#v", key, value)
		}
	}
	if profile.APIKey != "" && proxy["env_key"] != "OPENAI_API_KEY" {
		t.Errorf("proxy env_key = %#v, want OPENAI_API_KEY", proxy["env_key"])
	}
	if parsed["model_provider"] != "proxy" || parsed["model"] != profile.ModelID {
		t.Errorf("model selection = %#v/%#v, want proxy/%s", parsed["model_provider"], parsed["model"], profile.ModelID)
	}
	return parsed
}
