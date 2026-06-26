package agent

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"csgclaw/internal/utils"
)

const (
	RoleAgent   = "agent"
	RoleWorker  = "worker"
	RoleManager = "manager"
)

type Agent struct {
	ID               string                   `json:"id"`
	Name             string                   `json:"name"`
	Description      string                   `json:"description,omitempty"`
	Instructions     string                   `json:"instructions,omitempty"`
	RuntimeID        string                   `json:"runtime_id,omitempty"`
	RuntimeKind      string                   `json:"runtime_kind,omitempty"`
	Image            string                   `json:"image,omitempty"`
	Avatar           string                   `json:"avatar,omitempty"`
	BoxID            string                   `json:"box_id,omitempty"`
	RuntimeOptions   map[string]any           `json:"runtime_options,omitempty"`
	Role             string                   `json:"role"`
	Status           string                   `json:"status"`
	CreatedAt        time.Time                `json:"created_at"`
	UpdatedAt        time.Time                `json:"updated_at,omitempty"`
	Profile          string                   `json:"profile,omitempty"`
	AgentProfile     AgentProfile             `json:"agent_profile,omitempty"`
	ProfileComplete  bool                     `json:"profile_complete"`
	DetectionResults []ProfileDetectionResult `json:"detection_results,omitempty"`
}

func (a *Agent) UnmarshalJSON(data []byte) error {
	type agentJSON struct {
		ID               string                   `json:"id"`
		Name             string                   `json:"name"`
		Description      string                   `json:"description,omitempty"`
		Instructions     string                   `json:"instructions,omitempty"`
		RuntimeID        string                   `json:"runtime_id,omitempty"`
		RuntimeKind      string                   `json:"runtime_kind,omitempty"`
		Runtime          *RuntimeRecord           `json:"runtime,omitempty"`
		Image            string                   `json:"image,omitempty"`
		Avatar           string                   `json:"avatar,omitempty"`
		BoxID            string                   `json:"box_id,omitempty"`
		RuntimeOptions   map[string]any           `json:"runtime_options,omitempty"`
		Role             string                   `json:"role"`
		Status           string                   `json:"status"`
		CreatedAt        time.Time                `json:"created_at"`
		UpdatedAt        time.Time                `json:"updated_at,omitempty"`
		ModelConfig      json.RawMessage          `json:"model_config"`
		Profile          json.RawMessage          `json:"profile"`
		AgentProfile     AgentProfile             `json:"agent_profile,omitempty"`
		ProfileComplete  bool                     `json:"profile_complete"`
		DetectionResults []ProfileDetectionResult `json:"detection_results,omitempty"`
	}
	var decoded agentJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	out := Agent{
		ID:               decoded.ID,
		Name:             decoded.Name,
		Description:      decoded.Description,
		Instructions:     decoded.Instructions,
		RuntimeID:        decoded.RuntimeID,
		RuntimeKind:      decoded.RuntimeKind,
		Image:            decoded.Image,
		Avatar:           decoded.Avatar,
		BoxID:            decoded.BoxID,
		RuntimeOptions:   utils.CloneAnyMap(decoded.RuntimeOptions),
		Role:             decoded.Role,
		Status:           decoded.Status,
		CreatedAt:        decoded.CreatedAt,
		UpdatedAt:        decoded.UpdatedAt,
		AgentProfile:     cloneProfile(decoded.AgentProfile),
		ProfileComplete:  decoded.ProfileComplete,
		DetectionResults: append([]ProfileDetectionResult(nil), decoded.DetectionResults...),
	}
	if decoded.Runtime != nil {
		rt := normalizeRuntimeRecord(*decoded.Runtime)
		if strings.TrimSpace(out.RuntimeID) == "" && strings.TrimSpace(rt.ID) != "" {
			out.RuntimeID = rt.ID
		}
		if strings.TrimSpace(out.RuntimeKind) == "" {
			out.RuntimeKind = rt.Kind
		}
		if strings.TrimSpace(out.BoxID) == "" {
			out.BoxID = rt.SandboxID
		}
		if strings.TrimSpace(out.Status) == "" && rt.State != "" {
			out.Status = string(rt.State)
		}
		if len(out.RuntimeOptions) == 0 && len(rt.Options) > 0 {
			out.RuntimeOptions = utils.CloneAnyMap(rt.Options)
		}
	}
	profilePayload := decoded.ModelConfig
	if len(profilePayload) == 0 || string(profilePayload) == "null" {
		profilePayload = decoded.Profile
	}
	if len(profilePayload) > 0 && string(profilePayload) != "null" {
		var profile AgentProfile
		if err := json.Unmarshal(profilePayload, &profile); err == nil {
			out.AgentProfile = profile
			out.Profile = profileSelector(profile)
		} else {
			var selector string
			if err := json.Unmarshal(profilePayload, &selector); err != nil {
				return err
			}
			out.Profile = strings.TrimSpace(selector)
		}
	}
	*a = out
	return nil
}

type CreateAgentSpec struct {
	ID             string         `json:"id,omitempty"`
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	Instructions   string         `json:"instructions,omitempty"`
	Image          string         `json:"image,omitempty"`
	Avatar         string         `json:"-"`
	RuntimeKind    string         `json:"runtime_kind,omitempty"`
	FromTemplate   string         `json:"from_template,omitempty"`
	Role           string         `json:"role,omitempty"`
	Status         string         `json:"status,omitempty"`
	CreatedAt      time.Time      `json:"created_at,omitempty"`
	UpdatedAt      time.Time      `json:"updated_at,omitempty"`
	Profile        string         `json:"profile,omitempty"`
	RuntimeOptions map[string]any `json:"runtime_options,omitempty"`
	AgentProfile   AgentProfile   `json:"agent_profile,omitempty"`
}

func (s *CreateAgentSpec) UnmarshalJSON(data []byte) error {
	type createAgentSpecJSON struct {
		ID             string          `json:"id,omitempty"`
		Name           string          `json:"name"`
		Description    string          `json:"description,omitempty"`
		Instructions   string          `json:"instructions,omitempty"`
		Image          string          `json:"image,omitempty"`
		Avatar         string          `json:"-"`
		RuntimeKind    string          `json:"runtime_kind,omitempty"`
		Runtime        *RuntimeRecord  `json:"runtime,omitempty"`
		FromTemplate   string          `json:"from_template,omitempty"`
		Role           string          `json:"role,omitempty"`
		Status         string          `json:"status,omitempty"`
		CreatedAt      time.Time       `json:"created_at,omitempty"`
		UpdatedAt      time.Time       `json:"updated_at,omitempty"`
		ModelConfig    json.RawMessage `json:"model_config,omitempty"`
		Profile        json.RawMessage `json:"profile,omitempty"`
		RuntimeOptions map[string]any  `json:"runtime_options,omitempty"`
		AgentProfile   AgentProfile    `json:"agent_profile,omitempty"`
	}
	var decoded createAgentSpecJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	out := CreateAgentSpec{
		ID:             decoded.ID,
		Name:           decoded.Name,
		Description:    decoded.Description,
		Instructions:   decoded.Instructions,
		Image:          decoded.Image,
		RuntimeKind:    decoded.RuntimeKind,
		FromTemplate:   decoded.FromTemplate,
		Role:           decoded.Role,
		Status:         decoded.Status,
		CreatedAt:      decoded.CreatedAt,
		UpdatedAt:      decoded.UpdatedAt,
		RuntimeOptions: utils.CloneAnyMap(decoded.RuntimeOptions),
		AgentProfile:   cloneProfile(decoded.AgentProfile),
	}
	if decoded.Runtime != nil {
		rt := normalizeRuntimeRecord(*decoded.Runtime)
		if strings.TrimSpace(out.RuntimeKind) == "" {
			out.RuntimeKind = rt.Kind
		}
		if strings.TrimSpace(out.Status) == "" && rt.State != "" {
			out.Status = string(rt.State)
		}
		if len(out.RuntimeOptions) == 0 && len(rt.Options) > 0 {
			out.RuntimeOptions = utils.CloneAnyMap(rt.Options)
		}
	}
	profilePayload := decoded.ModelConfig
	if len(profilePayload) == 0 || string(profilePayload) == "null" {
		profilePayload = decoded.Profile
	}
	if len(profilePayload) > 0 && string(profilePayload) != "null" {
		var profile AgentProfile
		if err := json.Unmarshal(profilePayload, &profile); err == nil {
			out.AgentProfile = profile
			out.Profile = profileSelector(profile)
		} else {
			var selector string
			if err := json.Unmarshal(profilePayload, &selector); err != nil {
				return err
			}
			out.Profile = strings.TrimSpace(selector)
		}
	}
	*s = out
	return nil
}

type UpdateRequest struct {
	Name           *string         `json:"name,omitempty"`
	Description    *string         `json:"description,omitempty"`
	Instructions   *string         `json:"instructions,omitempty"`
	Image          *string         `json:"image,omitempty"`
	Avatar         *string         `json:"-"`
	Profile        *string         `json:"profile,omitempty"`
	RuntimeOptions *map[string]any `json:"runtime_options,omitempty"`
	AgentProfile   *AgentProfile   `json:"agent_profile,omitempty"`
	FieldMask      []string        `json:"field_mask,omitempty"`
}

func (r *UpdateRequest) UnmarshalJSON(data []byte) error {
	type updateRequestJSON struct {
		Name           *string         `json:"name,omitempty"`
		Description    *string         `json:"description,omitempty"`
		Instructions   *string         `json:"instructions,omitempty"`
		Image          *string         `json:"image,omitempty"`
		Avatar         *string         `json:"-"`
		ModelConfig    json.RawMessage `json:"model_config,omitempty"`
		Profile        json.RawMessage `json:"profile,omitempty"`
		Runtime        *RuntimeRecord  `json:"runtime,omitempty"`
		RuntimeOptions *map[string]any `json:"runtime_options,omitempty"`
		AgentProfile   *AgentProfile   `json:"agent_profile,omitempty"`
		FieldMask      []string        `json:"field_mask,omitempty"`
	}
	var decoded updateRequestJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	out := UpdateRequest{
		Name:           decoded.Name,
		Description:    decoded.Description,
		Instructions:   decoded.Instructions,
		Image:          decoded.Image,
		RuntimeOptions: decoded.RuntimeOptions,
		AgentProfile:   decoded.AgentProfile,
		FieldMask:      append([]string(nil), decoded.FieldMask...),
	}
	profileField := ""
	profilePayload := decoded.ModelConfig
	if len(profilePayload) == 0 || string(profilePayload) == "null" {
		profilePayload = decoded.Profile
	}
	if len(profilePayload) > 0 && string(profilePayload) != "null" {
		var profile AgentProfile
		if err := json.Unmarshal(profilePayload, &profile); err == nil {
			out.AgentProfile = &profile
			profileField = "agent_profile"
		} else {
			var selector string
			if err := json.Unmarshal(profilePayload, &selector); err != nil {
				return err
			}
			selector = strings.TrimSpace(selector)
			out.Profile = &selector
			profileField = "profile"
		}
	}
	if decoded.Runtime != nil && len(decoded.Runtime.Options) > 0 {
		options := utils.CloneAnyMap(decoded.Runtime.Options)
		out.RuntimeOptions = &options
	}
	if len(out.FieldMask) > 0 {
		out.FieldMask = normalizeCompactUpdateFieldMask(out.FieldMask, profileField, decoded.Runtime != nil)
	}
	*r = out
	return nil
}

func normalizeCompactUpdateFieldMask(fieldMask []string, profileField string, hasRuntime bool) []string {
	if len(fieldMask) == 0 {
		return nil
	}
	out := make([]string, 0, len(fieldMask))
	seen := map[string]struct{}{}
	add := func(field string) {
		field = strings.ToLower(strings.TrimSpace(field))
		if field == "" {
			return
		}
		if _, ok := seen[field]; ok {
			return
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	for _, field := range fieldMask {
		normalized := strings.ToLower(strings.TrimSpace(field))
		switch normalized {
		case "profile", "model_config":
			if profileField != "" {
				add(profileField)
			} else {
				add(normalized)
			}
		case "runtime":
			if hasRuntime {
				add("runtime_options")
			} else {
				add(normalized)
			}
		default:
			add(normalized)
		}
	}
	return out
}

type CreateRequest struct {
	Spec      CreateAgentSpec
	Replace   bool
	FieldMask []string
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", RoleAgent:
		return RoleAgent
	case RoleWorker:
		return RoleWorker
	case RoleManager:
		return RoleManager
	default:
		return strings.ToLower(strings.TrimSpace(role))
	}
}

func isManagerAgent(a Agent) bool {
	return strings.EqualFold(strings.TrimSpace(a.Role), RoleManager) ||
		strings.EqualFold(strings.TrimSpace(a.Name), ManagerName) ||
		strings.EqualFold(strings.TrimSpace(a.ID), ManagerUserID)
}

func sortedAgentsFromMap(items map[string]Agent) []Agent {
	agents := make([]Agent, 0, len(items))
	for _, a := range items {
		agents = append(agents, *cloneAgent(&a))
	}
	slices.SortFunc(agents, func(a, b Agent) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			switch {
			case a.ID < b.ID:
				return -1
			case a.ID > b.ID:
				return 1
			default:
				return 0
			}
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	})
	return agents
}

func persistedAgentsFromMap(items map[string]Agent) []persistedAgent {
	agents := sortedAgentsFromMap(items)
	persisted := make([]persistedAgent, 0, len(agents))
	for _, a := range agents {
		persisted = append(persisted, newPersistedAgent(a))
	}
	return persisted
}

func cloneAgent(src *Agent) *Agent {
	if src == nil {
		return nil
	}
	dst := *src
	dst.AgentProfile = cloneProfile(src.AgentProfile)
	dst.DetectionResults = append([]ProfileDetectionResult(nil), src.DetectionResults...)
	dst.RuntimeOptions = utils.CloneAnyMap(src.RuntimeOptions)
	return &dst
}
