package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/server"
)

func TestServeForegroundPassesContextToServer(t *testing.T) {
	origRunServer := runServer
	origNewAgentService := newAgentServiceFn
	origNewIMService := newIMServiceFn
	t.Cleanup(func() {
		runServer = origRunServer
		newAgentServiceFn = origNewAgentService
		newIMServiceFn = origNewIMService
	})

	ctx := context.WithValue(context.Background(), struct{}{}, "serve-context")

	newAgentServiceFn = func(config.Config) (*agent.Service, error) {
		return nil, nil
	}
	newIMServiceFn = func() (*im.Service, error) {
		return nil, nil
	}

	called := false
	runServer = func(opts server.Options) error {
		called = true
		if opts.Context != ctx {
			t.Fatalf("Context = %v, want %v", opts.Context, ctx)
		}
		return nil
	}

	app := &App{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
	}
	cfg := config.Config{
		Server: config.ServerConfig{
			ListenAddr:       "127.0.0.1:18080",
			AdvertiseBaseURL: "http://example.test",
			AccessToken:      "pc-secret",
		},
		Model: config.ModelConfig{
			BaseURL: "http://llm.test",
			APIKey:  "sk-secret",
			ModelID: "model-test",
		},
		Bootstrap: config.BootstrapConfig{
			ManagerImage: "ghcr.io/example/manager:latest",
		},
	}

	if err := app.serveForeground(ctx, cfg); err != nil {
		t.Fatalf("serveForeground() error = %v", err)
	}
	if !called {
		t.Fatal("runServer was not called")
	}

	got := app.stdout.(*bytes.Buffer).String()
	for _, want := range []string{
		"effective config:\n",
		`listen_addr = "127.0.0.1:18080"`,
		`advertise_base_url = "http://example.test"`,
		`api_key = "sk*****et"`,
		`access_token = "pc*****et"`,
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
	if !strings.Contains(err.Error(), "--base-url") || !strings.Contains(err.Error(), "--api-key") || !strings.Contains(err.Error(), "--model-id") {
		t.Fatalf("validateModelConfig() error = %q, want missing model flags", err)
	}
}
