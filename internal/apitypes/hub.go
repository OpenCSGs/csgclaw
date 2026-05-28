package apitypes

import "time"

type CreateHubTemplateRequest struct {
	AgentID  string `json:"agent_id"`
	Registry string `json:"registry,omitempty"`
}

type ImageEnvContract struct {
	Name        string   `json:"name"`
	Required    bool     `json:"required,omitempty"`
	Secret      bool     `json:"secret,omitempty"`
	Default     string   `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
	Choices     []string `json:"choices,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	Example     string   `json:"example,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
}

type HubTemplate struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Role        string               `json:"role,omitempty"`
	RuntimeKind string               `json:"runtime_kind,omitempty"`
	Image       string               `json:"image,omitempty"`
	ImageEnv    []ImageEnvContract   `json:"image_env,omitempty"`
	Source      HubTemplateSource    `json:"source"`
	UpdatedAt   time.Time            `json:"updated_at,omitempty"`
	Workspace   HubTemplateWorkspace `json:"workspace,omitempty"`
}

type HubTemplateSource struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type HubTemplateWorkspace struct {
	Kind    string                      `json:"kind"`
	Entries []HubTemplateWorkspaceEntry `json:"entries,omitempty"`
}

type HubTemplateWorkspaceEntry struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Depth int    `json:"depth,omitempty"`
	Size  int64  `json:"size,omitempty"`
}

type HubTemplateWorkspaceFile struct {
	Path      string `json:"path"`
	Content   string `json:"content,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
}
