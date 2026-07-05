package apitypes

import (
	"encoding/json"
	"strings"
	"time"
)

func runtimeKindFromSelection(name string, sandboxEnabled bool) string {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "picoclaw":
		if sandboxEnabled {
			return "picoclaw"
		}
	case "openclaw":
		if sandboxEnabled {
			return "openclaw"
		}
	case "codex":
		if sandboxEnabled {
			return "codex_sandbox"
		}
		if !sandboxEnabled {
			return "codex"
		}
	}
	return ""
}

func runtimeSelectionForKind(kind string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case "picoclaw", "picoclaw_sandbox":
		return "picoclaw", true
	case "openclaw", "openclaw_sandbox":
		return "openclaw", true
	case "codex_sandbox":
		return "codex", true
	case "codex":
		return "codex", false
	default:
		return "", false
	}
}

type AgentRuntime struct {
	Kind           string           `json:"-"`
	Name           string           `json:"name,omitempty"`
	SandboxEnabled bool             `json:"sandbox_enabled,omitempty"`
	State          string           `json:"state,omitempty"`
	SandboxID      string           `json:"sandbox_id,omitempty"`
	Options        map[string]any   `json:"options,omitempty"`
	OptionSchemas  []map[string]any `json:"option_schemas,omitempty"`
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
	RuntimeName      string        `json:"runtime_name,omitempty"`
	SandboxEnabled   bool          `json:"sandbox_enabled,omitempty"`
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
		RuntimeID   string `json:"runtime_id"`
		RuntimeKind string `json:"runtime_kind"`
		RuntimeName string `json:"runtime_name"`
		Runtime     struct {
			Kind           string `json:"kind"`
			Name           string `json:"name"`
			SandboxEnabled *bool  `json:"sandbox_enabled"`
		} `json:"runtime"`
		SandboxEnabled *bool          `json:"sandbox_enabled"`
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
	if strings.TrimSpace(a.RuntimeName) == "" {
		a.RuntimeName = strings.TrimSpace(legacy.RuntimeName)
	}
	if strings.TrimSpace(a.RuntimeKind) == "" {
		a.RuntimeKind = strings.TrimSpace(legacy.Runtime.Kind)
	}
	if strings.TrimSpace(a.RuntimeName) == "" {
		a.RuntimeName = strings.TrimSpace(legacy.Runtime.Name)
	}
	if legacy.SandboxEnabled != nil {
		a.SandboxEnabled = *legacy.SandboxEnabled
	}
	if !a.SandboxEnabled && legacy.Runtime.SandboxEnabled != nil {
		a.SandboxEnabled = *legacy.Runtime.SandboxEnabled
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
	if strings.TrimSpace(a.RuntimeName) == "" {
		a.RuntimeName = strings.TrimSpace(a.Runtime.Name)
	}
	if strings.TrimSpace(a.RuntimeName) == "" {
		if name, _ := runtimeSelectionForKind(a.RuntimeKind); name != "" {
			a.RuntimeName = name
		}
	}
	if !a.SandboxEnabled {
		a.SandboxEnabled = a.Runtime.SandboxEnabled
	}
	if !a.SandboxEnabled {
		_, a.SandboxEnabled = runtimeSelectionForKind(a.RuntimeKind)
	}
	if strings.TrimSpace(a.Status) == "" {
		a.Status = strings.TrimSpace(a.Runtime.State)
	}
	if strings.TrimSpace(a.BoxID) == "" {
		a.BoxID = strings.TrimSpace(a.Runtime.SandboxID)
	}
	if strings.TrimSpace(a.RuntimeKind) == "" {
		a.RuntimeKind = runtimeKindFromSelection(a.RuntimeName, a.SandboxEnabled)
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
	RuntimeKind    string              `json:"-"`
	RuntimeName    string              `json:"runtime_name,omitempty"`
	SandboxEnabled bool                `json:"sandbox_enabled,omitempty"`
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

type createAgentRequestJSON struct {
	ID             string              `json:"id,omitempty"`
	Name           string              `json:"name"`
	Description    string              `json:"description,omitempty"`
	Instructions   string              `json:"instructions,omitempty"`
	Image          string              `json:"image,omitempty"`
	RuntimeKind    string              `json:"runtime_kind,omitempty"`
	RuntimeName    string              `json:"runtime_name,omitempty"`
	SandboxEnabled bool                `json:"sandbox_enabled,omitempty"`
	FromTemplate   string              `json:"from_template,omitempty"`
	Replace        bool                `json:"replace,omitempty"`
	FieldMask      []string            `json:"field_mask,omitempty"`
	Role           string              `json:"role,omitempty"`
	Status         string              `json:"status,omitempty"`
	CreatedAt      time.Time           `json:"created_at,omitempty"`
	Runtime        AgentRuntime        `json:"runtime,omitempty"`
	RuntimeOptions map[string]any      `json:"runtime_options,omitempty"`
	ModelConfig    *CreateAgentProfile `json:"model_config,omitempty"`
	Profile        json.RawMessage     `json:"profile,omitempty"`
	AgentProfile   *CreateAgentProfile `json:"agent_profile,omitempty"`
}

func (r CreateAgentRequest) MarshalJSON() ([]byte, error) {
	runtime := r.Runtime
	if strings.TrimSpace(runtime.Kind) == "" {
		runtime.Kind = strings.TrimSpace(r.RuntimeKind)
		if runtime.Kind == "" {
			runtime.Kind = runtimeKindFromSelection(r.RuntimeName, r.SandboxEnabled)
		}
	}
	if strings.TrimSpace(runtime.Name) == "" {
		runtime.Name = strings.TrimSpace(r.RuntimeName)
		if runtime.Name == "" {
			runtime.Name, runtime.SandboxEnabled = runtimeSelectionForKind(runtime.Kind)
		}
	}
	if !runtime.SandboxEnabled {
		runtime.SandboxEnabled = r.SandboxEnabled
	}
	var profile json.RawMessage
	if selector := strings.TrimSpace(r.Profile); selector != "" {
		payload, err := json.Marshal(selector)
		if err != nil {
			return nil, err
		}
		profile = payload
	}
	return json.Marshal(createAgentRequestJSON{
		ID:             r.ID,
		Name:           r.Name,
		Description:    r.Description,
		Instructions:   r.Instructions,
		Image:          r.Image,
		RuntimeName:    runtime.Name,
		SandboxEnabled: runtime.SandboxEnabled,
		FromTemplate:   r.FromTemplate,
		Replace:        r.Replace,
		FieldMask:      r.FieldMask,
		Role:           r.Role,
		Status:         r.Status,
		CreatedAt:      r.CreatedAt,
		Runtime:        runtime,
		RuntimeOptions: r.RuntimeOptions,
		ModelConfig:    r.ProfileConfig,
		Profile:        profile,
		AgentProfile:   r.AgentProfile,
	})
}

func (r *CreateAgentRequest) UnmarshalJSON(data []byte) error {
	var decoded createAgentRequestJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var runtimeFields struct {
		SandboxEnabled *bool `json:"sandbox_enabled,omitempty"`
		Runtime        struct {
			Kind           string         `json:"kind,omitempty"`
			Name           string         `json:"name,omitempty"`
			SandboxEnabled *bool          `json:"sandbox_enabled,omitempty"`
			Options        map[string]any `json:"options,omitempty"`
		} `json:"runtime,omitempty"`
	}
	if err := json.Unmarshal(data, &runtimeFields); err != nil {
		return err
	}
	out := CreateAgentRequest{
		ID:             decoded.ID,
		Name:           decoded.Name,
		Description:    decoded.Description,
		Instructions:   decoded.Instructions,
		Image:          decoded.Image,
		RuntimeKind:    strings.TrimSpace(decoded.RuntimeKind),
		RuntimeName:    strings.TrimSpace(decoded.RuntimeName),
		SandboxEnabled: decoded.SandboxEnabled,
		FromTemplate:   decoded.FromTemplate,
		Replace:        decoded.Replace,
		FieldMask:      decoded.FieldMask,
		Role:           decoded.Role,
		Status:         decoded.Status,
		CreatedAt:      decoded.CreatedAt,
		Runtime:        decoded.Runtime,
		RuntimeOptions: decoded.RuntimeOptions,
		ProfileConfig:  decoded.ModelConfig,
		AgentProfile:   decoded.AgentProfile,
	}
	if runtimeFields.SandboxEnabled != nil {
		out.SandboxEnabled = *runtimeFields.SandboxEnabled
	}
	if strings.TrimSpace(runtimeFields.Runtime.Kind) != "" {
		out.Runtime.Kind = strings.TrimSpace(runtimeFields.Runtime.Kind)
	}
	if strings.TrimSpace(runtimeFields.Runtime.Name) != "" {
		out.Runtime.Name = strings.TrimSpace(runtimeFields.Runtime.Name)
	}
	if runtimeFields.Runtime.SandboxEnabled != nil && !out.SandboxEnabled {
		out.SandboxEnabled = *runtimeFields.Runtime.SandboxEnabled
	}
	if len(out.Runtime.Options) == 0 && len(runtimeFields.Runtime.Options) > 0 {
		out.Runtime.Options = runtimeFields.Runtime.Options
	}
	if len(decoded.Profile) > 0 && string(decoded.Profile) != "null" {
		var selector string
		if err := json.Unmarshal(decoded.Profile, &selector); err == nil {
			out.Profile = strings.TrimSpace(selector)
		} else {
			var profile CreateAgentProfile
			if err := json.Unmarshal(decoded.Profile, &profile); err != nil {
				return err
			}
			if out.ProfileConfig == nil {
				out.ProfileConfig = &profile
			}
		}
	}
	out.Runtime.Kind = strings.TrimSpace(out.Runtime.Kind)
	out.Runtime.Name = strings.TrimSpace(out.Runtime.Name)
	if strings.TrimSpace(out.RuntimeKind) == "" {
		out.RuntimeKind = strings.TrimSpace(out.Runtime.Kind)
	}
	if strings.TrimSpace(out.RuntimeName) == "" {
		out.RuntimeName = strings.TrimSpace(out.Runtime.Name)
	}
	if !out.SandboxEnabled {
		out.SandboxEnabled = out.Runtime.SandboxEnabled
	}
	if strings.TrimSpace(out.RuntimeName) == "" {
		if name, _ := runtimeSelectionForKind(out.RuntimeKind); name != "" {
			out.RuntimeName = name
		}
	}
	if !out.SandboxEnabled {
		_, out.SandboxEnabled = runtimeSelectionForKind(out.RuntimeKind)
	}
	if len(out.RuntimeOptions) == 0 && len(out.Runtime.Options) > 0 {
		out.RuntimeOptions = out.Runtime.Options
	}
	if strings.TrimSpace(out.RuntimeKind) == "" {
		out.RuntimeKind = runtimeKindFromSelection(out.RuntimeName, out.SandboxEnabled)
	}
	out.Runtime.SandboxEnabled = out.SandboxEnabled
	*r = out
	return nil
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
