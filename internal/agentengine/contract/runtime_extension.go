package contract

import (
	"context"
	"encoding/json"
	"time"
)

// RuntimeExtensionInterface manages independently reconcilable Runtime
// contributions for one Agent. Extension source payloads remain private to the
// registered Source and Driver and never cross this resource boundary.
type RuntimeExtensionInterface interface {
	Apply(ctx context.Context, request RuntimeExtensionApplyRequest) (RuntimeExtension, error)
	Get(ctx context.Context, name string) (RuntimeExtension, error)
	List(ctx context.Context) ([]RuntimeExtension, error)
	Delete(ctx context.Context, name string) error
}

type RuntimeExtensionApplyRequest struct {
	Spec            RuntimeExtensionSpec `json:"spec"`
	ResourceVersion string               `json:"resource_version,omitempty"`
}

type RuntimeExtensionSpec struct {
	Name          string                        `json:"name"`
	Kind          string                        `json:"kind"`
	Source        RuntimeExtensionSourceRef     `json:"source"`
	FailurePolicy RuntimeExtensionFailurePolicy `json:"failure_policy,omitempty"`
}

type RuntimeExtensionSourceRef struct {
	Provider string `json:"provider"`
	Ref      string `json:"ref"`
}

type RuntimeExtensionFailurePolicy string

const (
	RuntimeExtensionOptional     RuntimeExtensionFailurePolicy = "optional"
	RuntimeExtensionBlockRuntime RuntimeExtensionFailurePolicy = "block_runtime"
)

type RuntimeExtensionState string

const (
	RuntimeExtensionConfigured  RuntimeExtensionState = "configured"
	RuntimeExtensionUnavailable RuntimeExtensionState = "unavailable"
	RuntimeExtensionError       RuntimeExtensionState = "error"
)

type RuntimeExtension struct {
	AgentID         string                 `json:"agent_id"`
	ResourceVersion string                 `json:"resource_version"`
	Spec            RuntimeExtensionSpec   `json:"spec"`
	Status          RuntimeExtensionStatus `json:"status"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

type RuntimeExtensionStatus struct {
	State              RuntimeExtensionState `json:"state"`
	Generation         int64                 `json:"generation"`
	ObservedGeneration int64                 `json:"observed_generation,omitempty"`
	SourceRevision     string                `json:"source_revision,omitempty"`
	Reason             string                `json:"reason,omitempty"`
	Message            string                `json:"message,omitempty"`
	RuntimeLoaded      bool                  `json:"runtime_loaded,omitempty"`
	CheckedAt          time.Time             `json:"checked_at,omitempty"`
	AppliedAt          time.Time             `json:"applied_at,omitempty"`
}

// ResolvedRuntimeExtension is an in-memory handoff between a Source and a
// Runtime Driver. Payload may contain secrets and must never be persisted or
// logged.
type ResolvedRuntimeExtension struct {
	SourceRevision string
	Payload        json.RawMessage
}

type RuntimeExtensionSource interface {
	Resolve(ctx context.Context, agentID, ref string) (ResolvedRuntimeExtension, error)
}
