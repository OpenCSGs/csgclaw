package team

import (
	"context"
	"fmt"
	"log"
	"strings"
)

type Projector struct {
	adapter TeamChannelAdapter
	logger  *log.Logger
}

func NewProjector(adapter TeamChannelAdapter, logger *log.Logger) *Projector {
	if logger == nil {
		logger = log.Default()
	}
	return &Projector{
		adapter: adapter,
		logger:  logger,
	}
}

func (p *Projector) Project(ctx context.Context, meta TeamMeta, events []TeamEvent) error {
	if p == nil || p.adapter == nil || len(events) == 0 {
		return nil
	}
	if meta.Channel != "" && p.adapter.Channel() != meta.Channel {
		return fmt.Errorf("channel adapter mismatch: team=%s adapter=%s", meta.Channel, p.adapter.Channel())
	}

	plans := buildProjectionPlans(events)
	for _, plan := range plans {
		if strings.TrimSpace(plan.content) == "" {
			continue
		}
		if _, err := p.adapter.SendMessage(ctx, SendMessageRequest{
			Room: RoomRef{
				Channel: firstNonEmpty(meta.Channel, p.adapter.Channel()),
				RoomID:  meta.RoomID,
			},
			SenderBotID:    firstNonEmpty(plan.senderID, meta.LeadBotID),
			Kind:           "team_event",
			Content:        plan.content,
			IdempotencyKey: fmt.Sprintf("team:%s:event:%d", meta.ID, plan.anchorSeq),
		}); err != nil {
			return err
		}
	}
	return nil
}

type projectionPlan struct {
	anchorSeq int64
	senderID  string
	content   string
}

func buildProjectionPlans(events []TeamEvent) []projectionPlan {
	plans := make([]projectionPlan, 0, len(events))
	for i := 0; i < len(events); {
		if size := taskBatchProjectionSize(events[i:]); size > 1 {
			batch := events[i : i+size]
			plans = append(plans, projectionPlan{
				anchorSeq: batch[0].Seq,
				senderID:  batch[0].ActorID,
				content:   renderTaskBatchCreated(batch),
			})
			i += size
			continue
		}
		if content := renderSingleEvent(events[i]); strings.TrimSpace(content) != "" {
			plans = append(plans, projectionPlan{
				anchorSeq: events[i].Seq,
				senderID:  events[i].ActorID,
				content:   content,
			})
		}
		i++
	}
	return plans
}

func taskBatchProjectionSize(events []TeamEvent) int {
	if len(events) < 2 {
		return 0
	}
	first := events[0]
	if first.Type != "task.created" {
		return 0
	}
	size := 1
	for size < len(events) {
		next := events[size]
		if next.Type != "task.created" || next.ActorID != first.ActorID {
			break
		}
		size++
	}
	if size < 2 {
		return 0
	}
	return size
}

func renderTaskBatchCreated(events []TeamEvent) string {
	lines := []string{fmt.Sprintf("[team] %s created %d tasks:", renderActor(events[0].ActorID), len(events))}
	for _, event := range events {
		lines = append(lines, fmt.Sprintf("- %s %s", renderTaskLabel(event.TaskID), strings.TrimSpace(event.Summary)))
	}
	return strings.Join(lines, "\n")
}

func renderSingleEvent(event TeamEvent) string {
	switch event.Type {
	case "team.created":
		return fmt.Sprintf("[team] Team enabled: %s", firstNonEmpty(strings.TrimSpace(event.Summary), event.TeamID))
	case "task.created":
		return fmt.Sprintf("[team] Task created: %s %s", renderTaskLabel(event.TaskID), strings.TrimSpace(event.Summary))
	case "task.assigned":
		return fmt.Sprintf("[team] %s assigned %s to %s", renderActor(event.ActorID), renderTaskLabel(event.TaskID), renderActor(event.TargetID))
	case "task.claimed":
		return fmt.Sprintf("[team] %s claimed %s", renderActor(event.ActorID), renderTaskLabel(event.TaskID))
	case "task.blocked":
		return fmt.Sprintf("[team] %s blocked %s\nreason: %s", renderActor(event.ActorID), renderTaskLabel(event.TaskID), strings.TrimSpace(event.Summary))
	case "task.completed":
		return fmt.Sprintf("[team] %s completed %s\nresult: %s", renderActor(event.ActorID), renderTaskLabel(event.TaskID), strings.TrimSpace(event.Summary))
	case "task.failed":
		return fmt.Sprintf("[team] %s failed %s\nerror: %s", renderActor(event.ActorID), renderTaskLabel(event.TaskID), strings.TrimSpace(event.Summary))
	case "task.cancelled":
		if summary := strings.TrimSpace(event.Summary); summary != "" {
			return fmt.Sprintf("[team] %s cancelled %s\nreason: %s", renderActor(event.ActorID), renderTaskLabel(event.TaskID), summary)
		}
		return fmt.Sprintf("[team] %s cancelled %s", renderActor(event.ActorID), renderTaskLabel(event.TaskID))
	case "approval.requested":
		if strings.TrimSpace(event.TaskID) != "" {
			return fmt.Sprintf("[approval] %s requested approval for %s\nsummary: %s\nReply in this room with: approve %s or reject %s <reason>", renderActor(event.ActorID), renderTaskLabel(event.TaskID), strings.TrimSpace(event.Summary), renderTaskLabel(event.TaskID), renderTaskLabel(event.TaskID))
		}
		return fmt.Sprintf("[team] %s requested approval %s\nsummary: %s", renderActor(event.ActorID), renderApprovalLabel(event.TargetID), strings.TrimSpace(event.Summary))
	case "approval.resolved":
		return fmt.Sprintf("[team] %s resolved approval %s\nstatus: %s", renderActor(event.ActorID), renderApprovalLabel(event.TargetID), strings.TrimSpace(event.Summary))
	case "presence.changed":
		return fmt.Sprintf("[team] %s is now %s", renderActor(event.ActorID), strings.TrimSpace(event.Summary))
	case "projection.failed":
		return ""
	default:
		return ""
	}
}

func renderActor(actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return "system"
	}
	return "@" + actorID
}

func renderTaskLabel(taskID string) string {
	if strings.TrimSpace(taskID) == "" {
		return "task"
	}
	return taskID
}

func renderApprovalLabel(approvalID string) string {
	if strings.TrimSpace(approvalID) == "" {
		return "approval"
	}
	return approvalID
}
