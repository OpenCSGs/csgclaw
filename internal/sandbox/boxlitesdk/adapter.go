// Package boxlitesdk adapts the BoxLite SDK to the generic sandbox interfaces.
package boxlitesdk

import (
	"context"
	"fmt"

	boxlitesdk "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/sandbox"
)

const providerName = "boxlite-sdk"

// Provider opens BoxLite-backed sandbox runtimes.
type Provider struct{}

// NewProvider returns a BoxLite sandbox provider.
func NewProvider() Provider {
	return Provider{}
}

// Name returns the provider name.
func (Provider) Name() string {
	return providerName
}

// Open creates a BoxLite runtime rooted at homeDir.
func (Provider) Open(_ context.Context, homeDir string) (sandbox.Runtime, error) {
	rt, err := boxlitesdk.NewRuntime(boxlitesdk.WithHomeDir(homeDir))
	if err != nil {
		return nil, wrapError("open boxlite runtime", err)
	}
	return &Runtime{runtime: rt}, nil
}

// Runtime wraps a BoxLite runtime.
type Runtime struct {
	runtime *boxlitesdk.Runtime
}

var _ sandbox.Provider = Provider{}
var _ sandbox.Runtime = (*Runtime)(nil)

// Create creates and starts a BoxLite box from a generic sandbox create spec.
func (r *Runtime) Create(ctx context.Context, spec sandbox.CreateSpec) (sandbox.Instance, error) {
	if r == nil || r.runtime == nil {
		return nil, fmt.Errorf("invalid boxlite runtime")
	}
	opts, err := boxOptions(spec)
	if err != nil {
		return nil, err
	}
	box, err := r.runtime.Create(ctx, spec.Image, opts...)
	if err != nil {
		return nil, wrapError("create boxlite box", err)
	}
	if err := box.Start(ctx); err != nil {
		_ = box.Close()
		return nil, wrapError("start boxlite box", err)
	}
	return &Instance{box: box}, nil
}

// Get returns a handle for an existing BoxLite box by ID or name.
func (r *Runtime) Get(ctx context.Context, idOrName string) (sandbox.Instance, error) {
	if r == nil || r.runtime == nil {
		return nil, fmt.Errorf("invalid boxlite runtime")
	}
	box, err := r.runtime.Get(ctx, idOrName)
	if err != nil {
		return nil, wrapError("get boxlite box", err)
	}
	return &Instance{box: box}, nil
}

// Remove removes a BoxLite box. Force maps to BoxLite ForceRemove.
func (r *Runtime) Remove(ctx context.Context, idOrName string, opts sandbox.RemoveOptions) error {
	if r == nil || r.runtime == nil {
		return fmt.Errorf("invalid boxlite runtime")
	}
	var err error
	if opts.Force {
		err = r.runtime.ForceRemove(ctx, idOrName)
	} else {
		err = r.runtime.Remove(ctx, idOrName)
	}
	return wrapError("remove boxlite box", err)
}

// Close releases the BoxLite runtime handle.
func (r *Runtime) Close() error {
	if r == nil || r.runtime == nil {
		return nil
	}
	err := r.runtime.Close()
	r.runtime = nil
	return wrapError("close boxlite runtime", err)
}

// Instance wraps a BoxLite box handle.
type Instance struct {
	box *boxlitesdk.Box
}

var _ sandbox.Instance = (*Instance)(nil)

// Start starts the BoxLite box.
func (i *Instance) Start(ctx context.Context) error {
	if i == nil || i.box == nil {
		return fmt.Errorf("invalid boxlite box")
	}
	return wrapError("start boxlite box", i.box.Start(ctx))
}

// Stop stops the BoxLite box. BoxLite currently does not expose force or
// timeout controls on Stop, so unsupported options are rejected explicitly.
func (i *Instance) Stop(ctx context.Context, opts sandbox.StopOptions) error {
	if i == nil || i.box == nil {
		return fmt.Errorf("invalid boxlite box")
	}
	if opts.Force {
		return fmt.Errorf("unsupported sandbox option: force stop")
	}
	if opts.Timeout != 0 {
		return fmt.Errorf("unsupported sandbox option: stop timeout")
	}
	return wrapError("stop boxlite box", i.box.Stop(ctx))
}

// Info returns runtime-neutral BoxLite box metadata.
func (i *Instance) Info(ctx context.Context) (sandbox.Info, error) {
	if i == nil || i.box == nil {
		return sandbox.Info{}, fmt.Errorf("invalid boxlite box")
	}
	info, err := i.box.Info(ctx)
	if err != nil {
		return sandbox.Info{}, wrapError("read boxlite box info", err)
	}
	return boxInfo(*info), nil
}

// Run executes a command inside the BoxLite box.
func (i *Instance) Run(ctx context.Context, spec sandbox.CommandSpec) (sandbox.CommandResult, error) {
	if i == nil || i.box == nil {
		return sandbox.CommandResult{}, fmt.Errorf("invalid boxlite box")
	}
	cmd := i.box.Command(spec.Name, spec.Args...)
	cmd.Stdout = spec.Stdout
	cmd.Stderr = spec.Stderr
	if err := cmd.Run(ctx); err != nil {
		return sandbox.CommandResult{}, wrapError("run boxlite command", err)
	}
	return sandbox.CommandResult{ExitCode: cmd.ExitCode()}, nil
}

// Close releases the BoxLite box handle without stopping or removing it.
func (i *Instance) Close() error {
	if i == nil || i.box == nil {
		return nil
	}
	err := i.box.Close()
	i.box = nil
	return wrapError("close boxlite box", err)
}

func boxInfo(info boxlitesdk.BoxInfo) sandbox.Info {
	return sandbox.Info{
		ID:        info.ID,
		Name:      info.Name,
		State:     boxState(info.State),
		CreatedAt: info.CreatedAt,
	}
}

func boxState(state boxlitesdk.State) sandbox.State {
	switch state {
	case boxlitesdk.StateConfigured:
		return sandbox.StateCreated
	case boxlitesdk.StateRunning:
		return sandbox.StateRunning
	case boxlitesdk.StateStopping:
		return sandbox.StateUnknown
	case boxlitesdk.StateStopped:
		return sandbox.StateStopped
	default:
		return sandbox.StateUnknown
	}
}

func wrapError(op string, err error) error {
	if err == nil {
		return nil
	}
	if boxlitesdk.IsNotFound(err) {
		return fmt.Errorf("%s: %w: %w", op, sandbox.ErrNotFound, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}
