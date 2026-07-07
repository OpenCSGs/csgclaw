package scheduledtask

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/agenttask"
	"csgclaw/internal/taskcore"
)

const schedulerCreatedBy = "scheduler"

var ErrActiveTask = errors.New("scheduled task already has an active generated task")

type agentTaskRunner interface {
	CreateAgentTask(context.Context, agenttask.CreateInput) (taskcore.Task, error)
	List() []taskcore.Task
}

type taskStore interface {
	Load() (state, error)
	Save(state) error
}

type Service struct {
	mu        sync.Mutex
	store     taskStore
	agentTask agentTaskRunner
	now       func() time.Time
	state     state
	inFlight  map[string]struct{}
}

func NewService(store taskStore, agentTask *agenttask.Service, opts ...Option) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("scheduled task store is required")
	}
	current, err := store.Load()
	if err != nil {
		return nil, err
	}
	s := &Service{
		store:     store,
		agentTask: agentTask,
		now:       time.Now,
		state:     current,
		inFlight:  make(map[string]struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

type Option func(*Service)

func WithNowFunc(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithAgentTaskRunner(runner agentTaskRunner) Option {
	return func(s *Service) {
		s.agentTask = runner
	}
}

func (s *Service) List() []Task {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Task(nil), s.state.Tasks...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].NextRunAt.Equal(out[j].NextRunAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].NextRunAt.Before(out[j].NextRunAt)
	})
	return out
}

func (s *Service) Runs(taskID string) []Run {
	taskID = strings.TrimSpace(taskID)
	if s == nil || taskID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Run, 0)
	for _, run := range s.state.Runs {
		if run.ScheduledTaskID == taskID {
			out = append(out, run)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].TriggeredAt.After(out[j].TriggeredAt)
	})
	return out
}

func (s *Service) Create(input CreateInput) (Task, error) {
	if s == nil {
		return Task{}, fmt.Errorf("scheduled task service is required")
	}
	title := strings.TrimSpace(input.Title)
	agentID := agent.CanonicalID(input.AgentID)
	prompt := strings.TrimSpace(input.Prompt)
	recurrence := normalizeRecurrence(input.Recurrence)
	if title == "" {
		return Task{}, fmt.Errorf("title is required")
	}
	if agentID == "" {
		return Task{}, fmt.Errorf("agent_id is required")
	}
	if prompt == "" {
		return Task{}, fmt.Errorf("prompt is required")
	}
	if input.FirstRunAt.IsZero() {
		return Task{}, fmt.Errorf("first_run_at is required")
	}
	now := s.now()
	item := Task{
		Title:      title,
		AgentID:    agentID,
		Prompt:     prompt,
		Recurrence: recurrence,
		Enabled:    input.Enabled,
		NextRunAt:  input.FirstRunAt,
		ExpiresAt:  cloneTime(input.ExpiresAt),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := validateSchedule(item); err != nil {
		return Task{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneServiceState(s.state)
	item.ID = fmt.Sprintf("scheduled-task-%d", next.NextTaskID)
	next.NextTaskID++
	next.Tasks = append(next.Tasks, item)
	if err := s.store.Save(next); err != nil {
		return Task{}, err
	}
	s.state = next
	return item, nil
}

func (s *Service) Update(input UpdateInput) (Task, error) {
	if s == nil {
		return Task{}, fmt.Errorf("scheduled task service is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.indexLocked(input.ID)
	if idx < 0 {
		return Task{}, fmt.Errorf("scheduled task not found")
	}
	item := s.state.Tasks[idx]
	if input.Title != nil {
		item.Title = strings.TrimSpace(*input.Title)
	}
	if input.AgentID != nil {
		item.AgentID = agent.CanonicalID(*input.AgentID)
	}
	if input.Prompt != nil {
		item.Prompt = strings.TrimSpace(*input.Prompt)
	}
	if input.Recurrence != nil {
		item.Recurrence = normalizeRecurrence(*input.Recurrence)
	}
	if input.NextRunAt != nil {
		item.NextRunAt = *input.NextRunAt
	}
	if input.ExpiresAt != nil {
		item.ExpiresAt = cloneTime(*input.ExpiresAt)
	}
	if input.Enabled != nil {
		item.Enabled = *input.Enabled
	}
	if item.Title == "" {
		return Task{}, fmt.Errorf("title is required")
	}
	if item.AgentID == "" {
		return Task{}, fmt.Errorf("agent_id is required")
	}
	if item.Prompt == "" {
		return Task{}, fmt.Errorf("prompt is required")
	}
	if item.Enabled && item.NextRunAt.IsZero() {
		return Task{}, fmt.Errorf("next_run_at is required")
	}
	if err := validateSchedule(item); err != nil {
		return Task{}, err
	}
	item.UpdatedAt = s.now()
	next := cloneServiceState(s.state)
	next.Tasks[idx] = item
	if err := s.store.Save(next); err != nil {
		return Task{}, err
	}
	s.state = next
	return item, nil
}

func (s *Service) Delete(id string) error {
	if s == nil {
		return fmt.Errorf("scheduled task service is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.indexLocked(id)
	if idx < 0 {
		return fmt.Errorf("scheduled task not found")
	}
	next := cloneServiceState(s.state)
	next.Tasks = append(next.Tasks[:idx], next.Tasks[idx+1:]...)
	if err := s.store.Save(next); err != nil {
		return err
	}
	s.state = next
	return nil
}

func (s *Service) RunNow(ctx context.Context, id string) (Run, error) {
	if s == nil {
		return Run{}, fmt.Errorf("scheduled task service is required")
	}
	s.mu.Lock()
	idx := s.indexLocked(id)
	if idx < 0 {
		s.mu.Unlock()
		return Run{}, fmt.Errorf("scheduled task not found")
	}
	item := s.state.Tasks[idx]
	advanceSchedule := item.Enabled && !item.NextRunAt.IsZero() && !item.NextRunAt.After(s.now()) && !expired(item, s.now())
	s.mu.Unlock()
	return s.trigger(ctx, item, advanceSchedule, false)
}

func (s *Service) TriggerDue(ctx context.Context) []Run {
	now := s.now()
	s.mu.Lock()
	due := make([]Task, 0)
	for _, item := range s.state.Tasks {
		if item.Enabled && !item.NextRunAt.IsZero() && !item.NextRunAt.After(now) && !expired(item, now) && !s.hasActiveRunLocked(item.ID) && !s.inFlightLocked(item.ID) {
			due = append(due, item)
		}
	}
	s.mu.Unlock()
	runs := make([]Run, 0, len(due))
	for _, item := range due {
		run, err := s.trigger(ctx, item, true, true)
		if err == nil {
			runs = append(runs, run)
		}
	}
	return runs
}

func (s *Service) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	s.TriggerDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.TriggerDue(ctx)
		}
	}
}

func (s *Service) trigger(ctx context.Context, item Task, advanceSchedule bool, skipIfActive bool) (Run, error) {
	now := s.now()
	s.mu.Lock()
	if s.inFlightLocked(item.ID) || s.hasActiveRunLocked(item.ID) {
		s.mu.Unlock()
		return Run{}, ErrActiveTask
	}
	run := Run{
		ID:              fmt.Sprintf("scheduled-run-%d", s.state.NextRunID),
		ScheduledTaskID: item.ID,
		TriggeredAt:     now,
		Status:          StatusTriggered,
		TaskID:          taskIDForRunID(fmt.Sprintf("scheduled-run-%d", s.state.NextRunID)),
	}
	next := cloneServiceState(s.state)
	next.NextRunID++
	next.Runs = append(next.Runs, run)
	if err := s.store.Save(next); err != nil {
		s.mu.Unlock()
		return Run{}, err
	}
	s.state = next
	s.inFlight[item.ID] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.inFlight, item.ID)
		s.mu.Unlock()
	}()

	if s.agentTask == nil {
		run.Status = StatusFailed
		run.Error = "agent task service is not configured"
	} else {
		task, err := s.agentTask.CreateAgentTask(ctx, agenttask.CreateInput{
			ID:        run.TaskID,
			AgentID:   item.AgentID,
			Title:     item.Title,
			Body:      item.Prompt,
			CreatedBy: schedulerCreatedBy,
		})
		if err != nil {
			run.Status = StatusFailed
			run.Error = err.Error()
		} else {
			run.TaskID = task.ID
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if skipIfActive && run.Status == StatusFailed && s.hasActiveRunLocked(item.ID) {
		return Run{}, ErrActiveTask
	}
	next = cloneServiceState(s.state)
	if idx := indexTask(next.Tasks, item.ID); idx >= 0 {
		updated := next.Tasks[idx]
		updated.LastRunAt = &now
		updated.UpdatedAt = now
		if advanceSchedule {
			scheduledNext, ok := nextRunAt(updated, now)
			updated.NextRunAt = scheduledNext
			updated.Enabled = ok && !expired(updated, scheduledNext)
		}
		next.Tasks[idx] = updated
	}
	for idx := len(next.Runs) - 1; idx >= 0; idx-- {
		if next.Runs[idx].ID == run.ID {
			next.Runs[idx] = run
			break
		}
	}
	err := s.store.Save(next)
	if err == nil {
		s.state = next
	}
	return run, err
}

func (s *Service) inFlightLocked(id string) bool {
	if s == nil || s.inFlight == nil {
		return false
	}
	_, ok := s.inFlight[strings.TrimSpace(id)]
	return ok
}

func (s *Service) hasActiveRunLocked(id string) bool {
	if s == nil || s.agentTask == nil {
		return false
	}
	taskID := ""
	for i := len(s.state.Runs) - 1; i >= 0; i-- {
		run := s.state.Runs[i]
		if run.ScheduledTaskID == id && strings.TrimSpace(run.TaskID) != "" {
			taskID = strings.TrimSpace(run.TaskID)
			break
		}
	}
	if taskID == "" {
		return false
	}
	for _, task := range s.agentTask.List() {
		if task.ID == taskID {
			return !isTerminalTaskStatus(task.Status)
		}
	}
	return false
}

func isTerminalTaskStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case taskcore.StatusCompleted, taskcore.StatusFailed, taskcore.StatusCancelled:
		return true
	default:
		return false
	}
}

func (s *Service) indexLocked(id string) int {
	return indexTask(s.state.Tasks, id)
}

func indexTask(tasks []Task, id string) int {
	id = strings.TrimSpace(id)
	for i, item := range tasks {
		if item.ID == id {
			return i
		}
	}
	return -1
}

func validateSchedule(item Task) error {
	if item.Enabled && item.ExpiresAt != nil && !item.NextRunAt.IsZero() && item.ExpiresAt.Before(item.NextRunAt) {
		return fmt.Errorf("expires_at must not be before next_run_at")
	}
	return nil
}

func normalizeRecurrence(value string) string {
	switch strings.TrimSpace(value) {
	case RecurrenceDaily, RecurrenceWeekly, RecurrenceMonthly:
		return strings.TrimSpace(value)
	default:
		return RecurrenceOnce
	}
}

func nextRunAt(item Task, from time.Time) (time.Time, bool) {
	switch normalizeRecurrence(item.Recurrence) {
	case RecurrenceDaily:
		return advanceUntilAfter(item.NextRunAt, from, func(t time.Time) time.Time { return t.AddDate(0, 0, 1) }), true
	case RecurrenceWeekly:
		return advanceUntilAfter(item.NextRunAt, from, func(t time.Time) time.Time { return t.AddDate(0, 0, 7) }), true
	case RecurrenceMonthly:
		return advanceUntilAfter(item.NextRunAt, from, func(t time.Time) time.Time { return t.AddDate(0, 1, 0) }), true
	default:
		return time.Time{}, false
	}
}

func advanceUntilAfter(next, from time.Time, advance func(time.Time) time.Time) time.Time {
	if next.IsZero() {
		next = from
	}
	for !next.After(from) {
		next = advance(next)
	}
	return next
}

func expired(item Task, at time.Time) bool {
	return item.ExpiresAt != nil && at.After(*item.ExpiresAt)
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	next := *value
	return &next
}

func cloneServiceState(input state) state {
	out := input
	out.Tasks = append([]Task(nil), input.Tasks...)
	out.Runs = append([]Run(nil), input.Runs...)
	return out
}

func taskIDForRunID(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ""
	}
	return "task-" + runID
}
