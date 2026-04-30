package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	agentcmd "csgclaw/cli/agent"
	"csgclaw/cli/bot"
	"csgclaw/cli/command"
	completioncmd "csgclaw/cli/completion"
	"csgclaw/cli/member"
	"csgclaw/cli/message"
	modelcmd "csgclaw/cli/model"
	onboardcmd "csgclaw/cli/onboard"
	"csgclaw/cli/room"
	servecmd "csgclaw/cli/serve"
	usercmd "csgclaw/cli/user"
	appversion "csgclaw/internal/version"
)

const (
	envBaseURL     = "CSGCLAW_BASE_URL"
	envAccessToken = "CSGCLAW_ACCESS_TOKEN"
)

type App struct {
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
	httpClient HTTPClient
	commands   map[string]command.Command
	order      []command.Command
}

type GlobalOptions struct {
	Endpoint string
	Token    string
	Output   string
	Config   string
	Version  bool
}

func New() *App {
	app := &App{
		stdin:      os.Stdin,
		stdout:     os.Stdout,
		stderr:     os.Stderr,
		httpClient: &http.Client{},
	}
	app.registerDefaultCommands()
	return app
}

func (a *App) AddCommand(commands ...command.Command) {
	if a.commands == nil {
		a.commands = make(map[string]command.Command)
	}
	for _, cmd := range commands {
		if _, exists := a.commands[cmd.Name()]; !exists {
			a.order = append(a.order, cmd)
		}
		a.commands[cmd.Name()] = cmd
	}
}

func (a *App) registerDefaultCommands() {
	a.AddCommand(
		onboardcmd.NewCmd(),
		servecmd.NewServeCmd(),
		servecmd.NewStopCmd(),
		agentcmd.NewCmd(),
		modelcmd.NewCmd(),
		usercmd.NewCmd(),
		bot.NewCmd(),
		room.NewCmd(),
		member.NewCmd(),
		message.NewCmd(),
		completioncmd.NewCmd("csgclaw", completioncmd.FullSpec()),
		completioncmd.NewCompleteCmd("csgclaw", completioncmd.FullSpec()),
		servecmd.NewInternalServeCmd(),
	)
}

func (a *App) ensureDefaultCommands() {
	if a.commands == nil {
		a.registerDefaultCommands()
	}
}

func (a *App) Execute(ctx context.Context, args []string) error {
	a.ensureDefaultCommands()
	globals, rest, err := a.parseGlobalOptions(args)
	if err != nil {
		return err
	}
	if globals.Version {
		return a.printVersion(globals.Output)
	}
	if len(rest) == 0 {
		a.usage()
		return flag.ErrHelp
	}

	cmd, ok := a.commands[rest[0]]
	if !ok {
		a.usage()
		return fmt.Errorf("unknown command %q", rest[0])
	}
	return cmd.Run(ctx, a.commandContext(), rest[1:], globals.commandOptions())
}

func (a *App) parseGlobalOptions(args []string) (GlobalOptions, []string, error) {
	fs := flag.NewFlagSet("csgclaw", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	fs.Usage = a.usage

	opts := GlobalOptions{
		Endpoint: os.Getenv(envBaseURL),
		Token:    os.Getenv(envAccessToken),
	}
	fs.StringVar(&opts.Endpoint, "endpoint", opts.Endpoint, "HTTP server endpoint")
	fs.StringVar(&opts.Token, "token", opts.Token, "API authentication token")
	fs.StringVar(&opts.Output, "output", "", "output format: table or json")
	fs.StringVar(&opts.Config, "config", "", "path to config file")
	fs.BoolVar(&opts.Version, "version", false, "print version and exit")
	fs.BoolVar(&opts.Version, "V", false, "print version and exit")

	globalArgs, rest := splitLeadingFlags(args)
	if err := fs.Parse(globalArgs); err != nil {
		return GlobalOptions{}, nil, err
	}
	if opts.Output == "" {
		opts.Output = a.defaultOutput(rest)
	}
	if _, err := command.NormalizeOutput(opts.Output); err != nil {
		return GlobalOptions{}, nil, err
	}

	return opts, rest, nil
}

func splitLeadingFlags(args []string) ([]string, []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return args[:i], args[i+1:]
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return args[:i], args[i:]
		}
		if consumesValue(arg) && !strings.Contains(arg, "=") && i+1 < len(args) {
			i++
		}
	}
	return args, nil
}

func (a *App) defaultOutput(rest []string) string {
	if isSpecialOutputCommand(rest) {
		return "table"
	}
	return command.DefaultOutput(a.stdout)
}

func isSpecialOutputCommand(rest []string) bool {
	if len(rest) == 0 {
		return true
	}
	switch rest[0] {
	case "serve", "stop", "_serve":
		return true
	case "agent":
		return len(rest) > 1 && rest[1] == "logs"
	default:
		return false
	}
}

func consumesValue(arg string) bool {
	switch arg {
	case "--endpoint", "--token", "--output", "--config":
		return true
	default:
		return false
	}
}

func (a *App) usage() {
	a.ensureDefaultCommands()
	fmt.Fprintln(a.stderr, "csgclaw manages local CSGClaw agents.")
	fmt.Fprintln(a.stderr)
	fmt.Fprintln(a.stderr, "Usage:")
	fmt.Fprintln(a.stderr, "  csgclaw [global-flags] <command> [args]")
	fmt.Fprintln(a.stderr)
	fmt.Fprintln(a.stderr, "Available Commands:")
	for _, cmd := range a.order {
		if hidden, ok := cmd.(interface{ Hidden() bool }); ok && hidden.Hidden() {
			continue
		}
		fmt.Fprintf(a.stderr, "  %-8s %s\n", cmd.Name(), cmd.Summary())
	}
	fmt.Fprintln(a.stderr)
	fmt.Fprintln(a.stderr, "Examples:")
	fmt.Fprintln(a.stderr, "  csgclaw -h")
	fmt.Fprintln(a.stderr, "  csgclaw --version")
	fmt.Fprintln(a.stderr, "  csgclaw serve -h")
	fmt.Fprintln(a.stderr, "  csgclaw agent -h")
	fmt.Fprintln(a.stderr, "  csgclaw agent create -h")
	fmt.Fprintln(a.stderr, "  csgclaw message create --channel csgclaw --room-id room-1 --sender-id u-admin --content hello")
	fmt.Fprintln(a.stderr)
	fmt.Fprintln(a.stderr, "Global flags:")
	fmt.Fprintf(a.stderr, "  --endpoint string   HTTP server endpoint (default %s)\n", envBaseURL)
	fmt.Fprintf(a.stderr, "  --token string      API authentication token (default %s)\n", envAccessToken)
	fmt.Fprintln(a.stderr, "  --output string     Output format: table or json")
	fmt.Fprintln(a.stderr, "  --config string     Path to config file")
	fmt.Fprintln(a.stderr, "  --version, -V       Print version and exit")
}

func (a *App) printVersion(output string) error {
	version := appversion.Current()
	if output == "json" {
		return command.WriteJSON(a.stdout, map[string]string{
			"program": "csgclaw",
			"version": version,
		})
	}
	fmt.Fprintf(a.stdout, "csgclaw version %s\n", version)
	return nil
}

func (a *App) usageCommandGroup(command string, summary string, usageLine string, subcommands []string) {
	fmt.Fprintf(a.stderr, "%s\n\n", summary)
	fmt.Fprintln(a.stderr, "Usage:")
	fmt.Fprintf(a.stderr, "  %s\n\n", usageLine)
	fmt.Fprintln(a.stderr, "Available Subcommands:")
	for _, line := range subcommands {
		fmt.Fprintf(a.stderr, "  %s\n", line)
	}
	fmt.Fprintln(a.stderr)
	fmt.Fprintf(a.stderr, "Run `csgclaw %s <subcommand> -h` for subcommand details.\n", command)
}

func (a *App) newCommandFlagSet(name string, usageLine string, summary string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	fs.Usage = func() {
		if summary != "" {
			fmt.Fprintf(a.stderr, "%s\n\n", summary)
		}
		fmt.Fprintln(a.stderr, "Usage:")
		fmt.Fprintf(a.stderr, "  %s\n", usageLine)
		if hasFlags(fs) {
			fmt.Fprintln(a.stderr)
			fmt.Fprintln(a.stderr, "Flags:")
			fs.PrintDefaults()
		}
	}
	return fs
}

func hasFlags(fs *flag.FlagSet) bool {
	hasAny := false
	fs.VisitAll(func(*flag.Flag) {
		hasAny = true
	})
	return hasAny
}

func isHelpArg(arg string) bool {
	return arg == "-h" || arg == "--help" || arg == "help"
}

func (g GlobalOptions) commandOptions() command.GlobalOptions {
	return command.GlobalOptions{
		Endpoint: g.Endpoint,
		Token:    g.Token,
		Output:   g.Output,
		Config:   g.Config,
	}
}

func (a *App) commandContext() *command.Context {
	return &command.Context{
		Program:    "csgclaw",
		Stdin:      a.stdin,
		Stdout:     a.stdout,
		Stderr:     a.stderr,
		HTTPClient: a.httpClient,
	}
}
