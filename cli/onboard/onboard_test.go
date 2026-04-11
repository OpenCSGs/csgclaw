package onboard

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/cli/command"
	"csgclaw/internal/bot"
	"csgclaw/internal/config"
)

func TestRunRequiresLLMFlagsForFirstTimeSetup(t *testing.T) {
	origCreateManager := CreateManagerBot
	origEnsureIMBootstrapState := EnsureIMBootstrapState
	t.Cleanup(func() {
		CreateManagerBot = origCreateManager
		EnsureIMBootstrapState = origEnsureIMBootstrapState
	})

	CreateManagerBot = func(context.Context, string, string, config.Config, bool) (bot.Bot, error) {
		t.Fatal("bot manager create should not run when config is incomplete")
		return bot.Bot{}, nil
	}
	EnsureIMBootstrapState = func(string) error {
		t.Fatal("IM bootstrap should not run when config is incomplete")
		return nil
	}

	run := testContext()
	err := NewCmd().Run(context.Background(), run, nil, command.GlobalOptions{Config: filepath.Join(t.TempDir(), "config.toml")})
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "--base-url") || !strings.Contains(err.Error(), "--api-key") || !strings.Contains(err.Error(), "--models") {
		t.Fatalf("Run() error = %q, want all required LLM flags", err)
	}
}

func TestRunReusesExistingLLMConfig(t *testing.T) {
	origCreateManager := CreateManagerBot
	origEnsureIMBootstrapState := EnsureIMBootstrapState
	t.Cleanup(func() {
		CreateManagerBot = origCreateManager
		EnsureIMBootstrapState = origEnsureIMBootstrapState
	})

	callCount := 0
	CreateManagerBot = func(_ context.Context, _, _ string, cfg config.Config, _ bool) (bot.Bot, error) {
		callCount++
		if got, want := cfg.Models.Default, "default.gpt-test"; got != want {
			t.Fatalf("cfg.Models.Default = %q, want %q", got, want)
		}
		provider, ok := cfg.Models.Providers["default"]
		if !ok {
			t.Fatal(`cfg.Models.Providers["default"] missing`)
		}
		if provider.BaseURL != "http://llm.test" || provider.APIKey != "secret" {
			t.Fatalf("provider config = %#v, want preserved base_url/api_key", provider)
		}
		if got, want := strings.Join(provider.Models, ","), "gpt-test,gpt-test-mini"; got != want {
			t.Fatalf("provider models = %q, want %q", got, want)
		}
		if got, want := provider.ReasoningEffort, "medium"; got != want {
			t.Fatalf("provider reasoning_effort = %q, want %q", got, want)
		}
		if cfg.Model.BaseURL != "http://llm.test" || cfg.Model.APIKey != "secret" || cfg.Model.ModelID != "gpt-test" || cfg.Model.ReasoningEffort != "medium" {
			t.Fatalf("model config = %#v, want resolved default values", cfg.Model)
		}
		return bot.Bot{}, nil
	}
	EnsureIMBootstrapState = func(string) error { return nil }

	configPath := filepath.Join(t.TempDir(), "config.toml")
	run := testContext()
	cmd := NewCmd()

	args := []string{
		"--base-url", "http://llm.test",
		"--api-key", "secret",
		"--models", "gpt-test,gpt-test-mini",
		"--reasoning-effort", "medium",
	}
	if err := cmd.Run(context.Background(), run, args, command.GlobalOptions{Config: configPath}); err != nil {
		t.Fatalf("initial Run() error = %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		`[models]`,
		`default = "default.gpt-test"`,
		`[models.providers.default]`,
		`models = ["gpt-test", "gpt-test-mini"]`,
		`reasoning_effort = "medium"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("saved config missing %q:\n%s", want, content)
		}
	}

	if err := cmd.Run(context.Background(), run, nil, command.GlobalOptions{Config: configPath}); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	if callCount != 2 {
		t.Fatalf("agent bootstrap call count = %d, want 2", callCount)
	}
}

func testContext() *command.Context {
	return &command.Context{
		Program: "csgclaw",
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}
}
