package apitypes

import "time"

type Team struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	LeadAgentID    string    `json:"lead_agent_id"`
	LeadAgentName  string    `json:"lead_agent_name,omitempty"`
	MemberAgentIDs []string  `json:"member_agent_ids,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type TeamTask struct {
	ID                  string     `json:"id"`
	AssignmentType      string     `json:"assignment_type,omitempty"`
	AssignmentID        string     `json:"assignment_id,omitempty"`
	TeamID              string     `json:"team_id"`
	ExecutionChannel    string     `json:"execution_channel"`
	RoomID              string     `json:"room_id"`
	ParentID            string     `json:"parent_id,omitempty"`
	Title               string     `json:"title"`
	Body                string     `json:"body"`
	Status              string     `json:"status"`
	CreatedBy           string     `json:"created_by"`
	CreatedByAgentName  string     `json:"created_by_agent_name,omitempty"`
	AssignedTo          string     `json:"assigned_to,omitempty"`
	AssignedToAgentName string     `json:"assigned_to_agent_name,omitempty"`
	ClaimedBy           string     `json:"claimed_by,omitempty"`
	ClaimedByAgentName  string     `json:"claimed_by_agent_name,omitempty"`
	DependsOn           []string   `json:"depends_on,omitempty"`
	Priority            int        `json:"priority,omitempty"`
	PlanSummary         string     `json:"plan_summary,omitempty"`
	DispatchedAt        *time.Time `json:"dispatched_at,omitempty"`
	DeadlineAt          *time.Time `json:"deadline_at,omitempty"`
	TimeoutAt           *time.Time `json:"timeout_at,omitempty"`
	Result              string     `json:"result,omitempty"`
	Error               string     `json:"error,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
}

type GlobalTask struct {
	TeamTask
	TeamTitle string `json:"team_title,omitempty"`
	RoomTitle string `json:"room_title,omitempty"`
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

type TeamPresence struct {
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

type TeamEvent struct {
	Seq             int64     `json:"seq"`
	TeamID          string    `json:"team_id"`
	Channel         string    `json:"channel,omitempty"`
	RoomID          string    `json:"room_id"`
	Type            string    `json:"type"`
	ActorID         string    `json:"actor_id,omitempty"`
	ActorAgentName  string    `json:"actor_agent_name,omitempty"`
	TaskID          string    `json:"task_id,omitempty"`
	TargetID        string    `json:"target_id,omitempty"`
	TargetAgentName string    `json:"target_agent_name,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type CreateTeamRequest struct {
	Title                string   `json:"title,omitempty"`
	LeadAgentID          string   `json:"lead_agent_id,omitempty"`
	LeadParticipantID    string   `json:"lead_participant_id,omitempty"`
	MemberAgentIDs       []string `json:"member_agent_ids,omitempty"`
	MemberParticipantIDs []string `json:"member_participant_ids,omitempty"`
}

type PatchTeamRequest struct {
	Title          string   `json:"title,omitempty"`
	LeadAgentID    string   `json:"lead_agent_id,omitempty"`
	MemberAgentIDs []string `json:"member_agent_ids,omitempty"`
	Status         string   `json:"status,omitempty"`
}

type CreateTeamTaskRequest struct {
	ParentID   string     `json:"parent_id,omitempty"`
	Title      string     `json:"title"`
	Body       string     `json:"body,omitempty"`
	AssignTo   string     `json:"assign_to,omitempty"`
	DependsOn  []string   `json:"depends_on,omitempty"`
	Priority   int        `json:"priority,omitempty"`
	DeadlineAt *time.Time `json:"deadline_at,omitempty"`
	TimeoutAt  *time.Time `json:"timeout_at,omitempty"`
}

type CreateTeamTasksBatchRequest struct {
	CreatedBy        string                       `json:"created_by"`
	ExecutionChannel string                       `json:"execution_channel,omitempty"`
	Tasks            []CreateTeamBatchTaskRequest `json:"tasks"`
}

type CreateTeamBatchTaskRequest struct {
	IDRef         string     `json:"id_ref,omitempty"`
	ParentID      string     `json:"parent_id,omitempty"`
	ParentRef     string     `json:"parent_ref,omitempty"`
	Title         string     `json:"title"`
	Body          string     `json:"body,omitempty"`
	AssignTo      string     `json:"assign_to,omitempty"`
	DependsOnRefs []string   `json:"depends_on_refs,omitempty"`
	Priority      int        `json:"priority,omitempty"`
	DeadlineAt    *time.Time `json:"deadline_at,omitempty"`
	TimeoutAt     *time.Time `json:"timeout_at,omitempty"`
}

type TeamTaskIDRef struct {
	IDRef  string `json:"id_ref"`
	TaskID string `json:"task_id"`
}

type CreateTeamTasksBatchResponse struct {
	Tasks  []TeamTask      `json:"tasks"`
	IDRefs []TeamTaskIDRef `json:"id_refs,omitempty"`
}

type PlanTeamTaskRequest struct {
	ActorID   string `json:"actor_id,omitempty"`
	AutoStart bool   `json:"auto_start,omitempty"`
}

type PlanTeamTaskResponse struct {
	Task           TeamTask   `json:"task"`
	CreatedTasks   []TeamTask `json:"created_tasks"`
	AlreadyPlanned bool       `json:"already_planned"`
	Planning       bool       `json:"planning,omitempty"`
	Started        bool       `json:"started,omitempty"`
	ScheduledTasks int        `json:"scheduled_tasks,omitempty"`
}

type StartTeamTaskRequest struct {
	ActorID string `json:"actor_id,omitempty"`
}

type StartTeamTaskResponse struct {
	Task           TeamTask `json:"task"`
	ScheduledTasks int      `json:"scheduled_tasks"`
}

type PatchTeamTaskRequest struct {
	Status string `json:"status,omitempty"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type AssignTeamTaskRequest struct {
	ParticipantID string `json:"participant_id"`
}

type ClaimTeamTaskRequest struct {
	ParticipantID string `json:"participant_id"`
}

type ClaimNextTeamTaskRequest struct {
	TeamID        string `json:"team_id,omitempty"`
	ParticipantID string `json:"participant_id"`
}

type CreateAgentTaskRequest struct {
	AgentID   string `json:"agent_id"`
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	CreatedBy string `json:"created_by,omitempty"`
}

type ClaimAgentTaskRequest struct {
	ParticipantID string `json:"participant_id"`
}

type PatchAgentTaskRequest struct {
	ActorID string `json:"actor_id,omitempty"`
	Status  string `json:"status,omitempty"`
	Result  string `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type ScheduledTask struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	AgentID    string     `json:"agent_id"`
	AgentName  string     `json:"agent_name,omitempty"`
	Prompt     string     `json:"prompt"`
	Recurrence string     `json:"recurrence"`
	Enabled    bool       `json:"enabled"`
	NextRunAt  time.Time  `json:"next_run_at"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type ScheduledTaskRun struct {
	ID              string    `json:"id"`
	ScheduledTaskID string    `json:"scheduled_task_id"`
	TriggeredAt     time.Time `json:"triggered_at"`
	Status          string    `json:"status"`
	TaskID          string    `json:"task_id,omitempty"`
	Error           string    `json:"error,omitempty"`
}

type CreateScheduledTaskRequest struct {
	Title      string     `json:"title"`
	AgentID    string     `json:"agent_id"`
	Prompt     string     `json:"prompt"`
	Recurrence string     `json:"recurrence"`
	FirstRunAt time.Time  `json:"first_run_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Enabled    *bool      `json:"enabled,omitempty"`
}

type PatchScheduledTaskRequest struct {
	Title      *string    `json:"title,omitempty"`
	AgentID    *string    `json:"agent_id,omitempty"`
	Prompt     *string    `json:"prompt,omitempty"`
	Recurrence *string    `json:"recurrence,omitempty"`
	NextRunAt  *time.Time `json:"next_run_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Enabled    *bool      `json:"enabled,omitempty"`
}

type CreateTeamApprovalRequest struct {
	TaskID      string `json:"task_id,omitempty"`
	RequestedBy string `json:"requested_by"`
	ApproverID  string `json:"approver_id,omitempty"`
	Kind        string `json:"kind"`
	Summary     string `json:"summary"`
	Payload     string `json:"payload,omitempty"`
}

type ResolveTeamApprovalRequest struct {
	ApproverID string `json:"approver_id,omitempty"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
}

type UpsertTeamPresenceRequest struct {
	ParticipantID string `json:"participant_id"`
	UserID        string `json:"user_id,omitempty"`
	AgentID       string `json:"agent_id,omitempty"`
	Role          string `json:"role,omitempty"`
	State         string `json:"state"`
	CurrentTaskID string `json:"current_task_id,omitempty"`
	Summary       string `json:"summary,omitempty"`
}

type TeamPresenceHeartbeatResponse struct {
	Presence             TeamPresence `json:"presence"`
	NextHeartbeatSeconds int          `json:"next_heartbeat_seconds,omitempty"`
}
