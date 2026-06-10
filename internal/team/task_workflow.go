package team

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrManagerPlannerFailed = errors.New("manager planner failed")

type TaskPlanner interface {
	PlanTask(ctx context.Context, meta TeamMeta, parent TeamTask) (PlanTaskInput, error)
}

type PlanTaskWorkflowInput struct {
	TeamID    string
	TaskID    string
	ActorID   string
	AutoStart bool
}

type PlanTaskWorkflowResult struct {
	Parent         TeamTask
	Tasks          []TeamTask
	AlreadyPlanned bool
	Started        bool
	ScheduledCount int
}

type StartTaskWithExecutionRoomInput struct {
	TeamID  string
	TaskID  string
	ActorID string
}

func (s *Service) CreateTaskWithExecutionRoom(ctx context.Context, input CreateTaskInput, adapter TeamChannelAdapter, directory ExecutionRoomDirectory) (TeamTask, error) {
	task, err := s.CreateTask(input)
	if err != nil {
		return TeamTask{}, err
	}
	if strings.TrimSpace(task.ParentID) != "" {
		return task, nil
	}
	return s.ensureAndBindParentExecutionRoom(ctx, input.TeamID, task.ID, input.CreatedBy, adapter, directory)
}

func (s *Service) CreateTasksWithExecutionRoom(ctx context.Context, input CreateTaskBatchInput, adapter TeamChannelAdapter, directory ExecutionRoomDirectory) (CreateTasksResult, error) {
	result, err := s.CreateTasks(input)
	if err != nil {
		return CreateTasksResult{}, err
	}
	for _, task := range result.Tasks {
		if strings.TrimSpace(task.ParentID) != "" {
			continue
		}
		if _, err := s.ensureAndBindParentExecutionRoom(ctx, input.TeamID, task.ID, input.CreatedBy, adapter, directory); err != nil {
			return CreateTasksResult{}, err
		}
	}
	for i, task := range result.Tasks {
		if updated, ok := s.GetTask(input.TeamID, task.ID); ok {
			result.Tasks[i] = updated
		}
	}
	return result, nil
}

func (s *Service) ensureAndBindParentExecutionRoom(ctx context.Context, teamID, taskID, actorID string, adapter TeamChannelAdapter, directory ExecutionRoomDirectory) (TeamTask, error) {
	meta, parent, err := s.requireTaskSnapshot(teamID, taskID)
	if err != nil {
		return TeamTask{}, err
	}
	if strings.TrimSpace(parent.ParentID) != "" {
		return parent, nil
	}
	if ExecutionRoomBound(parent, meta) {
		return parent, nil
	}
	roomID, err := s.EnsureTaskExecutionRoom(ctx, adapter, directory, meta, parent)
	if err != nil {
		return TeamTask{}, err
	}
	return s.BindTaskExecutionRoom(BindTaskExecutionRoomInput{
		TeamID:     teamID,
		TaskID:     taskID,
		ActorID:    actorID,
		TaskRoomID: roomID,
	})
}

func (s *Service) PlanTaskWithOptionalStart(ctx context.Context, input PlanTaskWorkflowInput, adapter TeamChannelAdapter, directory ExecutionRoomDirectory, planner TaskPlanner) (PlanTaskWorkflowResult, error) {
	teamID := strings.TrimSpace(input.TeamID)
	taskID := strings.TrimSpace(input.TaskID)
	actorID := strings.TrimSpace(input.ActorID)

	planInput := PlanTaskInput{
		TeamID:  teamID,
		TaskID:  taskID,
		ActorID: actorID,
	}
	taskRoomID := ""
	if input.AutoStart {
		meta, parent, err := s.requireTaskSnapshot(teamID, taskID)
		if err != nil {
			return PlanTaskWorkflowResult{}, err
		}
		if strings.TrimSpace(parent.ParentID) != "" {
			return PlanTaskWorkflowResult{}, fmt.Errorf("%w: only parent tasks can have execution rooms", ErrTaskTransitionInvalid)
		}
		roomID, err := s.EnsureTaskExecutionRoom(ctx, adapter, directory, meta, parent)
		if err != nil {
			return PlanTaskWorkflowResult{}, err
		}
		taskRoomID = roomID
		if _, err := s.BindTaskExecutionRoom(BindTaskExecutionRoomInput{
			TeamID:     teamID,
			TaskID:     taskID,
			ActorID:    actorID,
			TaskRoomID: taskRoomID,
		}); err != nil {
			return PlanTaskWorkflowResult{}, err
		}
	}

	if !s.taskHasChildren(teamID, taskID) {
		meta, parent, err := s.requireTaskSnapshot(teamID, taskID)
		if err != nil {
			return PlanTaskWorkflowResult{}, err
		}
		if planner == nil {
			return PlanTaskWorkflowResult{}, fmt.Errorf("%w: llm bridge is not configured", ErrManagerPlannerUnavailable)
		}
		planned, err := planner.PlanTask(ctx, meta, parent)
		if err != nil {
			if errors.Is(err, ErrManagerPlannerUnavailable) {
				return PlanTaskWorkflowResult{}, err
			}
			return PlanTaskWorkflowResult{}, fmt.Errorf("%w: %w", ErrManagerPlannerFailed, err)
		}
		planInput.PlanSummary = planned.PlanSummary
		planInput.Tasks = planned.Tasks
		if strings.TrimSpace(planned.ActorID) != "" {
			planInput.ActorID = strings.TrimSpace(planned.ActorID)
		}
	}

	planned, err := s.PlanTask(planInput)
	if err != nil {
		return PlanTaskWorkflowResult{}, err
	}
	result := PlanTaskWorkflowResult{
		Parent:         planned.Parent,
		Tasks:          planned.Tasks,
		AlreadyPlanned: planned.AlreadyPlanned,
	}
	if input.AutoStart && taskStatusIsUnstarted(planned.Parent.Status) {
		meta, parent, err := s.requireTaskSnapshot(teamID, taskID)
		if err != nil {
			return PlanTaskWorkflowResult{}, err
		}
		roomID, err := s.EnsureTaskExecutionRoom(ctx, adapter, directory, meta, parent)
		if err != nil {
			return PlanTaskWorkflowResult{}, err
		}
		if roomID != "" {
			taskRoomID = roomID
		}
		started, err := s.StartTask(StartTaskInput{
			TeamID:     teamID,
			TaskID:     taskID,
			ActorID:    actorID,
			TaskRoomID: taskRoomID,
		})
		if err != nil {
			return PlanTaskWorkflowResult{}, err
		}
		result.Parent = started.Parent
		result.Started = true
		result.ScheduledCount = started.ScheduledCount
		result.Tasks = s.taskChildren(teamID, taskID)
	}
	return result, nil
}

func (s *Service) StartTaskWithExecutionRoom(ctx context.Context, input StartTaskWithExecutionRoomInput, adapter TeamChannelAdapter, directory ExecutionRoomDirectory) (StartTaskResult, error) {
	teamID := strings.TrimSpace(input.TeamID)
	taskID := strings.TrimSpace(input.TaskID)
	meta, parent, err := s.requireTaskSnapshot(teamID, taskID)
	if err != nil {
		return StartTaskResult{}, err
	}
	taskRoomID, err := s.EnsureTaskExecutionRoom(ctx, adapter, directory, meta, parent)
	if err != nil {
		return StartTaskResult{}, err
	}
	return s.StartTask(StartTaskInput{
		TeamID:     teamID,
		TaskID:     taskID,
		ActorID:    strings.TrimSpace(input.ActorID),
		TaskRoomID: taskRoomID,
	})
}

func (s *Service) requireTaskSnapshot(teamID, taskID string) (TeamMeta, TeamTask, error) {
	meta, found := s.GetTeam(teamID)
	if !found {
		return TeamMeta{}, TeamTask{}, ErrTeamNotFound
	}
	task, found := s.GetTask(teamID, taskID)
	if !found {
		return TeamMeta{}, TeamTask{}, ErrTaskNotFound
	}
	return meta, task, nil
}

func (s *Service) taskHasChildren(teamID, parentID string) bool {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return false
	}
	for _, task := range s.ListTasks(teamID) {
		if task.ParentID == parentID {
			return true
		}
	}
	return false
}

func (s *Service) taskChildren(teamID, parentID string) []TeamTask {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return nil
	}
	tasks := s.ListTasks(teamID)
	out := make([]TeamTask, 0, len(tasks))
	for _, task := range tasks {
		if task.ParentID == parentID {
			out = append(out, task)
		}
	}
	return out
}
