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

	cliproxysdk "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	_ "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator/builtin"
)

const (
	LocalAPIKey       = "local"
	ProviderCodex     = "codex"
	ProviderAnthropic = "anthropic"

	reservedLegacyCLIProxyPort = 8300 + 17

	configDirName  = "cliproxy"
	configFileName = "config.yaml"

	configDirEnv = "CSGCLAW_CLIPROXY_CONFIG_DIR"
	authDirEnv   = "CSGCLAW_CLIPROXY_AUTH_DIR"
)

type Service struct {
	mu      sync.Mutex
	started bool
	baseURL string
	cancel  context.CancelFunc
	errCh   chan error
	client  *http.Client
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

	svc, err := cliproxysdk.NewBuilder().
		WithConfig(cfg).
		WithConfigPath(cfgPath).
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
	rawProvider := provider
	baseURL, err := s.BaseURL(ctx)
	if err != nil {
		return "", err
	}
	provider = providerPath(provider)
	if provider == "" {
		return "", fmt.Errorf("unsupported cliproxy provider %q", rawProvider)
	}
	return strings.TrimRight(baseURL, "/") + "/api/provider/" + provider + "/v1", nil
}

func (s *Service) ListModels(ctx context.Context, provider string) ([]string, error) {
	if err := s.EnsureStarted(ctx); err != nil {
		return nil, err
	}
	registryProvider := registryProvider(provider)
	if registryProvider == "" {
		return nil, fmt.Errorf("unsupported cliproxy provider %q", provider)
	}
	models := registeredModels(registryProvider)
	if len(models) > 0 {
		return models, nil
	}
	return nil, fmt.Errorf("no %s models registered in embedded cliproxy", registryProvider)
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

func fallbackModels(provider string) []string {
	switch provider {
	case ProviderCodex:
		return []string{
			"gpt-5.5",
			"gpt-5.4",
			"gpt-5.4-mini",
			"gpt-5.3-codex",
			"gpt-5.3-codex-spark",
			"gpt-5.2",
			"codex-auto-review",
		}
	case "claude":
		return []string{
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
	s.started = false
	s.baseURL = ""
	s.cancel = nil
	s.errCh = nil
	s.mu.Unlock()

	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
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
		Host:           "127.0.0.1",
		Port:           port,
		AuthDir:        authDir,
		CommercialMode: true,
		LoggingToFile:  false,
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{LocalAPIKey},
		},
	}
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
		authDir = "~/.cli-proxy-api"
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
	dir, err := config.DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configDirName), nil
}

func writeConfigFile(path string, cfg *sdkconfig.Config) error {
	if cfg == nil {
		return fmt.Errorf("embedded cliproxy config is nil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create embedded cliproxy config dir: %w", err)
	}
	content := strings.Join([]string{
		"host: " + yamlString(cfg.Host),
		"port: " + strconv.Itoa(cfg.Port),
		"auth-dir: " + yamlString(cfg.AuthDir),
		"api-keys:",
		"  - " + yamlString(LocalAPIKey),
		"remote-management:",
		"  allow-remote: false",
		"  secret-key: " + yamlString(""),
		"  disable-control-panel: true",
		"pprof:",
		"  enable: false",
		"  addr: " + yamlString("127.0.0.1:0"),
		"commercial-mode: true",
		"logging-to-file: false",
		"",
	}, "\n")
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

func providerPath(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderCodex:
		return ProviderCodex
	case "claude_code", "claude-code", "claude", ProviderAnthropic:
		return ProviderAnthropic
	default:
		return ""
	}
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
