package hub

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
	ID           string
	Name         string
	Description  string
	Role         string
	RuntimeKind  string
	Image        string
	ImageEnv     []apitypes.ImageEnvContract
	WorkspaceRef WorkspaceRef
	Source       RegistryRef
	UpdatedAt    time.Time
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
	Image        string
	WorkspaceRef WorkspaceRef
	UpdatedAt    time.Time
}
