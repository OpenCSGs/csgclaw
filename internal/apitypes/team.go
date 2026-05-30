package apitypes

import "time"

type Team struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"room_id"`
	Channel   string    `json:"channel"`
	Title     string    `json:"title"`
	LeadBotID string    `json:"lead_bot_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TeamTask struct {
	ID          string     `json:"id"`
	TeamID      string     `json:"team_id"`
	RoomID      string     `json:"room_id"`
	ParentID    string     `json:"parent_id,omitempty"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	Status      string     `json:"status"`
	CreatedBy   string     `json:"created_by"`
	AssignedTo  string     `json:"assigned_to,omitempty"`
	ClaimedBy   string     `json:"claimed_by,omitempty"`
	DependsOn   []string   `json:"depends_on,omitempty"`
	Priority    int        `json:"priority,omitempty"`
	DeadlineAt  *time.Time `json:"deadline_at,omitempty"`
	TimeoutAt   *time.Time `json:"timeout_at,omitempty"`
	Result      string     `json:"result,omitempty"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
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
	BotID           string    `json:"bot_id"`
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

type CreateTeamRequest struct {
	Channel      string   `json:"channel"`
	RoomID       string   `json:"room_id,omitempty"`
	Title        string   `json:"title,omitempty"`
	LeadBotID    string   `json:"lead_bot_id"`
	MemberBotIDs []string `json:"member_bot_ids,omitempty"`
}

type PatchTeamRequest struct {
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
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
	CreatedBy string                       `json:"created_by"`
	Tasks     []CreateTeamBatchTaskRequest `json:"tasks"`
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

type PatchTeamTaskRequest struct {
	Status string `json:"status,omitempty"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type AssignTeamTaskRequest struct {
	BotID string `json:"bot_id"`
}

type ClaimTeamTaskRequest struct {
	BotID string `json:"bot_id"`
}

type ClaimNextTeamTaskRequest struct {
	TeamID string `json:"team_id,omitempty"`
	BotID  string `json:"bot_id"`
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
	BotID         string `json:"bot_id"`
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
