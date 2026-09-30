package cliproxy

import (
	"context"
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

	"github.com/gin-gonic/gin"
	cliproxyapi "github.com/router-for-me/CLIProxyAPI/v8/sdk/api"
	sdkhandlers "github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
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
	mu      sync.Mutex
	started bool
	baseURL string
	cancel  context.CancelFunc
	errCh   chan error
	client  *http.Client
	catalog *modelCatalog
}

var defaultService = &Service{
	client: &http.Client{Timeout: 5 * time.Second},
}

func Default() *Service {
	return defaultService
}

func (s *Service) EnsureStarted(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("cliproxy service is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	if s.started {
		baseURL := s.baseURL
		errCh := s.errCh
		s.mu.Unlock()
		return s.waitHealthy(ctx, baseURL, errCh)
	}

	cfg, cfgPath, baseURL, err := buildConfig()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if err = writeConfigFile(cfgPath, cfg); err != nil {
		s.mu.Unlock()
		return err
	}
	importExistingAuth(ctx, cfg.AuthDir)

	catalog := &modelCatalog{}
	svc, err := cliproxysdk.NewBuilder().
		WithConfig(cfg).
		WithConfigPath(cfgPath).
		WithServerOptions(
			cliproxyapi.WithMiddleware(skipEmbeddedHealthzAccessLog()),
			cliproxyapi.WithRouterConfigurator(func(_ *gin.Engine, handler *sdkhandlers.BaseAPIHandler, _ *sdkconfig.Config) {
				catalog.mu.Lock()
				catalog.manager = handler.AuthManager
				catalog.mu.Unlock()
				cliproxysdk.SetGlobalModelRegistryHook(catalog)
			}),
		).
		Build()
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("build embedded cliproxy: %w", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		err := svc.Run(runCtx)
		if err == context.Canceled {
			err = nil
		}
		errCh <- err
	}()

	s.started = true
	s.baseURL = baseURL
	s.cancel = cancel
	s.errCh = errCh
	s.catalog = catalog
	s.mu.Unlock()

	if err = s.waitHealthy(ctx, baseURL, errCh); err != nil {
		_ = s.Shutdown(context.Background())
		return err
	}
	return nil
}

func (s *Service) BaseURL(ctx context.Context) (string, error) {
	if err := s.EnsureStarted(ctx); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.baseURL, nil
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
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		s.reconcileModelCatalog(ctx)
		models := registeredModels(registryProvider)
		if len(models) == 0 {
			return nil, fmt.Errorf("no %s models registered in embedded cliproxy", registryProvider)
		}
		if catalogComplete(registryProvider, models) {
			return models, nil
		}
		// SDK startup and credential reload can rebind the embedded catalog
		// while its asynchronous registration hook is supplementing it.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, fmt.Errorf("waiting for latest %s model catalog", registryProvider)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (s *Service) reconcileModelCatalog(ctx context.Context) {
	s.mu.Lock()
	catalog := s.catalog
	s.mu.Unlock()
	if catalog != nil {
		catalog.reconcile(ctx)
	}
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
			"gpt-6-astra",
			"gpt-6.1-sol",
			"gpt-6-sol",
			"gpt-6-luna",
			"gpt-5.6-sol",
			"gpt-5.6-terra",
			"gpt-5.6-luna",
			"gpt-5.5",
			"gpt-5.4",
			"gpt-5.4-mini",
			"gpt-5.3-codex-spark",
			"gpt-image-1.5",
			"gpt-image-2",
			"gpt-image-2.5",
			"gpt-image-2.5-flare",
			"gpt-image-2.5-sunburst",
			"codex-auto-review",
		}
	case "claude":
		return []string{
			"claude-opus-5-5",
			"claude-sonnet-5-5",
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
	cancel := s.cancel
	errCh := s.errCh
	catalog := s.catalog
	s.started = false
	s.baseURL = ""
	s.cancel = nil
	s.errCh = nil
	s.catalog = nil
	s.mu.Unlock()

	if cancel == nil {
		return nil
	}
	if catalog != nil {
		catalog.stop()
		cliproxysdk.SetGlobalModelRegistryHook(nil)
	}
	cancel()
	select {
	case err := <-errCh:
		return err
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

func (s *Service) waitHealthy(ctx context.Context, baseURL string, errCh <-chan error) error {
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			if err == nil {
				return nil
			}
			return fmt.Errorf("embedded cliproxy stopped during startup: %w", err)
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/healthz", nil)
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
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timed out")
	}
	return fmt.Errorf("embedded cliproxy did not become healthy at %s: %w", baseURL, lastErr)
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
