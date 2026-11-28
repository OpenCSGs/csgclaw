package cliproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/modelprovider"

	"github.com/gin-gonic/gin"
	cliproxyapi "github.com/router-for-me/CLIProxyAPI/v8/sdk/api"
	cliproxysdk "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
	_ "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator/builtin"
)

const (
	LocalAPIKey       = "local"
	ProviderCodex     = "codex"
	ProviderAnthropic = "anthropic"

	reservedLegacyCLIProxyPort = 8300 + 17

	authDirName    = "cliproxy-auth"
	configDirName  = authDirName
	configFileName = "config.yaml"

	configDirEnv = "CSGCLAW_CLIPROXY_CONFIG_DIR"
	authDirEnv   = "CSGCLAW_CLIPROXY_AUTH_DIR"
	// systemProxyURLEnv carries the Electron-resolved system proxy only to the
	// embedded CLIProxy used by the Codex and Claude Code model providers. It
	// intentionally does not use HTTP_PROXY/HTTPS_PROXY, which would route all
	// Go sidecar traffic, including OpenCSG and custom providers, through it.
	systemProxyURLEnv = "CSGCLAW_CLIPROXY_SYSTEM_PROXY_URL"

	embeddedCLIProxySkipGinLogKey = "__gin_skip_request_logging__"

	// A single managed OAuth credential is the common CSGClaw setup. Absorb one
	// transient upstream failure inside CLIProxyAPI so its short model cooldown
	// does not escape to Codex as an immediate synthetic 429.
	embeddedCLIProxyRequestRetry                  = 1
	embeddedCLIProxyMaxRetryIntervalSeconds       = 30
	embeddedCLIProxyTransientErrorCooldownSeconds = 10
	embeddedCLIProxyStreamingBootstrapRetries     = 1
)

type Service struct {
	mu         sync.Mutex
	run        *serviceRun
	client     *http.Client
	quota      accountQuotas
	codexQuota modelprovider.AccountUsageCache
}

type serviceRun struct {
	ctx         context.Context
	cancel      context.CancelFunc
	startupDone chan struct{}
	done        chan struct{}
	baseURL     string // Published by closing startupDone.
	startupErr  error  // Published by closing startupDone.
	err         error  // Published by closing done.
}

func (run *serviceRun) finishStartup(baseURL string, err error) {
	run.baseURL = baseURL
	run.startupErr = err
	close(run.startupDone)
}

var defaultService = &Service{
	client: &http.Client{Timeout: 5 * time.Second},
}

func Default() *Service {
	return defaultService
}

func (s *Service) EnsureStarted(ctx context.Context) error {
	_, err := s.ensureStarted(ctx)
	return err
}

func (s *Service) ensureStarted(ctx context.Context, serverOptions ...cliproxyapi.ServerOption) (string, error) {
	if s == nil {
		return "", fmt.Errorf("cliproxy service is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	s.mu.Lock()
	run := s.run
	if run == nil {
		// Shutdown owns this lifetime. Request contexts only bound readiness
		// waits, even when the first request starts the daemon.
		runCtx, cancel := context.WithCancel(context.Background())
		run = &serviceRun{
			ctx: runCtx, cancel: cancel,
			startupDone: make(chan struct{}), done: make(chan struct{}),
		}
		s.run = run
		go s.runService(run, serverOptions)
	}
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-run.startupDone:
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if run.startupErr != nil {
			return "", run.startupErr
		}
		if err := run.ctx.Err(); err != nil {
			return "", err
		}
		return run.baseURL, nil
	}
}

func (s *Service) runService(run *serviceRun, serverOptions []cliproxyapi.ServerOption) {
	err := s.serve(run, serverOptions)
	if !errors.Is(err, context.Canceled) {
		run.err = err
	}
	run.cancel()
	s.mu.Lock()
	s.run = nil
	select {
	case <-run.startupDone:
	default:
		// Setup errors can finish the run without ever starting the SDK.
		run.finishStartup("", err)
	}
	close(run.done)
	s.mu.Unlock()
}

func (s *Service) serve(run *serviceRun, serverOptions []cliproxyapi.ServerOption) error {
	cfg, cfgPath, baseURL, err := buildConfig()
	if err != nil {
		return err
	}
	if err = writeConfigFile(cfgPath, cfg); err != nil {
		return err
	}
	importExistingAuth(run.ctx, cfg.AuthDir)
	if err := run.ctx.Err(); err != nil {
		return err
	}

	serverOptions = append(serverOptions, cliproxyapi.WithMiddleware(skipEmbeddedHealthzAccessLog()))
	svc, err := cliproxysdk.NewBuilder().
		WithConfig(cfg).
		WithConfigPath(cfgPath).
		WithServerOptions(serverOptions...).
		Build()
	if err != nil {
		return fmt.Errorf("build embedded cliproxy: %w", err)
	}

	registerAccountQuotaObserver(&s.quota)
	stopped := make(chan struct{})
	var daemonErr error
	go func() {
		daemonErr = svc.Run(run.ctx)
		close(stopped)
	}()

	if err = s.waitHealthy(run.ctx, baseURL, stopped); err != nil {
		run.cancel()
		select {
		case <-stopped:
		default:
			// Release readiness waiters while the SDK winds down. Keep this
			// run registered until it exits so another daemon cannot overlap it.
			run.finishStartup("", err)
			<-stopped
		}
		if daemonErr != nil && !errors.Is(daemonErr, context.Canceled) {
			return fmt.Errorf("embedded cliproxy stopped during startup: %w", daemonErr)
		}
		return err
	}
	run.finishStartup(baseURL, nil)
	<-stopped
	return daemonErr
}

func (s *Service) BaseURL(ctx context.Context) (string, error) {
	return s.ensureStarted(ctx)
}

func (s *Service) ProviderBaseURL(ctx context.Context, provider string) (string, error) {
	baseURL, err := s.BaseURL(ctx)
	if err != nil {
		return "", err
	}
	if _, err := s.ListModels(ctx, provider); err != nil {
		return "", err
	}
	return strings.TrimRight(baseURL, "/") + "/v1", nil
}

func (s *Service) ListModels(ctx context.Context, provider string) ([]string, error) {
	if err := s.EnsureStarted(ctx); err != nil {
		return nil, err
	}
	registryProvider := registryProvider(provider)
	if registryProvider == "" {
		return nil, fmt.Errorf("unsupported cliproxy provider %q", provider)
	}
	if err := waitForProviderModels(ctx, provider); err != nil {
		return nil, err
	}
	models := registeredModels(registryProvider)
	if len(models) == 0 {
		return nil, fmt.Errorf("no %s models registered in embedded cliproxy", registryProvider)
	}
	return models, nil
}

func (s *Service) ListModelChoices(ctx context.Context, provider string) ([]string, error) {
	models, err := s.ListModels(ctx, provider)
	if err == nil {
		return models, nil
	}
	registryProvider := registryProvider(provider)
	if registryProvider == "" {
		return nil, fmt.Errorf("unsupported cliproxy provider %q", provider)
	}
	models = fallbackModels(registryProvider)
	if len(models) > 0 {
		return models, nil
	}
	return nil, err
}

func registeredModels(provider string) []string {
	modelInfos := cliproxysdk.GlobalModelRegistry().GetAvailableModelsByProvider(provider)
	models := make([]string, 0, len(modelInfos))
	seen := make(map[string]struct{}, len(modelInfos))
	for _, model := range modelInfos {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	sort.Strings(models)
	return models
}

func waitForProviderModels(ctx context.Context, provider string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	authProvider, _, err := normalizeAuthProvider(provider)
	if err != nil {
		return err
	}
	registryProvider := registryProvider(provider)
	if registryProvider == "" {
		return fmt.Errorf("unsupported cliproxy provider %q", provider)
	}
	cfg, err := authConfig()
	if err != nil {
		return err
	}
	existing, err := findAuth(ctx, cfg.AuthDir, authProvider)
	if err != nil || existing == nil {
		return err
	}
	if len(registeredModels(registryProvider)) > 0 {
		return nil
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
		if len(registeredModels(registryProvider)) > 0 {
			return nil
		}
	}
	return fmt.Errorf("no %s models registered in embedded cliproxy after loading %s auth from %s", registryProvider, authProvider, cfg.AuthDir)
}

func fallbackModels(provider string) []string {
	switch provider {
	case ProviderCodex:
		return []string{
			"gpt-6.2-astra",
			"gpt-6.2-sol",
			"gpt-6-astra",
			"gpt-6.1-sol",
			"gpt-6.1-terra",
			"gpt-6-sol",
			"gpt-6-luna",
			"gpt-6-chat-thinking",
			"gpt-5.6-sol",
			"gpt-5.6-terra",
			"gpt-5.6-luna",
			"gpt-image-1.5",
			"gpt-image-2",
			"gpt-image-2.5",
			"gpt-image-2.5-flare",
			"gpt-image-2.5-sunburst",
			"gpt-image-2.6",
			"gpt-image-2.6-flare",
			"gpt-image-2.6-sunburst",
			"codex-auto-review",
		}
	case "claude":
		return []string{
			"claude-opus-5-5",
			"claude-sonnet-5-5",
			"claude-haiku-5-5",
			"claude-fable-5-2",
			"claude-fable-5-1",
			"claude-fable-5",
			"claude-opus-5",
			"claude-sonnet-5",
			"claude-opus-4-8",
			"claude-opus-4-7",
			"claude-opus-4-6",
			"claude-sonnet-4-6",
			"claude-opus-4-5-20251101",
			"claude-sonnet-4-5-20250929",
			"claude-haiku-4-5-20251001",
			"claude-opus-4-1-20250805",
			"claude-opus-4-20250514",
			"claude-sonnet-4-20250514",
			"claude-3-7-sonnet-20250219",
			"claude-3-5-haiku-20241022",
		}
	default:
		return nil
	}
}

func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	run := s.run
	if run != nil {
		run.cancel()
	}
	s.mu.Unlock()

	if run == nil {
		return nil
	}
	select {
	case <-run.done:
		return run.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func skipEmbeddedHealthzAccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c != nil && c.Request != nil && c.Request.URL != nil && c.Request.URL.Path == "/healthz" {
			c.Set(embeddedCLIProxySkipGinLogKey, true)
		}
		c.Next()
	}
}

func (s *Service) waitHealthy(ctx context.Context, baseURL string, stopped <-chan struct{}) error {
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()
	var lastErr error
	for healthCtx.Err() == nil {
		select {
		case <-stopped:
			return fmt.Errorf("embedded cliproxy stopped before becoming healthy")
		default:
		}

		req, err := http.NewRequestWithContext(healthCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/healthz", nil)
		if err != nil {
			return fmt.Errorf("build embedded cliproxy health request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("status %s", resp.Status)
		} else {
			lastErr = err
		}
		select {
		case <-healthCtx.Done():
		case <-poll.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("embedded cliproxy did not become healthy at %s: %w", baseURL, errors.Join(healthCtx.Err(), lastErr))
}

func buildConfig() (*sdkconfig.Config, string, string, error) {
	port, err := freePort()
	if err != nil {
		return nil, "", "", err
	}
	cfgDir, err := configDir()
	if err != nil {
		return nil, "", "", err
	}
	authDir, err := configuredAuthDir()
	if err != nil {
		return nil, "", "", err
	}
	cfg := &sdkconfig.Config{
		Host:                          "127.0.0.1",
		Port:                          port,
		AuthDir:                       authDir,
		CommercialMode:                true,
		LoggingToFile:                 false,
		RequestRetry:                  embeddedCLIProxyRequestRetry,
		MaxRetryInterval:              embeddedCLIProxyMaxRetryIntervalSeconds,
		TransientErrorCooldownSeconds: embeddedCLIProxyTransientErrorCooldownSeconds,
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{LocalAPIKey},
			Streaming: sdkconfig.StreamingConfig{
				BootstrapRetries: embeddedCLIProxyStreamingBootstrapRetries,
			},
		},
	}
	// Keep image generation on the explicit Images API; automatic injection into
	// chat bypasses Agent image-model selection and channel attachment delivery.
	if err := cfg.DisableImageGeneration.UnmarshalJSON([]byte(`"chat"`)); err != nil {
		return nil, "", "", err
	}
	cfg.ProxyURL = configuredProxyURL()
	cfg.RemoteManagement.AllowRemote = false
	cfg.RemoteManagement.SecretKey = ""
	cfg.RemoteManagement.DisableControlPanel = true
	cfg.Pprof.Enable = false
	cfg.Pprof.Addr = "127.0.0.1:0"
	cfgPath := filepath.Join(cfgDir, configFileName)
	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	return cfg, cfgPath, baseURL, nil
}

func configuredAuthDir() (string, error) {
	authDir := strings.TrimSpace(os.Getenv(authDirEnv))
	if authDir == "" {
		dir, err := config.DefaultDomainDir(authDirName)
		if err != nil {
			return "", fmt.Errorf("resolve embedded cliproxy auth dir: %w", err)
		}
		return dir, nil
	}
	expanded, err := expandHomePath(authDir)
	if err != nil {
		return "", fmt.Errorf("resolve embedded cliproxy auth dir: %w", err)
	}
	return expanded, nil
}

func expandHomePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}

func configDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv(configDirEnv)); dir != "" {
		return dir, nil
	}
	return config.DefaultDomainDir(configDirName)
}

func configuredProxyURL() string {
	for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", systemProxyURLEnv} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func writeConfigFile(path string, cfg *sdkconfig.Config) error {
	if cfg == nil {
		return fmt.Errorf("embedded cliproxy config is nil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create embedded cliproxy config dir: %w", err)
	}
	lines := []string{
		"host: " + yamlString(cfg.Host),
		"port: " + strconv.Itoa(cfg.Port),
		"auth-dir: " + yamlString(cfg.AuthDir),
		"api-keys:",
		"  - " + yamlString(LocalAPIKey),
		"request-retry: " + strconv.Itoa(cfg.RequestRetry),
		"max-retry-interval: " + strconv.Itoa(cfg.MaxRetryInterval),
		"transient-error-cooldown-seconds: " + strconv.Itoa(cfg.TransientErrorCooldownSeconds),
		"streaming:",
		"  bootstrap-retries: " + strconv.Itoa(cfg.Streaming.BootstrapRetries),
	}
	if proxyURL := strings.TrimSpace(cfg.ProxyURL); proxyURL != "" {
		lines = append(lines, "proxy-url: "+yamlString(proxyURL))
	}
	lines = append(lines,
		"remote-management:",
		"  allow-remote: false",
		"  secret-key: "+yamlString(""),
		"  disable-control-panel: true",
		"pprof:",
		"  enable: false",
		"  addr: "+yamlString("127.0.0.1:0"),
		"commercial-mode: true",
		"logging-to-file: false",
		"",
	)
	content := strings.Join(lines, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write embedded cliproxy config: %w", err)
	}
	return nil
}

func yamlString(value string) string {
	return strconv.Quote(value)
}

func freePort() (int, error) {
	for attempt := 0; attempt < 16; attempt++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, fmt.Errorf("allocate embedded cliproxy port: %w", err)
		}
		addr, ok := ln.Addr().(*net.TCPAddr)
		_ = ln.Close()
		if !ok || addr.Port <= 0 {
			return 0, fmt.Errorf("allocate embedded cliproxy port: unexpected address %v", ln.Addr())
		}
		if addr.Port != reservedLegacyCLIProxyPort {
			return addr.Port, nil
		}
	}
	return 0, fmt.Errorf("allocate embedded cliproxy port: refused reserved legacy CLIProxy port repeatedly")
}

func registryProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderCodex:
		return ProviderCodex
	case "claude_code", "claude-code", "claude", ProviderAnthropic:
		return "claude"
	default:
		return ""
	}
}
