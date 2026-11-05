package apitypes

import (
	"bytes"
	"csgclaw/internal/modelprovider"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func runtimeKindFromSelection(name string, sandboxEnabled bool) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if !sandboxEnabled && (name == "codex" || name == "dsh") {
		return name
	}
	return ""
}

func runtimeSelectionForKind(kind string) (string, bool) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "codex" || kind == "dsh" {
		return kind, false
	}
	return "", false
}

type AgentRuntime struct {
	Kind            string               `json:"-"`
	Image           string               `json:"image,omitempty"`
	Name            string               `json:"name,omitempty"`
	SandboxEnabled  bool                 `json:"sandbox_enabled,omitempty"`
	SandboxProvider string               `json:"sandbox_provider,omitempty"`
	State           string               `json:"state,omitempty"`
	Availability    *RuntimeAvailability `json:"availability,omitempty"`
	StartupPending  bool                 `json:"startup_pending,omitempty"`
	SandboxID       string               `json:"sandbox_id,omitempty"`
	Options         map[string]any       `json:"options,omitempty"`
	OptionSchemas   []map[string]any     `json:"option_schemas,omitempty"`
}

// RuntimeAvailability is the latest transient application-readiness
// observation. It does not alter the durable lifecycle state in State.
type RuntimeAvailability struct {
	State     string     `json:"state"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Reason    string     `json:"reason,omitempty"`
}

type AgentProfile struct {
	ImageGeneration      *modelprovider.ImageGenerationConfig `json:"image_generation"`
	VideoGeneration      *modelprovider.VideoGenerationConfig `json:"video_generation,omitempty"`
	ModelProviderID      string                               `json:"model_provider_id,omitempty"`
	BaseURL              string                               `json:"base_url,omitempty"`
	APIKey               string                               `json:"api_key,omitempty"`
	APIKeySet            bool                                 `json:"api_key_set,omitempty"`
	APIKeyPreview        string                               `json:"api_key_preview,omitempty"`
	Headers              map[string]string                    `json:"headers,omitempty"`
	ModelID              string                               `json:"model_id,omitempty"`
	ReasoningEffort      string                               `json:"reasoning_effort,omitempty"`
	EnableFastMode       bool                                 `json:"enable_fast_mode,omitempty"`
	RequestOptions       map[string]any                       `json:"request_options,omitempty"`
	Env                  map[string]string                    `json:"env,omitempty"`
	EnvRestartRequired   bool                                 `json:"env_restart_required,omitempty"`
	ImageUpgradeRequired bool                                 `json:"image_upgrade_required,omitempty"`
	DetectionResults     []ProfileDetectionResult             `json:"detection_results,omitempty"`
}

type ProfileDetectionResult struct {
	Provider string `json:"provider"`
	Status   string `json:"status"`
	ModelID  string `json:"model_id,omitempty"`
	Error    string `json:"error,omitempty"`
}

type Agent struct {
	ArchivedAt       *time.Time     `json:"archived_at,omitempty"`
	ArchivedBy       string         `json:"archived_by,omitempty"`
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Description      string         `json:"description,omitempty"`
	Instructions     string         `json:"instructions,omitempty"`
	MemorySupported  bool           `json:"memory_supported,omitempty"`
	Runtime          AgentRuntime   `json:"runtime,omitempty"`
	RuntimeID        string         `json:"-"`
	RuntimeKind      string         `json:"-"`
	RuntimeName      string         `json:"-"`
	SandboxEnabled   bool           `json:"-"`
	MCPServers       map[string]any `json:"mcp_servers,omitempty"`
	Image            string         `json:"-"`
	Avatar           string         `json:"-"`
	BoxID            string         `json:"-"`
	Role             string         `json:"role"`
	Status           string         `json:"-"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at,omitempty"`
	Profile          string         `json:"-"`
	ProfileConfig    AgentProfile   `json:"model_config,omitempty"`
	UserID           string         `json:"user_id,omitempty"`
	UserName         string         `json:"user_name,omitempty"`
	ParticipantIDs   []string       `json:"participant_ids,omitempty"`
	ParticipantNames []string       `json:"participant_names,omitempty"`
	Participants     []Participant  `json:"participants,omitempty"`
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
	a.Image = a.Runtime.Image
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

// AgentRuntimeInput contains only writable Runtime configuration.
type AgentRuntimeInput struct {
	Name           *string         `json:"name,omitempty"`
	SandboxEnabled *bool           `json:"sandbox_enabled,omitempty"`
	Image          *string         `json:"image,omitempty"`
	Options        *map[string]any `json:"options,omitempty"`
}

type CreateAgentRequest struct {
	ID            string              `json:"id,omitempty"`
	Name          string              `json:"name"`
	Description   string              `json:"description,omitempty"`
	Instructions  string              `json:"instructions,omitempty"`
	FromTemplate  string              `json:"from_template,omitempty"`
	Role          string              `json:"role,omitempty"`
	Status        string              `json:"status,omitempty"`
	CreatedAt     time.Time           `json:"created_at,omitempty"`
	Runtime       *AgentRuntimeInput  `json:"runtime,omitempty"`
	MCPServers    map[string]any      `json:"mcp_servers,omitempty"`
	MCPServersSet bool                `json:"-"`
	ModelConfig   *CreateAgentProfile `json:"model_config,omitempty"`
}

func (r CreateAgentRequest) MarshalJSON() ([]byte, error) {
	type request CreateAgentRequest
	var servers json.RawMessage
	if r.MCPServersSet || r.MCPServers != nil {
		data, err := json.Marshal(r.MCPServers)
		if err != nil {
			return nil, err
		}
		servers = data
	}
	return json.Marshal(struct {
		request
		MCPServers json.RawMessage `json:"mcp_servers,omitempty"`
	}{request(r), servers})
}

func (r *CreateAgentRequest) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("request must be a JSON object")
	}
	type request CreateAgentRequest
	var decoded request
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*r = CreateAgentRequest(decoded)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, r.MCPServersSet = fields["mcp_servers"]
	return nil
}

type CreateAgentProfile struct {
	Selector        string                               `json:"selector,omitempty"`
	ImageGeneration *modelprovider.ImageGenerationConfig `json:"image_generation,omitempty"`
	VideoGeneration *modelprovider.VideoGenerationConfig `json:"video_generation,omitempty"`
	ModelProviderID string                               `json:"model_provider_id,omitempty"`
	BaseURL         string                               `json:"base_url,omitempty"`
	APIKey          string                               `json:"api_key,omitempty"`
	Headers         map[string]string                    `json:"headers,omitempty"`
	ModelID         string                               `json:"model_id,omitempty"`
	ReasoningEffort string                               `json:"reasoning_effort,omitempty"`
	EnableFastMode  bool                                 `json:"enable_fast_mode,omitempty"`
	RequestOptions  map[string]any                       `json:"request_options,omitempty"`
	Env             map[string]string                    `json:"env,omitempty"`
}
