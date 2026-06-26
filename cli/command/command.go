package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"unicode"

	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"

	"golang.org/x/term"
	textwidth "golang.org/x/text/width"
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

	return renderTable(w,
		[]tableColumn{
			{Header: "COMMAND"},
			{Header: "ACTION"},
			{Header: "STATUS"},
			{Header: "ID"},
		},
		[][]string{{result.Command, result.Action, result.Status, displayValueField(result.ID)}},
	)
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

func RenderParticipants(output string, w io.Writer, participants []apitypes.Participant) error {
	switch output {
	case "", "table":
		return RenderParticipantsTable(w, participants)
	case "json":
		return WriteJSON(w, participants)
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

func RenderTeams(output string, w io.Writer, teams []apitypes.Team) error {
	switch output {
	case "", "table":
		return RenderTeamsTable(w, teams)
	case "json":
		return WriteJSON(w, teams)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func RenderTeamTasks(output string, w io.Writer, tasks []apitypes.TeamTask) error {
	switch output {
	case "", "table":
		return RenderTeamTasksTable(w, tasks)
	case "json":
		return WriteJSON(w, tasks)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func RenderTeamApprovals(output string, w io.Writer, approvals []apitypes.TeamApproval) error {
	switch output {
	case "", "table":
		return RenderTeamApprovalsTable(w, approvals)
	case "json":
		return WriteJSON(w, approvals)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func RenderAgentsTable(w io.Writer, agents []apitypes.Agent) error {
	showParticipants := agentsHaveParticipants(agents)
	columns := []tableColumn{
		{Header: "ID"},
		{Header: "NAME"},
		{Header: "ROLE"},
		{Header: "STATUS"},
		{Header: "RUNTIME"},
		{Header: "MODEL"},
	}
	if showParticipants {
		columns = append(columns, tableColumn{Header: "PARTICIPANTS"})
	}
	columns = append(columns, tableColumn{Header: "IMAGE"})
	rows := make([][]string, 0, len(agents))
	for _, a := range agents {
		status := displayAgentStatus(a)
		runtimeKind := displayAgentRuntime(a)
		model := displayAgentModel(a)
		row := []string{a.ID, a.Name, a.Role, status, runtimeKind, model}
		if showParticipants {
			row = append(row, displayParticipantList(a.ParticipantNames, a.ParticipantIDs))
		}
		row = append(row, displayAgentField(a.Image))
		rows = append(rows, row)
	}
	return renderTable(w, columns, rows)
}

func agentsHaveParticipants(agents []apitypes.Agent) bool {
	for _, item := range agents {
		if len(item.ParticipantNames) > 0 || len(item.ParticipantIDs) > 0 || len(item.Participants) > 0 {
			return true
		}
	}
	return false
}

func displayAgentField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func displayAgentStatus(a apitypes.Agent) string {
	if status := strings.TrimSpace(a.Status); status != "" {
		return status
	}
	return displayAgentField(a.Runtime.State)
}

func displayAgentRuntime(a apitypes.Agent) string {
	if runtimeKind := strings.TrimSpace(a.RuntimeKind); runtimeKind != "" {
		return runtimeKind
	}
	return displayAgentField(a.Runtime.Kind)
}

func displayAgentModel(a apitypes.Agent) string {
	if profile := strings.TrimSpace(a.Profile); profile != "" {
		return profile
	}
	providerID := strings.TrimSpace(a.ProfileConfig.ModelProviderID)
	modelID := strings.TrimSpace(a.ProfileConfig.ModelID)
	switch {
	case providerID != "" && modelID != "":
		return providerID + "." + modelID
	case modelID != "":
		return modelID
	default:
		return "-"
	}
}

func RenderParticipantsTable(w io.Writer, participants []apitypes.Participant) error {
	rows := make([][]string, 0, len(participants))
	for _, p := range participants {
		rows = append(rows, []string{
			displayValueField(p.ID),
			displayValueField(p.Name),
			displayValueField(p.Type),
			displayValueField(p.Channel),
			displayNameWithID(p.AgentName, p.AgentID),
			displayNameWithID(p.UserName, firstNonEmpty(p.UserID, p.ChannelUserRef)),
			displayValueField(p.ChannelAppRef),
			displayValueField(p.LifecycleStatus),
		})
	}
	return renderTable(w,
		[]tableColumn{
			{Header: "ID"},
			{Header: "NAME"},
			{Header: "TYPE"},
			{Header: "CHANNEL"},
			{Header: "AGENT"},
			{Header: "USER"},
			{Header: "APP_REF"},
			{Header: "STATUS"},
		},
		rows,
	)
}

func displayValueField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func displayNameWithID(name, id string) string {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	switch {
	case name != "" && id != "" && name != id:
		return name + "(" + id + ")"
	case name != "":
		return name
	case id != "":
		return id
	default:
		return "-"
	}
}

func displayParticipantList(names, ids []string) string {
	if len(names) == 0 && len(ids) == 0 {
		return "-"
	}
	if len(names) == 0 {
		return displayList(ids)
	}
	values := make([]string, 0, len(names))
	for i, name := range names {
		id := ""
		if i < len(ids) {
			id = ids[i]
		}
		values = append(values, displayNameWithID(name, id))
	}
	return strings.Join(values, ",")
}

func displayList(values []string) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return "-"
	}
	return strings.Join(out, ",")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func RenderRoomsTable(w io.Writer, rooms []apitypes.Room) error {
	showNames := roomsHaveMemberNames(rooms)
	columns := []tableColumn{
		{Header: "ID"},
		{Header: "TITLE"},
		{Header: "DIRECT"},
		{Header: "MEMBERS"},
	}
	if showNames {
		columns = append(columns, tableColumn{Header: "MEMBER_NAMES"})
	}
	columns = append(columns, tableColumn{Header: "MESSAGES"})
	rows := make([][]string, 0, len(rooms))
	for _, room := range rooms {
		row := []string{room.ID, room.Title, fmt.Sprintf("%t", room.IsDirect), fmt.Sprintf("%d", len(room.Members))}
		if showNames {
			row = append(row, displayList(room.MemberNames))
		}
		row = append(row, fmt.Sprintf("%d", len(room.Messages)))
		rows = append(rows, row)
	}
	return renderTable(w, columns, rows)
}

func roomsHaveMemberNames(rooms []apitypes.Room) bool {
	for _, room := range rooms {
		if len(room.MemberNames) > 0 {
			return true
		}
	}
	return false
}

func RenderUsersTable(w io.Writer, users []apitypes.User) error {
	rows := make([][]string, 0, len(users))
	for _, user := range users {
		rows = append(rows, []string{user.ID, user.Name, user.Role, fmt.Sprintf("%t", user.IsOnline)})
	}
	return renderTable(w,
		[]tableColumn{
			{Header: "ID"},
			{Header: "NAME"},
			{Header: "ROLE"},
			{Header: "ONLINE"},
		},
		rows,
	)
}

func RenderMessagesTable(w io.Writer, messages []apitypes.Message) error {
	rows := make([][]string, 0, len(messages))
	for _, message := range messages {
		rows = append(rows, []string{message.ID, message.SenderID, message.Kind, message.Content})
	}
	return renderTable(w,
		[]tableColumn{
			{Header: "ID"},
			{Header: "SENDER"},
			{Header: "KIND"},
			{Header: "CONTENT"},
		},
		rows,
	)
}

func RenderTeamsTable(w io.Writer, teams []apitypes.Team) error {
	rows := make([][]string, 0, len(teams))
	for _, item := range teams {
		rows = append(rows, []string{displayValueField(item.ID), displayValueField(item.RoomID), displayValueField(item.Channel), displayNameWithID(item.LeadAgentName, item.LeadAgentID), displayValueField(item.Status), displayValueField(item.Title)})
	}
	return renderTable(w,
		[]tableColumn{
			{Header: "ID"},
			{Header: "ROOM"},
			{Header: "CHANNEL"},
			{Header: "LEAD_AGENT"},
			{Header: "STATUS"},
			{Header: "TITLE"},
		},
		rows,
	)
}

func RenderTeamTasksTable(w io.Writer, tasks []apitypes.TeamTask) error {
	rows := make([][]string, 0, len(tasks))
	for _, item := range tasks {
		rows = append(rows, []string{displayValueField(item.ID), displayValueField(item.TeamID), displayValueField(item.Status), displayNameWithID(item.AssignedToAgentName, item.AssignedTo), displayNameWithID(item.ClaimedByAgentName, item.ClaimedBy), fmt.Sprintf("%d", item.Priority), displayValueField(item.Title)})
	}
	return renderTable(w,
		[]tableColumn{
			{Header: "ID"},
			{Header: "TEAM"},
			{Header: "STATUS"},
			{Header: "ASSIGNED"},
			{Header: "CLAIMED"},
			{Header: "PRIORITY"},
			{Header: "TITLE"},
		},
		rows,
	)
}

func RenderTeamApprovalsTable(w io.Writer, approvals []apitypes.TeamApproval) error {
	rows := make([][]string, 0, len(approvals))
	for _, item := range approvals {
		rows = append(rows, []string{displayValueField(item.ID), displayValueField(item.TeamID), displayValueField(item.TaskID), displayValueField(item.Status), displayValueField(item.RequestedBy), displayValueField(item.ApproverID), displayValueField(item.Summary)})
	}
	return renderTable(w,
		[]tableColumn{
			{Header: "ID"},
			{Header: "TEAM"},
			{Header: "TASK"},
			{Header: "STATUS"},
			{Header: "REQUESTED_BY"},
			{Header: "APPROVER"},
			{Header: "SUMMARY"},
		},
		rows,
	)
}

type tableColumn struct {
	Header string
}

func renderTable(w io.Writer, columns []tableColumn, rows [][]string) error {
	if len(columns) == 0 {
		return nil
	}
	table := make([][]string, 0, len(rows)+1)
	header := make([]string, len(columns))
	for i, col := range columns {
		header[i] = col.Header
	}
	table = append(table, header)
	table = append(table, rows...)

	widths := make([]int, len(columns))
	cells := make([][]string, len(table))
	for rowIdx, row := range table {
		cells[rowIdx] = make([]string, len(columns))
		for colIdx := range columns {
			cell := ""
			if colIdx < len(row) {
				cell = row[colIdx]
			}
			cell = cleanTableCell(cell)
			cells[rowIdx][colIdx] = cell
			if width := displayWidth(cell); width > widths[colIdx] {
				widths[colIdx] = width
			}
		}
	}

	for _, row := range cells {
		for colIdx, cell := range row {
			if _, err := io.WriteString(w, cell); err != nil {
				return err
			}
			if colIdx == len(row)-1 {
				continue
			}
			padding := widths[colIdx] - displayWidth(cell) + 2
			if padding < 2 {
				padding = 2
			}
			if _, err := io.WriteString(w, strings.Repeat(" ", padding)); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func cleanTableCell(value string) string {
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer("\t", " ", "\r", " ", "\n", " ")
	return replacer.Replace(value)
}

func displayWidth(value string) int {
	width := 0
	for _, r := range value {
		width += runeDisplayWidth(r)
	}
	return width
}

func runeDisplayWidth(r rune) int {
	if r == 0 || unicode.IsControl(r) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
		return 0
	}
	switch textwidth.LookupRune(r).Kind() {
	case textwidth.EastAsianFullwidth, textwidth.EastAsianWide:
		return 2
	default:
		return 1
	}
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
