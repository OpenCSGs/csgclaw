package apitypes

import (
	"encoding/json"
	"strings"
	"time"
)

type AgentRuntime struct {
	Kind          string           `json:"kind,omitempty"`
	State         string           `json:"state,omitempty"`
	SandboxID     string           `json:"sandbox_id,omitempty"`
	Options       map[string]any   `json:"options,omitempty"`
	OptionSchemas []map[string]any `json:"option_schemas,omitempty"`
}

type AgentProfile struct {
	ModelProviderID      string                   `json:"model_provider_id,omitempty"`
	BaseURL              string                   `json:"base_url,omitempty"`
	APIKey               string                   `json:"api_key,omitempty"`
	APIKeySet            bool                     `json:"api_key_set,omitempty"`
	APIKeyPreview        string                   `json:"api_key_preview,omitempty"`
	Headers              map[string]string        `json:"headers,omitempty"`
	ModelID              string                   `json:"model_id,omitempty"`
	ReasoningEffort      string                   `json:"reasoning_effort,omitempty"`
	EnableFastMode       bool                     `json:"enable_fast_mode,omitempty"`
	RequestOptions       map[string]any           `json:"request_options,omitempty"`
	Env                  map[string]string        `json:"env,omitempty"`
	EnvRestartRequired   bool                     `json:"env_restart_required,omitempty"`
	ImageUpgradeRequired bool                     `json:"image_upgrade_required,omitempty"`
	DetectionResults     []ProfileDetectionResult `json:"detection_results,omitempty"`
}

type ProfileDetectionResult struct {
	Provider string `json:"provider"`
	Status   string `json:"status"`
	ModelID  string `json:"model_id,omitempty"`
	Error    string `json:"error,omitempty"`
}

type Agent struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Description      string        `json:"description,omitempty"`
	Instructions     string        `json:"instructions,omitempty"`
	Runtime          AgentRuntime  `json:"runtime,omitempty"`
	RuntimeID        string        `json:"-"`
	RuntimeKind      string        `json:"-"`
	Image            string        `json:"image,omitempty"`
	Avatar           string        `json:"-"`
	BoxID            string        `json:"-"`
	Role             string        `json:"role"`
	Status           string        `json:"-"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at,omitempty"`
	Profile          string        `json:"-"`
	ProfileConfig    AgentProfile  `json:"model_config,omitempty"`
	UserID           string        `json:"user_id,omitempty"`
	UserName         string        `json:"user_name,omitempty"`
	ParticipantIDs   []string      `json:"participant_ids,omitempty"`
	ParticipantNames []string      `json:"participant_names,omitempty"`
	Participants     []Participant `json:"participants,omitempty"`
}

func (a *Agent) UnmarshalJSON(data []byte) error {
	type agentAlias Agent
	type agentJSON struct {
		agentAlias
		ModelConfig json.RawMessage `json:"model_config"`
		Profile     json.RawMessage `json:"profile"`
	}
	var decoded agentJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*a = Agent(decoded.agentAlias)
	profilePayload := decoded.ModelConfig
	if len(profilePayload) == 0 || string(profilePayload) == "null" {
		profilePayload = decoded.Profile
	}
	if len(profilePayload) > 0 && string(profilePayload) != "null" {
		var profile AgentProfile
		if err := json.Unmarshal(profilePayload, &profile); err == nil {
			a.ProfileConfig = profile
			a.Profile = profileSelector(profile)
		} else {
			var selector string
			if err := json.Unmarshal(profilePayload, &selector); err != nil {
				return err
			}
			a.Profile = strings.TrimSpace(selector)
		}
	}
	var legacy struct {
		RuntimeID      string         `json:"runtime_id"`
		RuntimeKind    string         `json:"runtime_kind"`
		Status         string         `json:"status"`
		BoxID          string         `json:"box_id"`
		RuntimeOptions map[string]any `json:"runtime_options"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	if strings.TrimSpace(a.RuntimeID) == "" {
		a.RuntimeID = strings.TrimSpace(legacy.RuntimeID)
	}
	if strings.TrimSpace(a.RuntimeKind) == "" {
		a.RuntimeKind = strings.TrimSpace(legacy.RuntimeKind)
	}
	if strings.TrimSpace(a.Status) == "" {
		a.Status = strings.TrimSpace(legacy.Status)
	}
	if strings.TrimSpace(a.BoxID) == "" {
		a.BoxID = strings.TrimSpace(legacy.BoxID)
	}
	if len(a.Runtime.Options) == 0 && len(legacy.RuntimeOptions) > 0 {
		a.Runtime.Options = legacy.RuntimeOptions
	}
	if strings.TrimSpace(a.RuntimeKind) == "" {
		a.RuntimeKind = strings.TrimSpace(a.Runtime.Kind)
	}
	if strings.TrimSpace(a.Status) == "" {
		a.Status = strings.TrimSpace(a.Runtime.State)
	}
	if strings.TrimSpace(a.BoxID) == "" {
		a.BoxID = strings.TrimSpace(a.Runtime.SandboxID)
	}
	return nil
}

func profileSelector(profile AgentProfile) string {
	provider := strings.TrimSpace(profile.ModelProviderID)
	model := strings.TrimSpace(profile.ModelID)
	switch {
	case provider != "" && model != "":
		return provider + "." + model
	case model != "":
		return model
	default:
		return ""
	}
}

type CreateAgentRequest struct {
	ID             string              `json:"id,omitempty"`
	Name           string              `json:"name"`
	Description    string              `json:"description,omitempty"`
	Instructions   string              `json:"instructions,omitempty"`
	Image          string              `json:"image,omitempty"`
	RuntimeKind    string              `json:"runtime_kind,omitempty"`
	FromTemplate   string              `json:"from_template,omitempty"`
	Replace        bool                `json:"replace,omitempty"`
	FieldMask      []string            `json:"field_mask,omitempty"`
	Role           string              `json:"role,omitempty"`
	Status         string              `json:"status,omitempty"`
	CreatedAt      time.Time           `json:"created_at,omitempty"`
	Runtime        AgentRuntime        `json:"runtime,omitempty"`
	RuntimeOptions map[string]any      `json:"runtime_options,omitempty"`
	Profile        string              `json:"-"`
	ProfileConfig  *CreateAgentProfile `json:"model_config,omitempty"`
	AgentProfile   *CreateAgentProfile `json:"agent_profile,omitempty"`
}

func (r CreateAgentRequest) MarshalJSON() ([]byte, error) {
	type createAgentRequestJSON struct {
		ID             string              `json:"id,omitempty"`
		Name           string              `json:"name"`
		Description    string              `json:"description,omitempty"`
		Instructions   string              `json:"instructions,omitempty"`
		Image          string              `json:"image,omitempty"`
		RuntimeKind    string              `json:"runtime_kind,omitempty"`
		FromTemplate   string              `json:"from_template,omitempty"`
		Replace        bool                `json:"replace,omitempty"`
		FieldMask      []string            `json:"field_mask,omitempty"`
		Role           string              `json:"role,omitempty"`
		Status         string              `json:"status,omitempty"`
		CreatedAt      time.Time           `json:"created_at,omitempty"`
		Runtime        AgentRuntime        `json:"runtime,omitempty"`
		RuntimeOptions map[string]any      `json:"runtime_options,omitempty"`
		ModelConfig    *CreateAgentProfile `json:"model_config,omitempty"`
		Profile        string              `json:"profile,omitempty"`
		AgentProfile   *CreateAgentProfile `json:"agent_profile,omitempty"`
	}
	profile := strings.TrimSpace(r.Profile)
	return json.Marshal(createAgentRequestJSON{
		ID:             r.ID,
		Name:           r.Name,
		Description:    r.Description,
		Instructions:   r.Instructions,
		Image:          r.Image,
		RuntimeKind:    r.RuntimeKind,
		FromTemplate:   r.FromTemplate,
		Replace:        r.Replace,
		FieldMask:      r.FieldMask,
		Role:           r.Role,
		Status:         r.Status,
		CreatedAt:      r.CreatedAt,
		Runtime:        r.Runtime,
		RuntimeOptions: r.RuntimeOptions,
		ModelConfig:    r.ProfileConfig,
		Profile:        profile,
		AgentProfile:   r.AgentProfile,
	})
}

type CreateAgentProfile struct {
	ModelProviderID string            `json:"model_provider_id,omitempty"`
	BaseURL         string            `json:"base_url,omitempty"`
	APIKey          string            `json:"api_key,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	ModelID         string            `json:"model_id,omitempty"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty"`
	EnableFastMode  bool              `json:"enable_fast_mode,omitempty"`
	RequestOptions  map[string]any    `json:"request_options,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
}
