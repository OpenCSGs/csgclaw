package agenttask

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	"csgclaw/internal/taskcore"
)

type participantLookup interface {
	List(opts participant.ListOptions) []apitypes.Participant
}

type Service struct {
	core         *taskcore.Service
	im           *im.Service
	agents       *agent.Service
	participants participantLookup
}

type CreateInput struct {
	ID        string
	AgentID   string
	Title     string
	Body      string
	CreatedBy string
}

type ClaimInput struct {
	TaskID        string
	ParticipantID string
}

type UpdateInput struct {
	TaskID  string
	ActorID string
	Status  string
	Result  string
	Error   string
	Reason  string
}

func NewService(core *taskcore.Service, imSvc *im.Service, agentSvc *agent.Service, participantSvc participantLookup) *Service {
	return &Service{
		core:         core,
		im:           imSvc,
		agents:       agentSvc,
		participants: participantSvc,
	}
}

func (s *Service) CreateAgentTask(ctx context.Context, input CreateInput) (taskcore.Task, error) {
	_ = ctx
	if s == nil || s.core == nil {
		return taskcore.Task{}, fmt.Errorf("task core service is not configured")
	}
	if s.im == nil {
		return taskcore.Task{}, fmt.Errorf("im service is not configured")
	}
	agentID := agent.CanonicalID(input.AgentID)
	if agentID == "" {
		return taskcore.Task{}, fmt.Errorf("agent_id is required")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return taskcore.Task{}, fmt.Errorf("title is required")
	}

	name, description, err := s.agentIdentity(agentID)
	if err != nil {
		return taskcore.Task{}, err
	}
	user, room, err := s.im.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID:          agentID,
		Name:        name,
		Description: description,
		Role:        agent.RoleWorker,
	})
	if err != nil {
		return taskcore.Task{}, err
	}
	if room == nil {
		return taskcore.Task{}, fmt.Errorf("agent direct room was not created")
	}

	assignedTo := s.participantIDForAgentID(agentID)
	task, err := s.core.CreateRoot(taskcore.CreateRootInput{
		ID:               strings.TrimSpace(input.ID),
		AssignmentType:   taskcore.AssignmentTypeAgent,
		AssignmentID:     agentID,
		Title:            title,
		Body:             strings.TrimSpace(input.Body),
		CreatedBy:        firstNonEmpty(strings.TrimSpace(input.CreatedBy), im.AdminUserID),
		AssignedTo:       assignedTo,
		ExecutionChannel: "csgclaw",
		RoomID:           room.ID,
	})
	if err != nil {
		return taskcore.Task{}, err
	}

	if _, err := s.im.DeliverEvent(im.DeliverEventRequest{
		RoomID:    room.ID,
		SenderID:  im.AdminUserID,
		MentionID: user.ID,
		Content:   renderInitialMessage(task),
		MessageID: "agent-task-" + task.ID + "-created",
		Event: &im.EventPayload{
			Key:       "task_assigned",
			ActorID:   firstNonEmpty(strings.TrimSpace(input.CreatedBy), im.AdminUserID),
			Title:     renderTaskAssignmentEventTitle(task.ID, task.Title),
			TargetIDs: []string{user.ID},
		},
	}); err != nil {
		return taskcore.Task{}, err
	}
	return task, nil
}

func (s *Service) List() []taskcore.Task {
	if s == nil || s.core == nil {
		return nil
	}
	tasks := s.core.ListGlobal()
	out := make([]taskcore.Task, 0, len(tasks))
	for _, task := range tasks {
		if task.AssignmentType == taskcore.AssignmentTypeAgent {
			out = append(out, task)
		}
	}
	return out
}

func (s *Service) Claim(input ClaimInput) (taskcore.Task, error) {
	if s == nil || s.core == nil {
		return taskcore.Task{}, fmt.Errorf("task core service is not configured")
	}
	task, err := s.core.Claim(taskcore.ClaimInput{
		TaskID:        input.TaskID,
		ParticipantID: strings.TrimSpace(input.ParticipantID),
	})
	if err != nil {
		return taskcore.Task{}, err
	}
	if task.AssignmentType != taskcore.AssignmentTypeAgent {
		return taskcore.Task{}, taskcore.ErrTaskNotFound
	}
	return task, nil
}

func (s *Service) Update(input UpdateInput) (taskcore.Task, error) {
	if s == nil || s.core == nil {
		return taskcore.Task{}, fmt.Errorf("task core service is not configured")
	}
	var (
		task taskcore.Task
		err  error
	)
	switch strings.TrimSpace(input.Status) {
	case taskcore.StatusCompleted:
		task, err = s.core.Complete(taskcore.CompleteInput{
			TaskID:  input.TaskID,
			ActorID: strings.TrimSpace(input.ActorID),
			Result:  strings.TrimSpace(input.Result),
		})
	case taskcore.StatusFailed:
		task, err = s.core.Fail(taskcore.FailInput{
			TaskID:  input.TaskID,
			ActorID: strings.TrimSpace(input.ActorID),
			Error:   strings.TrimSpace(input.Error),
		})
	case taskcore.StatusBlocked:
		task, err = s.core.Block(taskcore.BlockInput{
			TaskID:  input.TaskID,
			ActorID: strings.TrimSpace(input.ActorID),
			Reason:  strings.TrimSpace(input.Reason),
		})
	default:
		return taskcore.Task{}, fmt.Errorf("unsupported status update %q", input.Status)
	}
	if err != nil {
		return taskcore.Task{}, err
	}
	if task.AssignmentType != taskcore.AssignmentTypeAgent {
		return taskcore.Task{}, taskcore.ErrTaskNotFound
	}
	return task, nil
}

func (s *Service) agentIdentity(agentID string) (string, string, error) {
	if s != nil && s.agents != nil {
		if got, ok := s.agents.Agent(agentID); ok {
			return firstNonEmpty(strings.TrimSpace(got.Name), fallbackAgentName(agentID)), strings.TrimSpace(got.Description), nil
		}
		return "", "", fmt.Errorf("agent %q not found", agentID)
	}
	return fallbackAgentName(agentID), "", nil
}

func (s *Service) participantIDForAgentID(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	if s != nil && s.participants != nil {
		for _, item := range s.participants.List(participant.ListOptions{Channel: participant.ChannelCSGClaw, AgentID: agentID}) {
			if participantID := strings.TrimSpace(item.ID); participantID != "" {
				return participantID
			}
		}
	}
	if agentID == agent.ManagerUserID {
		return agent.ManagerParticipantID
	}
	suffix := strings.TrimPrefix(agentID, agent.AgentIDPrefix)
	if suffix == "" {
		suffix = agentID
	}
	return "pt-" + suffix
}

func renderInitialMessage(task taskcore.Task) string {
	var b strings.Builder
	b.WriteString("Task ")
	b.WriteString(task.ID)
	b.WriteString(" assigned to you.\n\nTitle: ")
	b.WriteString(task.Title)
	if strings.TrimSpace(task.Body) != "" {
		b.WriteString("\n\nDescription:\n")
		b.WriteString(strings.TrimSpace(task.Body))
	}
	b.WriteString("\n\nClaim it with:\n")
	b.WriteString("csgclaw-cli task claim --task ")
	b.WriteString(task.ID)
	b.WriteString(" --participant-id <worker_participant_id>")
	b.WriteString("\n\nWhen finished, update it with:\n")
	b.WriteString("csgclaw-cli task update --task ")
	b.WriteString(task.ID)
	b.WriteString(" --actor-id <worker_participant_id> --status completed --result \"<summary>\"")
	return b.String()
}

func renderTaskAssignmentEventTitle(taskID, title string) string {
	taskLabel := strings.TrimSpace(taskID)
	if taskLabel == "" {
		taskLabel = "task"
	}
	title = compactTaskAssignmentTitle(title)
	if title == "" {
		return taskLabel
	}
	return taskLabel + " [" + title + "]"
}

func compactTaskAssignmentTitle(title string) string {
	const maxRunes = 5
	title = strings.Join(strings.Fields(strings.TrimSpace(title)), " ")
	if title == "" {
		return ""
	}
	runes := []rune(title)
	if len(runes) <= maxRunes {
		return title
	}
	return string(runes[:maxRunes]) + "..."
}

func fallbackAgentName(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == agent.ManagerUserID {
		return agent.ManagerName
	}
	name := strings.TrimPrefix(agentID, agent.AgentIDPrefix)
	name = strings.TrimPrefix(name, "u-")
	if name == "" {
		name = "agent"
	}
	return name
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
