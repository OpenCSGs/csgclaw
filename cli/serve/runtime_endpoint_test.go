package serve

import (
	"context"
	"net"
	"strings"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	"csgclaw/internal/server"
)

func TestServeRuntimeProfilesUseBoundListener(t *testing.T) {
	for _, mode := range []string{"automatic", "prebound", "desktop"} {
		t.Run(mode, func(t *testing.T) {
			restore := stubServeDependencies(t)
			defer restore()
			t.Setenv("HOME", t.TempDir())
			options := serveOptions{}
			wantAddr := ""
			if mode != "automatic" {
				listener, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				options.Listener = listener
				wantAddr = listener.Addr().String()
			}
			if mode == "desktop" {
				listener, err := net.Listen("tcp4", "0.0.0.0:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				options.SandboxListener = listener
				wantAddr = listener.Addr().String()
			}
			profileAddr := ""
			NewAgentService = func(cfg config.Config, _ feishu.AgentCredentialProvider) (*agent.Controller, error) {
				profileAddr = cfg.Server.ListenAddr
				return &agent.Controller{}, nil
			}
			called := false
			RunServer = func(opts server.Options) error {
				called = true
				if opts.Listener == nil {
					t.Fatal("listener was not bound before runtime construction")
				}
				if wantAddr == "" {
					wantAddr = opts.Listener.Addr().String()
				}
				if profileAddr != wantAddr || opts.ListenAddr != wantAddr || strings.HasSuffix(profileAddr, ":0") {
					t.Fatalf("runtime address = %q, server address = %q, want %q", profileAddr, opts.ListenAddr, wantAddr)
				}
				return nil
			}
			listenAddr := "127.0.0.1:63423"
			if mode == "automatic" {
				listenAddr = "127.0.0.1:0"
			}
			cfg := config.Config{Server: config.ServerConfig{ListenAddr: listenAddr}}
			if err := serveForegroundWithConfigPath(context.Background(), testContext(), cfg, "", "json", options); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("server was not started")
			}
			if cfg.Server.ListenAddr != listenAddr {
				t.Fatal("runtime listener changed the saved configuration")
			}
			if mode == "automatic" {
				listener, err := net.Listen("tcp", wantAddr)
				if err != nil {
					t.Fatalf("dynamic listener was not closed after shutdown: %v", err)
				}
				listener.Close()
			}
		})
	}
}
