package team

import (
	"context"
	"fmt"
	"strings"
)

type CommandParser struct {
	svc         *Service
	adapter     TeamChannelAdapter
	allowSender func(string) bool
}

func NewCommandParser(svc *Service, adapter TeamChannelAdapter, allowSender func(string) bool) *CommandParser {
	return &CommandParser{
		svc:         svc,
		adapter:     adapter,
		allowSender: allowSender,
	}
}

func (p *CommandParser) HandleMessage(ctx context.Context, roomID string, senderID string, content string) bool {
	if p == nil || p.svc == nil || p.adapter == nil {
		return false
	}
	meta, ok := p.svc.FindTeamByRoom(roomID)
	if !ok {
		return false
	}

	cmd, recognized, err := parseFixedTextCommand(content)
	if !recognized {
		return false
	}
	if err != nil {
		p.sendFeedback(ctx, meta, fmt.Sprintf("[team] Command failed for %s: %v", renderActor(senderID), err))
		return true
	}
	if p.allowSender != nil && !p.allowSender(strings.TrimSpace(senderID)) {
		p.sendFeedback(ctx, meta, fmt.Sprintf("[team] Command failed for %s: only human room members can use team override commands", renderActor(senderID)))
		return true
	}

	switch cmd.name {
	case "approve", "reject":
		approval, found := p.svc.FindPendingApprovalByTask(meta.ID, cmd.taskID)
		if !found {
			p.sendFeedback(ctx, meta, fmt.Sprintf("[team] Command failed for %s: no pending approval found for %s", renderActor(senderID), cmd.taskID))
			return true
		}
		status := ApprovalStatusApproved
		if cmd.name == "reject" {
			status = ApprovalStatusRejected
		}
		if _, err := p.svc.ResolveApproval(ResolveApprovalInput{
			TeamID:     meta.ID,
			ApprovalID: approval.ID,
			ApproverID: strings.TrimSpace(senderID),
			Status:     status,
			Resolution: cmd.reason,
		}); err != nil {
			p.sendFeedback(ctx, meta, fmt.Sprintf("[team] Command failed for %s: %v", renderActor(senderID), err))
		}
		return true
	case "cancel":
		if _, err := p.svc.CancelTask(CancelTaskInput{
			TeamID:  meta.ID,
			TaskID:  cmd.taskID,
			ActorID: strings.TrimSpace(senderID),
			Reason:  cmd.reason,
		}); err != nil {
			p.sendFeedback(ctx, meta, fmt.Sprintf("[team] Command failed for %s: %v", renderActor(senderID), err))
		}
		return true
	case "reassign":
		if _, err := p.svc.AssignTask(AssignTaskInput{
			TeamID:     meta.ID,
			TaskID:     cmd.taskID,
			AssignedTo: strings.TrimSpace(cmd.target),
			ActorID:    strings.TrimSpace(senderID),
		}); err != nil {
			p.sendFeedback(ctx, meta, fmt.Sprintf("[team] Command failed for %s: %v", renderActor(senderID), err))
		}
		return true
	default:
		return false
	}
}

type fixedTextCommand struct {
	name   string
	taskID string
	reason string
	target string
}

func parseFixedTextCommand(content string) (fixedTextCommand, bool, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return fixedTextCommand{}, false, nil
	}
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return fixedTextCommand{}, false, nil
	}
	cmd := strings.ToLower(fields[0])
	switch cmd {
	case "approve":
		if len(fields) != 2 {
			return fixedTextCommand{}, true, fmt.Errorf("usage: approve <task_id>")
		}
		return fixedTextCommand{name: cmd, taskID: fields[1]}, true, nil
	case "reject":
		if len(fields) < 3 {
			return fixedTextCommand{}, true, fmt.Errorf("usage: reject <task_id> <reason>")
		}
		return fixedTextCommand{name: cmd, taskID: fields[1], reason: strings.TrimSpace(strings.Join(fields[2:], " "))}, true, nil
	case "cancel":
		if len(fields) < 2 {
			return fixedTextCommand{}, true, fmt.Errorf("usage: cancel <task_id>")
		}
		reason := ""
		if len(fields) > 2 {
			reason = strings.TrimSpace(strings.Join(fields[2:], " "))
		}
		return fixedTextCommand{name: cmd, taskID: fields[1], reason: reason}, true, nil
	case "reassign":
		if len(fields) != 3 {
			return fixedTextCommand{}, true, fmt.Errorf("usage: reassign <task_id> <bot_id>")
		}
		return fixedTextCommand{name: cmd, taskID: fields[1], target: strings.TrimPrefix(fields[2], "@")}, true, nil
	default:
		return fixedTextCommand{}, false, nil
	}
}

func (p *CommandParser) sendFeedback(ctx context.Context, meta TeamMeta, content string) {
	if strings.TrimSpace(content) == "" || p.adapter == nil {
		return
	}
	_, _ = p.adapter.SendMessage(ctx, SendMessageRequest{
		Room: RoomRef{
			Channel: firstNonEmpty(meta.Channel, p.adapter.Channel()),
			RoomID:  meta.RoomID,
		},
		SenderBotID: meta.LeadBotID,
		Kind:        "team_event",
		Content:     content,
	})
}
