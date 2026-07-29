package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	"csgclaw/cli/command"
	"csgclaw/internal/agent"
	appbootstrap "csgclaw/internal/app"
	"csgclaw/internal/config"
	"csgclaw/internal/desktop"
	"csgclaw/internal/server"
)

const (
	maxDesktopControlMessageBytes = 64 * 1024
	desktopParentEOFGracePeriod   = 1500 * time.Millisecond
	desktopRendererListenAddr     = "127.0.0.1:18791"
)

func (desktopServeCmd) Name() string {
	return "_desktop-serve"
}

func (desktopServeCmd) Summary() string {
	return "Internal Electron sidecar entrypoint."
}

func (desktopServeCmd) Hidden() bool {
	return true
}

func (desktopServeCmd) Run(parent context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("_desktop-serve", run.Program+" _desktop-serve [flags]", "Internal Electron sidecar entrypoint.")
	configPathFlag := fs.String("config", globals.Config, "config file path")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn, error")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := validateServeInstallation(); err != nil {
		return err
	}

	restoreLogger, err := configureServeLogger(run.Stderr, *logLevel)
	if err != nil {
		return err
	}
	defer restoreLogger()

	reader := bufio.NewReaderSize(run.Stdin, maxDesktopControlMessageBytes)
	bootstrap, err := readDesktopBootstrap(reader)
	if err != nil {
		return err
	}

	instanceLock, err := appbootstrap.AcquireInstanceLock()
	if err != nil {
		return err
	}
	defer instanceLock.Release()

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	go monitorDesktopControlChannel(ctx, cancel, reader)

	configPath := strings.TrimSpace(*configPathFlag)
	if configPath == "" {
		configPath, err = config.DefaultPath()
		if err != nil {
			return err
		}
	}
	if err := ensureServeBootstrapState(ctx, configPath, false); err != nil {
		return err
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	rendererListener, sandboxListener, err := listenDesktopEndpoints()
	if err != nil {
		return err
	}
	defer rendererListener.Close()
	defer sandboxListener.Close()

	// Agent runtime configuration uses the sandbox listener, while Electron only
	// receives the separate Renderer loopback URL in the ready message.
	cfg.Server.ListenAddr = sandboxListener.Addr().String()
	cfg.Server.AdvertiseBaseURL = ""
	rendererBaseURL := "http://" + rendererListener.Addr().String()
	sandboxManagerBaseURL := agent.ResolveManagerBaseURLForSandboxProvider(
		cfg.Server,
		cfg.Sandbox.Resolved().Provider,
	)

	ready, err := desktop.NewReadyMessage(bootstrap.InstanceID, rendererBaseURL)
	if err != nil {
		return err
	}

	return serveForegroundWithConfigPath(ctx, run, cfg, configPath, "", serveOptions{
		NoBrowser:       true,
		Quiet:           true,
		Distribution:    "electron",
		Listener:        rendererListener,
		SandboxListener: sandboxListener,
		Desktop: &server.DesktopOptions{
			BaseURL:           ready.BaseURL,
			SessionToken:      bootstrap.SessionToken,
			ServerAccessToken: cfg.Server.AccessToken,
			ServerAccessHosts: desktopServerAccessHosts(sandboxManagerBaseURL, sandboxListener.Addr()),
		},
		OnReady: func() {
			if err := json.NewEncoder(run.Stdout).Encode(ready); err != nil {
				cancel()
			}
		},
	})
}

func listenDesktopEndpoints() (net.Listener, net.Listener, error) {
	rendererListener, err := net.Listen("tcp4", desktopRendererListenAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("listen on desktop renderer loopback address: %w", err)
	}
	sandboxListener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		_ = rendererListener.Close()
		return nil, nil, fmt.Errorf("listen on desktop sandbox address: %w", err)
	}
	return rendererListener, sandboxListener, nil
}

func desktopServerAccessHosts(runtimeBaseURL string, listenerAddr net.Addr) []string {
	if listenerAddr == nil {
		return nil
	}
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(runtimeBaseURL), "/"))
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Hostname() == "" {
		return nil
	}
	_, listenerPort, err := net.SplitHostPort(listenerAddr.String())
	if err != nil || parsed.Port() != listenerPort {
		return nil
	}
	loopbackHost := net.JoinHostPort("127.0.0.1", listenerPort)
	if strings.EqualFold(parsed.Host, loopbackHost) {
		return []string{loopbackHost}
	}
	return []string{loopbackHost, parsed.Host}
}

func readDesktopBootstrap(reader *bufio.Reader) (desktop.BootstrapMessage, error) {
	line, err := reader.ReadString('\n')
	if err != nil && !(err == io.EOF && len(line) > 0) {
		return desktop.BootstrapMessage{}, fmt.Errorf("read desktop bootstrap message: %w", err)
	}
	if len(line) > maxDesktopControlMessageBytes {
		return desktop.BootstrapMessage{}, fmt.Errorf("desktop bootstrap message is too large")
	}

	var message desktop.BootstrapMessage
	if err := json.Unmarshal([]byte(line), &message); err != nil {
		return desktop.BootstrapMessage{}, fmt.Errorf("decode desktop bootstrap message: %w", err)
	}
	if err := message.Validate(); err != nil {
		return desktop.BootstrapMessage{}, err
	}
	return message, nil
}

func monitorDesktopControlChannel(ctx context.Context, cancel context.CancelFunc, reader *bufio.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), maxDesktopControlMessageBytes)
	for scanner.Scan() {
		var message desktop.ControlMessage
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			continue
		}
		if message.Type == desktop.MessageTypeShutdown {
			cancel()
			return
		}
	}

	timer := time.NewTimer(desktopParentEOFGracePeriod)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
		cancel()
	}
}
