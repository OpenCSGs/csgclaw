package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/server"
)

var (
	runServer         = server.Run
	newAgentServiceFn = newAgentService
	newIMServiceFn    = newIMService
)

func (a *App) runServe(ctx context.Context, args []string, globals GlobalOptions) error {
	fs := a.newCommandFlagSet("serve", "csgclaw serve [-d|--daemon] [flags]", "Start the local HTTP server.")
	daemon := fs.Bool("daemon", false, "run server in background")
	fs.BoolVar(daemon, "d", false, "run server in background")

	defaultLogPath, err := defaultServerLogPath()
	if err != nil {
		return err
	}
	defaultPIDPath, err := defaultServerPIDPath()
	if err != nil {
		return err
	}
	logPath := fs.String("log", defaultLogPath, "log file path, daemon mode only")
	pidPath := fs.String("pid", defaultPIDPath, "pid file path, daemon mode only")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig(globals.Config)
	if err != nil {
		return err
	}
	if err := validateModelConfig(cfg); err != nil {
		return err
	}
	if globals.Endpoint != "" {
		cfg.Server.AdvertiseBaseURL = strings.TrimRight(globals.Endpoint, "/")
	}

	if *daemon {
		return a.serveBackground(cfg, globals, *logPath, *pidPath)
	}
	return a.serveForeground(ctx, cfg)
}

func (a *App) runInternalServe(ctx context.Context, args []string, globals GlobalOptions) error {
	fs := a.newCommandFlagSet("_serve", "csgclaw _serve [flags]", "Internal server entrypoint.")
	pidPath := fs.String("pid", "", "pid file path")
	configPathFlag := fs.String("config", globals.Config, "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *pidPath != "" {
		if err := writePIDFile(*pidPath, os.Getpid()); err != nil {
			return err
		}
		defer removePIDFile(*pidPath)
	}

	cfg, err := loadConfig(*configPathFlag)
	if err != nil {
		return err
	}
	if err := validateModelConfig(cfg); err != nil {
		return err
	}
	if globals.Endpoint != "" {
		cfg.Server.AdvertiseBaseURL = strings.TrimRight(globals.Endpoint, "/")
	}

	a.printEffectiveConfig(cfg)
	svc, err := newAgentServiceFn(cfg)
	if err != nil {
		return err
	}
	imSvc, err := newIMServiceFn()
	if err != nil {
		return err
	}
	return a.startServer(ctx, cfg, svc, imSvc)
}

func (a *App) serveForeground(ctx context.Context, cfg config.Config) error {
	svc, err := newAgentServiceFn(cfg)
	if err != nil {
		return err
	}
	imSvc, err := newIMServiceFn()
	if err != nil {
		return err
	}
	apiURL := apiBaseURL(cfg.Server)
	imURL := imOpenURL(apiURL)

	a.printEffectiveConfig(cfg)
	fmt.Fprintf(a.stdout, "CSGClaw IM is available at: %s\n", imURL)
	fmt.Fprintln(a.stdout, "Open this URL in your browser after startup.")

	return a.startServer(ctx, cfg, svc, imSvc)
}

func (a *App) serveBackground(cfg config.Config, globals GlobalOptions, logPath, pidPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	childArgs := []string{"_serve", "--pid", pidPath}
	if globals.Config != "" {
		childArgs = append(childArgs, "--config", globals.Config)
	}
	cmd := exec.Command(exe, childArgs...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	apiURL := apiBaseURL(cfg.Server)
	if err := waitForHealthy(apiURL, 5*time.Second); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("server process started (pid %d) but health check failed: %w; see %s", cmd.Process.Pid, err, logPath)
	}

	fmt.Fprintf(a.stdout, "server started in background (pid %d)\n", cmd.Process.Pid)
	fmt.Fprintf(a.stdout, "im: %s\n", imOpenURL(apiURL))
	fmt.Fprintf(a.stdout, "api: %s\n", apiURL)
	fmt.Fprintf(a.stdout, "log: %s\n", logPath)
	fmt.Fprintf(a.stdout, "pid: %s\n", pidPath)
	return nil
}

func (a *App) runStop(args []string, _ GlobalOptions) error {
	fs := a.newCommandFlagSet("stop", "csgclaw stop [flags]", "Stop the local HTTP server.")
	defaultPIDPath, err := defaultServerPIDPath()
	if err != nil {
		return err
	}
	pidPath := fs.String("pid", defaultPIDPath, "pid file path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	pid, err := readPIDFile(*pidPath)
	if err != nil {
		return err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			removePIDFile(*pidPath)
			fmt.Fprintf(a.stdout, "removed stale pid file %s\n", *pidPath)
			return nil
		}
		return fmt.Errorf("signal process %d: %w", pid, err)
	}
	fmt.Fprintf(a.stdout, "sent SIGTERM to server process %d\n", pid)
	return nil
}

func defaultServerLogPath() (string, error) {
	dir, err := config.DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "server.log"), nil
}

func defaultServerPIDPath() (string, error) {
	dir, err := config.DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "server.pid"), nil
}

func writePIDFile(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	return nil
}

func readPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read pid file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse pid file: %w", err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("parse pid file: invalid pid %d", pid)
	}
	return pid, nil
}

func removePIDFile(path string) {
	_ = os.Remove(path)
}

func waitForHealthy(apiBaseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(timeout)
	url := strings.TrimRight(apiBaseURL, "/") + "/healthz"

	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if err == nil {
			lastErr = fmt.Errorf("status %s", resp.Status)
			_ = resp.Body.Close()
		} else {
			lastErr = err
		}
		time.Sleep(200 * time.Millisecond)
	}

	if lastErr == nil {
		lastErr = errors.New("timed out")
	}
	return lastErr
}

func imOpenURL(apiBaseURL string) string {
	return strings.TrimRight(apiBaseURL, "/") + "/"
}

func apiBaseURL(server config.ServerConfig) string {
	if server.AdvertiseBaseURL != "" {
		return strings.TrimRight(server.AdvertiseBaseURL, "/")
	}

	port := config.ListenPort(server.ListenAddr)
	if server.ListenAddr == "" {
		return config.DefaultAPIBaseURL()
	}
	host := "127.0.0.1"
	if parsedHost, _, err := net.SplitHostPort(server.ListenAddr); err == nil {
		if parsedHost != "" && parsedHost != "0.0.0.0" && parsedHost != "::" {
			host = parsedHost
		}
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

func (a *App) startServer(ctx context.Context, cfg config.Config, svc *agent.Service, imSvc *im.Service) error {
	imBus := im.NewBus()
	return runServer(server.Options{
		ListenAddr: cfg.Server.ListenAddr,
		Service:    svc,
		IM:         imSvc,
		IMBus:      imBus,
		PicoClaw:   im.NewPicoClawBridge(cfg.Server.AccessToken),
		Context:    ctx,
	})
}

func (a *App) printEffectiveConfig(cfg config.Config) {
	fmt.Fprintf(a.stdout, "effective config:\n%s", formatEffectiveConfig(cfg))
}

func formatEffectiveConfig(cfg config.Config) string {
	return fmt.Sprintf(`[server]
listen_addr = %q
advertise_base_url = %q
access_token = %q

[model]
base_url = %q
api_key = %q
model_id = %q

[bootstrap]
manager_image = %q
`, cfg.Server.ListenAddr, cfg.Server.AdvertiseBaseURL, partiallyMaskSecret(cfg.Server.AccessToken), cfg.Model.BaseURL, partiallyMaskSecret(cfg.Model.APIKey), cfg.Model.ModelID, cfg.Bootstrap.ManagerImage)
}

func partiallyMaskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return strings.Repeat("*", len(value))
	}
	return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
}

func loadConfig(path string) (config.Config, error) {
	if path == "" {
		return config.LoadDefault()
	}
	return config.Load(path)
}

func configPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	return config.DefaultPath()
}

func validateModelConfig(cfg config.Config) error {
	missing := cfg.Model.MissingFields()
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"model config is incomplete (%s); run `csgclaw onboard --base-url <url> --api-key <key> --model-id <model>`",
		strings.Join(missingModelFlags(missing), ", "),
	)
}

func missingModelFlags(fields []string) []string {
	flags := make([]string, 0, len(fields))
	for _, field := range fields {
		switch field {
		case "base_url":
			flags = append(flags, "--base-url")
		case "api_key":
			flags = append(flags, "--api-key")
		case "model_id":
			flags = append(flags, "--model-id")
		default:
			flags = append(flags, field)
		}
	}
	return flags
}

func newAgentService(cfg config.Config) (*agent.Service, error) {
	agentsPath, err := config.DefaultAgentsPath()
	if err != nil {
		return nil, err
	}
	return agent.NewService(cfg.Model, cfg.Server, cfg.Bootstrap.ManagerImage, agentsPath)
}

func newIMService() (*im.Service, error) {
	imStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		return nil, err
	}
	return im.NewServiceFromPath(imStatePath)
}
