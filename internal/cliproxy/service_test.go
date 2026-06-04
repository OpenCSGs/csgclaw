package cliproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	cliproxysdk "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
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

func TestEmbeddedCLIProxyRegistersImportedCodexAuthModels(t *testing.T) {
	home := t.TempDir()
	authDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(configDirEnv, t.TempDir())
	t.Setenv(authDirEnv, authDir)

	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{
		"tokens": {
			"access_token": "`+testJWT(t, `{"exp":1893456000}`)+`",
			"refresh_token": "refresh",
			"id_token": "`+testJWT(t, `{"exp":1893456000}`)+`",
			"account_id": "acct_123"
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	svc := &Service{client: &http.Client{Timeout: 5 * time.Second}}
	if err := svc.EnsureStarted(ctx); err != nil {
		t.Fatalf("EnsureStarted() error = %v", err)
	}
	defer func() {
		if err := svc.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown() error = %v", err)
		}
	}()

	models, err := svc.ListModels(ctx, ProviderCodex)
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if !containsString(models, "gpt-5.5") {
		t.Fatalf("models = %v, want gpt-5.5 registered for imported codex auth", models)
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestBuildConfigUsesPrivateNonReservedPortAndWritesConfig(t *testing.T) {
	clearStandardProxyEnv(t)
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
	if strings.Contains(text, "proxy-url:") {
		t.Fatalf("generated config unexpectedly contains proxy-url:\n%s", text)
	}
}

func TestBuildConfigUsesStandardProxyEnvironmentForCLIProxyAPI(t *testing.T) {
	clearStandardProxyEnv(t)
	t.Setenv(configDirEnv, t.TempDir())
	t.Setenv(authDirEnv, filepath.Join(t.TempDir(), "auth"))
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7890")

	cfg, cfgPath, _, err := buildConfig()
	if err != nil {
		t.Fatalf("buildConfig returned error: %v", err)
	}
	if cfg.ProxyURL != "http://127.0.0.1:7890" {
		t.Fatalf("ProxyURL = %q, want inherited HTTPS proxy", cfg.ProxyURL)
	}
	if err := writeConfigFile(cfgPath, cfg); err != nil {
		t.Fatalf("writeConfigFile returned error: %v", err)
	}
	content, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if text := string(content); !strings.Contains(text, `proxy-url: "http://127.0.0.1:7890"`) {
		t.Fatalf("generated config missing proxy-url:\n%s", text)
	}
}

func TestSkipEmbeddedHealthzAccessLogMarksOnlyHealthz(t *testing.T) {
	mode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(mode) })

	router := gin.New()
	router.Use(skipEmbeddedHealthzAccessLog())
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"skip": ginContextBool(c, embeddedCLIProxySkipGinLogKey)})
	})
	router.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"skip": ginContextBool(c, embeddedCLIProxySkipGinLogKey)})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if got := strings.TrimSpace(rec.Body.String()); got != `{"skip":true}` {
		t.Fatalf("healthz skip response = %s, want true", got)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if got := strings.TrimSpace(rec.Body.String()); got != `{"skip":false}` {
		t.Fatalf("api skip response = %s, want false", got)
	}
}

func ginContextBool(c *gin.Context, key string) bool {
	value, ok := c.Get(key)
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}

func TestConfiguredAuthDirExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(authDirEnv, "")

	got, err := configuredAuthDir()
	if err != nil {
		t.Fatalf("configuredAuthDir returned error: %v", err)
	}
	want := filepath.Join(home, ".csgclaw", "auth")
	if got != want {
		t.Fatalf("configuredAuthDir = %q, want %q", got, want)
	}
}

func TestConfigDirDefaultsToAuthDomain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(configDirEnv, "")

	got, err := configDir()
	if err != nil {
		t.Fatalf("configDir returned error: %v", err)
	}
	want := filepath.Join(home, ".csgclaw", "auth")
	if got != want {
		t.Fatalf("configDir = %q, want %q", got, want)
	}
}

func clearStandardProxyEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"HTTPS_PROXY",
		"https_proxy",
		"HTTP_PROXY",
		"http_proxy",
	} {
		t.Setenv(name, "")
	}
}
