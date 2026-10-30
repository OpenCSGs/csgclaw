package template

import (
	"time"

	"csgclaw/internal/apitypes"
)

const (
	RegistryKindBuiltin = "builtin"
	RegistryKindLocal   = "local"
	RegistryKindRemote  = "remote"

	WorkspaceKindDir     = "dir"
	WorkspaceKindTarball = "tarball"
)

type Template struct {
	LatestRevision int
	Publications   []apitypes.TemplatePublication
	UID            string
	Revision       int
	SourceAgentID  string
	ID             string
	Namespace      string
	Avatar         string
	DisplayName    string
	Name           string
	Description    string
	Role           string
	RuntimeKind    string
	Version        string
	Image          string
	ImageEnv       []apitypes.ImageEnvContract
	RuntimeOptions map[string]any
	ModelConfig    *apitypes.TemplateModelConfig
	Metadata       *TemplateMetadata
	WorkspaceRef   WorkspaceRef
	Source         RegistryRef
	UpdatedAt      time.Time
}

type TemplateMetadata struct {
	SensitiveCheck *TemplateSensitiveCheck
}

type TemplateSensitiveCheck struct {
	Status         string
	FailureDetails []TemplateSensitiveCheckFailure
}

type TemplateSensitiveCheckFailure struct {
	Path    string
	Status  string
	Message string
}

type RegistryRef struct {
	Name string
	Kind string
}

type WorkspaceRef struct {
	Kind             string
	Path             string
	InstructionsPath string
	SkillsPath       string
	MemoryPath       string
	MCPServersJSON   string
	Temporary        bool
}

type PublishSpec struct {
	ReplaceRemote  bool
	RemotePath     string
	UID            string
	Revision       int
	SourceAgentID  string
	CommitMessage  string
	Author         *VersionAuthor
	Registry       string
	IncludeMemory  bool
	ID             string
	Avatar         string
	DisplayName    string
	Name           string
	Description    string
	RuntimeKind    string
	Version        string
	Image          string
	RuntimeOptions map[string]any
	ModelConfig    *apitypes.TemplateModelConfig
	WorkspaceRef   WorkspaceRef
	MCPServers     map[string]any
	UpdatedAt      time.Time
}
