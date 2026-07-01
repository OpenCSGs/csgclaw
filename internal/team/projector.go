package team

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
)

type Projector struct {
	registry *AdapterRegistry
	logger   *log.Logger
}

func NewProjector(adapter TeamChannelAdapter, logger *log.Logger) *Projector {
	return NewProjectorWithRegistry(NewAdapterRegistry(adapter), logger)
}

func NewProjectorWithRegistry(registry *AdapterRegistry, logger *log.Logger) *Projector {
	if logger == nil {
		logger = log.Default()
	}
	return &Projector{
		registry: registry,
		logger:   logger,
	}
}

func (p *Projector) Project(ctx context.Context, meta TeamMeta, events []TeamEvent) error {
	if p == nil || p.registry == nil || len(events) == 0 {
		return nil
	}
	for _, event := range events {
		channel := NormalizeExecutionChannel(event.Channel)
		adapter, ok := p.registry.Adapter(channel)
		if !ok {
			return fmt.Errorf("team channel adapter is not configured for %q", channel)
		}
		leadParticipantID := participantIDForAgentID(adapter, meta.LeadAgentID)
		renderer := newProjectionRenderer(adapter, leadParticipantID)
		plans := buildProjectionPlans([]TeamEvent{event}, renderer, meta)
		for _, plan := range plans {
			if strings.TrimSpace(plan.content) == "" {
				continue
			}
			if strings.TrimSpace(plan.channel) == "" {
				plan.channel = channel
			}
			roomID := strings.TrimSpace(plan.roomID)
			if roomID == "" {
				continue
			}
			if _, err := adapter.SendMessage(ctx, SendMessageRequest{
				Room: RoomRef{
					Channel: plan.channel,
					RoomID:  roomID,
				},
				SenderParticipantID: projectionSenderParticipantID(firstNonEmpty(plan.senderID, leadParticipantID), leadParticipantID),
				MentionID:           strings.TrimSpace(plan.mentionID),
				Kind:                firstNonEmpty(plan.kind, "team_event"),
				Content:             plan.content,
				IdempotencyKey:      projectionIdempotencyKey(meta.ID, plan),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

type projectionPlan struct {
	anchorSeq int64
	channel   string
	senderID  string
	mentionID string
	roomID    string
	kind      string
	eventType string
	content   string
}

func buildProjectionPlans(events []TeamEvent, renderer projectionRenderer, meta TeamMeta) []projectionPlan {
	plans := make([]projectionPlan, 0, len(events))
	for i := 0; i < len(events); {
		if size := taskBatchProjectionSize(events[i:]); size > 1 {
			i += size
			continue
		}
		if events[i].Type == EventTaskCreated {
			i++
			continue
		}
		if events[i].Type == EventTaskDispatched {
			size := dispatchBatchProjectionSize(events[i:])
			batch := events[i : i+size]
			for _, event := range batch {
				plans = append(plans, projectionPlan{
					anchorSeq: event.Seq,
					channel:   NormalizeExecutionChannel(event.Channel),
					senderID:  event.ActorID,
					mentionID: projectionMentionID(event),
					roomID:    strings.TrimSpace(event.RoomID),
					kind:      "message",
					eventType: EventTaskDispatched,
					content:   renderTaskDispatched(event, renderer, meta),
				})
			}
			i += size
			continue
		}
		if content := renderSingleEvent(events[i], renderer, meta); strings.TrimSpace(content) != "" {
			plans = append(plans, projectionPlan{
				anchorSeq: events[i].Seq,
				channel:   NormalizeExecutionChannel(events[i].Channel),
				senderID:  events[i].ActorID,
				mentionID: projectionMentionID(events[i]),
				roomID:    strings.TrimSpace(events[i].RoomID),
				eventType: events[i].Type,
				content:   content,
			})
		}
		i++
	}
	return plans
}

func dispatchBatchProjectionSize(events []TeamEvent) int {
	size := 0
	for size < len(events) && events[size].Type == EventTaskDispatched {
		size++
	}
	return size
}

func projectionIdempotencyKey(teamID string, plan projectionPlan) string {
	return fmt.Sprintf("team:%s:event:%d", teamID, plan.anchorSeq)
}

func projectionMentionID(event TeamEvent) string {
	if event.Type != EventTaskDispatched {
		return ""
	}
	return strings.TrimSpace(event.TargetID)
}

func taskBatchProjectionSize(events []TeamEvent) int {
	if len(events) < 2 {
		return 0
	}
	first := events[0]
	if first.Type != EventTaskCreated {
		return 0
	}
	size := 1
	for size < len(events) {
		next := events[size]
		if next.Type != EventTaskCreated || next.ActorID != first.ActorID {
			break
		}
		size++
	}
	if size < 2 {
		return 0
	}
	return size
}

func renderTaskBatchCreated(events []TeamEvent, renderer projectionRenderer) string {
	items := make([]string, 0, len(events))
	for _, event := range events {
		items = append(items, fmt.Sprintf("%s%s%s", renderTaskLabel(event.TaskID), renderTitleSuffix(compactProjectionSummary(event.Summary)), renderPlainTargetSuffix(event.TargetID, renderer)))
	}
	return fmt.Sprintf("%s created %d tasks: %s", renderer.actor(events[0].ActorID), len(events), strings.Join(items, "; "))
}

func isExecutionRoom(meta TeamMeta, roomID string) bool {
	roomID = strings.TrimSpace(roomID)
	return roomID != ""
}

func renderSingleEvent(event TeamEvent, renderer projectionRenderer, meta TeamMeta) string {
	inExecRoom := isExecutionRoom(meta, event.RoomID)
	switch event.Type {
	case EventTeamCreated:
		return ""
	case EventTeamUpdated:
		return ""
	case EventTaskCreated:
		return ""
	case EventTaskPlanned:
		return fmt.Sprintf("%s completed planning for %s", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID))
	case EventTaskExecutionRoom:
		return ""
	case EventTaskStarted:
		return ""
	case EventTaskDispatched:
		return renderTaskDispatched(event, renderer, meta)
	case EventTaskAssigned:
		return fmt.Sprintf("%s assigned %s to %s", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID), renderer.actor(event.TargetID))
	case EventTaskClaimed:
		return fmt.Sprintf("%s claimed %s", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID))
	case EventTaskBlocked:
		if summary := compactProjectionSummary(event.Summary); summary != "" {
			return fmt.Sprintf("%s blocked %s: %s", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID), summary)
		}
		return fmt.Sprintf("%s blocked %s", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID))
	case EventTaskCompleted:
		return fmt.Sprintf("%s completed %s", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID))
	case EventTaskFailed:
		if summary := compactProjectionSummary(event.Summary); summary != "" {
			return fmt.Sprintf("%s failed %s: %s", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID), summary)
		}
		return fmt.Sprintf("%s failed %s", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID))
	case EventTaskCancelled:
		if summary := compactProjectionSummary(event.Summary); summary != "" {
			return fmt.Sprintf("%s cancelled %s: %s", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID), summary)
		}
		return fmt.Sprintf("%s cancelled %s", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID))
	case EventApprovalRequested:
		summary := compactProjectionSummary(event.Summary)
		if strings.TrimSpace(event.TaskID) != "" {
			if inExecRoom {
				if summary != "" {
					return fmt.Sprintf("%s requested approval for %s: %s. Reply: approve %s or reject %s <reason>", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID), summary, renderTaskLabel(event.TaskID), renderTaskLabel(event.TaskID))
				}
				return fmt.Sprintf("%s requested approval for %s. Reply: approve %s or reject %s <reason>", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID), renderTaskLabel(event.TaskID), renderTaskLabel(event.TaskID))
			}
			if summary != "" {
				return fmt.Sprintf("%s requested approval for %s: %s. Reply: approve %s or reject %s <reason>", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID), summary, renderTaskLabel(event.TaskID), renderTaskLabel(event.TaskID))
			}
			return fmt.Sprintf("%s requested approval for %s. Reply: approve %s or reject %s <reason>", renderer.actor(event.ActorID), renderTaskLabel(event.TaskID), renderTaskLabel(event.TaskID), renderTaskLabel(event.TaskID))
		}
		return fmt.Sprintf("%s requested approval %s: %s", renderer.actor(event.ActorID), renderApprovalLabel(event.TargetID), summary)
	case EventApprovalResolved:
		return fmt.Sprintf("%s resolved approval %s: %s", renderer.actor(event.ActorID), renderApprovalLabel(event.TargetID), compactProjectionSummary(event.Summary))
	case EventPresenceUpdated:
		return fmt.Sprintf("%s is now %s", renderer.actor(event.ActorID), strings.TrimSpace(event.Summary))
	case EventProjectionFailed:
		return ""
	default:
		return ""
	}
}

func projectionSenderParticipantID(actorID, leadAgentID string) string {
	rawActorID := strings.TrimSpace(actorID)
	actorID = cleanParticipantID(actorID)
	leadAgentID = cleanParticipantID(leadAgentID)
	if actorID == "" || rawActorID == "web" {
		return leadAgentID
	}
	return actorID
}

type participantDisplayNameResolver interface {
	ParticipantDisplayName(participantID string) string
}

type projectionRenderer struct {
	systemActorName string
	displayName     func(string) string
}

func newProjectionRenderer(adapter TeamChannelAdapter, leadAgentID string) projectionRenderer {
	resolver, _ := adapter.(participantDisplayNameResolver)
	leadName := ""
	if resolver != nil {
		leadName = strings.TrimSpace(resolver.ParticipantDisplayName(leadAgentID))
	}
	return projectionRenderer{
		systemActorName: firstNonEmpty(leadName, "manager"),
		displayName: func(participantID string) string {
			if resolver == nil {
				return ""
			}
			return resolver.ParticipantDisplayName(participantID)
		},
	}
}

func (r projectionRenderer) actor(actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return "system"
	}
	if actorID == "web" {
		return firstNonEmpty(strings.TrimSpace(r.systemActorName), "manager")
	}
	if r.displayName != nil {
		if name := strings.TrimSpace(r.displayName(actorID)); name != "" {
			return name
		}
	}
	return actorID
}

func renderActor(actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return "system"
	}
	if actorID == "web" {
		return "Web"
	}
	return actorID
}

func renderTargetSuffix(targetID string, renderer projectionRenderer) string {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return ""
	}
	return " -> " + renderer.actor(targetID)
}

func renderPlainTargetSuffix(targetID string, renderer projectionRenderer) string {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return ""
	}
	return ", assignee: " + renderer.actor(targetID)
}

func renderTaskDispatched(event TeamEvent, renderer projectionRenderer, meta TeamMeta) string {
	taskLabel := renderTaskLabel(event.TaskID)
	claim := fmt.Sprintf("csgclaw-cli team task claim --team %s --task %s --participant-id %s", event.TeamID, event.TaskID, event.TargetID)
	if isExecutionRoom(meta, event.RoomID) {
		// Mention prefix is added by IM delivery; keep body focused on task assignment.
		return fmt.Sprintf("dispatched %s. Claim: %s", taskLabel, claim)
	}
	return fmt.Sprintf("%s dispatched %s to %s. Claim: %s",
		renderer.actor(event.ActorID),
		taskLabel,
		renderer.actor(event.TargetID),
		claim,
	)
}

func renderTaskLabel(taskID string) string {
	if strings.TrimSpace(taskID) == "" {
		return "task"
	}
	return taskID
}

func renderTitleSuffix(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	return " " + strconv.Quote(title)
}

func renderApprovalLabel(approvalID string) string {
	if strings.TrimSpace(approvalID) == "" {
		return "approval"
	}
	return approvalID
}

func compactProjectionSummary(summary string) string {
	const maxRunes = 140
	summary = strings.Join(strings.Fields(strings.TrimSpace(summary)), " ")
	if summary == "" {
		return ""
	}
	runes := []rune(summary)
	if len(runes) <= maxRunes {
		return summary
	}
	return string(runes[:maxRunes]) + "..."
}
