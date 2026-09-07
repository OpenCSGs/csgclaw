package apitypes

import "csgclaw/internal/taskcore"

// RoomTask is the room contract over the independent task core, not a Team DTO.
type RoomTask struct {
	taskcore.Task
	AssignedToAgentName string `json:"assigned_to_agent_name,omitempty"`
}
type PlanRoomTaskResponse struct {
	Task           RoomTask   `json:"task"`
	CreatedTasks   []RoomTask `json:"created_tasks"`
	Started        bool       `json:"started"`
	ScheduledTasks int        `json:"scheduled_tasks"`
}
type StartRoomTaskResponse struct {
	Task RoomTask `json:"task"`
}
type UpdateRoomTaskRequest struct {
	ActorID string `json:"actor_id"`
	Attempt int    `json:"attempt"`
	Status  string `json:"status"`
	Result  string `json:"result"`
	Error   string `json:"error"`
	Reason  string `json:"reason"`
}
type ClaimRoomTaskRequest struct {
	ParticipantID string `json:"participant_id"`
	Attempt       int    `json:"attempt"`
}
type ReviewRoomTaskRequest struct {
	Attempt int    `json:"attempt"`
	Accept  bool   `json:"accept"`
	Summary string `json:"summary"`
}

type RoomTaskMessageRequest struct {
	ActorID   string `json:"actor_id"`
	TargetID  string `json:"target_id"`
	Content   string `json:"content"`
	MessageID string `json:"message_id"`
}

type RoomTaskPlanItem struct {
	IDRef         string   `json:"id_ref"`
	Title         string   `json:"title"`
	Body          string   `json:"body,omitempty"`
	AssignedTo    string   `json:"assigned_to"`
	DependsOnRefs []string `json:"depends_on_refs,omitempty"`
}
type PlanRoomTaskRequest struct {
	Append    bool               `json:"append,omitempty"`
	RequestID string             `json:"request_id,omitempty"`
	Summary   string             `json:"summary,omitempty"`
	Tasks     []RoomTaskPlanItem `json:"tasks"`
	AutoStart bool               `json:"auto_start"`
}
