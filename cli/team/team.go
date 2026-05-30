package team

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"csgclaw/cli/command"
	"csgclaw/internal/apitypes"
)

type cmd struct{}

func NewCmd() command.Command {
	return cmd{}
}

func (cmd) Name() string {
	return "team"
}

func (cmd) Summary() string {
	return "Manage agent teams."
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
	case "list":
		return c.runList(ctx, run, args[1:], globals)
	case "create":
		return c.runCreate(ctx, run, args[1:], globals)
	case "task":
		return c.runTask(ctx, run, args[1:], globals)
	case "approval":
		return c.runApproval(ctx, run, args[1:], globals)
	default:
		c.usage(run)
		return fmt.Errorf("unknown team subcommand %q", args[0])
	}
}

func (c cmd) usage(run *command.Context) {
	run.UsageCommandGroup(c, run.Program+" team <subcommand> [flags]", []string{
		"list                        List teams",
		"create                      Create a team or enable team mode on a room",
		"task list                   List tasks for a team",
		"task create-batch           Create tasks from a JSON file",
		"task claim-next             Claim the next available task",
		"task update                 Update a task status",
		"approval list               List approvals for a team",
		"approval create             Create an approval request",
		"approval resolve            Resolve an approval request",
	})
}

func (c cmd) runList(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("team list", run.Program+" team list", "List teams.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("team list does not accept positional arguments")
	}
	items, err := run.APIClient(globals).ListTeams(ctx)
	if err != nil {
		return err
	}
	return command.RenderTeams(globals.Output, run.Stdout, items)
}

func (c cmd) runCreate(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("team create", run.Program+" team create [flags]", "Create a team or enable team mode on an existing room.")
	channel := fs.String("channel", "csgclaw", "channel name")
	roomID := fs.String("room-id", "", "existing room id")
	title := fs.String("title", "", "team title")
	leadBotID := fs.String("lead-bot-id", "", "lead bot id")
	memberBotIDs := fs.String("member-bot-ids", "", "comma-separated worker bot ids")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("team create does not accept positional arguments")
	}
	if *leadBotID == "" {
		return fmt.Errorf("lead_bot_id is required")
	}
	item, err := run.APIClient(globals).CreateTeam(ctx, apitypes.CreateTeamRequest{
		Channel:      *channel,
		RoomID:       *roomID,
		Title:        *title,
		LeadBotID:    *leadBotID,
		MemberBotIDs: command.ParseCSV(*memberBotIDs),
	})
	if err != nil {
		return err
	}
	return command.RenderTeams(globals.Output, run.Stdout, []apitypes.Team{item})
}

func (c cmd) runTask(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	if len(args) == 0 || command.IsHelpArg(args[0]) {
		run.UsageCommandGroup(subcommandGroup("team task", "Manage team tasks."), run.Program+" team task <subcommand> [flags]", []string{
			"list                        List tasks for a team",
			"create-batch                Create tasks from a JSON file",
			"assign                      Reassign a task to a worker",
			"claim-next                  Claim the next available task",
			"update                      Update a task status",
		})
		return flag.ErrHelp
	}
	switch args[0] {
	case "list":
		return c.runTaskList(ctx, run, args[1:], globals)
	case "create-batch":
		return c.runTaskCreateBatch(ctx, run, args[1:], globals)
	case "assign":
		return c.runTaskAssign(ctx, run, args[1:], globals)
	case "claim-next":
		return c.runTaskClaimNext(ctx, run, args[1:], globals)
	case "update":
		return c.runTaskUpdate(ctx, run, args[1:], globals)
	default:
		return fmt.Errorf("unknown team task subcommand %q", args[0])
	}
}

func (c cmd) runTaskList(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("team task list", run.Program+" team task list --team <id>", "List tasks for a team.")
	teamID := fs.String("team", "", "team id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("team task list does not accept positional arguments")
	}
	if *teamID == "" {
		return fmt.Errorf("team is required")
	}
	items, err := run.APIClient(globals).ListTeamTasks(ctx, *teamID)
	if err != nil {
		return err
	}
	return command.RenderTeamTasks(globals.Output, run.Stdout, items)
}

func (c cmd) runTaskCreateBatch(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("team task create-batch", run.Program+" team task create-batch --team <id> --created-by <bot> --file <tasks.json>", "Create a batch of tasks from a JSON file.")
	teamID := fs.String("team", "", "team id")
	createdBy := fs.String("created-by", "", "creator bot id")
	filePath := fs.String("file", "", "path to tasks JSON file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("team task create-batch does not accept positional arguments")
	}
	if *teamID == "" {
		return fmt.Errorf("team is required")
	}
	if *createdBy == "" {
		return fmt.Errorf("created_by is required")
	}
	if *filePath == "" {
		return fmt.Errorf("file is required")
	}
	data, err := os.ReadFile(*filePath)
	if err != nil {
		return err
	}
	var req apitypes.CreateTeamTasksBatchRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("decode batch file: %w", err)
	}
	req.CreatedBy = *createdBy
	resp, err := run.APIClient(globals).CreateTeamTasksBatch(ctx, *teamID, req)
	if err != nil {
		return err
	}
	return command.RenderTeamTasks(globals.Output, run.Stdout, resp.Tasks)
}

func (c cmd) runTaskClaimNext(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("team task claim-next", run.Program+" team task claim-next [flags]", "Claim the next available task.")
	teamID := fs.String("team", "", "team id")
	botID := fs.String("bot-id", "", "worker bot id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("team task claim-next does not accept positional arguments")
	}
	if *botID == "" {
		return fmt.Errorf("bot_id is required")
	}
	item, err := run.APIClient(globals).ClaimNextTeamTask(ctx, apitypes.ClaimNextTeamTaskRequest{
		TeamID: *teamID,
		BotID:  *botID,
	})
	if err != nil {
		return err
	}
	return command.RenderTeamTasks(globals.Output, run.Stdout, []apitypes.TeamTask{item})
}

func (c cmd) runTaskAssign(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("team task assign", run.Program+" team task assign --team <id> --task <id> --bot-id <bot> --actor-id <actor>", "Assign a team task to a worker.")
	teamID := fs.String("team", "", "team id")
	taskID := fs.String("task", "", "task id")
	botID := fs.String("bot-id", "", "worker bot id")
	actorID := fs.String("actor-id", "", "actor id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("team task assign does not accept positional arguments")
	}
	if *teamID == "" || *taskID == "" || *botID == "" || *actorID == "" {
		return fmt.Errorf("team, task, bot_id, and actor_id are required")
	}
	item, err := run.APIClient(globals).AssignTeamTask(ctx, *teamID, *taskID, *actorID, *botID)
	if err != nil {
		return err
	}
	return command.RenderTeamTasks(globals.Output, run.Stdout, []apitypes.TeamTask{item})
}

func (c cmd) runTaskUpdate(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("team task update", run.Program+" team task update --team <id> --task <id> --actor-id <bot> --status <status>", "Update a team task status.")
	teamID := fs.String("team", "", "team id")
	taskID := fs.String("task", "", "task id")
	actorID := fs.String("actor-id", "", "actor bot id")
	status := fs.String("status", "", "new status: blocked, completed, or failed")
	result := fs.String("result", "", "task result text")
	errorText := fs.String("error", "", "task error text")
	reason := fs.String("reason", "", "blocking reason")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("team task update does not accept positional arguments")
	}
	if *teamID == "" || *taskID == "" || *actorID == "" || *status == "" {
		return fmt.Errorf("team, task, actor_id, and status are required")
	}
	item, err := run.APIClient(globals).UpdateTeamTask(ctx, *teamID, *taskID, *actorID, apitypes.PatchTeamTaskRequest{
		Status: *status,
		Result: *result,
		Error:  *errorText,
		Reason: *reason,
	})
	if err != nil {
		return err
	}
	return command.RenderTeamTasks(globals.Output, run.Stdout, []apitypes.TeamTask{item})
}

func (c cmd) runApproval(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	if len(args) == 0 || command.IsHelpArg(args[0]) {
		run.UsageCommandGroup(subcommandGroup("team approval", "Manage team approvals."), run.Program+" team approval <subcommand> [flags]", []string{
			"list                        List approvals for a team",
			"create                      Create an approval request",
			"resolve                     Resolve an approval request",
		})
		return flag.ErrHelp
	}
	switch args[0] {
	case "list":
		return c.runApprovalList(ctx, run, args[1:], globals)
	case "create":
		return c.runApprovalCreate(ctx, run, args[1:], globals)
	case "resolve":
		return c.runApprovalResolve(ctx, run, args[1:], globals)
	default:
		return fmt.Errorf("unknown team approval subcommand %q", args[0])
	}
}

func (c cmd) runApprovalList(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("team approval list", run.Program+" team approval list --team <id>", "List approvals for a team.")
	teamID := fs.String("team", "", "team id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("team approval list does not accept positional arguments")
	}
	if *teamID == "" {
		return fmt.Errorf("team is required")
	}
	items, err := run.APIClient(globals).ListTeamApprovals(ctx, *teamID)
	if err != nil {
		return err
	}
	return command.RenderTeamApprovals(globals.Output, run.Stdout, items)
}

func (c cmd) runApprovalCreate(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("team approval create", run.Program+" team approval create --team <id> --requested-by <bot> --kind <kind> --summary <text>", "Create an approval request.")
	teamID := fs.String("team", "", "team id")
	taskID := fs.String("task-id", "", "task id")
	requestedBy := fs.String("requested-by", "", "requesting bot id")
	approverID := fs.String("approver-id", "", "approver bot id")
	kind := fs.String("kind", "", "approval kind")
	summary := fs.String("summary", "", "approval summary")
	payload := fs.String("payload", "", "approval payload")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("team approval create does not accept positional arguments")
	}
	if *teamID == "" || *requestedBy == "" || *kind == "" || *summary == "" {
		return fmt.Errorf("team, requested_by, kind, and summary are required")
	}
	item, err := run.APIClient(globals).CreateTeamApproval(ctx, *teamID, apitypes.CreateTeamApprovalRequest{
		TaskID:      *taskID,
		RequestedBy: *requestedBy,
		ApproverID:  *approverID,
		Kind:        *kind,
		Summary:     *summary,
		Payload:     *payload,
	})
	if err != nil {
		return err
	}
	return command.RenderTeamApprovals(globals.Output, run.Stdout, []apitypes.TeamApproval{item})
}

func (c cmd) runApprovalResolve(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("team approval resolve", run.Program+" team approval resolve --team <id> --approval <id> --status <status>", "Resolve an approval request.")
	teamID := fs.String("team", "", "team id")
	approvalID := fs.String("approval", "", "approval id")
	approverID := fs.String("approver-id", "", "approver bot id")
	status := fs.String("status", "", "approval status")
	reason := fs.String("reason", "", "resolution reason")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("team approval resolve does not accept positional arguments")
	}
	if *teamID == "" || *approvalID == "" || *status == "" {
		return fmt.Errorf("team, approval, and status are required")
	}
	item, err := run.APIClient(globals).ResolveTeamApproval(ctx, *teamID, *approvalID, apitypes.ResolveTeamApprovalRequest{
		ApproverID: *approverID,
		Status:     *status,
		Reason:     *reason,
	})
	if err != nil {
		return err
	}
	return command.RenderTeamApprovals(globals.Output, run.Stdout, []apitypes.TeamApproval{item})
}

type commandGroup struct {
	name    string
	summary string
}

func subcommandGroup(name, summary string) commandGroup {
	return commandGroup{name: name, summary: summary}
}

func (g commandGroup) Name() string    { return g.name }
func (g commandGroup) Summary() string { return g.summary }
func (g commandGroup) Run(context.Context, *command.Context, []string, command.GlobalOptions) error {
	return flag.ErrHelp
}
