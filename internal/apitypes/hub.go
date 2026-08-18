package apitypes

import "time"

type CreateHubTemplateRequest struct {
	AgentID     string  `json:"agent_id,omitempty"`
	TemplateID  string  `json:"template_id,omitempty"`
	Registry    string  `json:"registry,omitempty"`
	Name        string  `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Deploy      bool    `json:"deploy,omitempty"`
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
	Namespace   string               `json:"namespace,omitempty"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Role        string               `json:"role,omitempty"`
	RuntimeKind string               `json:"runtime_kind,omitempty"`
	Version     string               `json:"version,omitempty"`
	Image       string               `json:"image,omitempty"`
	ImageEnv    []ImageEnvContract   `json:"image_env,omitempty"`
	Metadata    *HubTemplateMetadata `json:"metadata,omitempty"`
	Source      HubTemplateSource    `json:"source"`
	UpdatedAt   time.Time            `json:"updated_at,omitempty"`
	Workspace   HubTemplateWorkspace `json:"workspace,omitempty"`
}

type HubTemplateMetadata struct {
	SensitiveCheck *HubTemplateSensitiveCheck `json:"sensitive_check,omitempty"`
}

type HubTemplateSensitiveCheck struct {
	Status         string                                   `json:"status"`
	FailureDetails []HubTemplateSensitiveCheckFailureDetail `json:"failure_details,omitempty"`
}

type HubTemplateSensitiveCheckFailureDetail struct {
	Path    string `json:"path,omitempty"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}

type HubTemplateSource struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type HubTemplateWorkspace struct {
	Kind    string                      `json:"kind"`
	Entries []HubTemplateWorkspaceEntry `json:"entries,omitempty"`
}

type HubTemplateWorkspaceEntry = WorkspaceEntry

type HubTemplateWorkspaceFile = WorkspaceFile
