package task

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/taskcore"
)

func (c cmd) runRoomAction(ctx context.Context, run *command.Context, action string, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("task "+action, run.Program+" task "+action+" [flags]", "Manage on-demand room work.")
	roomID := fs.String("room", "", "room id; required only for room-level operations")
	taskID := fs.String("task", "", "task id; its room is resolved automatically")
	target := fs.String("target", "", "mentioned room participant")
	messageID := fs.String("message-id", "", "stable message id; reuse on retries")
	appendPlan := fs.Bool("append", false, "append children without replacing the existing plan")
	requestID := fs.String("request-id", "", "stable plan extension id; reuse on retries")
	actorID := fs.String("actor-id", "", "requester or worker participant id")
	sourceID := fs.String("source-message", "", "original user message id or stable Manager request id")
	title := fs.String("title", "", "task title")
	body := fs.String("body", "", "task requirements and deliverables")
	planJSON := fs.String("plan-json", "", `structured plan: {"summary":"...","tasks":[{"id_ref":"dev","title":"...","assigned_to":"pt-..."}]}`)
	planFile := fs.String("plan-file", "", "read structured plan JSON from a file")
	attempt := fs.Int("attempt", 0, "execution attempt from the dispatch")
	accept := fs.Bool("accept", false, "accept the submitted result")
	outcome := fs.String("outcome", "", "manager assessment: succeeded, issues, failed, or stopped")
	result := fs.String("result", "", "deliverables, test outcomes, or assessment")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("task %s does not accept positional arguments", action)
	}

	client := run.APIClient(globals)
	resolvedRoomID := strings.TrimSpace(*roomID)
	if strings.TrimSpace(*taskID) != "" {
		assignment, err := client.GetTask(ctx, strings.TrimSpace(*taskID))
		if err != nil {
			return err
		}
		if assignment.AssignmentType != taskcore.AssignmentTypeRoom {
			if action == "get" {
				return command.WriteJSON(run.Stdout, assignment)
			}
			return fmt.Errorf("task %s belongs to %s %s, not a room", *taskID, assignment.AssignmentType, assignment.AssignmentID)
		}
		if resolvedRoomID != "" && resolvedRoomID != assignment.AssignmentID {
			return fmt.Errorf("task %s belongs to room %s, not %s", *taskID, assignment.AssignmentID, resolvedRoomID)
		}
		resolvedRoomID = assignment.AssignmentID
	}

	roomLevel := action == "context" || action == "submit" || action == "retry-delivery"
	if roomLevel && resolvedRoomID == "" {
		return fmt.Errorf("room is required for task %s", action)
	}
	if !roomLevel && strings.TrimSpace(*taskID) == "" {
		return fmt.Errorf("task is required")
	}

	var items []apitypes.RoomTask
	switch action {
	case "context":
		got, err := client.RoomTaskContext(ctx, resolvedRoomID)
		if err != nil {
			return err
		}
		return command.WriteJSON(run.Stdout, got)
	case "submit":
		if strings.TrimSpace(*sourceID) == "" || strings.TrimSpace(*actorID) == "" || strings.TrimSpace(*title) == "" {
			return fmt.Errorf("source-message, actor-id and title are required")
		}
		got, err := client.CreateRoomTask(ctx, resolvedRoomID, apitypes.CreateRoomTaskRequest{
			Title: *title, Body: *body, CreatedBy: *actorID, SourceMessageID: *sourceID,
		})
		if err != nil {
			return err
		}
		items = []apitypes.RoomTask{got}
	case "retry-delivery":
		got, err := client.RetryRoomDelivery(ctx, resolvedRoomID)
		if err != nil {
			return err
		}
		items = got
	case "get":
		got, err := client.GetRoomTask(ctx, resolvedRoomID, *taskID)
		if err != nil {
			return err
		}
		return command.WriteJSON(run.Stdout, got)
	case "plan":
		if *planFile != "" {
			if *planJSON != "" {
				return fmt.Errorf("use either plan-file or plan-json")
			}
			data, err := os.ReadFile(*planFile)
			if err != nil {
				return fmt.Errorf("read plan-file: %w", err)
			}
			*planJSON = string(data)
		}
		var plan apitypes.PlanRoomTaskRequest
		if err := json.Unmarshal([]byte(*planJSON), &plan); err != nil || len(plan.Tasks) == 0 {
			return fmt.Errorf("plan-json must contain a nonempty tasks array with id_ref, title, assigned_to and optional depends_on_refs")
		}
		plan.AutoStart = true
		plan.Append = plan.Append || *appendPlan
		if strings.TrimSpace(*requestID) != "" {
			plan.RequestID = strings.TrimSpace(*requestID)
		}
		if plan.Append && plan.RequestID == "" {
			return fmt.Errorf("request-id is required when appending tasks")
		}
		got, err := client.PlanRoomTask(ctx, resolvedRoomID, *taskID, plan)
		if err != nil {
			return err
		}
		items = append([]apitypes.RoomTask{got.Task}, got.CreatedTasks...)
	case "start":
		got, err := client.StartRoomTask(ctx, resolvedRoomID, *taskID)
		if err != nil {
			return err
		}
		items = []apitypes.RoomTask{got.Task}
	case "dispatch":
		got, err := client.DispatchRoomTask(ctx, resolvedRoomID, *taskID, *target)
		if err != nil {
			return err
		}
		items = []apitypes.RoomTask{got}
	case "review":
		if *attempt < 1 || strings.TrimSpace(*result) == "" {
			return fmt.Errorf("attempt and result are required")
		}
		got, err := client.ReviewRoomTask(ctx, resolvedRoomID, *taskID, apitypes.ReviewRoomTaskRequest{Attempt: *attempt, Accept: *accept, Summary: *result})
		if err != nil {
			return err
		}
		items = []apitypes.RoomTask{got}
	case "message":
		if strings.TrimSpace(*actorID) == "" || strings.TrimSpace(*target) == "" || strings.TrimSpace(*messageID) == "" || strings.TrimSpace(*body) == "" {
			return fmt.Errorf("actor-id, target, message-id and body are required")
		}
		got, err := client.SendRoomTaskMessage(ctx, resolvedRoomID, *taskID, apitypes.RoomTaskMessageRequest{ActorID: *actorID, TargetID: *target, MessageID: *messageID, Content: *body})
		if err != nil {
			return err
		}
		return command.WriteJSON(run.Stdout, got)
	case "stop":
		got, err := client.StopRoomTask(ctx, resolvedRoomID, *taskID)
		if err != nil {
			return err
		}
		items = []apitypes.RoomTask{got}
	case "recover":
		if *attempt < 1 || strings.TrimSpace(*result) == "" {
			return fmt.Errorf("attempt and result are required")
		}
		got, err := client.RecoverRoomTask(ctx, resolvedRoomID, *taskID, *attempt, *result)
		if err != nil {
			return err
		}
		items = []apitypes.RoomTask{got}
	case "report":
		if strings.TrimSpace(*outcome) == "" || strings.TrimSpace(*result) == "" {
			return fmt.Errorf("outcome and result are required")
		}
		got, err := client.ReportRoomTask(ctx, resolvedRoomID, *taskID, *outcome, *result)
		if err != nil {
			return err
		}
		items = []apitypes.RoomTask{got}
	default:
		return fmt.Errorf("unsupported task action %q", action)
	}
	return command.WriteJSON(run.Stdout, items)
}
