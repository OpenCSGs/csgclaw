package model

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"csgclaw/cli/command"
	"csgclaw/internal/cliproxy"
)

func TestRunLoginCodex(t *testing.T) {
	restore := stubLoginProvider(func(ctx context.Context, provider string, opts cliproxy.LoginOptions) (cliproxy.AuthStatus, error) {
		if provider != "codex" {
			t.Fatalf("provider = %q, want codex", provider)
		}
		if opts.NoBrowser {
			t.Fatalf("NoBrowser = true, want false")
		}
		return cliproxy.AuthStatus{Provider: "codex", Authenticated: true, Source: "codex-home"}, nil
	})
	defer restore()

	var out bytes.Buffer
	run := &command.Context{Program: "csgclaw", Stdin: strings.NewReader(""), Stdout: &out, Stderr: &bytes.Buffer{}}
	if err := NewCmd().Run(context.Background(), run, []string{"auth", "login", "codex"}, command.GlobalOptions{Output: "table"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "codex auth ready (codex-home)") {
		t.Fatalf("output = %q, want codex ready message", got)
	}
}

func TestRunLoginClaudeCodeNoBrowser(t *testing.T) {
	restore := stubLoginProvider(func(ctx context.Context, provider string, opts cliproxy.LoginOptions) (cliproxy.AuthStatus, error) {
		if provider != "claude_code" {
			t.Fatalf("provider = %q, want claude_code", provider)
		}
		if !opts.NoBrowser {
			t.Fatalf("NoBrowser = false, want true")
		}
		return cliproxy.AuthStatus{Provider: "claude_code", Authenticated: true, Source: "oauth"}, nil
	})
	defer restore()

	var out bytes.Buffer
	run := &command.Context{Program: "csgclaw", Stdin: strings.NewReader(""), Stdout: &out, Stderr: &bytes.Buffer{}}
	if err := NewCmd().Run(context.Background(), run, []string{"auth", "login", "claude-code", "--no-browser"}, command.GlobalOptions{Output: "json"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, `"provider": "claude_code"`) || !strings.Contains(got, `"source": "oauth"`) {
		t.Fatalf("output = %q, want claude_code oauth json", got)
	}
}

func TestRunLoginRejectsUnsupportedProvider(t *testing.T) {
	run := &command.Context{Program: "csgclaw", Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	err := NewCmd().Run(context.Background(), run, []string{"auth", "login", "gemini"}, command.GlobalOptions{})
	if err == nil || !strings.Contains(err.Error(), "unsupported auth provider") {
		t.Fatalf("Run() error = %v, want unsupported provider", err)
	}
}

func TestRunLoginCancelsWhileProviderIsWaiting(t *testing.T) {
	for _, provider := range []string{"codex", "claude-code"} {
		t.Run(provider, func(t *testing.T) {
			release := make(chan struct{})
			restore := stubLoginProvider(func(ctx context.Context, provider string, opts cliproxy.LoginOptions) (cliproxy.AuthStatus, error) {
				<-release
				return cliproxy.AuthStatus{}, nil
			})
			defer restore()

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			run := &command.Context{Program: "csgclaw", Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
			err := NewCmd().Run(ctx, run, []string{"auth", "login", provider}, command.GlobalOptions{})
			close(release)

			if err == nil || !strings.Contains(err.Error(), "login canceled") {
				t.Fatalf("Run() error = %v, want login canceled", err)
			}
			if errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v, want user-facing cancellation error", err)
			}
		})
	}
}

func stubLoginProvider(fn func(context.Context, string, cliproxy.LoginOptions) (cliproxy.AuthStatus, error)) func() {
	previous := loginProvider
	loginProvider = fn
	return func() { loginProvider = previous }
}
