package apitypes

import "time"

type FeishuBotAppInfo struct {
	AgentID       string `json:"agent_id"`
	ParticipantID string `json:"participant_id"`
	AppID         string `json:"app_id"`
	AppSecret     string `json:"app_secret,omitempty"`
}

type AgentLarkCLIInitResponse struct {
	Status        string `json:"status"`
	AgentID       string `json:"agent_id"`
	ParticipantID string `json:"participant_id"`
	AppID         string `json:"app_id"`
	Generation    int64  `json:"generation"`
	RuntimeLoaded bool   `json:"runtime_loaded"`
	Warning       string `json:"warning,omitempty"`
	RestartStatus string `json:"restart_status,omitempty"`
	RestartError  string `json:"restart_error,omitempty"`
}

type AgentLarkCLIStatus struct {
	Bound              bool       `json:"bound"`
	Available          bool       `json:"available"`
	State              string     `json:"state"`
	Error              string     `json:"error,omitempty"`
	AppID              string     `json:"app_id,omitempty"`
	Generation         int64      `json:"generation,omitempty"`
	ObservedGeneration int64      `json:"observed_generation,omitempty"`
	RuntimeLoaded      bool       `json:"runtime_loaded"`
	CleanupPending     bool       `json:"cleanup_pending,omitempty"`
	Reason             string     `json:"reason,omitempty"`
	CheckedAt          *time.Time `json:"checked_at,omitempty"`
	BoundAt            *time.Time `json:"bound_at,omitempty"`
}
