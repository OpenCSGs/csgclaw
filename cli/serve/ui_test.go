package serve

import (
	"context"
	"slices"
	"strings"
	"testing"

	"csgclaw/cli/command"
	"csgclaw/internal/config"
	"csgclaw/internal/server"
)

func TestAPIOnlyPropagatesToDaemon(t *testing.T) {
	args := backgroundServeArgs("cfg.toml", "pid", "info", serveOptions{APIOnly: true, OpenBrowser: true})
	if !slices.Contains(args, "--api-only") || !slices.Contains(args, "--no-auth-detect") || slices.Contains(args, "--browser") {
		t.Fatalf("daemon arguments: %v", args)
	}
}

func TestAPIOnlyDoesNotInstallUIOrAdvertiseIM(t *testing.T) {
	for _, output := range []string{"table", "json"} {
		t.Run(output, func(t *testing.T) {
			restore := stubServeDependencies(t)
			defer restore()
			called := false
			RunServer = func(opts server.Options) error {
				called = true
				if opts.UI != nil {
					t.Fatal("API-only mode installed a UI")
				}
				return nil
			}
			run := testContext()
			cfg := config.Config{Server: config.ServerConfig{AdvertiseBaseURL: "http://example.test/base"}}
			if err := serveForegroundWithConfigPath(context.Background(), run, cfg, "", output, serveOptions{APIOnly: true, OpenBrowser: true}); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("backend did not start")
			}
			text := run.Stdout.(interface{ String() string }).String()
			if strings.Contains(text, "IM is available") || strings.Contains(text, `"im_url"`) || !strings.Contains(text, "http://example.test/base") {
				t.Fatalf("API-only startup output: %s", text)
			}
		})
	}
}

func TestUISelectionMatchesBuild(t *testing.T) {
	handler, err := (serveOptions{}).uiHandler()
	if err != nil {
		t.Fatal(err)
	}
	if got := handler != nil; got != embeddedUIAvailable {
		t.Fatalf("default UI present=%v, build supports UI=%v", got, embeddedUIAvailable)
	}
	handler, err = (serveOptions{APIOnly: true}).uiHandler()
	if handler != nil || err != nil {
		t.Fatal("API-only selected a UI")
	}
	if !embeddedUIAvailable {
		args := backgroundServeArgs("cfg.toml", "pid", "info", serveOptions{OpenBrowser: true})
		if !slices.Contains(args, "--api-only") || slices.Contains(args, "--browser") {
			t.Fatalf("headless daemon arguments: %v", args)
		}
		err := NewDesktopServeCmd().Run(context.Background(), testContext(), nil, command.GlobalOptions{})
		if err == nil || !strings.Contains(err.Error(), "requires a build with the bundled UI") {
			t.Fatalf("headless desktop entry: %v", err)
		}
	}
}
