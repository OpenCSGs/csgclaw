package task

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/taskcore"
)

type cmd struct{}

func NewCmd() command.Command {
	return cmd{}
}

func (cmd) Name() string {
	return "task"
}

func (cmd) Summary() string {
	return "Manage tasks across rooms, teams, and agents."
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
	case "claim":
		return c.runClaim(ctx, run, args[1:], globals)
	case "update":
		return c.runUpdate(ctx, run, args[1:], globals)
	case "context", "submit", "get", "plan", "start", "dispatch", "review", "message", "stop", "recover", "report", "retry-delivery":
		return c.runRoomAction(ctx, run, args[0], args[1:], globals)
	default:
		c.usage(run)
		return fmt.Errorf("unknown task subcommand %q", args[0])
	}
}

func (c cmd) usage(run *command.Context) {
	run.UsageCommandGroup(c, run.Program+" task <subcommand> [flags]", []string{
		"list                        List global tasks or tasks in --room",
		"create                      Create an agent task",
		"claim                       Claim a task; assignment is resolved from its id",
		"update                      Update a task; assignment is resolved from its id",
		"context                     Inspect an on-demand room's task context",
		"submit                      Create a Manager parent task in --room",
		"get                          Read a task with assignment-specific details",
		"plan | start | dispatch     Plan or schedule room work",
		"review | report             Review or summarize room work",
		"message                     Send a task-scoped message",
		"stop | recover              Control a room task execution",
		"retry-delivery              Retry persisted task messages in --room",
	})
}

func (c cmd) runList(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("task list", run.Program+" task list", "List global tasks.")
	roomID := fs.String("room", "", "limit the list to one room")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("task list does not accept positional arguments")
	}
	client := run.APIClient(globals)
	if strings.TrimSpace(*roomID) != "" {
		items, err := client.ListRoomTasks(ctx, strings.TrimSpace(*roomID))
		if err != nil {
			return err
		}
		return command.WriteJSON(run.Stdout, items)
	}
	items, err := client.ListGlobalTasks(ctx)
	if err != nil {
		return err
	}
	return command.RenderGlobalTasks(globals.Output, run.Stdout, items)
}

func (c cmd) runCreate(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("task create", run.Program+" task create --agent-id <agent> --title <title>", "Create an agent task.")
	agentID := fs.String("agent-id", "", "agent id")
	title := fs.String("title", "", "task title")
	body := fs.String("body", "", "task body")
	createdBy := fs.String("created-by", "manager", "creator participant id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("task create does not accept positional arguments")
	}
	if strings.TrimSpace(*agentID) == "" || strings.TrimSpace(*title) == "" {
		return fmt.Errorf("agent_id and title are required")
	}
	item, err := run.APIClient(globals).CreateAgentTask(ctx, apitypes.CreateAgentTaskRequest{
		AgentID:   strings.TrimSpace(*agentID),
		Title:     strings.TrimSpace(*title),
		Body:      strings.TrimSpace(*body),
		CreatedBy: strings.TrimSpace(*createdBy),
	})
	if err != nil {
		return err
	}
	return command.RenderTasks(globals.Output, run.Stdout, []apitypes.TeamTask{item})
}

func (c cmd) runClaim(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("task claim", run.Program+" task claim --task <id> [--attempt <n>] [--actor-id <participant>]", "Claim a task; room tasks derive the actor from the runtime caller.")
	taskID := fs.String("task", "", "task id")
	actorID := fs.String("actor-id", "", "worker participant id")
	attempt := fs.Int("attempt", 0, "room task execution attempt")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("task claim does not accept positional arguments")
	}
	if strings.TrimSpace(*taskID) == "" {
		return fmt.Errorf("task is required")
	}
	client := run.APIClient(globals)
	assignment, err := client.GetTask(ctx, *taskID)
	if err != nil {
		return err
	}
	switch assignment.AssignmentType {
	case taskcore.AssignmentTypeRoom:
		if *attempt < 1 {
			return fmt.Errorf("attempt is required for room tasks")
		}
		item, err := client.ClaimRoomTask(ctx, assignment.AssignmentID, *taskID, *attempt)
		if err != nil {
			return err
		}
		return command.WriteJSON(run.Stdout, item)
	case taskcore.AssignmentTypeTeam:
		if strings.TrimSpace(*actorID) == "" {
			return fmt.Errorf("actor_id is required for team tasks")
		}
		item, err := client.ClaimTeamTask(ctx, assignment.AssignmentID, *taskID, strings.TrimSpace(*actorID))
		if err != nil {
			return err
		}
		return command.RenderTeamTasks(globals.Output, run.Stdout, []apitypes.TeamTask{item})
	case taskcore.AssignmentTypeAgent:
		if strings.TrimSpace(*actorID) == "" {
			return fmt.Errorf("actor_id is required for agent tasks")
		}
		item, err := client.ClaimAgentTask(ctx, *taskID, strings.TrimSpace(*actorID))
		if err != nil {
			return err
		}
		return command.RenderTasks(globals.Output, run.Stdout, []apitypes.TeamTask{item})
	default:
		return fmt.Errorf("task %s has unsupported assignment_type %q", *taskID, assignment.AssignmentType)
	}
}

func (c cmd) runUpdate(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("task update", run.Program+" task update --task <id> --status <status> [--attempt <n>] [--actor-id <participant>]", "Update a task; room tasks derive the actor from the runtime caller.")
	taskID := fs.String("task", "", "task id")
	actorID := fs.String("actor-id", "", "actor participant id")
	status := fs.String("status", "", "new status: blocked, completed, or failed")
	result := fs.String("result", "", "task result text")
	errorText := fs.String("error", "", "task error text")
	reason := fs.String("reason", "", "blocking reason")
	attempt := fs.Int("attempt", 0, "room task execution attempt")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("task update does not accept positional arguments")
	}
	if strings.TrimSpace(*taskID) == "" || strings.TrimSpace(*status) == "" {
		return fmt.Errorf("task and status are required")
	}
	if !isSupportedStatus(*status) {
		return fmt.Errorf("status must be one of: blocked, completed, failed")
	}
	client := run.APIClient(globals)
	assignment, err := client.GetTask(ctx, *taskID)
	if err != nil {
		return err
	}
	switch assignment.AssignmentType {
	case taskcore.AssignmentTypeRoom:
		if *attempt < 1 {
			return fmt.Errorf("attempt is required for room tasks")
		}
		item, err := client.UpdateRoomTask(ctx, assignment.AssignmentID, *taskID, apitypes.UpdateRoomTaskRequest{
			Attempt: *attempt,
			Status:  strings.TrimSpace(*status),
			Result:  strings.TrimSpace(*result),
			Error:   strings.TrimSpace(*errorText),
			Reason:  strings.TrimSpace(*reason),
		})
		if err != nil {
			return err
		}
		return command.WriteJSON(run.Stdout, item)
	case taskcore.AssignmentTypeTeam:
		if strings.TrimSpace(*actorID) == "" {
			return fmt.Errorf("actor_id is required for team tasks")
		}
		item, err := client.UpdateTeamTask(ctx, assignment.AssignmentID, *taskID, strings.TrimSpace(*actorID), apitypes.PatchTeamTaskRequest{
			Status: strings.TrimSpace(*status), Result: strings.TrimSpace(*result), Error: strings.TrimSpace(*errorText), Reason: strings.TrimSpace(*reason),
		})
		if err != nil {
			return err
		}
		return command.RenderTeamTasks(globals.Output, run.Stdout, []apitypes.TeamTask{item})
	case taskcore.AssignmentTypeAgent:
		if strings.TrimSpace(*actorID) == "" {
			return fmt.Errorf("actor_id is required for agent tasks")
		}
		item, err := client.UpdateAgentTask(ctx, *taskID, apitypes.PatchAgentTaskRequest{
			ActorID: strings.TrimSpace(*actorID), Status: strings.TrimSpace(*status), Result: strings.TrimSpace(*result), Error: strings.TrimSpace(*errorText), Reason: strings.TrimSpace(*reason),
		})
		if err != nil {
			return err
		}
		return command.RenderTasks(globals.Output, run.Stdout, []apitypes.TeamTask{item})
	default:
		return fmt.Errorf("task %s has unsupported assignment_type %q", *taskID, assignment.AssignmentType)
	}
}

func isSupportedStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case taskcore.StatusBlocked, taskcore.StatusCompleted, taskcore.StatusFailed:
		return true
	default:
		return false
	}
}
