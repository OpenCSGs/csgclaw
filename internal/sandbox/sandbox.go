// Package sandbox defines the runtime-agnostic sandbox interfaces used by
// agent execution code.
package sandbox

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound is returned when a sandbox runtime or instance cannot find the
// requested instance.
var ErrNotFound = errors.New("sandbox not found")

// ErrUnavailable is returned when the sandbox provider's control plane cannot
// be reached. It is distinct from ErrNotFound: an unavailable provider does
// not establish that a previously known sandbox has stopped or been removed.
var ErrUnavailable = errors.New("sandbox provider unavailable")

// ErrBusy is returned when a sandbox provider temporarily rejects an operation
// because its runtime home or control plane is already in use. It is neither a
// missing instance nor a lifecycle transition.
var ErrBusy = errors.New("sandbox runtime busy")

// IsNotFound reports whether err represents a missing sandbox instance.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsUnavailable reports whether err represents an unreachable sandbox
// provider or control plane.
func IsUnavailable(err error) bool {
	return errors.Is(err, ErrUnavailable)
}

// IsBusy reports whether err represents temporary sandbox runtime contention.
func IsBusy(err error) bool {
	return errors.Is(err, ErrBusy)
}

// Provider opens sandbox runtimes for a concrete backend.
type Provider interface {
	Name() string
	Open(ctx context.Context, homeDir string) (Runtime, error)
	ListImages(ctx context.Context, homeDir string) ([]string, error)
}

// AvailabilityChecker is an optional provider capability for verifying that
// the provider control plane can accept lifecycle operations. Callers should
// bound checks with a short context deadline because an unavailable daemon must
// not delay application shutdown.
type AvailabilityChecker interface {
	CheckAvailability(ctx context.Context) error
}

// Runtime manages sandbox instances under one sandbox home.
type Runtime interface {
	// Create creates and starts an instance from spec.
	Create(ctx context.Context, spec CreateSpec) (Instance, error)
	Get(ctx context.Context, idOrName string) (Instance, error)
	Remove(ctx context.Context, idOrName string, opts RemoveOptions) error
	Close() error
}

// Instance is a handle to one sandbox instance.
type Instance interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context, opts StopOptions) error
	Info(ctx context.Context) (Info, error)
	Run(ctx context.Context, spec CommandSpec) (CommandResult, error)
	Close() error
}

// Info contains runtime-neutral instance metadata.
type Info struct {
	ID        string
	Name      string
	State     State
	CreatedAt time.Time
}

// State is the runtime-neutral lifecycle state of a sandbox instance.
type State string

const (
	StateUnknown State = "unknown"
	StateCreated State = "created"
	StateRunning State = "running"
	StateStopped State = "stopped"
	StateExited  State = "exited"
)

// CreateSpec describes a sandbox instance to create and start.
type CreateSpec struct {
	Image      string
	Name       string
	Detach     bool
	AutoRemove bool
	// RunUser sets the process user inside the sandbox (docker/boxlite --user).
	// Format: uid or uid:gid.
	RunUser    string
	Env        map[string]string
	Mounts     []Mount
	Entrypoint []string
	Cmd        []string
}

// Mount describes a host path mounted into a sandbox instance.
type Mount struct {
	HostPath  string
	GuestPath string
	ReadOnly  bool
}

// CommandSpec describes a command executed inside a sandbox instance.
type CommandSpec struct {
	Name   string
	Args   []string
	Stdout io.Writer
	Stderr io.Writer
}

// CommandResult contains the result of running a command.
type CommandResult struct {
	ExitCode int
}

// StopOptions controls instance stop behavior.
type StopOptions struct {
	Timeout time.Duration
	Force   bool
}

// RemoveOptions controls instance removal behavior.
type RemoveOptions struct {
	Force bool
}
