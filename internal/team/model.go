package team

import "time"

const (
	TeamStatusActive   = "active"
	TeamStatusPaused   = "paused"
	TeamStatusArchived = "archived"

	PresenceStateIdle            = "idle"
	PresenceStateBusy            = "busy"
	PresenceStateBlocked         = "blocked"
	PresenceStateWaitingApproval = "waiting_approval"
	PresenceStateOffline         = "offline"

	TaskStatusPending    = "pending"
	TaskStatusAssigned   = "assigned"
	TaskStatusInProgress = "in_progress"
	TaskStatusBlocked    = "blocked"
	TaskStatusCompleted  = "completed"
	TaskStatusFailed     = "failed"
	TaskStatusCancelled  = "cancelled"

	ApprovalStatusPending   = "pending"
	ApprovalStatusApproved  = "approved"
	ApprovalStatusRejected  = "rejected"
	ApprovalStatusCancelled = "cancelled"

	ProjectionStatusPending = "pending"
	ProjectionStatusSent    = "sent"
	ProjectionStatusFailed  = "failed"

	EventTeamCreated       = "team.created"
	EventTaskCreated       = "task.created"
	EventTaskPlanned       = "task.planned"
	EventTaskExecutionRoom = "task.execution_room"
	EventTaskStarted       = "task.started"
	EventTaskDispatched    = "task.dispatched"
	EventTaskAssigned      = "task.assigned"
	EventTaskClaimed       = "task.claimed"
	EventTaskBlocked       = "task.blocked"
	EventTaskCompleted     = "task.completed"
	EventTaskFailed        = "task.failed"
	EventTaskCancelled     = "task.cancelled"
	EventApprovalRequested = "approval.requested"
	EventApprovalResolved  = "approval.resolved"
	EventPresenceUpdated   = "presence.updated"
	EventProjectionFailed  = "projection.failed"
)

// TeamMeta marks that a room has team orchestration enabled.
type TeamMeta struct {
	ID          string    `json:"id"`
	RoomID      string    `json:"room_id"`
	Channel     string    `json:"channel"`
	Title       string    `json:"title"`
	LeadAgentID string    `json:"lead_agent_id"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MemberPresence captures runtime state derived from room members plus participant identity.
type MemberPresence struct {
	TeamID          string    `json:"team_id"`
	ParticipantID   string    `json:"participant_id"`
	UserID          string    `json:"user_id"`
	AgentID         string    `json:"agent_id"`
	Role            string    `json:"role"`
	State           string    `json:"state"`
	CurrentTaskID   string    `json:"current_task_id,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TeamTask is the authoritative task state, independent of room message projection.
type TeamTask struct {
	ID           string     `json:"id"`
	TeamID       string     `json:"team_id"`
	RoomID       string     `json:"room_id"`
	ParentID     string     `json:"parent_id,omitempty"`
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	Status       string     `json:"status"`
	CreatedBy    string     `json:"created_by"`
	AssignedTo   string     `json:"assigned_to,omitempty"`
	ClaimedBy    string     `json:"claimed_by,omitempty"`
	DependsOn    []string   `json:"depends_on,omitempty"`
	Priority     int        `json:"priority,omitempty"`
	PlanSummary  string     `json:"plan_summary,omitempty"`
	DispatchedAt *time.Time `json:"dispatched_at,omitempty"`
	DeadlineAt   *time.Time `json:"deadline_at,omitempty"`
	TimeoutAt    *time.Time `json:"timeout_at,omitempty"`
	Result       string     `json:"result,omitempty"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

type TeamApproval struct {
	ID          string     `json:"id"`
	TeamID      string     `json:"team_id"`
	RoomID      string     `json:"room_id"`
	TaskID      string     `json:"task_id,omitempty"`
	RequestedBy string     `json:"requested_by"`
	ApproverID  string     `json:"approver_id,omitempty"`
	Kind        string     `json:"kind"`
	Summary     string     `json:"summary"`
	Payload     string     `json:"payload,omitempty"`
	Status      string     `json:"status"`
	Resolution  string     `json:"resolution,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

type TeamEvent struct {
	Seq       int64     `json:"seq"`
	TeamID    string    `json:"team_id"`
	RoomID    string    `json:"room_id"`
	Type      string    `json:"type"`
	ActorID   string    `json:"actor_id,omitempty"`
	TaskID    string    `json:"task_id,omitempty"`
	TargetID  string    `json:"target_id,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// TeamProjection is a deferred Phase 1c+ shape for recording projector delivery outcomes.
type TeamProjection struct {
	EventSeq  int64     `json:"event_seq"`
	TeamID    string    `json:"team_id"`
	Channel   string    `json:"channel"`
	RoomID    string    `json:"room_id"`
	MessageID string    `json:"message_id,omitempty"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}
