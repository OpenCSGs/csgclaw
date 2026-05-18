package runtime

import (
	"context"

	"csgclaw/internal/config"
)

// Provisioner is an optional runtime capability for host-side preparation that
// must happen before Runtime.New.
//
// Provision should own idempotent preparation steps such as workspace layout
// choice, template seeding, config generation, runtime-owned env/config
// materialization, and host directory setup. It should not return the final
// runtime handle or replace Runtime.New.
type Provisioner interface {
	Provision(ctx context.Context, req ProvisionRequest) error
}

// ProvisionRequest contains the narrow, preparation-oriented inputs that a
// runtime may need before Runtime.New is called.
//
// This request intentionally stays smaller than Spec: it carries agent identity
// and runtime-facing profile data for host-side setup, but not the final
// execution handle creation.
type ProvisionRequest struct {
	RuntimeID        string
	AgentID          string
	AgentName        string
	Profile          Profile
	WorkspaceOverlay string
	Gateway          *GatewayProvision
}

// GatewayProvision carries the host-side data needed by sandbox gateway
// runtimes during Provision. These inputs are consumed once to materialize
// config, workspace, and related host assets before Runtime.New.
type GatewayProvision struct {
	ModelFallback     string
	Server            config.ServerConfig
	ManagerBaseURL    string
	AgentHome         string
	ProjectsRoot      string
	WorkspaceTemplate string
}
