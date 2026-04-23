package serve

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/cli/command"
	"csgclaw/internal/agent"
	"csgclaw/internal/bot"
	"csgclaw/internal/channel"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	"csgclaw/internal/server"
)

func TestServeForegroundPreflightsCSGHubLiteProvider(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()

	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer local" {
			t.Fatalf("Authorization = %q, want Bearer local", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"Qwen/Qwen3-0.6B-GGUF"}]}`))
	}))
	defer modelServer.Close()

	cfg := csgHubLiteServeConfig(modelServer.URL + "/v1")
	if err := serveForeground(context.Background(), testContext(), cfg, "json"); err != nil {
		t.Fatalf("serveForeground() error = %v", err)
	}
}

func TestServeForegroundFailsBeforeManagerWhenCSGHubLiteUnavailable(t *testing.T) {
	origNewAgentService := NewAgentService
	t.Cleanup(func() {
		NewAgentService = origNewAgentService
	})
	NewAgentService = func(config.Config) (*agent.Service, error) {
		t.Fatal("NewAgentService should not run when csghub-lite preflight fails")
		return nil, nil
	}

	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer modelServer.Close()

	err := serveForeground(context.Background(), testContext(), csgHubLiteServeConfig(modelServer.URL+"/v1"), "json")
	if err == nil {
		t.Fatal("serveForeground() error = nil, want preflight error")
	}
	if !strings.Contains(err.Error(), "csghub-lite provider is not reachable") || !strings.Contains(err.Error(), "csghub-lite run <model>") {
		t.Fatalf("serveForeground() error = %q, want csghub-lite start hint", err)
	}
}

func TestServeForegroundPassesContextToServer(t *testing.T) {
	origRunServer := RunServer
	origNewAgentService := NewAgentService
	origNewBotService := NewBotService
	origNewIMService := NewIMService
	origNewFeishuService := NewFeishuService
	origNewLLMService := NewLLMService
	origEnsureBootstrapManager := EnsureBootstrapManager
	t.Cleanup(func() {
		RunServer = origRunServer
		NewAgentService = origNewAgentService
		NewBotService = origNewBotService
		NewIMService = origNewIMService
		NewFeishuService = origNewFeishuService
		NewLLMService = origNewLLMService
		EnsureBootstrapManager = origEnsureBootstrapManager
	})

	ctx := context.WithValue(context.Background(), struct{}{}, "serve-context")
	svc := &agent.Service{}

	NewAgentService = func(config.Config) (*agent.Service, error) {
		return svc, nil
	}
	NewIMService = func() (*im.Service, error) {
		return nil, nil
	}
	wantBotSvc := &bot.Service{}
	NewBotService = func() (*bot.Service, error) {
		return wantBotSvc, nil
	}
	NewFeishuService = func(cfg config.Config) (*channel.FeishuService, error) {
		if got, want := cfg.Channels.Feishu["manager"].AppID, "cli_manager"; got != want {
			t.Fatalf("manager app_id = %q, want %q", got, want)
		}
		return nil, nil
	}
	NewLLMService = func(config.Config, *agent.Service) (*llm.Service, error) {
		return nil, nil
	}

	called := false
	bootstrapped := false
	EnsureBootstrapManager = func(gotCtx context.Context, gotSvc *agent.Service, forceRecreate bool) error {
		bootstrapped = true
		if gotCtx != ctx {
			t.Fatalf("EnsureBootstrapManager context = %v, want %v", gotCtx, ctx)
		}
		if gotSvc != svc {
			t.Fatalf("EnsureBootstrapManager service = %p, want %p", gotSvc, svc)
		}
		if forceRecreate {
			t.Fatal("EnsureBootstrapManager forceRecreate = true, want false")
		}
		return nil
	}
	RunServer = func(opts server.Options) error {
		called = true
		if opts.Context != ctx {
			t.Fatalf("Context = %v, want %v", opts.Context, ctx)
		}
		if opts.Bot != wantBotSvc {
			t.Fatalf("Bot = %v, want injected bot service", opts.Bot)
		}
		if !opts.NoAuth {
			t.Fatal("NoAuth = false, want true")
		}
		return nil
	}

	run := testContext()
	cfg := config.Config{
		Server: config.ServerConfig{
			ListenAddr:       "127.0.0.1:18080",
			AdvertiseBaseURL: "http://example.test",
			AccessToken:      "pc-secret",
			NoAuth:           true,
		},
		Model: config.ModelConfig{
			Provider: "llm-api",
			BaseURL:  "http://llm.test",
			APIKey:   "sk-secret",
			ModelID:  "model-test",
		},
		Models: config.SingleProfileLLM(config.ModelConfig{
			BaseURL: "http://llm.test",
			APIKey:  "sk-secret",
			ModelID: "model-test",
		}),
		Bootstrap: config.BootstrapConfig{
			ManagerImage: "ghcr.io/example/manager:latest",
		},
		Channels: config.ChannelsConfig{
			FeishuAdminOpenID: "ou_admin",
			Feishu: map[string]config.FeishuConfig{
				"manager": {
					AppID:     "cli_manager",
					AppSecret: "manager-secret",
				},
			},
		},
	}

	if err := serveForeground(ctx, run, cfg, "table"); err != nil {
		t.Fatalf("serveForeground() error = %v", err)
	}
	if !called {
		t.Fatal("RunServer was not called")
	}
	if !bootstrapped {
		t.Fatal("EnsureBootstrapManager was not called")
	}

	got := run.Stdout.(*bytes.Buffer).String()
	for _, want := range []string{
		"effective config:\n",
		`listen_addr = "127.0.0.1:18080"`,
		`advertise_base_url = "http://example.test"`,
		`api_key = "sk*****et"`,
		`access_token = "pc*****et"`,
		`no_auth = true`,
		`[sandbox]`,
		fmt.Sprintf(`provider = %q`, config.DefaultSandboxProvider),
		fmt.Sprintf(`home_dir_name = %q`, config.DefaultSandboxHomeDirName),
		fmt.Sprintf(`boxlite_cli_path = %q`, config.DefaultBoxLiteCLIPath),
		`[models]`,
		`default = "default.model-test"`,
		`[models.providers.default]`,
		`[channels.feishu]`,
		`admin_open_id = "ou_admin"`,
		`[channels.feishu.manager]`,
		`app_id = "cli_manager"`,
		`app_secret = "ma**********et"`,
		"CSGClaw IM is available at: http://example.test/",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "sk-secret") {
		t.Fatalf("stdout leaked model API key:\n%s", got)
	}
	if strings.Contains(got, "pc-secret") {
		t.Fatalf("stdout leaked server access token:\n%s", got)
	}
	if strings.Contains(got, "manager-secret") {
		t.Fatalf("stdout leaked feishu app secret:\n%s", got)
	}
}

func TestFormatEffectiveConfigPrintsExpandedMaskedEnvValues(t *testing.T) {
	t.Setenv("PORT", "18080")
	t.Setenv("IP", "1.2.3.4")
	t.Setenv("ACCESS_TOKEN", "pc-env-secret")
	t.Setenv("MODEL_SELECTOR", "remote.gpt-env")
	t.Setenv("MODEL_BASE_HOST", "models.example.test")
	t.Setenv("MODEL_API_KEY", "sk-env-secret")
	t.Setenv("MODEL_ID", "gpt-env")
	t.Setenv("FEISHU_APP_ID", "cli_env")
	t.Setenv("FEISHU_APP_SECRET", "feishu-env-secret")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "0.0.0.0:${PORT}"
advertise_base_url = "http://${IP}:${PORT}"
access_token = "${ACCESS_TOKEN}"
no_auth = true

[models]
default = "${MODEL_SELECTOR}"

[models.providers.remote]
base_url = "https://${MODEL_BASE_HOST}/v1"
api_key = "${MODEL_API_KEY}"
models = ["${MODEL_ID}"]

[channels.feishu.manager]
app_id = "${FEISHU_APP_ID}"
app_secret = "${FEISHU_APP_SECRET}"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	got := formatEffectiveConfig(cfg)
	for _, want := range []string{
		`listen_addr = "0.0.0.0:18080"`,
		`advertise_base_url = "http://1.2.3.4:18080"`,
		`access_token = "pc*********et"`,
		`no_auth = true`,
		`default = "remote.gpt-env"`,
		`base_url = "https://models.example.test/v1"`,
		`api_key = "sk*********et"`,
		`models = ["gpt-env"]`,
		`app_id = "cli_env"`,
		`app_secret = "fe*************et"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("effective config missing %q:\n%s", want, got)
		}
	}
	for _, leaked := range []string{
		"${PORT}",
		"${ACCESS_TOKEN}",
		"${MODEL_API_KEY}",
		"${FEISHU_APP_SECRET}",
		"pc-env-secret",
		"sk-env-secret",
		"feishu-env-secret",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("effective config leaked %q:\n%s", leaked, got)
		}
	}
}

func TestSandboxServiceOptionsSupportsConfiguredProvider(t *testing.T) {
	opts, err := sandboxServiceOptions(config.SandboxConfig{
		Provider:          config.BoxLiteCLIProvider,
		HomeDirName:       "sandbox-home",
		BoxLiteCLIPath:    "/opt/boxlite/bin/boxlite",
		DebianRegistries:  []string{"registry.a"},
	})
	if err != nil {
		t.Fatalf("sandboxServiceOptions() error = %v", err)
	}
	if len(opts) != 2 {
		t.Fatalf("len(opts) = %d, want 2", len(opts))
	}
}

func TestNewAgentServiceRejectsUnsupportedSandboxProvider(t *testing.T) {
	_, err := newAgentService(config.Config{
		Sandbox: config.SandboxConfig{
			Provider:    "docker",
			HomeDirName: "docker",
		},
	})
	if err == nil {
		t.Fatal("newAgentService() error = nil, want unsupported sandbox provider")
	}
	if !strings.Contains(err.Error(), `unsupported sandbox provider "docker"`) {
		t.Fatalf("newAgentService() error = %q, want unsupported sandbox provider", err)
	}
}

func csgHubLiteServeConfig(baseURL string) config.Config {
	return config.Config{
		Server: config.ServerConfig{
			ListenAddr:  "127.0.0.1:18080",
			AccessToken: "pc-secret",
		},
		Models: config.LLMConfig{
			Default:        "csghub-lite.Qwen/Qwen3-0.6B-GGUF",
			DefaultProfile: "csghub-lite.Qwen/Qwen3-0.6B-GGUF",
			Providers: map[string]config.ProviderConfig{
				"csghub-lite": {
					BaseURL: baseURL,
					APIKey:  "local",
					Models:  []string{"Qwen/Qwen3-0.6B-GGUF"},
				},
			},
		},
		Bootstrap: config.BootstrapConfig{
			ManagerImage: "ghcr.io/example/manager:latest",
		},
	}
}

func stubServeDependencies(t *testing.T) func() {
	t.Helper()
	origRunServer := RunServer
	origNewAgentService := NewAgentService
	origNewBotService := NewBotService
	origNewIMService := NewIMService
	origNewFeishuService := NewFeishuService
	origNewLLMService := NewLLMService
	origEnsureBootstrapManager := EnsureBootstrapManager
	RunServer = func(server.Options) error { return nil }
	NewAgentService = func(config.Config) (*agent.Service, error) { return &agent.Service{}, nil }
	NewBotService = func() (*bot.Service, error) { return &bot.Service{}, nil }
	NewIMService = func() (*im.Service, error) { return nil, nil }
	NewFeishuService = func(config.Config) (*channel.FeishuService, error) { return nil, nil }
	NewLLMService = func(config.Config, *agent.Service) (*llm.Service, error) { return nil, nil }
	EnsureBootstrapManager = func(context.Context, *agent.Service, bool) error { return nil }
	return func() {
		RunServer = origRunServer
		NewAgentService = origNewAgentService
		NewBotService = origNewBotService
		NewIMService = origNewIMService
		NewFeishuService = origNewFeishuService
		NewLLMService = origNewLLMService
		EnsureBootstrapManager = origEnsureBootstrapManager
	}
}

func TestPartiallyMaskSecret(t *testing.T) {
	cases := map[string]string{
		"":          "",
		"abc":       "***",
		"abcd":      "****",
		"abcde":     "ab*de",
		"abcdef":    "ab**ef",
		"sk-secret": "sk*****et",
	}

	for input, want := range cases {
		if got := partiallyMaskSecret(input); got != want {
			t.Fatalf("partiallyMaskSecret(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAPIBaseURLDefaultsToLocalhost(t *testing.T) {
	got := apiBaseURL(config.ServerConfig{ListenAddr: "0.0.0.0:19090"})
	want := "http://127.0.0.1:19090"
	if got != want {
		t.Fatalf("apiBaseURL() = %q, want %q", got, want)
	}
}

func TestAPIBaseURLPrefersAdvertiseBaseURL(t *testing.T) {
	got := apiBaseURL(config.ServerConfig{
		ListenAddr:       "0.0.0.0:19090",
		AdvertiseBaseURL: "http://example.test/base/",
	})
	want := "http://example.test/base"
	if got != want {
		t.Fatalf("apiBaseURL() = %q, want %q", got, want)
	}
}

func TestAPIBaseURLFallsBackToSharedDefault(t *testing.T) {
	got := apiBaseURL(config.ServerConfig{})
	if got != config.DefaultAPIBaseURL() {
		t.Fatalf("apiBaseURL() = %q, want %q", got, config.DefaultAPIBaseURL())
	}
}

func TestValidateModelConfigRequiresOnboardWhenIncomplete(t *testing.T) {
	err := validateModelConfig(config.Config{})
	if err == nil {
		t.Fatal("validateModelConfig() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "csgclaw onboard") {
		t.Fatalf("validateModelConfig() error = %q, want onboard guidance", err)
	}
	if !strings.Contains(err.Error(), "--base-url") || !strings.Contains(err.Error(), "--api-key") || !strings.Contains(err.Error(), "--models") {
		t.Fatalf("validateModelConfig() error = %q, want missing model flags", err)
	}
}

func testContext() *command.Context {
	return &command.Context{
		Program: "csgclaw",
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}
}
