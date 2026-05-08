package serve

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/cli/command"
	"csgclaw/internal/agent"
	"csgclaw/internal/bot"
	"csgclaw/internal/channel"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	internalonboard "csgclaw/internal/onboard"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/server"
)

func TestServeForegroundPreflightsCSGHubLiteProvider(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()

	var gotModelCfg config.ModelConfig
	CheckModelProvider = func(_ context.Context, modelCfg config.ModelConfig) error {
		gotModelCfg = modelCfg
		return nil
	}

	cfg := csgHubLiteServeConfig("http://127.0.0.1:11435/v1")
	if err := serveForeground(context.Background(), testContext(), cfg, "json"); err != nil {
		t.Fatalf("serveForeground() error = %v", err)
	}
	if got, want := gotModelCfg.BaseURL, "http://127.0.0.1:11435/v1"; got != want {
		t.Fatalf("CheckModelProvider baseURL = %q, want %q", got, want)
	}
	if got, want := gotModelCfg.APIKey, "local"; got != want {
		t.Fatalf("CheckModelProvider apiKey = %q, want %q", got, want)
	}
}

func TestServeRunAutoBootstrapsWhenStateIncomplete(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()

	origDetectBootstrapState := DetectBootstrapState
	origEnsureBootstrapState := EnsureBootstrapState
	t.Cleanup(func() {
		DetectBootstrapState = origDetectBootstrapState
		EnsureBootstrapState = origEnsureBootstrapState
	})

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := (config.Config{
		Server: config.ServerConfig{
			ListenAddr:  "127.0.0.1:18080",
			AccessToken: "pc-secret",
		},
		Sandbox: config.SandboxConfig{
			Provider:    config.DefaultSandboxProvider,
			HomeDirName: config.DefaultSandboxHomeDirName,
		},
	}).Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var ensureCalls int
	DetectBootstrapState = func(opts internalonboard.DetectStateOptions) (internalonboard.DetectStateResult, error) {
		if opts.ConfigPath != configPath {
			t.Fatalf("DetectStateOptions.ConfigPath = %q, want %q", opts.ConfigPath, configPath)
		}
		return internalonboard.DetectStateResult{ConfigPath: configPath}, nil
	}
	EnsureBootstrapState = func(_ context.Context, opts internalonboard.EnsureStateOptions) (internalonboard.EnsureStateResult, error) {
		ensureCalls++
		if opts.ConfigPath != configPath {
			t.Fatalf("EnsureStateOptions.ConfigPath = %q, want %q", opts.ConfigPath, configPath)
		}
		return internalonboard.EnsureStateResult{
			ConfigPath: configPath,
			Config: config.Config{
				Bootstrap: config.BootstrapConfig{
					ManagerImageOverride: "ghcr.io/example/manager:latest",
				},
			},
		}, nil
	}

	run := testContext()
	err := NewServeCmd().Run(context.Background(), run, nil, command.GlobalOptions{
		Config: configPath,
		Output: "json",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if ensureCalls != 1 {
		t.Fatalf("EnsureBootstrapState calls = %d, want 1", ensureCalls)
	}
	if got := run.Stderr.(*bytes.Buffer).String(); !strings.Contains(got, "auto-initializing local state") {
		t.Fatalf("stderr missing auto-bootstrap log:\n%s", got)
	}
}

func TestServeRunSkipsAutoBootstrapWhenStateComplete(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()

	origDetectBootstrapState := DetectBootstrapState
	origEnsureBootstrapState := EnsureBootstrapState
	t.Cleanup(func() {
		DetectBootstrapState = origDetectBootstrapState
		EnsureBootstrapState = origEnsureBootstrapState
	})

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := (config.Config{
		Server: config.ServerConfig{
			ListenAddr:  "127.0.0.1:18080",
			AccessToken: "pc-secret",
		},
		Sandbox: config.SandboxConfig{
			Provider:    config.DefaultSandboxProvider,
			HomeDirName: config.DefaultSandboxHomeDirName,
		},
	}).Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	DetectBootstrapState = func(opts internalonboard.DetectStateOptions) (internalonboard.DetectStateResult, error) {
		if opts.ConfigPath != configPath {
			t.Fatalf("DetectStateOptions.ConfigPath = %q, want %q", opts.ConfigPath, configPath)
		}
		return internalonboard.DetectStateResult{
			ConfigPath:           configPath,
			ConfigExists:         true,
			ConfigComplete:       true,
			IMBootstrapComplete:  true,
			ManagerAgentComplete: true,
			ManagerBotComplete:   true,
		}, nil
	}
	EnsureBootstrapState = func(context.Context, internalonboard.EnsureStateOptions) (internalonboard.EnsureStateResult, error) {
		t.Fatal("EnsureBootstrapState should not be called when bootstrap is complete")
		return internalonboard.EnsureStateResult{}, nil
	}

	run := testContext()
	err := NewServeCmd().Run(context.Background(), run, nil, command.GlobalOptions{
		Config: configPath,
		Output: "json",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := run.Stderr.(*bytes.Buffer).String(); strings.Contains(got, "auto-initializing local state") {
		t.Fatalf("stderr should not contain auto-bootstrap log:\n%s", got)
	}
}

func TestServeForegroundContinuesWhenCSGHubLiteUnavailable(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()

	CheckModelProvider = func(context.Context, config.ModelConfig) error {
		return fmt.Errorf("provider unavailable")
	}

	err := serveForeground(context.Background(), testContext(), csgHubLiteServeConfig("http://127.0.0.1:11435/v1"), "json")
	if err != nil {
		t.Fatalf("serveForeground() error = %v, want startup to continue with dynamic profile setup", err)
	}
}

func TestServeForegroundOpensIMURLWhenBrowserAllowed(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()

	openedCh := make(chan string, 1)
	WaitForHealthy = func(string, time.Duration) error { return nil }
	var opened string
	OpenBrowser = func(rawURL string) error {
		opened = rawURL
		openedCh <- rawURL
		return nil
	}

	run := testContext()
	cfg := config.Config{
		Server: config.ServerConfig{
			AdvertiseBaseURL: "http://example.test/base",
		},
	}
	if err := serveForeground(context.Background(), run, cfg, "table"); err != nil {
		t.Fatalf("serveForeground() error = %v", err)
	}
	select {
	case <-openedCh:
	case <-time.After(time.Second):
		t.Fatal("OpenBrowser was not called after server readiness")
	}
	if got, want := opened, "http://example.test/base/"; got != want {
		t.Fatalf("OpenBrowser() URL = %q, want %q", got, want)
	}
	if got := run.Stdout.(*bytes.Buffer).String(); !strings.Contains(got, "Opened this URL in your browser.") {
		t.Fatalf("stdout missing browser-open confirmation:\n%s", got)
	}
}

func TestServeForegroundPrintsManualIMURLWhenBrowserNotAllowed(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()

	readyCh := make(chan struct{}, 1)
	WaitForHealthy = func(string, time.Duration) error { return nil }
	OpenBrowser = func(string) error {
		readyCh <- struct{}{}
		return fmt.Errorf("not allowed")
	}

	run := testContext()
	stdout := newNotifyingBuffer("Open this URL in your browser after startup.")
	run.Stdout = stdout
	cfg := config.Config{
		Server: config.ServerConfig{
			AdvertiseBaseURL: "http://example.test/base",
		},
	}
	if err := serveForeground(context.Background(), run, cfg, "table"); err != nil {
		t.Fatalf("serveForeground() error = %v", err)
	}
	select {
	case <-readyCh:
	case <-time.After(time.Second):
		t.Fatal("OpenBrowser fallback path was not reached after server readiness")
	}
	select {
	case <-stdout.Seen():
	case <-time.After(time.Second):
		t.Fatal("manual-open hint was not printed after browser fallback")
	}
	got := stdout.String()
	if !strings.Contains(got, "CSGClaw IM is available at: http://example.test/base/") {
		t.Fatalf("stdout missing IM URL:\n%s", got)
	}
	if !strings.Contains(got, "Open this URL in your browser after startup.") {
		t.Fatalf("stdout missing manual-open hint:\n%s", got)
	}
}

func TestServeForegroundWaitsForHealthyBeforeOpeningBrowser(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()

	waitStarted := make(chan struct{}, 1)
	releaseWait := make(chan struct{})
	openedCh := make(chan struct{}, 1)
	WaitForHealthy = func(string, time.Duration) error {
		waitStarted <- struct{}{}
		<-releaseWait
		return nil
	}
	OpenBrowser = func(string) error {
		openedCh <- struct{}{}
		return nil
	}

	run := testContext()
	cfg := config.Config{
		Server: config.ServerConfig{
			AdvertiseBaseURL: "http://example.test/base",
		},
	}
	if err := serveForeground(context.Background(), run, cfg, "table"); err != nil {
		t.Fatalf("serveForeground() error = %v", err)
	}
	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("WaitForHealthy was not called")
	}
	select {
	case <-openedCh:
		t.Fatal("OpenBrowser called before health check completed")
	default:
	}
	close(releaseWait)
	select {
	case <-openedCh:
	case <-time.After(time.Second):
		t.Fatal("OpenBrowser was not called after health check completed")
	}
}

func TestServeRunRepeatedAutoBootstrapRemainsIdempotent(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()

	origDetectBootstrapState := DetectBootstrapState
	origEnsureBootstrapState := EnsureBootstrapState
	t.Cleanup(func() {
		DetectBootstrapState = origDetectBootstrapState
		EnsureBootstrapState = origEnsureBootstrapState
	})

	configPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := config.Config{
		Server: config.ServerConfig{
			ListenAddr:  "127.0.0.1:18080",
			AccessToken: "pc-secret",
		},
		Sandbox: config.SandboxConfig{
			Provider:    config.DefaultSandboxProvider,
			HomeDirName: config.DefaultSandboxHomeDirName,
		},
	}
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	complete := false
	ensureCalls := 0
	DetectBootstrapState = func(opts internalonboard.DetectStateOptions) (internalonboard.DetectStateResult, error) {
		if opts.ConfigPath != configPath {
			t.Fatalf("DetectStateOptions.ConfigPath = %q, want %q", opts.ConfigPath, configPath)
		}
		return internalonboard.DetectStateResult{
			ConfigPath:           configPath,
			ConfigExists:         true,
			ConfigComplete:       complete,
			IMBootstrapComplete:  complete,
			ManagerAgentComplete: complete,
			ManagerBotComplete:   complete,
		}, nil
	}
	EnsureBootstrapState = func(_ context.Context, opts internalonboard.EnsureStateOptions) (internalonboard.EnsureStateResult, error) {
		ensureCalls++
		if opts.ConfigPath != configPath {
			t.Fatalf("EnsureStateOptions.ConfigPath = %q, want %q", opts.ConfigPath, configPath)
		}
		complete = true
		return internalonboard.EnsureStateResult{ConfigPath: configPath, Config: cfg}, nil
	}

	run1 := testContext()
	if err := NewServeCmd().Run(context.Background(), run1, nil, command.GlobalOptions{Config: configPath}); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	run2 := testContext()
	if err := NewServeCmd().Run(context.Background(), run2, nil, command.GlobalOptions{Config: configPath}); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	if ensureCalls != 1 {
		t.Fatalf("EnsureBootstrapState calls = %d, want 1 across repeated serve runs", ensureCalls)
	}
	if got := run1.Stderr.(*bytes.Buffer).String(); !strings.Contains(got, "auto-initializing local state") {
		t.Fatalf("first stderr missing auto-bootstrap log:\n%s", got)
	}
	if got := run2.Stderr.(*bytes.Buffer).String(); strings.Contains(got, "auto-initializing local state") {
		t.Fatalf("second stderr should not contain auto-bootstrap log:\n%s", got)
	}
}

func TestServeForegroundPassesContextToServer(t *testing.T) {
	origRunServer := RunServer
	origNewAgentService := NewAgentService
	origNewBotService := NewBotService
	origNewIMService := NewIMService
	origNewFeishuService := NewFeishuService
	origNewLLMService := NewLLMService
	origStartConfiguredAgents := StartConfiguredAgents
	origNewCodexBridgeManager := NewCodexBridgeManager
	origEnsureCLIProxy := EnsureCLIProxy
	origShutdownCLIProxy := ShutdownCLIProxy
	t.Cleanup(func() {
		RunServer = origRunServer
		NewAgentService = origNewAgentService
		NewBotService = origNewBotService
		NewIMService = origNewIMService
		NewFeishuService = origNewFeishuService
		NewLLMService = origNewLLMService
		StartConfiguredAgents = origStartConfiguredAgents
		NewCodexBridgeManager = origNewCodexBridgeManager
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
	startCalled := make(chan struct{})
	releaseStart := make(chan struct{})
	startReturned := make(chan struct{})
	startErrors := make(chan string, 4)
	StartConfiguredAgents = func(gotCtx context.Context, gotSvc *agent.Service) error {
		defer close(startReturned)
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
		if opts.Upgrade == nil {
			return fmt.Errorf("Upgrade = nil, want configured manager")
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
		`debian_registries_override = []`,
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

func TestServeForegroundStartsConfiguredAgentsOnReady(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()

	started := make(chan struct{})
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
	case <-time.After(time.Second):
		t.Fatal("StartConfiguredAgents was not called from OnReady")
	}
}

func TestServeForegroundPassesConfigPathToServer(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()

	var gotConfigPath string
	RunServer = func(opts server.Options) error {
		gotConfigPath = opts.ConfigPath
		return nil
	}

	if err := serveForegroundWithConfigPath(context.Background(), testContext(), config.Config{
		Server: config.ServerConfig{ListenAddr: "127.0.0.1:18080"},
	}, "/tmp/csgclaw.toml", "json"); err != nil {
		t.Fatalf("serveForegroundWithConfigPath() error = %v", err)
	}

	if gotConfigPath != "/tmp/csgclaw.toml" {
		t.Fatalf("RunServer config path = %q, want %q", gotConfigPath, "/tmp/csgclaw.toml")
	}
}

func TestServeForegroundStartsCodexBridgesAfterConfiguredAgents(t *testing.T) {
	restore := stubServeDependencies(t)
	defer restore()

	origNewCodexBridgeManager := NewCodexBridgeManager
	t.Cleanup(func() {
		NewCodexBridgeManager = origNewCodexBridgeManager
	})

	startedAgents := make(chan struct{})
	releaseAgents := make(chan struct{})
	bridgeStarted := make(chan struct{})
	bridgeClosed := make(chan struct{})

	StartConfiguredAgents = func(context.Context, *agent.Service) error {
		close(startedAgents)
		<-releaseAgents
		return nil
	}
	NewCodexBridgeManager = func(config.Config, *agent.Service) (codexBridgeManager, error) {
		return &fakeCodexBridgeManager{
			start: func(context.Context) error {
				select {
				case <-startedAgents:
				default:
					t.Fatal("codex bridge started before configured agents")
				}
				close(bridgeStarted)
				return nil
			},
			close: func() {
				close(bridgeClosed)
			},
		}, nil
	}
	RunServer = func(opts server.Options) error {
		if opts.OnReady == nil {
			return fmt.Errorf("OnReady is nil")
		}
		opts.OnReady()
		close(releaseAgents)
		if opts.Context == nil {
			return fmt.Errorf("Context is nil")
		}
		return nil
	}

	if err := serveForeground(context.Background(), testContext(), config.Config{Server: config.ServerConfig{ListenAddr: "127.0.0.1:18080"}}, "json"); err != nil {
		t.Fatalf("serveForeground() error = %v", err)
	}
	select {
	case <-bridgeStarted:
	case <-time.After(time.Second):
		t.Fatal("codex bridge manager did not start")
	}
	select {
	case <-bridgeClosed:
	case <-time.After(time.Second):
		t.Fatal("codex bridge manager did not close")
	}
}

func TestShouldStartCodexBridge(t *testing.T) {
	cases := []struct {
		name  string
		agent agent.Agent
		want  bool
	}{
		{
			name: "running codex worker with complete profile",
			agent: agent.Agent{
				ID:              "u-alice",
				Role:            agent.RoleWorker,
				RuntimeKind:     agent.RuntimeKindCodex,
				Status:          string(agentruntime.StateRunning),
				ProfileComplete: true,
			},
			want: true,
		},
		{
			name: "stopped worker",
			agent: agent.Agent{
				ID:              "u-alice",
				Role:            agent.RoleWorker,
				RuntimeKind:     agent.RuntimeKindCodex,
				Status:          string(agentruntime.StateStopped),
				ProfileComplete: true,
			},
		},
		{
			name: "manager is excluded",
			agent: agent.Agent{
				ID:              agent.ManagerUserID,
				Role:            agent.RoleManager,
				RuntimeKind:     agent.RuntimeKindCodex,
				Status:          string(agentruntime.StateRunning),
				ProfileComplete: true,
			},
		},
		{
			name: "non-codex worker is excluded",
			agent: agent.Agent{
				ID:              "u-alice",
				Role:            agent.RoleWorker,
				RuntimeKind:     agent.RuntimeKindPicoClawSandbox,
				Status:          string(agentruntime.StateRunning),
				ProfileComplete: true,
			},
		},
		{
			name: "incomplete profile is excluded",
			agent: agent.Agent{
				ID:          "u-alice",
				Role:        agent.RoleWorker,
				RuntimeKind: agent.RuntimeKindCodex,
				Status:      string(agentruntime.StateRunning),
			},
		},
	}

	for _, tc := range cases {
		if got := shouldStartCodexBridge(tc.agent); got != tc.want {
			t.Fatalf("%s: shouldStartCodexBridge() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestServeForegroundPreservesManagerImageOverride(t *testing.T) {
	origRunServer := RunServer
	origNewAgentService := NewAgentService
	origStartConfiguredAgents := StartConfiguredAgents
	t.Cleanup(func() {
		RunServer = origRunServer
		NewAgentService = origNewAgentService
		StartConfiguredAgents = origStartConfiguredAgents
	})
	RunServer = func(opts server.Options) error {
		if opts.OnReady != nil {
			go opts.OnReady()
		}
		return nil
	}
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
			Provider:    config.BoxLiteCLIProvider,
			HomeDirName: config.DefaultSandboxHomeDirName,
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
provider = "boxlite-cli"
home_dir_name = "boxlite"
# using default debian registries: ["harbor.opencsg.com", "docker.io"]
debian_registries_override = []

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
		Provider:                 config.BoxLiteCLIProvider,
		HomeDirName:              "sandbox-home",
		DebianRegistriesOverride: []string{"registry.a"},
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
			Provider:    "not-a-sandbox-backend",
			HomeDirName: "runtime",
		},
	})
	if err == nil {
		t.Fatal("newAgentService() error = nil, want unsupported sandbox provider")
	}
	if !strings.Contains(err.Error(), `unsupported sandbox provider "not-a-sandbox-backend"`) {
		t.Fatalf("newAgentService() error = %q, want unsupported sandbox provider", err)
	}
}

func TestNewAgentServiceRegistersCodexRuntime(t *testing.T) {
	svc, err := newAgentService(config.Config{
		Sandbox: config.SandboxConfig{
			Provider:    config.DefaultSandboxProvider,
			HomeDirName: config.DefaultSandboxHomeDirName,
		},
	})
	if err != nil {
		t.Fatalf("newAgentService() error = %v", err)
	}
	if _, err := svc.Runtime(agentruntime.KindCodex); err != nil {
		t.Fatalf("svc.Runtime(codex) error = %v", err)
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
	origStartConfiguredAgents := StartConfiguredAgents
	origNewCodexBridgeManager := NewCodexBridgeManager
	origEnsureCLIProxy := EnsureCLIProxy
	origShutdownCLIProxy := ShutdownCLIProxy
	origDetectBootstrapState := DetectBootstrapState
	origEnsureBootstrapState := EnsureBootstrapState
	origCheckModelProvider := CheckModelProvider
	origOpenBrowser := OpenBrowser
	origWaitForHealthy := WaitForHealthy
	RunServer = func(opts server.Options) error {
		if opts.OnReady != nil {
			go opts.OnReady()
		}
		return nil
	}
	NewAgentService = func(config.Config) (*agent.Service, error) { return &agent.Service{}, nil }
	NewBotService = func() (*bot.Service, error) { return &bot.Service{}, nil }
	NewIMService = func(*im.Bus) (*im.Service, error) { return nil, nil }
	NewFeishuService = func(config.Config) (*channel.FeishuService, error) { return nil, nil }
	NewLLMService = func(config.Config, *agent.Service) (*llm.Service, error) { return nil, nil }
	StartConfiguredAgents = func(context.Context, *agent.Service) error { return nil }
	NewCodexBridgeManager = func(config.Config, *agent.Service) (codexBridgeManager, error) { return nil, nil }
	EnsureCLIProxy = func(context.Context) error { return nil }
	ShutdownCLIProxy = func(context.Context) error { return nil }
	CheckModelProvider = checkModelProvider
	OpenBrowser = func(string) error { return nil }
	WaitForHealthy = func(string, time.Duration) error { return nil }
	DetectBootstrapState = func(internalonboard.DetectStateOptions) (internalonboard.DetectStateResult, error) {
		return internalonboard.DetectStateResult{
			ConfigExists:         true,
			ConfigComplete:       true,
			IMBootstrapComplete:  true,
			ManagerAgentComplete: true,
			ManagerBotComplete:   true,
		}, nil
	}
	EnsureBootstrapState = internalonboard.EnsureState
	return func() {
		RunServer = origRunServer
		NewAgentService = origNewAgentService
		NewBotService = origNewBotService
		NewIMService = origNewIMService
		NewFeishuService = origNewFeishuService
		NewLLMService = origNewLLMService
		StartConfiguredAgents = origStartConfiguredAgents
		NewCodexBridgeManager = origNewCodexBridgeManager
		EnsureCLIProxy = origEnsureCLIProxy
		ShutdownCLIProxy = origShutdownCLIProxy
		DetectBootstrapState = origDetectBootstrapState
		EnsureBootstrapState = origEnsureBootstrapState
		CheckModelProvider = origCheckModelProvider
		OpenBrowser = origOpenBrowser
		WaitForHealthy = origWaitForHealthy
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

type notifyingBuffer struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	needle string
	seen   chan struct{}
	once   sync.Once
}

func newNotifyingBuffer(needle string) *notifyingBuffer {
	return &notifyingBuffer{
		needle: needle,
		seen:   make(chan struct{}),
	}
}

func (b *notifyingBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n, err := b.buf.Write(p)
	if strings.Contains(b.buf.String(), b.needle) {
		b.once.Do(func() { close(b.seen) })
	}
	return n, err
}

func (b *notifyingBuffer) Seen() <-chan struct{} {
	return b.seen
}

func (b *notifyingBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type fakeCodexBridgeManager struct {
	start  func(context.Context) error
	ensure func(context.Context, agent.Agent) error
	stop   func(string)
	close  func()
}

func (m *fakeCodexBridgeManager) Start(ctx context.Context) error {
	if m != nil && m.start != nil {
		return m.start(ctx)
	}
	return nil
}

func (m *fakeCodexBridgeManager) EnsureAgent(ctx context.Context, a agent.Agent) error {
	if m != nil && m.ensure != nil {
		return m.ensure(ctx, a)
	}
	return nil
}

func (m *fakeCodexBridgeManager) StopAgent(agentID string) {
	if m != nil && m.stop != nil {
		m.stop(agentID)
	}
}

func (m *fakeCodexBridgeManager) Close() {
	if m != nil && m.close != nil {
		m.close()
	}
}
