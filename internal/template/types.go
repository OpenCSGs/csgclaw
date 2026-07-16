package template

import (
	"time"

	"csgclaw/internal/apitypes"
)

const (
	AgentFileSchemaVersion = "agentfile/v1"

	RegistryKindBuiltin = "builtin"
	RegistryKindLocal   = "local"
	RegistryKindRemote  = "remote"

	WorkspaceKindDir     = "dir"
	WorkspaceKindTarball = "tarball"
)

type Template struct {
	ID            string
	SchemaVersion string
	Name          string
	Description   string
	Role          string
	RuntimeKind   string
	Version       string
	Tags          []string
	Image         string
	ImageEnv      []apitypes.ImageEnvContract
	WorkspaceRef  WorkspaceRef
	Source        RegistryRef
	UpdatedAt     time.Time
}

type RegistryRef struct {
	Name string
	Kind string
}

type WorkspaceRef struct {
	Kind string
	Path string
}

type PublishSpec struct {
	Registry     string
	ID           string
	Name         string
	Description  string
	Role         string
	RuntimeKind  string
	Version      string
	Tags         []string
	Image        string
	WorkspaceRef WorkspaceRef
	UpdatedAt    time.Time
}
