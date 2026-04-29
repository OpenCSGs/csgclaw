package serve

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestServeForegroundContinuesWhenCSGHubLiteUnavailable(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer modelServer.Close()

	err := serveForeground(context.Background(), testContext(), csgHubLiteServeConfig(modelServer.URL+"/v1"), "json")
	if err != nil {
		t.Fatalf("serveForeground() error = %v, want startup to continue with dynamic profile setup", err)
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
	origStartConfiguredAgents := StartConfiguredAgents
	origEnsureCLIProxy := EnsureCLIProxy
	origShutdownCLIProxy := ShutdownCLIProxy
	t.Cleanup(func() {
		RunServer = origRunServer
		NewAgentService = origNewAgentService
		NewBotService = origNewBotService
		NewIMService = origNewIMService
		NewFeishuService = origNewFeishuService
		NewLLMService = origNewLLMService
		EnsureBootstrapManager = origEnsureBootstrapManager
		StartConfiguredAgents = origStartConfiguredAgents
		EnsureCLIProxy = origEnsureCLIProxy
		ShutdownCLIProxy = origShutdownCLIProxy
	})

	ctx := context.WithValue(context.Background(), struct{}{}, "serve-context")
	svc := &agent.Service{}

	NewAgentService = func(config.Config) (*agent.Service, error) {
		return svc, nil
	}
	NewIMService = func(*im.Bus) (*im.Service, error) {
		return nil, nil
	}
	wantBotSvc := &bot.Service{}
	NewBotService = func() (*bot.Service, error) {
		return wantBotSvc, nil
	}
	NewFeishuService = func(cfg config.Config) (*channel.FeishuService, error) {
		if got, want := cfg.Channels.Feishu["manager"].AppID, "cli_manager"; got != want {
			return nil, fmt.Errorf("manager app_id = %q, want %q", got, want)
		}
		return nil, nil
	}
	NewLLMService = func(config.Config, *agent.Service) (*llm.Service, error) {
		return nil, nil
	}
	EnsureCLIProxy = func(context.Context) error { return nil }
	ShutdownCLIProxy = func(context.Context) error { return nil }

	called := false
	bootstrapped := false
	startCalled := make(chan struct{})
	releaseStart := make(chan struct{})
	startReturned := make(chan struct{})
	startErrors := make(chan string, 4)
	EnsureBootstrapManager = func(gotCtx context.Context, gotSvc *agent.Service, forceRecreate bool) error {
		bootstrapped = true
		if gotCtx != ctx {
			return fmt.Errorf("EnsureBootstrapManager context = %v, want %v", gotCtx, ctx)
		}
		if gotSvc != svc {
			return fmt.Errorf("EnsureBootstrapManager service = %p, want %p", gotSvc, svc)
		}
		if forceRecreate {
			return fmt.Errorf("EnsureBootstrapManager forceRecreate = true, want false")
		}
		return nil
	}
	StartConfiguredAgents = func(gotCtx context.Context, gotSvc *agent.Service) error {
		defer close(startReturned)
		if !bootstrapped {
			startErrors <- "StartConfiguredAgents called before EnsureBootstrapManager"
		}
		if gotCtx != ctx {
			startErrors <- fmt.Sprintf("StartConfiguredAgents context = %v, want %v", gotCtx, ctx)
		}
		if gotSvc != svc {
			startErrors <- fmt.Sprintf("StartConfiguredAgents service = %p, want %p", gotSvc, svc)
		}
		close(startCalled)
		<-releaseStart
		return nil
	}
	RunServer = func(opts server.Options) error {
		called = true
		if opts.Context != ctx {
			return fmt.Errorf("Context = %v, want %v", opts.Context, ctx)
		}
		if opts.Bot != wantBotSvc {
			return fmt.Errorf("Bot = %v, want injected bot service", opts.Bot)
		}
		if !opts.NoAuth {
			return fmt.Errorf("NoAuth = false, want true")
		}
		if opts.OnReady == nil {
			return fmt.Errorf("OnReady is nil")
		}
		go opts.OnReady()
		return nil
	}
	releasedStart := false
	releaseConfiguredAgentStart := func() {
		if !releasedStart {
			close(releaseStart)
			releasedStart = true
		}
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
			ManagerImageOverride: "ghcr.io/example/manager:latest",
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

	done := make(chan error, 1)
	go func() {
		done <- serveForeground(ctx, run, cfg, "table")
	}()

	select {
	case err := <-done:
		if err != nil {
			releaseConfiguredAgentStart()
			t.Fatalf("serveForeground() error = %v", err)
		}
	case <-time.After(time.Second):
		releaseConfiguredAgentStart()
		t.Fatal("serveForeground blocked on StartConfiguredAgents; want async agent startup")
	}
	select {
	case <-startCalled:
	case <-time.After(time.Second):
		releaseConfiguredAgentStart()
		t.Fatal("StartConfiguredAgents was not called")
	}
	releaseConfiguredAgentStart()
	select {
	case <-startReturned:
	case <-time.After(time.Second):
		t.Fatal("StartConfiguredAgents did not return after release")
	}
	close(startErrors)
	for msg := range startErrors {
		t.Error(msg)
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

func TestConfigureServeLoggerRejectsUnsupportedLevel(t *testing.T) {
	_, err := configureServeLogger(&bytes.Buffer{}, "trace")
	if err == nil {
		t.Fatal("configureServeLogger() error = nil, want unsupported-level error")
	}
	if !strings.Contains(err.Error(), `unsupported log level "trace"`) {
		t.Fatalf("configureServeLogger() error = %q, want unsupported-level error", err)
	}
}

func TestConfigureServeLoggerSetsDebugLevel(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prev)
	})

	restore, err := configureServeLogger(&bytes.Buffer{}, "debug")
	if err != nil {
		t.Fatalf("configureServeLogger() error = %v", err)
	}
	defer restore()

	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("default logger debug level is disabled, want enabled")
	}
}

func TestServeForegroundStartsConfiguredAgentsWhenBootstrapFails(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()

	started := make(chan struct{})
	EnsureBootstrapManager = func(context.Context, *agent.Service, bool) error {
		return fmt.Errorf("runtime lock")
	}
	StartConfiguredAgents = func(context.Context, *agent.Service) error {
		close(started)
		return nil
	}
	RunServer = func(opts server.Options) error {
		if opts.OnReady == nil {
			return fmt.Errorf("OnReady is nil")
		}
		opts.OnReady()
		return nil
	}

	if err := serveForeground(context.Background(), testContext(), config.Config{Server: config.ServerConfig{ListenAddr: "127.0.0.1:18080"}}, "json"); err != nil {
		t.Fatalf("serveForeground() error = %v", err)
	}
	select {
	case <-started:
	default:
		t.Fatal("StartConfiguredAgents was not called after bootstrap failure")
	}
}

func TestServeForegroundPreservesManagerImageOverride(t *testing.T) {
	origRunServer := RunServer
	origNewAgentService := NewAgentService
	origEnsureBootstrapManager := EnsureBootstrapManager
	origStartConfiguredAgents := StartConfiguredAgents
	t.Cleanup(func() {
		RunServer = origRunServer
		NewAgentService = origNewAgentService
		EnsureBootstrapManager = origEnsureBootstrapManager
		StartConfiguredAgents = origStartConfiguredAgents
	})
	RunServer = func(server.Options) error { return nil }
	EnsureBootstrapManager = func(context.Context, *agent.Service, bool) error { return nil }
	StartConfiguredAgents = func(context.Context, *agent.Service) error { return nil }

	cfg := config.Config{
		Server: config.ServerConfig{
			ListenAddr:  "127.0.0.1:18080",
			AccessToken: "pc-secret",
		},
		Models: config.SingleProfileLLM(config.ModelConfig{
			BaseURL: "http://llm.test",
			APIKey:  "sk-secret",
			ModelID: "model-test",
		}),
		Bootstrap: config.BootstrapConfig{
			ManagerImageOverride: "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.4.24.0",
		},
	}
	NewAgentService = func(got config.Config) (*agent.Service, error) {
		if got.Bootstrap.ManagerImageOverride != "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.4.24.0" {
			t.Fatalf("manager image override = %q, want preserved override", got.Bootstrap.ManagerImageOverride)
		}
		return &agent.Service{}, nil
	}

	if err := serveForeground(context.Background(), testContext(), cfg, "json"); err != nil {
		t.Fatalf("serveForeground() error = %v", err)
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
		fmt.Sprintf(`# using default image: %q`, config.DefaultManagerImage),
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

func TestFormatEffectiveConfigFormatsSectionsWithoutExtraWhitespace(t *testing.T) {
	cfg := config.Config{
		Server: config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://192.168.2.52:18080",
			AccessToken:      "your_access_token",
			NoAuth:           true,
		},
		Models: config.SingleProfileLLM(config.ModelConfig{
			BaseURL: "http://127.0.0.1:4000",
			APIKey:  "sk-secret",
			ModelID: "local.minimax-m2.5",
		}),
		Bootstrap: config.BootstrapConfig{
			ManagerImageOverride: "ghcr.io/russellluo/picoclaw:2026.4.25",
		},
		Sandbox: config.SandboxConfig{
			Provider:         config.BoxLiteCLIProvider,
			HomeDirName:      config.DefaultSandboxHomeDirName,
			DebianRegistries: config.DefaultDebianRegistries,
		},
	}

	want := `[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://192.168.2.52:18080"
access_token = "yo*************en"
no_auth = true

[bootstrap]
manager_image_override = "ghcr.io/russellluo/picoclaw:2026.4.25"

[sandbox]
debian_registries = ["harbor.opencsg.com", "docker.io"]
provider = "boxlite-cli"
home_dir_name = "boxlite"

[models]
default = "default.local.minimax-m2.5"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk*****et"
models = ["local.minimax-m2.5"]
`
	if got := formatEffectiveConfig(cfg); got != want {
		t.Fatalf("formatEffectiveConfig() mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSandboxServiceOptionsSupportsConfiguredProvider(t *testing.T) {
	opts, err := sandboxServiceOptions(config.SandboxConfig{
		Provider:         config.BoxLiteCLIProvider,
		HomeDirName:      "sandbox-home",
		DebianRegistries: []string{"registry.a"},
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
			ManagerImageOverride: "ghcr.io/example/manager:latest",
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
	origStartConfiguredAgents := StartConfiguredAgents
	origEnsureCLIProxy := EnsureCLIProxy
	origShutdownCLIProxy := ShutdownCLIProxy
	RunServer = func(server.Options) error { return nil }
	NewAgentService = func(config.Config) (*agent.Service, error) { return &agent.Service{}, nil }
	NewBotService = func() (*bot.Service, error) { return &bot.Service{}, nil }
	NewIMService = func(*im.Bus) (*im.Service, error) { return nil, nil }
	NewFeishuService = func(config.Config) (*channel.FeishuService, error) { return nil, nil }
	NewLLMService = func(config.Config, *agent.Service) (*llm.Service, error) { return nil, nil }
	EnsureBootstrapManager = func(context.Context, *agent.Service, bool) error { return nil }
	StartConfiguredAgents = func(context.Context, *agent.Service) error { return nil }
	EnsureCLIProxy = func(context.Context) error { return nil }
	ShutdownCLIProxy = func(context.Context) error { return nil }
	return func() {
		RunServer = origRunServer
		NewAgentService = origNewAgentService
		NewBotService = origNewBotService
		NewIMService = origNewIMService
		NewFeishuService = origNewFeishuService
		NewLLMService = origNewLLMService
		EnsureBootstrapManager = origEnsureBootstrapManager
		StartConfiguredAgents = origStartConfiguredAgents
		EnsureCLIProxy = origEnsureCLIProxy
		ShutdownCLIProxy = origShutdownCLIProxy
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

func TestParseServeLogLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"":        slog.LevelInfo,
		"info":    slog.LevelInfo,
		"DEBUG":   slog.LevelDebug,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
	}

	for input, want := range cases {
		got, err := parseServeLogLevel(input)
		if err != nil {
			t.Fatalf("parseServeLogLevel(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("parseServeLogLevel(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestValidateModelConfigAllowsDynamicProfileSetupWhenIncomplete(t *testing.T) {
	err := validateModelConfig(config.Config{})
	if err != nil {
		t.Fatalf("validateModelConfig() error = %v, want nil for UI-driven dynamic setup", err)
	}
}

func testContext() *command.Context {
	return &command.Context{
		Program: "csgclaw",
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}
}
