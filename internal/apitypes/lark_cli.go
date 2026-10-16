package apitypes

type FeishuBotAppInfo struct {
	AgentID       string `json:"agent_id"`
	ParticipantID string `json:"participant_id"`
	AppID         string `json:"app_id"`
	AppSecret     string `json:"app_secret,omitempty"`
}
