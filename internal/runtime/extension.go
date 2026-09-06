package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var ErrExtensionUnsupported = errors.New("runtime extension is unsupported")

// ExtensionDesired is the transient, resolved Runtime-facing view of one
// Agent Engine RuntimeExtension. Payload may contain secrets and must not be
// persisted or logged by Runtime implementations.
type ExtensionDesired struct {
	Name               string
	Kind               string
	Generation         int64
	SourceRevision     string
	Payload            json.RawMessage
	DeferRuntimeReload bool
}

type ExtensionResult struct {
	State           string
	Reason          string
	Message         string
	RuntimeLoaded   bool
	RestartRequired bool
	CheckedAt       time.Time
}

// ExtensionProjection is Runtime-private derived state. It is never part of an
// Engine resource response and contains no resolved Source payload.
type ExtensionProjection struct {
	Name           string            `json:"name"`
	Kind           string            `json:"kind"`
	Generation     int64             `json:"generation"`
	SourceRevision string            `json:"source_revision"`
	Digest         string            `json:"digest"`
	Root           string            `json:"root"`
	Environment    map[string]string `json:"environment,omitempty"`
	Instructions   string            `json:"instructions,omitempty"`
}

// PreparedExtension owns only managed staging. Activate is reversible until
// Cleanup; Cleanup retains an activated generation and removes obsolete staging.
type PreparedExtension interface {
	Projection() ExtensionProjection
	Activate(context.Context) error
	Rollback(context.Context) error
	Cleanup(context.Context) error
}

// ExtensionPreparer validates/probes and stages without changing active state.
type ExtensionPreparer interface {
	PrepareExtension(context.Context, string, ExtensionDesired) (PreparedExtension, ExtensionResult, error)
}

// ExtensionHost projects the complete, Engine-ordered contribution set. Deletion
// does not require an installed Driver or executable, so failed cleanup is retryable.
type ExtensionHost interface {
	ExtensionProjections(string) ([]ExtensionProjection, error)
	RenderExtensions(context.Context, string, []ExtensionProjection) error
	PrepareExtensionDelete(context.Context, string, string) (PreparedExtension, error)
}

const (
	ExtensionStateConfigured  = "configured"
	ExtensionStateUnavailable = "unavailable"
	ExtensionStateError       = "error"
)

// ExtensionDriver owns Runtime-specific validation, layout, staging,
// activation, observation, and cleanup for one extension kind.
type ExtensionDriver interface {
	ExtensionPreparer
	ObserveExtension(ctx context.Context, agentID string, desired ExtensionDesired) (ExtensionResult, error)
}

// ExtensionDriverProvider is implemented by Runtime Adapters that support
// independently reconcilable Runtime extensions.
type ExtensionDriverProvider interface {
	RuntimeExtensionDriver(kind string) (ExtensionDriver, bool)
}
