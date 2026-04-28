package apitypes

import "time"

type Agent struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	Image           string    `json:"image,omitempty"`
	BoxID           string    `json:"box_id,omitempty"`
	Role            string    `json:"role"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	Profile         string    `json:"profile,omitempty"`
	Provider        string    `json:"provider,omitempty"`
	ModelID         string    `json:"model_id,omitempty"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
}

type CreateAgentRequest struct {
	ID           string             `json:"id,omitempty"`
	Name         string             `json:"name"`
	Description  string             `json:"description,omitempty"`
	Image        string             `json:"image,omitempty"`
	Replace      bool               `json:"replace,omitempty"`
	FieldMask    []string           `json:"field_mask,omitempty"`
	Role         string             `json:"role,omitempty"`
	Status       string             `json:"status,omitempty"`
	CreatedAt    time.Time          `json:"created_at,omitempty"`
	Profile      string             `json:"profile,omitempty"`
	ModelID      string             `json:"model_id,omitempty"`
	AgentProfile CreateAgentProfile `json:"agent_profile,omitempty"`
}

type CreateAgentProfile struct {
	Name            string            `json:"name,omitempty"`
	Description     string            `json:"description,omitempty"`
	Provider        string            `json:"provider,omitempty"`
	BaseURL         string            `json:"base_url,omitempty"`
	APIKey          string            `json:"api_key,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	ModelID         string            `json:"model_id,omitempty"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty"`
	EnableFastMode  bool              `json:"enable_fast_mode,omitempty"`
	RequestOptions  map[string]any    `json:"request_options,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	ProfileComplete bool              `json:"profile_complete"`
}
