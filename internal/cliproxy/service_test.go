package cliproxy

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	cliproxysdk "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

func TestProviderPath(t *testing.T) {
	tests := map[string]string{
		"codex":       "codex",
		"claude_code": "anthropic",
		"claude-code": "anthropic",
		"anthropic":   "anthropic",
	}
	for input, want := range tests {
		if got := providerPath(input); got != want {
			t.Fatalf("providerPath(%q) = %q, want %q", input, got, want)
		}
	}
	if got := providerPath("api"); got != "" {
		t.Fatalf("providerPath(api) = %q, want empty", got)
	}
	if got := registryProvider("claude_code"); got != "claude" {
		t.Fatalf("registryProvider(claude_code) = %q, want claude", got)
	}
	if got := registryProvider("codex"); got != "codex" {
		t.Fatalf("registryProvider(codex) = %q, want codex", got)
	}
}

func TestRegisteredModelsUsesCLIProxyProviderRegistry(t *testing.T) {
	registry := cliproxysdk.GlobalModelRegistry()
	clientID := "csgclaw-test-client"
	registry.UnregisterClient(clientID)
	defer registry.UnregisterClient(clientID)
	registry.RegisterClient(clientID, "codex", []*cliproxysdk.ModelInfo{
		{ID: "gpt-5.4"},
		{ID: "gpt-5.4"},
		{ID: "gpt-5.5"},
	})

	got := registeredModels("codex")
	want := []string{"gpt-5.4", "gpt-5.5"}
	if len(got) != len(want) {
		t.Fatalf("registeredModels len = %d (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("registeredModels[%d] = %q, want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestFallbackModelsCoverEmbeddedCLIProviders(t *testing.T) {
	for provider, wantFirst := range map[string]string{
		"codex":  "gpt-5.5",
		"claude": "claude-opus-4-7",
	} {
		models := fallbackModels(provider)
		if len(models) == 0 {
			t.Fatalf("fallbackModels(%q) returned no models", provider)
		}
		if models[0] != wantFirst {
			t.Fatalf("fallbackModels(%q)[0] = %q, want %q (all=%v)", provider, models[0], wantFirst, models)
		}
	}
}

func TestEmbeddedCLIProxyLoadsBuiltinTranslators(t *testing.T) {
	input := []byte(`{"model":"client-model","messages":[{"role":"user","content":"hello"}],"max_completion_tokens":1024}`)

	got := sdktranslator.TranslateRequest(
		sdktranslator.FormatOpenAI,
		sdktranslator.FormatCodex,
		"gpt-5.4",
		input,
		false,
	)
	text := string(got)
	if strings.Contains(text, "max_completion_tokens") {
		t.Fatalf("translated request leaked max_completion_tokens: %s", text)
	}
	if strings.Contains(text, "client-model") {
		t.Fatalf("translated request leaked client model: %s", text)
	}
	if !strings.Contains(text, `"model":"gpt-5.4"`) {
		t.Fatalf("translated request missing resolved model: %s", text)
	}
}

func TestBuildConfigUsesPrivateNonReservedPortAndWritesConfig(t *testing.T) {
	t.Setenv(configDirEnv, t.TempDir())
	t.Setenv(authDirEnv, filepath.Join(t.TempDir(), "auth"))

	cfg, cfgPath, baseURL, err := buildConfig()
	if err != nil {
		t.Fatalf("buildConfig returned error: %v", err)
	}
	if cfg.Host != "127.0.0.1" {
		t.Fatalf("Host = %q, want 127.0.0.1", cfg.Host)
	}
	if cfg.Port == reservedLegacyCLIProxyPort {
		t.Fatalf("Port = reserved fixed CLIProxy port")
	}
	if !strings.HasPrefix(baseURL, "http://127.0.0.1:") {
		t.Fatalf("baseURL = %q, want private localhost URL", baseURL)
	}
	if err := writeConfigFile(cfgPath, cfg); err != nil {
		t.Fatalf("writeConfigFile returned error: %v", err)
	}
	content, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		`host: "127.0.0.1"`,
		`api-keys:`,
		`  - "local"`,
		`allow-remote: false`,
		`disable-control-panel: true`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated config missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, strconv.Itoa(reservedLegacyCLIProxyPort)) {
		t.Fatalf("generated config contains reserved fixed port:\n%s", text)
	}
}

func TestConfiguredAuthDirExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(authDirEnv, "")

	got, err := configuredAuthDir()
	if err != nil {
		t.Fatalf("configuredAuthDir returned error: %v", err)
	}
	want := filepath.Join(home, ".cli-proxy-api")
	if got != want {
		t.Fatalf("configuredAuthDir = %q, want %q", got, want)
	}
}
