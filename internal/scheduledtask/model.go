package scheduledtask

import "time"

const (
	RecurrenceOnce    = "once"
	RecurrenceDaily   = "daily"
	RecurrenceWeekly  = "weekly"
	RecurrenceMonthly = "monthly"

	StatusTriggered = "triggered"
	StatusFailed    = "failed"
)

type Task struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	AgentID    string     `json:"agent_id"`
	Prompt     string     `json:"prompt"`
	Recurrence string     `json:"recurrence"`
	Enabled    bool       `json:"enabled"`
	NextRunAt  time.Time  `json:"next_run_at"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Run struct {
	ID              string    `json:"id"`
	ScheduledTaskID string    `json:"scheduled_task_id"`
	TriggeredAt     time.Time `json:"triggered_at"`
	Status          string    `json:"status"`
	TaskID          string    `json:"task_id,omitempty"`
	Error           string    `json:"error,omitempty"`
}

type CreateInput struct {
	Title      string
	AgentID    string
	Prompt     string
	Recurrence string
	FirstRunAt time.Time
	ExpiresAt  *time.Time
	Enabled    bool
}

type UpdateInput struct {
	ID         string
	Title      *string
	AgentID    *string
	Prompt     *string
	Recurrence *string
	NextRunAt  *time.Time
	ExpiresAt  **time.Time
	Enabled    *bool
}

type state struct {
	NextTaskID int    `json:"next_task_id"`
	NextRunID  int    `json:"next_run_id"`
	Tasks      []Task `json:"tasks"`
	Runs       []Run  `json:"runs"`
}
