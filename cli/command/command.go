package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"

	"golang.org/x/term"
)

type Command interface {
	Name() string
	Summary() string
	Run(context.Context, *Context, []string, GlobalOptions) error
}

type GlobalOptions struct {
	Endpoint string
	Token    string
	Output   string
	Config   string
}

type ActionResult struct {
	Command         string   `json:"command,omitempty"`
	Action          string   `json:"action,omitempty"`
	Status          string   `json:"status"`
	ID              string   `json:"id,omitempty"`
	Channel         string   `json:"channel,omitempty"`
	Message         string   `json:"message,omitempty"`
	PID             int      `json:"pid,omitempty"`
	IMURL           string   `json:"im_url,omitempty"`
	APIURL          string   `json:"api_url,omitempty"`
	LogPath         string   `json:"log_path,omitempty"`
	PIDPath         string   `json:"pid_path,omitempty"`
	ConfigPath      string   `json:"config_path,omitempty"`
	ManagerImage    string   `json:"manager_image,omitempty"`
	Users           []string `json:"users,omitempty"`
	ForceRecreated  bool     `json:"force_recreated,omitempty"`
	Logs            string   `json:"logs,omitempty"`
	Lines           int      `json:"lines,omitempty"`
	Follow          bool     `json:"follow,omitempty"`
	EffectiveConfig string   `json:"effective_config,omitempty"`
}

type Context struct {
	Program    string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	HTTPClient apiclient.HTTPClient
}

func (c *Context) APIClient(globals GlobalOptions) *apiclient.Client {
	return apiclient.New(globals.Endpoint, globals.Token, c.HTTPClient)
}

func (c *Context) UsageCommandGroup(cmd Command, usageLine string, subcommands []string) {
	fmt.Fprintf(c.Stderr, "%s\n\n", cmd.Summary())
	fmt.Fprintln(c.Stderr, "Usage:")
	fmt.Fprintf(c.Stderr, "  %s\n\n", usageLine)
	fmt.Fprintln(c.Stderr, "Available Subcommands:")
	for _, line := range subcommands {
		fmt.Fprintf(c.Stderr, "  %s\n", line)
	}
	fmt.Fprintln(c.Stderr)
	fmt.Fprintf(c.Stderr, "Run `%s %s <subcommand> -h` for subcommand details.\n", c.Program, cmd.Name())
}

func (c *Context) NewFlagSet(name string, usageLine string, summary string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	fs.Usage = func() {
		if summary != "" {
			fmt.Fprintf(c.Stderr, "%s\n\n", summary)
		}
		fmt.Fprintln(c.Stderr, "Usage:")
		fmt.Fprintf(c.Stderr, "  %s\n", usageLine)
		if HasFlags(fs) {
			fmt.Fprintln(c.Stderr)
			fmt.Fprintln(c.Stderr, "Flags:")
			fs.PrintDefaults()
		}
	}
	return fs
}

func IsHelpArg(arg string) bool {
	return arg == "-h" || arg == "--help" || arg == "help"
}

func HasFlags(fs *flag.FlagSet) bool {
	hasAny := false
	fs.VisitAll(func(*flag.Flag) {
		hasAny = true
	})
	return hasAny
}

func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func NormalizeOutput(output string) (string, error) {
	switch output {
	case "", "table":
		return "table", nil
	case "json":
		return "json", nil
	default:
		return "", fmt.Errorf("unsupported output format %q", output)
	}
}

func DefaultOutput(w io.Writer) string {
	file, ok := w.(interface{ Fd() uintptr })
	if !ok {
		return "table"
	}
	if term.IsTerminal(int(file.Fd())) {
		return "table"
	}
	return "json"
}

func RenderAction(output string, w io.Writer, result ActionResult) error {
	output, err := NormalizeOutput(output)
	if err != nil {
		return err
	}
	if output == "json" {
		return WriteJSON(w, result)
	}
	if result.Message != "" {
		_, err := fmt.Fprintln(w, result.Message)
		return err
	}

	tw := NewTableWriter(w)
	fmt.Fprintln(tw, "COMMAND\tACTION\tSTATUS\tID")
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", result.Command, result.Action, result.Status, displayBotField(result.ID))
	return tw.Flush()
}

func RenderBots(output string, w io.Writer, bots []apitypes.Bot) error {
	switch output {
	case "", "table":
		return RenderBotsTable(w, bots)
	case "json":
		return WriteJSON(w, bots)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func RenderAgents(output string, w io.Writer, agents []apitypes.Agent) error {
	switch output {
	case "", "table":
		return RenderAgentsTable(w, agents)
	case "json":
		return WriteJSON(w, agents)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func RenderRooms(output string, w io.Writer, rooms []apitypes.Room) error {
	switch output {
	case "", "table":
		return RenderRoomsTable(w, rooms)
	case "json":
		return WriteJSON(w, rooms)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func RenderUsers(output string, w io.Writer, users []apitypes.User) error {
	switch output {
	case "", "table":
		return RenderUsersTable(w, users)
	case "json":
		return WriteJSON(w, users)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func RenderMessages(output string, w io.Writer, messages []apitypes.Message) error {
	switch output {
	case "", "table":
		return RenderMessagesTable(w, messages)
	case "json":
		return WriteJSON(w, messages)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func RenderAgentsTable(w io.Writer, agents []apitypes.Agent) error {
	tw := NewTableWriter(w)
	fmt.Fprintln(tw, "ID\tNAME\tROLE\tSTATUS\tPROFILE")
	for _, a := range agents {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", a.ID, a.Name, a.Role, a.Status, displayAgentProfile(a.Profile))
	}
	return tw.Flush()
}

func displayAgentProfile(profile string) string {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return "-"
	}
	return profile
}

func RenderBotsTable(w io.Writer, bots []apitypes.Bot) error {
	tw := NewTableWriter(w)
	fmt.Fprintln(tw, "ID\tNAME\tDESCRIPTION\tROLE\tCHANNEL\tAGENT\tUSER\tAVAILABLE")
	for _, b := range bots {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%t\n", b.ID, b.Name, displayBotDescription(b.Description), b.Role, b.Channel, displayBotField(b.AgentID), displayBotField(b.UserID), b.Available)
	}
	return tw.Flush()
}

func displayBotDescription(value string) string {
	const maxRunes = 40

	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}

	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func displayBotField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func RenderRoomsTable(w io.Writer, rooms []apitypes.Room) error {
	tw := NewTableWriter(w)
	fmt.Fprintln(tw, "ID\tTITLE\tPARTICIPANTS\tMESSAGES")
	for _, room := range rooms {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\n", room.ID, room.Title, len(room.Participants), len(room.Messages))
	}
	return tw.Flush()
}

func RenderUsersTable(w io.Writer, users []apitypes.User) error {
	tw := NewTableWriter(w)
	fmt.Fprintln(tw, "ID\tNAME\tHANDLE\tROLE\tONLINE")
	for _, user := range users {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\n", user.ID, user.Name, user.Handle, user.Role, user.IsOnline)
	}
	return tw.Flush()
}

func RenderMessagesTable(w io.Writer, messages []apitypes.Message) error {
	tw := NewTableWriter(w)
	fmt.Fprintln(tw, "ID\tSENDER\tKIND\tCONTENT")
	for _, message := range messages {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", message.ID, message.SenderID, message.Kind, message.Content)
	}
	return tw.Flush()
}

func NewTableWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}

func ParseCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}
