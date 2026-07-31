package apitypes

import "time"

const (
	ParticipantWorkKindAgentTurn = "agent_turn"

	ParticipantWorkStateWorking = "working"
	ParticipantWorkStateIdle    = "idle"

	ParticipantWorkReasonStarted       = "started"
	ParticipantWorkReasonRenewed       = "renewed"
	ParticipantWorkReasonStatusUpdated = "status_updated"
	ParticipantWorkReasonStopRequested = "stop_requested"
	ParticipantWorkReasonStopFailed    = "stop_failed"
	ParticipantWorkReasonStopTimedOut  = "stop_timed_out"
	ParticipantWorkReasonReleased      = "released"
	ParticipantWorkReasonStopped       = "stopped"
	ParticipantWorkReasonExpired       = "expired"

	ParticipantWorkStopStateRequested = "stop_requested"
	ParticipantWorkStopStateFailed    = "stop_failed"
	ParticipantWorkStopStateTimedOut  = "stop_timed_out"
	ParticipantWorkStopStateStopped   = "stopped"

	ParticipantWorkOutcomeReleased     = "released"
	ParticipantWorkOutcomeStopped      = "stopped"
	ParticipantWorkOutcomeStopTimedOut = "stop_timed_out"

	ParticipantWorkCapabilityThinkingStatusV1 = "thinking_status_v1"
	ParticipantWorkCapabilityTurnStopV1       = "turn_stop_v1"
	ParticipantWorkCapabilityStageV1          = "work_stage_v1"

	ParticipantWorkPhaseWorking  = "working"
	ParticipantWorkPhaseThinking = "thinking"

	ParticipantWorkStagePreparingReply  = "preparing_reply"
	ParticipantWorkStageThinking        = "thinking"
	ParticipantWorkStageRunningTool     = "running_tool"
	ParticipantWorkStageGeneratingReply = "generating_reply"

	ParticipantThinkingFormatPlainText = "plain_text"
)

type ParticipantWorkLeaseRequest struct {
	RoomID       string `json:"room_id"`
	ThreadRootID string `json:"thread_root_id,omitempty"`
	RequestID    string `json:"request_id"`
	Kind         string `json:"kind"`
	TTLSeconds   *int   `json:"ttl_seconds,omitempty"`
}

type ParticipantThinkingStatus struct {
	Format    string `json:"format"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

type ParticipantWorkStatus struct {
	Sequence uint64                     `json:"sequence"`
	Phase    string                     `json:"phase"`
	Stage    string                     `json:"stage,omitempty"`
	Thinking *ParticipantThinkingStatus `json:"thinking,omitempty"`
}

type ParticipantWorkStatusPatchRequest struct {
	Capabilities []string                   `json:"capabilities"`
	Sequence     uint64                     `json:"sequence"`
	Phase        string                     `json:"phase"`
	Stage        string                     `json:"stage,omitempty"`
	Thinking     *ParticipantThinkingStatus `json:"thinking,omitempty"`
}

type ParticipantWorkUpdate struct {
	RegistryEpoch   string                 `json:"registry_epoch"`
	LeaseID         string                 `json:"lease_id"`
	ParticipantID   string                 `json:"participant_id"`
	UserID          string                 `json:"user_id"`
	RoomID          string                 `json:"room_id"`
	ThreadRootID    string                 `json:"thread_root_id,omitempty"`
	RequestID       string                 `json:"request_id"`
	Kind            string                 `json:"kind"`
	State           string                 `json:"state"`
	Reason          string                 `json:"reason"`
	Revision        uint64                 `json:"revision"`
	ExpiresAt       time.Time              `json:"expires_at"`
	Capabilities    []string               `json:"capabilities,omitempty"`
	Status          *ParticipantWorkStatus `json:"status,omitempty"`
	StopRequestedAt *time.Time             `json:"stop_requested_at,omitempty"`
	StopState       string                 `json:"stop_state,omitempty"`
	StopError       string                 `json:"stop_error,omitempty"`
}

type ParticipantWorkClosedResponse struct {
	Error         string `json:"error"`
	RegistryEpoch string `json:"registry_epoch"`
}

type ParticipantWorkStopRequest struct {
	RoomID    string `json:"room_id"`
	LeaseID   string `json:"lease_id"`
	RequestID string `json:"request_id"`
}

type ParticipantWorkStopResponse struct {
	Accepted      bool      `json:"accepted"`
	RegistryEpoch string    `json:"registry_epoch"`
	ParticipantID string    `json:"participant_id"`
	UserID        string    `json:"-"`
	RoomID        string    `json:"room_id"`
	ThreadRootID  string    `json:"-"`
	LeaseID       string    `json:"lease_id"`
	RequestID     string    `json:"request_id"`
	State         string    `json:"state"`
	RequestedAt   time.Time `json:"requested_at"`
}

type ParticipantWorkReleaseRequest struct {
	Outcome string `json:"outcome,omitempty"`
}
