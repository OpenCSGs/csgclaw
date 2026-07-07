package scheduledtask

import (
	"context"
	"testing"
	"time"

	"csgclaw/internal/agenttask"
	"csgclaw/internal/taskcore"
)

func TestCreateAndTriggerDueAdvancesSchedule(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 40, 0, 0, time.Local)
	svc := newTestService(t, now)
	item, err := svc.Create(CreateInput{
		Title:      "Daily check",
		AgentID:    "u-worker",
		Prompt:     "Summarize open work.",
		Recurrence: RecurrenceDaily,
		FirstRunAt: now.Add(-time.Minute),
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	runs := svc.TriggerDue(context.Background())
	if len(runs) != 1 {
		t.Fatalf("TriggerDue() runs len = %d, want 1", len(runs))
	}
	if runs[0].Status != StatusFailed || runs[0].ScheduledTaskID != item.ID {
		t.Fatalf("TriggerDue() run = %+v, want failed run for scheduled task", runs[0])
	}
	got := svc.List()[0]
	if !got.Enabled {
		t.Fatalf("scheduled task enabled = false, want true")
	}
	if !got.NextRunAt.After(now) {
		t.Fatalf("NextRunAt = %s, want after %s", got.NextRunAt, now)
	}
	if got.LastRunAt == nil || !got.LastRunAt.Equal(now) {
		t.Fatalf("LastRunAt = %v, want %s", got.LastRunAt, now)
	}
}

func TestRunNowDoesNotAdvanceSchedule(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 40, 0, 0, time.Local)
	svc := newTestService(t, now)
	nextRunAt := now.Add(time.Hour)
	item, err := svc.Create(CreateInput{
		Title:      "Weekly check",
		AgentID:    "worker",
		Prompt:     "Report status.",
		Recurrence: RecurrenceWeekly,
		FirstRunAt: nextRunAt,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := svc.RunNow(context.Background(), item.ID); err != nil {
		t.Fatalf("RunNow() error = %v", err)
	}
	got := svc.List()[0]
	if !got.NextRunAt.Equal(nextRunAt) {
		t.Fatalf("NextRunAt = %s, want unchanged %s", got.NextRunAt, nextRunAt)
	}
	if runs := svc.Runs(item.ID); len(runs) != 1 {
		t.Fatalf("Runs() len = %d, want 1", len(runs))
	}
}

func TestCreateRejectsExpirationBeforeFirstRun(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 40, 0, 0, time.Local)
	svc := newTestService(t, now)
	expiresAt := now.Add(time.Hour)
	if _, err := svc.Create(CreateInput{
		Title:      "Daily check",
		AgentID:    "worker",
		Prompt:     "Report status.",
		Recurrence: RecurrenceDaily,
		FirstRunAt: now.Add(2 * time.Hour),
		ExpiresAt:  &expiresAt,
		Enabled:    true,
	}); err == nil {
		t.Fatal("Create() error = nil, want expiration validation error")
	}
	if got := svc.List(); len(got) != 0 {
		t.Fatalf("List() len = %d, want 0 after rejected create", len(got))
	}
}

func TestUpdateRejectsExpirationBeforeNextRun(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 40, 0, 0, time.Local)
	svc := newTestService(t, now)
	item, err := svc.Create(CreateInput{
		Title:      "Daily check",
		AgentID:    "worker",
		Prompt:     "Report status.",
		Recurrence: RecurrenceDaily,
		FirstRunAt: now.Add(time.Hour),
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	expiresAt := now.Add(30 * time.Minute)
	expiresAtUpdate := &expiresAt
	if _, err := svc.Update(UpdateInput{ID: item.ID, ExpiresAt: &expiresAtUpdate}); err == nil {
		t.Fatal("Update() error = nil, want expiration validation error")
	}
	got := svc.List()[0]
	if got.ExpiresAt != nil {
		t.Fatalf("ExpiresAt = %v, want unchanged nil after rejected update", got.ExpiresAt)
	}
}

func TestUpdateAllowsDisabledCompletedOnceTask(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 40, 0, 0, time.Local)
	svc := newTestService(t, now)
	item, err := svc.Create(CreateInput{
		Title:      "One shot",
		AgentID:    "worker",
		Prompt:     "Report once.",
		Recurrence: RecurrenceOnce,
		FirstRunAt: now.Add(-time.Minute),
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	svc.TriggerDue(context.Background())

	disabled := false
	if _, err := svc.Update(UpdateInput{ID: item.ID, Enabled: &disabled}); err != nil {
		t.Fatalf("Update(disabled completed once) error = %v", err)
	}
}

func TestRunNowDueOnceTaskAdvancesScheduleAndPreventsAutoDuplicate(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 40, 0, 0, time.Local)
	runner := &fakeAgentTaskRunner{}
	svc := newTestServiceWithRunner(t, now, runner)
	item, err := svc.Create(CreateInput{
		Title:      "One shot",
		AgentID:    "worker",
		Prompt:     "Report once.",
		Recurrence: RecurrenceOnce,
		FirstRunAt: now.Add(-time.Minute),
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := svc.RunNow(context.Background(), item.ID); err != nil {
		t.Fatalf("RunNow() error = %v", err)
	}
	got := svc.List()[0]
	if got.Enabled {
		t.Fatalf("scheduled task enabled = true, want false")
	}
	if !got.NextRunAt.IsZero() {
		t.Fatalf("NextRunAt = %s, want zero", got.NextRunAt)
	}
	if runs := svc.TriggerDue(context.Background()); len(runs) != 0 {
		t.Fatalf("TriggerDue() runs len = %d, want 0", len(runs))
	}
	if len(runner.tasks) != 1 {
		t.Fatalf("created tasks len = %d, want 1", len(runner.tasks))
	}
}

func TestRunNowRejectsWhenGeneratedTaskIsActive(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 40, 0, 0, time.Local)
	runner := &fakeAgentTaskRunner{}
	svc := newTestServiceWithRunner(t, now, runner)
	item, err := svc.Create(CreateInput{
		Title:      "Manual check",
		AgentID:    "worker",
		Prompt:     "Report status.",
		Recurrence: RecurrenceDaily,
		FirstRunAt: now.Add(time.Hour),
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := svc.RunNow(context.Background(), item.ID); err != nil {
		t.Fatalf("RunNow(first) error = %v", err)
	}
	if _, err := svc.RunNow(context.Background(), item.ID); err != ErrActiveTask {
		t.Fatalf("RunNow(second) error = %v, want ErrActiveTask", err)
	}
}

func TestCreateDoesNotCommitMemoryStateWhenSaveFails(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 40, 0, 0, time.Local)
	store := &flakyStore{failOnSave: 1}
	svc, err := NewService(store, nil, WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if _, err := svc.Create(CreateInput{
		Title:      "Daily check",
		AgentID:    "worker",
		Prompt:     "Report status.",
		Recurrence: RecurrenceDaily,
		FirstRunAt: now,
		Enabled:    true,
	}); err != errFlakyStoreSave {
		t.Fatalf("Create() error = %v, want %v", err, errFlakyStoreSave)
	}
	if got := svc.List(); len(got) != 0 {
		t.Fatalf("List() len = %d, want 0 after failed save", len(got))
	}
}

func TestUpdateDoesNotCommitMemoryStateWhenSaveFails(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 40, 0, 0, time.Local)
	store := &flakyStore{failOnSave: 2}
	svc, err := NewService(store, nil, WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	item, err := svc.Create(CreateInput{
		Title:      "Daily check",
		AgentID:    "worker",
		Prompt:     "Report status.",
		Recurrence: RecurrenceDaily,
		FirstRunAt: now,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	title := "Updated check"
	if _, err := svc.Update(UpdateInput{ID: item.ID, Title: &title}); err != errFlakyStoreSave {
		t.Fatalf("Update() error = %v, want %v", err, errFlakyStoreSave)
	}
	got := svc.List()[0]
	if got.Title != item.Title {
		t.Fatalf("Title = %q, want unchanged %q after failed save", got.Title, item.Title)
	}
}

func TestDeleteDoesNotCommitMemoryStateWhenSaveFails(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 40, 0, 0, time.Local)
	store := &flakyStore{failOnSave: 2}
	svc, err := NewService(store, nil, WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	item, err := svc.Create(CreateInput{
		Title:      "Daily check",
		AgentID:    "worker",
		Prompt:     "Report status.",
		Recurrence: RecurrenceDaily,
		FirstRunAt: now,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := svc.Delete(item.ID); err != errFlakyStoreSave {
		t.Fatalf("Delete() error = %v, want %v", err, errFlakyStoreSave)
	}
	if got := svc.List(); len(got) != 1 || got[0].ID != item.ID {
		t.Fatalf("List() = %+v, want original task after failed save", got)
	}
}

func TestTriggerDueDoesNotDuplicateWhenFinalSaveFailsAfterTaskCreate(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 40, 0, 0, time.Local)
	store := &flakyStore{failOnSave: 3}
	runner := &fakeAgentTaskRunner{}
	svc, err := NewService(store, nil, WithNowFunc(func() time.Time { return now }), WithAgentTaskRunner(runner))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	item, err := svc.Create(CreateInput{
		Title:      "Daily check",
		AgentID:    "worker",
		Prompt:     "Report status.",
		Recurrence: RecurrenceDaily,
		FirstRunAt: now.Add(-time.Minute),
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if runs := svc.TriggerDue(context.Background()); len(runs) != 0 {
		t.Fatalf("TriggerDue() runs len = %d, want 0 when final save fails", len(runs))
	}
	if len(runner.tasks) != 1 {
		t.Fatalf("created tasks len = %d, want 1", len(runner.tasks))
	}
	if storedRuns := store.state.Runs; len(storedRuns) != 1 || storedRuns[0].TaskID != runner.tasks[0].ID {
		t.Fatalf("stored runs = %+v, want pre-saved run with generated task id %q", storedRuns, runner.tasks[0].ID)
	}
	got := svc.List()[0]
	if got.LastRunAt != nil {
		t.Fatalf("LastRunAt = %v, want nil after failed final save", got.LastRunAt)
	}
	if !got.NextRunAt.Equal(item.NextRunAt) {
		t.Fatalf("NextRunAt = %s, want unchanged %s after failed final save", got.NextRunAt, item.NextRunAt)
	}

	restarted, err := NewService(store, nil, WithNowFunc(func() time.Time { return now }), WithAgentTaskRunner(runner))
	if err != nil {
		t.Fatalf("NewService(restarted) error = %v", err)
	}
	if runs := restarted.TriggerDue(context.Background()); len(runs) != 0 {
		t.Fatalf("TriggerDue(restarted) runs len = %d, want 0 while generated task is active", len(runs))
	}
	if len(runner.tasks) != 1 {
		t.Fatalf("created tasks after restart len = %d, want 1", len(runner.tasks))
	}
	if stored := restarted.Runs(item.ID); len(stored) != 1 || stored[0].TaskID != runner.tasks[0].ID {
		t.Fatalf("Runs() = %+v, want original generated task id %q", stored, runner.tasks[0].ID)
	}
}

func newTestService(t *testing.T, now time.Time) *Service {
	return newTestServiceWithRunner(t, now, nil)
}

func newTestServiceWithRunner(t *testing.T, now time.Time, runner agentTaskRunner) *Service {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	svc, err := NewService(store, nil, WithNowFunc(func() time.Time { return now }), WithAgentTaskRunner(runner))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

type fakeAgentTaskRunner struct {
	tasks []taskcore.Task
}

func (r *fakeAgentTaskRunner) CreateAgentTask(_ context.Context, input agenttask.CreateInput) (taskcore.Task, error) {
	id := input.ID
	if id == "" {
		id = "task-" + string(rune('1'+len(r.tasks)))
	}
	task := taskcore.Task{
		ID:             id,
		AssignmentType: taskcore.AssignmentTypeAgent,
		AssignmentID:   input.AgentID,
		Title:          input.Title,
		Body:           input.Body,
		Status:         taskcore.StatusAssigned,
		CreatedBy:      input.CreatedBy,
	}
	r.tasks = append(r.tasks, task)
	return task, nil
}

func (r *fakeAgentTaskRunner) List() []taskcore.Task {
	return append([]taskcore.Task(nil), r.tasks...)
}

type flakyStore struct {
	state      state
	saveCount  int
	failOnSave int
}

func (s *flakyStore) Load() (state, error) {
	if s.state.NextTaskID == 0 {
		s.state.NextTaskID = 1
	}
	if s.state.NextRunID == 0 {
		s.state.NextRunID = 1
	}
	return s.state, nil
}

func (s *flakyStore) Save(next state) error {
	s.saveCount++
	if s.failOnSave > 0 && s.saveCount == s.failOnSave {
		return errFlakyStoreSave
	}
	s.state = cloneState(next)
	return nil
}

var errFlakyStoreSave = errTestStoreSave{}

type errTestStoreSave struct{}

func (errTestStoreSave) Error() string { return "test store save failed" }

func cloneState(input state) state {
	out := input
	out.Tasks = append([]Task(nil), input.Tasks...)
	out.Runs = append([]Run(nil), input.Runs...)
	return out
}
