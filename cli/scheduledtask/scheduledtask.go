package scheduledtask

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"csgclaw/cli/command"
	"csgclaw/internal/apitypes"
)

type cmd struct{}

func NewCmd() command.Command { return cmd{} }
func (cmd) Name() string      { return "scheduled-task" }
func (cmd) Summary() string   { return "Create and manage scheduled agent tasks." }

func (c cmd) Run(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	if len(args) == 0 || command.IsHelpArg(args[0]) {
		run.UsageCommandGroup(c, run.Program+" scheduled-task <subcommand> [flags]", []string{
			"context   Read current time, supported schedules, and available agents",
			"list      List saved scheduled tasks",
			"create    Save a scheduled task (including Manager assignments)",
			"update    Change a saved scheduled task",
		})
		return flag.ErrHelp
	}
	action := args[0]
	if action == "context" || action == "list" {
		fs := run.NewFlagSet("scheduled-task "+action, run.Program+" scheduled-task "+action, "Inspect scheduled tasks.")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if len(fs.Args()) != 0 {
			return fmt.Errorf("scheduled-task %s does not accept positional arguments", action)
		}
		client := run.APIClient(globals)
		if action == "context" {
			var result any
			if err := client.GetJSON(ctx, "/api/v1/scheduled-tasks/context", &result); err != nil {
				return err
			}
			return command.WriteJSON(run.Stdout, result)
		}
		var items []apitypes.ScheduledTask
		if err := client.GetJSON(ctx, "/api/v1/scheduled-tasks", &items); err != nil {
			return err
		}
		return command.WriteJSON(run.Stdout, items)
	}
	if action != "create" && action != "update" {
		return fmt.Errorf("unknown scheduled-task subcommand %q", action)
	}
	fs := run.NewFlagSet("scheduled-task "+action, run.Program+" scheduled-task "+action+" [flags]", "Save a schedule after reviewing its time, executor, and prompt.")
	id := fs.String("id", "", "scheduled task id (update)")
	title := fs.String("title", "", "task title")
	agentID := fs.String("agent-id", "", "agent id; use agent-manager for Manager")
	prompt := fs.String("prompt", "", "instructions executed at the scheduled time")
	recurrence := fs.String("recurrence", "once", "once, daily, weekly, or monthly")
	firstRun := fs.String("first-run-at", "", "first/next run in RFC3339 with an explicit UTC offset")
	timezone := fs.String("timezone", "", "IANA time zone, e.g. Asia/Shanghai")
	expires := fs.String("expires-at", "", "optional expiration in RFC3339")
	enabled := fs.Bool("enabled", true, "enable the schedule")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("scheduled-task %s does not accept positional arguments", action)
	}
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	payload := map[string]any{}
	for key, value := range map[string]string{"title": *title, "agent-id": *agentID, "prompt": *prompt, "recurrence": *recurrence, "timezone": *timezone} {
		if action == "create" || visited[key] {
			payload[strings.ReplaceAll(key, "-", "_")] = strings.TrimSpace(value)
		}
	}
	if action == "create" || visited["enabled"] {
		payload["enabled"] = *enabled
	}
	for key, value := range map[string]string{"first-run-at": *firstRun, "expires-at": *expires} {
		if value == "" && (action != "create" || key != "first-run-at") {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return fmt.Errorf("%s must be RFC3339 with an explicit UTC offset: %w", key, err)
		}
		field := strings.ReplaceAll(key, "-", "_")
		if action == "update" && key == "first-run-at" {
			field = "next_run_at"
		}
		payload[field] = parsed
	}
	if action == "create" && (strings.TrimSpace(*title) == "" || strings.TrimSpace(*agentID) == "" || strings.TrimSpace(*prompt) == "") {
		return fmt.Errorf("title, agent-id, prompt, and first-run-at are required")
	}
	if action == "update" && strings.TrimSpace(*id) == "" {
		return fmt.Errorf("id is required")
	}
	if action == "update" && len(payload) == 0 {
		return fmt.Errorf("at least one field to update is required")
	}
	path := "/api/v1/scheduled-tasks"
	method := http.MethodPost
	if action == "update" {
		path += "/" + url.PathEscape(strings.TrimSpace(*id))
		method = http.MethodPatch
	}
	var item apitypes.ScheduledTask
	if err := run.APIClient(globals).DoJSON(ctx, method, path, payload, &item); err != nil {
		return err
	}
	return command.WriteJSON(run.Stdout, item)
}
