package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"csgclaw/internal/agent"
	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"
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

type Context struct {
	Program    string
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

func RenderAgents(output string, w io.Writer, agents []agent.Agent) error {
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

func RenderAgentsTable(w io.Writer, agents []agent.Agent) error {
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
	fmt.Fprintln(tw, "ID\tNAME\tROLE\tCHANNEL\tAGENT\tUSER")
	for _, b := range bots {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", b.ID, b.Name, b.Role, b.Channel, b.AgentID, b.UserID)
	}
	return tw.Flush()
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
