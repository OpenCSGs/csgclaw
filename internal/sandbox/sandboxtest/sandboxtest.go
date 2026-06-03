// Package sandboxtest provides fake sandbox implementations for tests.
package sandboxtest

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"csgclaw/internal/sandbox"
)

// Provider is an in-memory sandbox provider for tests.
type Provider struct {
	NameValue      string
	OpenFunc       func(context.Context, string) (sandbox.Runtime, error)
	ListImagesFunc func(context.Context, string) ([]string, error)

	mu             sync.Mutex
	Runtimes       map[string]*Runtime
	OpenCalls      []string
	ListImageCalls []string
	Images         []string
}

var _ sandbox.Provider = (*Provider)(nil)

// NewProvider returns a fake sandbox provider.
func NewProvider() *Provider {
	return &Provider{
		NameValue: "fake",
		Runtimes:  make(map[string]*Runtime),
	}
}

// Name returns the fake provider name.
func (p *Provider) Name() string {
	if p == nil || p.NameValue == "" {
		return "fake"
	}
	return p.NameValue
}

// Open returns the runtime for homeDir, creating one if necessary.
func (p *Provider) Open(ctx context.Context, homeDir string) (sandbox.Runtime, error) {
	if p == nil {
		return nil, fmt.Errorf("fake sandbox provider is nil")
	}
	if p.OpenFunc != nil {
		return p.OpenFunc(ctx, homeDir)
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.OpenCalls = append(p.OpenCalls, homeDir)
	if p.Runtimes == nil {
		p.Runtimes = make(map[string]*Runtime)
	}
	rt := p.Runtimes[homeDir]
	if rt == nil {
		rt = NewRuntime()
		p.Runtimes[homeDir] = rt
	}
	return rt, nil
}

func (p *Provider) ListImages(ctx context.Context, homeDir string) ([]string, error) {
	if p == nil {
		return nil, fmt.Errorf("fake sandbox provider is nil")
	}
	if p.ListImagesFunc != nil {
		return p.ListImagesFunc(ctx, homeDir)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ListImageCalls = append(p.ListImageCalls, homeDir)
	return append([]string(nil), p.Images...), nil
}

// Runtime is an in-memory sandbox runtime for tests.
type Runtime struct {
	CreateFunc func(context.Context, sandbox.CreateSpec) (sandbox.Instance, error)
	GetFunc    func(context.Context, string) (sandbox.Instance, error)
	RemoveFunc func(context.Context, string, sandbox.RemoveOptions) error
	CloseFunc  func() error

	mu          sync.Mutex
	Instances   map[string]*Instance
	CreateCalls []sandbox.CreateSpec
	GetCalls    []string
	RemoveCalls []RemoveCall
	CloseCalls  int
}

var _ sandbox.Runtime = (*Runtime)(nil)

// RemoveCall records a Runtime.Remove call.
type RemoveCall struct {
	IDOrName string
	Options  sandbox.RemoveOptions
}

// NewRuntime returns a fake sandbox runtime.
func NewRuntime() *Runtime {
	return &Runtime{
		Instances: make(map[string]*Instance),
	}
}

// Create creates and stores a fake sandbox instance.
func (r *Runtime) Create(ctx context.Context, spec sandbox.CreateSpec) (sandbox.Instance, error) {
	if r == nil {
		return nil, fmt.Errorf("fake sandbox runtime is nil")
	}
	if r.CreateFunc != nil {
		return r.CreateFunc(ctx, spec)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.CreateCalls = append(r.CreateCalls, spec)
	inst := NewInstance(sandbox.Info{
		ID:        "box-" + spec.Name,
		Name:      spec.Name,
		State:     sandbox.StateCreated,
		CreatedAt: time.Unix(0, 0).UTC(),
	})
	r.storeLocked(inst)
	return inst, nil
}

// Get returns a fake sandbox instance by ID or name.
func (r *Runtime) Get(ctx context.Context, idOrName string) (sandbox.Instance, error) {
	if r == nil {
		return nil, fmt.Errorf("fake sandbox runtime is nil")
	}
	if r.GetFunc != nil {
		return r.GetFunc(ctx, idOrName)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.GetCalls = append(r.GetCalls, idOrName)
	inst := r.Instances[idOrName]
	if inst == nil {
		return nil, fmt.Errorf("%w: %s", sandbox.ErrNotFound, idOrName)
	}
	return inst, nil
}

// Remove removes a fake sandbox instance by ID or name.
func (r *Runtime) Remove(ctx context.Context, idOrName string, opts sandbox.RemoveOptions) error {
	if r == nil {
		return fmt.Errorf("fake sandbox runtime is nil")
	}
	if r.RemoveFunc != nil {
		return r.RemoveFunc(ctx, idOrName, opts)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.RemoveCalls = append(r.RemoveCalls, RemoveCall{IDOrName: idOrName, Options: opts})
	inst := r.Instances[idOrName]
	if inst == nil {
		return fmt.Errorf("%w: %s", sandbox.ErrNotFound, idOrName)
	}
	delete(r.Instances, inst.InfoValue.ID)
	delete(r.Instances, inst.InfoValue.Name)
	return nil
}

// Close records that the runtime was closed.
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	if r.CloseFunc != nil {
		return r.CloseFunc()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.CloseCalls++
	return nil
}

func (r *Runtime) storeLocked(inst *Instance) {
	if r.Instances == nil {
		r.Instances = make(map[string]*Instance)
	}
	if inst.InfoValue.ID != "" {
		r.Instances[inst.InfoValue.ID] = inst
	}
	if inst.InfoValue.Name != "" {
		r.Instances[inst.InfoValue.Name] = inst
	}
}

// Instance is an in-memory sandbox instance for tests.
type Instance struct {
	StartFunc func(context.Context) error
	StopFunc  func(context.Context, sandbox.StopOptions) error
	InfoFunc  func(context.Context) (sandbox.Info, error)
	RunFunc   func(context.Context, sandbox.CommandSpec) (sandbox.CommandResult, error)
	CloseFunc func() error

	mu         sync.Mutex
	InfoValue  sandbox.Info
	StartCalls int
	StopCalls  []sandbox.StopOptions
	RunCalls   []sandbox.CommandSpec
	CloseCalls int
}

var _ sandbox.Instance = (*Instance)(nil)

// NewInstance returns a fake sandbox instance.
func NewInstance(info sandbox.Info) *Instance {
	return &Instance{InfoValue: info}
}

// Start marks the fake instance as running.
func (i *Instance) Start(ctx context.Context) error {
	if i == nil {
		return fmt.Errorf("fake sandbox instance is nil")
	}
	if i.StartFunc != nil {
		return i.StartFunc(ctx)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.StartCalls++
	i.InfoValue.State = sandbox.StateRunning
	return nil
}

// Stop marks the fake instance as stopped.
func (i *Instance) Stop(ctx context.Context, opts sandbox.StopOptions) error {
	if i == nil {
		return fmt.Errorf("fake sandbox instance is nil")
	}
	if i.StopFunc != nil {
		return i.StopFunc(ctx, opts)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.StopCalls = append(i.StopCalls, opts)
	i.InfoValue.State = sandbox.StateStopped
	return nil
}

// Info returns fake instance metadata.
func (i *Instance) Info(ctx context.Context) (sandbox.Info, error) {
	if i == nil {
		return sandbox.Info{}, fmt.Errorf("fake sandbox instance is nil")
	}
	if i.InfoFunc != nil {
		return i.InfoFunc(ctx)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.InfoValue, nil
}

// Run records a fake command execution.
func (i *Instance) Run(ctx context.Context, spec sandbox.CommandSpec) (sandbox.CommandResult, error) {
	if i == nil {
		return sandbox.CommandResult{}, fmt.Errorf("fake sandbox instance is nil")
	}
	if i.RunFunc != nil {
		return i.RunFunc(ctx, spec)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.RunCalls = append(i.RunCalls, spec)
	if spec.Stdout != nil {
		_, _ = io.WriteString(spec.Stdout, "")
	}
	return sandbox.CommandResult{}, nil
}

// Close records that the instance was closed.
func (i *Instance) Close() error {
	if i == nil {
		return nil
	}
	if i.CloseFunc != nil {
		return i.CloseFunc()
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.CloseCalls++
	return nil
}
