package completion

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"

	"csgclaw/cli/command"
)

const (
	CommandName       = "completion"
	hiddenCommandName = "__complete"
)

var supportedShells = []string{"bash", "zsh", "fish"}

type FlagSpec struct {
	Name       string
	Short      string
	TakesValue bool
	Values     []string
}

type CommandSpec struct {
	Name     string
	Summary  string
	Hidden   bool
	Flags    []FlagSpec
	Children []CommandSpec
	Values   []string
}

type cmd struct {
	program string
	root    CommandSpec
}

type completeCmd struct {
	program string
	root    CommandSpec
}

func NewCmd(program string, root CommandSpec) command.Command {
	return cmd{program: program, root: root}
}

func NewCompleteCmd(program string, root CommandSpec) command.Command {
	return completeCmd{program: program, root: root}
}

func FullSpec() CommandSpec {
	return CommandSpec{
		Name:  "csgclaw",
		Flags: fullGlobalFlags(),
		Children: []CommandSpec{
			{
				Name:    "onboard",
				Summary: "Initialize local config and bootstrap state.",
				Flags: []FlagSpec{
					{Name: "debian-registries", TakesValue: true},
					{Name: "log-level", TakesValue: true, Values: logLevelValues()},
				},
			},
			{
				Name:    "serve",
				Summary: "Start the local HTTP server.",
				Flags: []FlagSpec{
					{Name: "daemon", Short: "d"},
					{Name: "log-level", TakesValue: true, Values: logLevelValues()},
					{Name: "log", TakesValue: true},
					{Name: "pid", TakesValue: true},
				},
			},
			{
				Name:    "stop",
				Summary: "Stop the local HTTP server.",
				Flags:   []FlagSpec{{Name: "pid", TakesValue: true}},
			},
			agentSpec(),
			modelSpec(),
			userSpec(),
			botSpec(),
			roomSpec(),
			memberSpec(),
			messageSpec(),
			completionSpec(),
		},
	}
}

func LiteSpec() CommandSpec {
	return CommandSpec{
		Name:  "csgclaw-cli",
		Flags: liteGlobalFlags(),
		Children: []CommandSpec{
			botSpec(),
			roomSpec(),
			memberSpec(),
			messageSpec(),
			completionSpec(),
		},
	}
}

func (cmd) Name() string {
	return CommandName
}

func (cmd) Summary() string {
	return "Generate shell completion scripts."
}

func (c cmd) Run(_ context.Context, run *command.Context, args []string, _ command.GlobalOptions) error {
	if len(args) == 0 || command.IsHelpArg(args[0]) {
		c.usage(run)
		return flag.ErrHelp
	}
	if len(args) != 1 {
		c.usage(run)
		return fmt.Errorf("%s completion requires exactly one shell", c.program)
	}

	shell := strings.ToLower(strings.TrimSpace(args[0]))
	if !isSupportedShell(shell) {
		c.usage(run)
		return fmt.Errorf("unsupported shell %q", args[0])
	}
	return Generate(run.Stdout, c.program, shell)
}

func (c cmd) usage(run *command.Context) {
	fmt.Fprintln(run.Stderr, "Generate shell completion scripts.")
	fmt.Fprintln(run.Stderr)
	fmt.Fprintln(run.Stderr, "Usage:")
	fmt.Fprintf(run.Stderr, "  %s completion <bash|zsh|fish>\n", c.program)
	fmt.Fprintln(run.Stderr)
	fmt.Fprintln(run.Stderr, "Examples:")
	fmt.Fprintf(run.Stderr, "  %s completion bash > /etc/bash_completion.d/%s\n", c.program, c.program)
	fmt.Fprintf(run.Stderr, "  %s completion zsh > \"${fpath[1]}/_%s\"\n", c.program, c.program)
	fmt.Fprintf(run.Stderr, "  %s completion fish > ~/.config/fish/completions/%s.fish\n", c.program, c.program)
}

func (completeCmd) Name() string {
	return hiddenCommandName
}

func (completeCmd) Summary() string {
	return "Complete shell words."
}

func (completeCmd) Hidden() bool {
	return true
}

func (c completeCmd) Run(_ context.Context, run *command.Context, args []string, _ command.GlobalOptions) error {
	for _, suggestion := range Complete(c.root, c.program, args) {
		fmt.Fprintln(run.Stdout, suggestion)
	}
	return nil
}

func Generate(w io.Writer, program, shell string) error {
	switch shell {
	case "bash":
		return generateBash(w, program)
	case "zsh":
		return generateZsh(w, program)
	case "fish":
		return generateFish(w, program)
	default:
		return fmt.Errorf("unsupported shell %q", shell)
	}
}

func Complete(root CommandSpec, program string, rawArgs []string) []string {
	args := stripProgram(rawArgs, program)
	if len(args) == 0 {
		return filterSuggestions(rootSuggestions(root), "")
	}

	current := args[len(args)-1]
	previous := args[:len(args)-1]
	node, valueFlag := resolveNode(root, previous)
	if valueFlag != nil {
		return filterSuggestions(valueFlag.Values, current)
	}
	if flag, prefix, ok := currentFlagValue(node, current); ok {
		return filterSuggestions(formatFlagValues(flag, prefix), current)
	}
	if strings.HasPrefix(current, "-") && current != "-" {
		return filterSuggestions(formatFlags(flagsForNode(node)), current)
	}

	return filterSuggestions(nodeSuggestions(node, current == ""), current)
}

func fullGlobalFlags() []FlagSpec {
	return append(liteGlobalFlags(), FlagSpec{Name: "config", TakesValue: true})
}

func liteGlobalFlags() []FlagSpec {
	return []FlagSpec{
		{Name: "endpoint", TakesValue: true},
		{Name: "token", TakesValue: true},
		{Name: "output", TakesValue: true, Values: []string{"table", "json"}},
		{Name: "version", Short: "V"},
	}
}

func agentSpec() CommandSpec {
	return CommandSpec{
		Name:    "agent",
		Summary: "Manage agents.",
		Children: []CommandSpec{
			{
				Name:    "list",
				Summary: "List agents",
				Flags: []FlagSpec{
					{Name: "filter", TakesValue: true, Values: []string{"running", "stopped"}},
				},
			},
			{
				Name:    "create",
				Summary: "Create an agent",
				Flags: []FlagSpec{
					{Name: "replace", Short: "r"},
					{Name: "force", Short: "f"},
					{Name: "id", TakesValue: true},
					{Name: "name", TakesValue: true},
					{Name: "description", TakesValue: true},
					{Name: "image", TakesValue: true},
					{Name: "profile", TakesValue: true},
				},
			},
			{Name: "start", Summary: "Start an agent"},
			{Name: "stop", Summary: "Stop an agent"},
			{
				Name:    "delete",
				Summary: "Delete agents",
				Flags: []FlagSpec{
					{Name: "all", Short: "a"},
					{Name: "force", Short: "f"},
				},
			},
			{
				Name:    "logs",
				Summary: "Show agent logs",
				Flags: []FlagSpec{
					{Name: "follow", Short: "f"},
					{Short: "n", TakesValue: true},
				},
			},
		},
	}
}

func modelSpec() CommandSpec {
	return CommandSpec{
		Name:    "model",
		Summary: "Manage model providers.",
		Children: []CommandSpec{
			{
				Name:    "auth",
				Summary: "Manage local model provider authentication",
				Children: []CommandSpec{
					{
						Name:    "login",
						Summary: "Login to codex or claude-code",
						Flags:   []FlagSpec{{Name: "no-browser"}},
						Values:  []string{"codex", "claude-code"},
					},
				},
			},
		},
	}
}

func userSpec() CommandSpec {
	return CommandSpec{
		Name:    "user",
		Summary: "Manage IM users.",
		Children: []CommandSpec{
			{Name: "list", Summary: "List users", Flags: channelFlags()},
			{
				Name:    "create",
				Summary: "Create a user",
				Flags: append(channelFlags(),
					FlagSpec{Name: "id", TakesValue: true},
					FlagSpec{Name: "name", TakesValue: true},
					FlagSpec{Name: "handle", TakesValue: true},
					FlagSpec{Name: "role", TakesValue: true},
					FlagSpec{Name: "avatar", TakesValue: true},
				),
			},
			{Name: "delete", Summary: "Remove a user", Flags: channelFlags()},
		},
	}
}

func botSpec() CommandSpec {
	return CommandSpec{
		Name:    "bot",
		Summary: "Manage bots.",
		Children: []CommandSpec{
			{
				Name:    "list",
				Summary: "List bots",
				Flags:   append(channelFlags(), FlagSpec{Name: "role", TakesValue: true, Values: roleValues()}),
			},
			{
				Name:    "create",
				Summary: "Create a bot",
				Flags: append(channelFlags(),
					FlagSpec{Name: "id", TakesValue: true},
					FlagSpec{Name: "name", TakesValue: true},
					FlagSpec{Name: "description", TakesValue: true},
					FlagSpec{Name: "role", TakesValue: true, Values: roleValues()},
					FlagSpec{Name: "model-id", TakesValue: true},
				),
			},
			{Name: "delete", Summary: "Delete a bot", Flags: channelFlags()},
		},
	}
}

func roomSpec() CommandSpec {
	return CommandSpec{
		Name:    "room",
		Summary: "Manage IM rooms.",
		Children: []CommandSpec{
			{Name: "list", Summary: "List rooms", Flags: channelFlags()},
			{
				Name:    "create",
				Summary: "Create a room",
				Flags: append(channelFlags(),
					FlagSpec{Name: "title", TakesValue: true},
					FlagSpec{Name: "description", TakesValue: true},
					FlagSpec{Name: "creator-id", TakesValue: true},
					FlagSpec{Name: "member-ids", TakesValue: true},
					FlagSpec{Name: "locale", TakesValue: true},
				),
			},
			{Name: "delete", Summary: "Delete a room", Flags: channelFlags()},
		},
	}
}

func memberSpec() CommandSpec {
	return CommandSpec{
		Name:    "member",
		Summary: "Manage IM room members.",
		Children: []CommandSpec{
			{
				Name:    "list",
				Summary: "List room members",
				Flags:   append(channelFlags(), FlagSpec{Name: "room-id", TakesValue: true}),
			},
			{
				Name:    "create",
				Summary: "Add a member to a room",
				Flags: append(channelFlags(),
					FlagSpec{Name: "room-id", TakesValue: true},
					FlagSpec{Name: "user-id", TakesValue: true},
					FlagSpec{Name: "inviter-id", TakesValue: true},
					FlagSpec{Name: "locale", TakesValue: true},
				),
			},
		},
	}
}

func messageSpec() CommandSpec {
	return CommandSpec{
		Name:    "message",
		Summary: "Manage IM messages.",
		Children: []CommandSpec{
			{
				Name:    "list",
				Summary: "List messages",
				Flags:   append(channelFlags(), FlagSpec{Name: "room-id", TakesValue: true}),
			},
			{
				Name:    "create",
				Summary: "Create a message",
				Flags: append(channelFlags(),
					FlagSpec{Name: "room-id", TakesValue: true},
					FlagSpec{Name: "sender-id", TakesValue: true},
					FlagSpec{Name: "content", TakesValue: true},
					FlagSpec{Name: "mention-id", TakesValue: true},
				),
			},
		},
	}
}

func completionSpec() CommandSpec {
	return CommandSpec{
		Name:    CommandName,
		Summary: "Generate shell completion scripts.",
		Values:  supportedShells,
	}
}

func channelFlags() []FlagSpec {
	return []FlagSpec{{Name: "channel", TakesValue: true, Values: []string{"csgclaw", "feishu"}}}
}

func logLevelValues() []string {
	return []string{"debug", "info", "warn", "error"}
}

func roleValues() []string {
	return []string{"manager", "worker"}
}

func isSupportedShell(shell string) bool {
	for _, supported := range supportedShells {
		if shell == supported {
			return true
		}
	}
	return false
}

func generateBash(w io.Writer, program string) error {
	name := safeFunctionName(program)
	_, err := fmt.Fprintf(w, `# bash completion for %[1]s

_%[2]s_completion() {
    COMPREPLY=()
    local suggestion
    while IFS= read -r suggestion; do
        COMPREPLY+=("$suggestion")
    done < <("${COMP_WORDS[0]}" __complete "${COMP_WORDS[@]}")
}

complete -F _%[2]s_completion %[1]s
`, program, name)
	return err
}

func generateZsh(w io.Writer, program string) error {
	name := safeFunctionName(program)
	_, err := fmt.Fprintf(w, `#compdef %[1]s
# zsh completion for %[1]s

_%[2]s_completion() {
    local -a completions
    local cmd="${words[1]}"
    completions=("${(@f)$("${cmd}" __complete "${words[@]}")}")
    compadd -- "${completions[@]}"
}

compdef _%[2]s_completion %[1]s
`, program, name)
	return err
}

func generateFish(w io.Writer, program string) error {
	name := safeFunctionName(program)
	_, err := fmt.Fprintf(w, `# fish completion for %[1]s

function __%[2]s_completion
    set -l tokens (commandline -opc)
    set -l current (commandline -ct)
    if test -z "$current"; or test (count $tokens) -eq 0; or test "$tokens[-1]" != "$current"
        set -a tokens "$current"
    end
    command %[1]s __complete $tokens
end

complete -c %[1]s -f -a "(__%[2]s_completion)"
`, program, name)
	return err
}

func stripProgram(args []string, program string) []string {
	if len(args) == 0 {
		return args
	}
	if isProgramToken(args[0], program) {
		return args[1:]
	}
	return args
}

func isProgramToken(token, program string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	return token == program || filepath.Base(token) == program
}

func resolveNode(root CommandSpec, args []string) (CommandSpec, *FlagSpec) {
	node := root
	var expecting *FlagSpec
	for _, arg := range args {
		if expecting != nil {
			expecting = nil
			continue
		}
		if isFlagToken(arg) {
			flag, hasValue := findFlag(node, arg)
			if flag != nil && flag.TakesValue && !hasValue {
				expecting = flag
			}
			continue
		}
		if child, ok := childByName(node, arg); ok {
			node = child
		}
	}
	return node, expecting
}

func currentFlagValue(node CommandSpec, current string) (*FlagSpec, string, bool) {
	if !isFlagToken(current) || !strings.Contains(current, "=") {
		return nil, "", false
	}
	flag, _ := findFlag(node, current)
	if flag == nil || !flag.TakesValue || len(flag.Values) == 0 {
		return nil, "", false
	}
	_, value, _ := strings.Cut(current, "=")
	return flag, value, true
}

func findFlag(node CommandSpec, token string) (*FlagSpec, bool) {
	name, hasValue, ok := flagName(token)
	if !ok {
		return nil, false
	}
	flags := flagsForNode(node)
	for i := range flags {
		flag := &flags[i]
		if flag.Name == name || flag.Short == name {
			return flag, hasValue
		}
	}
	return nil, hasValue
}

func flagName(token string) (string, bool, bool) {
	if !isFlagToken(token) {
		return "", false, false
	}
	name := strings.TrimLeft(token, "-")
	if name == "" {
		return "", false, false
	}
	name, _, hasValue := strings.Cut(name, "=")
	return name, hasValue, true
}

func isFlagToken(token string) bool {
	return strings.HasPrefix(token, "-") && token != "-"
}

func childByName(node CommandSpec, name string) (CommandSpec, bool) {
	for _, child := range node.Children {
		if child.Name == name {
			return child, true
		}
	}
	return CommandSpec{}, false
}

func rootSuggestions(root CommandSpec) []string {
	return nodeSuggestions(root, true)
}

func nodeSuggestions(node CommandSpec, includeFlags bool) []string {
	out := make([]string, 0, len(node.Children)+len(node.Values)+len(node.Flags)+2)
	for _, child := range node.Children {
		if child.Hidden {
			continue
		}
		out = append(out, child.Name)
	}
	out = append(out, node.Values...)
	if includeFlags {
		out = append(out, formatFlags(flagsForNode(node))...)
	}
	return out
}

func flagsForNode(node CommandSpec) []FlagSpec {
	flags := make([]FlagSpec, 0, len(node.Flags)+1)
	flags = append(flags, node.Flags...)
	flags = append(flags, FlagSpec{Name: "help", Short: "h"})
	return flags
}

func formatFlags(flags []FlagSpec) []string {
	out := make([]string, 0, len(flags)*2)
	for _, flag := range flags {
		if flag.Name != "" {
			out = append(out, "--"+flag.Name)
		}
		if flag.Short != "" {
			out = append(out, "-"+flag.Short)
		}
	}
	return out
}

func formatFlagValues(flag *FlagSpec, prefix string) []string {
	name := flag.Name
	if name == "" {
		name = flag.Short
	}
	marker := "--" + name + "="
	if flag.Name == "" {
		marker = "-" + name + "="
	}
	values := make([]string, 0, len(flag.Values))
	for _, value := range flag.Values {
		if strings.HasPrefix(value, prefix) {
			values = append(values, marker+value)
		}
	}
	return values
}

func filterSuggestions(suggestions []string, prefix string) []string {
	seen := make(map[string]bool, len(suggestions))
	out := make([]string, 0, len(suggestions))
	for _, suggestion := range suggestions {
		if seen[suggestion] || !strings.HasPrefix(suggestion, prefix) {
			continue
		}
		seen[suggestion] = true
		out = append(out, suggestion)
	}
	return out
}

func safeFunctionName(program string) string {
	program = strings.TrimSpace(program)
	if program == "" {
		return "csgclaw"
	}
	var b strings.Builder
	for _, r := range program {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		return "csgclaw"
	}
	return name
}
