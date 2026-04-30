package model

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/cliproxy"
)

type cmd struct{}

type loginResult struct {
	Provider      string `json:"provider"`
	Authenticated bool   `json:"authenticated"`
	Source        string `json:"source,omitempty"`
	Message       string `json:"message,omitempty"`
}

type loginProviderResult struct {
	status cliproxy.AuthStatus
	err    error
}

var loginProvider = func(ctx context.Context, provider string, opts cliproxy.LoginOptions) (cliproxy.AuthStatus, error) {
	return cliproxy.Default().Login(ctx, provider, opts)
}

func NewCmd() command.Command {
	return cmd{}
}

func (cmd) Name() string {
	return "model"
}

func (cmd) Summary() string {
	return "Manage model providers."
}

func (c cmd) Run(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	if len(args) == 0 {
		c.usage(run)
		return flag.ErrHelp
	}
	if command.IsHelpArg(args[0]) {
		c.usage(run)
		return flag.ErrHelp
	}

	switch args[0] {
	case "auth":
		return c.runAuth(ctx, run, args[1:], globals)
	default:
		c.usage(run)
		return fmt.Errorf("unknown model subcommand %q", args[0])
	}
}

func (c cmd) usage(run *command.Context) {
	run.UsageCommandGroup(c, run.Program+" model <subcommand> [flags]", []string{
		"auth login <provider>    Login to codex or claude-code",
	})
}

func (c cmd) runAuth(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	if len(args) == 0 {
		c.usageAuth(run)
		return flag.ErrHelp
	}
	if command.IsHelpArg(args[0]) {
		c.usageAuth(run)
		return flag.ErrHelp
	}

	switch args[0] {
	case "login":
		return c.runLogin(ctx, run, args[1:], globals)
	default:
		c.usageAuth(run)
		return fmt.Errorf("unknown model auth subcommand %q", args[0])
	}
}

func (c cmd) usageAuth(run *command.Context) {
	fmt.Fprintln(run.Stderr, "Manage local model provider authentication.")
	fmt.Fprintln(run.Stderr)
	fmt.Fprintln(run.Stderr, "Usage:")
	fmt.Fprintf(run.Stderr, "  %s model auth <subcommand> [flags]\n\n", run.Program)
	fmt.Fprintln(run.Stderr, "Available Subcommands:")
	fmt.Fprintln(run.Stderr, "  login <provider>    Login to codex or claude-code")
	fmt.Fprintln(run.Stderr)
	fmt.Fprintf(run.Stderr, "Run `%s model auth <subcommand> -h` for subcommand details.\n", run.Program)
}

func (c cmd) runLogin(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("model auth login", run.Program+" model auth login <provider> [flags]", "Login to a local CLIProxy provider.")
	noBrowserDefault, args := extractNoBrowserFlag(args)
	noBrowser := fs.Bool("no-browser", noBrowserDefault, "print the OAuth URL instead of opening a browser")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("model auth login requires exactly one provider")
	}

	provider := normalizeLoginProvider(rest[0])
	if provider == "" {
		return fmt.Errorf("unsupported auth provider %q", rest[0])
	}
	status, err := cancellableLoginProvider(ctx, provider, cliproxy.LoginOptions{
		NoBrowser: *noBrowser,
		Prompt:    promptFunc(run.Stdin, run.Stdout),
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("login canceled")
		}
		return err
	}
	return renderLogin(globals.Output, run.Stdout, loginResult{
		Provider:      status.Provider,
		Authenticated: status.Authenticated,
		Source:        status.Source,
		Message:       loginMessage(status),
	})
}

func cancellableLoginProvider(ctx context.Context, provider string, opts cliproxy.LoginOptions) (cliproxy.AuthStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	resultCh := make(chan loginProviderResult, 1)
	go func() {
		status, err := loginProvider(ctx, provider, opts)
		resultCh <- loginProviderResult{status: status, err: err}
	}()

	select {
	case result := <-resultCh:
		return result.status, result.err
	case <-ctx.Done():
		return cliproxy.AuthStatus{}, ctx.Err()
	}
}

func extractNoBrowserFlag(args []string) (bool, []string) {
	filtered := make([]string, 0, len(args))
	noBrowser := false
	for _, arg := range args {
		switch arg {
		case "--no-browser", "-no-browser":
			noBrowser = true
		default:
			filtered = append(filtered, arg)
		}
	}
	return noBrowser, filtered
}

func normalizeLoginProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex":
		return "codex"
	case "claude", "claude-code", "claude_code":
		return "claude_code"
	default:
		return ""
	}
}

func promptFunc(r io.Reader, w io.Writer) func(string) (string, error) {
	reader := bufio.NewReader(r)
	return func(prompt string) (string, error) {
		if strings.TrimSpace(prompt) != "" {
			if _, err := fmt.Fprint(w, prompt); err != nil {
				return "", err
			}
		}
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}
}

func loginMessage(status cliproxy.AuthStatus) string {
	if status.Authenticated {
		if status.Source != "" {
			return fmt.Sprintf("%s auth ready (%s)", status.Provider, status.Source)
		}
		return fmt.Sprintf("%s auth ready", status.Provider)
	}
	if status.Message != "" {
		return status.Message
	}
	return fmt.Sprintf("%s auth is not configured", status.Provider)
}

func renderLogin(output string, w io.Writer, result loginResult) error {
	output, err := command.NormalizeOutput(output)
	if err != nil {
		return err
	}
	if output == "json" {
		return command.WriteJSON(w, result)
	}
	_, err = fmt.Fprintln(w, result.Message)
	return err
}
