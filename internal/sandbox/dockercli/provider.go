// Package dockercli adapts the Docker CLI to the generic sandbox interfaces.
package dockercli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"csgclaw/internal/sandbox"
)

const providerName = "docker"

type Provider struct {
	path   string
	runner Runner
}

func NewProvider(opts ...ProviderOption) Provider {
	p := Provider{
		path:   defaultCLIPath,
		runner: execRunner{},
	}
	for _, opt := range opts {
		opt(&p)
	}
	p.path = resolvePath(p.path)
	if p.runner == nil {
		p.runner = execRunner{}
	}
	return p
}

func (Provider) Name() string {
	return providerName
}

func (p Provider) Open(_ context.Context, _ string) (sandbox.Runtime, error) {
	return &Runtime{
		path:   p.path,
		runner: p.runner,
	}, nil
}

type Runtime struct {
	path   string
	runner Runner
}

var _ sandbox.Provider = Provider{}
var _ sandbox.Runtime = (*Runtime)(nil)

func (r *Runtime) Create(ctx context.Context, spec sandbox.CreateSpec) (sandbox.Instance, error) {
	if err := r.valid(); err != nil {
		return nil, err
	}
	args, err := runArgs(spec)
	if err != nil {
		return nil, err
	}
	result, err := r.run(ctx, args, nil, nil)
	if err != nil {
		return nil, wrapRunError("container run", result, err)
	}
	id := strings.TrimSpace(string(result.Stdout))
	if id == "" {
		id = spec.Name
	}
	return &Instance{runtime: r, idOrName: id}, nil
}

func (r *Runtime) Get(ctx context.Context, idOrName string) (sandbox.Instance, error) {
	if err := r.valid(); err != nil {
		return nil, err
	}
	info, err := r.inspect(ctx, idOrName)
	if err != nil {
		return nil, err
	}
	id := info.ID
	if id == "" {
		id = idOrName
	}
	return &Instance{runtime: r, idOrName: id}, nil
}

func (r *Runtime) Remove(ctx context.Context, idOrName string, opts sandbox.RemoveOptions) error {
	if err := r.valid(); err != nil {
		return err
	}
	if strings.TrimSpace(idOrName) == "" {
		return fmt.Errorf("container id or name is required")
	}
	args := []string{"rm"}
	if opts.Force {
		args = append(args, "-f")
	}
	args = append(args, idOrName)
	result, err := r.run(ctx, args, nil, nil)
	return wrapRunError("container rm", result, err)
}

func (r *Runtime) Close() error {
	return nil
}

func (r *Runtime) valid() error {
	if r == nil || r.runner == nil {
		return fmt.Errorf("invalid container runtime")
	}
	if strings.TrimSpace(r.path) == "" {
		return fmt.Errorf("container runtime path is required")
	}
	return nil
}

func (r *Runtime) inspect(ctx context.Context, idOrName string) (sandbox.Info, error) {
	if strings.TrimSpace(idOrName) == "" {
		return sandbox.Info{}, fmt.Errorf("container id or name is required")
	}
	result, err := r.run(ctx, []string{"inspect", idOrName}, nil, nil)
	if err != nil {
		return sandbox.Info{}, wrapRunError("container inspect", result, err)
	}
	info, err := parseInspect(result.Stdout)
	if err != nil {
		if sandbox.IsNotFound(err) {
			return sandbox.Info{}, fmt.Errorf("container inspect: %w", err)
		}
		return sandbox.Info{}, err
	}
	return info, nil
}

func (r *Runtime) run(ctx context.Context, args []string, stdout, stderr interface{ Write([]byte) (int, error) }) (CommandResult, error) {
	req := CommandRequest{
		Path:   r.path,
		Args:   args,
		Stdout: stdout,
		Stderr: stderr,
	}
	return r.runner.Run(ctx, req)
}

type Instance struct {
	runtime  *Runtime
	idOrName string
}

var _ sandbox.Instance = (*Instance)(nil)

func (i *Instance) Start(ctx context.Context) error {
	if err := i.valid(); err != nil {
		return err
	}
	result, err := i.runtime.run(ctx, []string{"start", i.idOrName}, nil, nil)
	return wrapRunError("container start", result, err)
}

func (i *Instance) Stop(ctx context.Context, opts sandbox.StopOptions) error {
	if err := i.valid(); err != nil {
		return err
	}
	if opts.Force {
		result, err := i.runtime.run(ctx, []string{"kill", i.idOrName}, nil, nil)
		return wrapRunError("container kill", result, err)
	}
	args := []string{"stop"}
	if opts.Timeout > 0 {
		sec := int(opts.Timeout.Round(time.Second) / time.Second)
		if sec < 1 {
			sec = 1
		}
		args = append(args, "-t", strconv.Itoa(sec))
	}
	args = append(args, i.idOrName)
	result, err := i.runtime.run(ctx, args, nil, nil)
	return wrapRunError("container stop", result, err)
}

func (i *Instance) Info(ctx context.Context) (sandbox.Info, error) {
	if err := i.valid(); err != nil {
		return sandbox.Info{}, err
	}
	return i.runtime.inspect(ctx, i.idOrName)
}

func (i *Instance) Run(ctx context.Context, spec sandbox.CommandSpec) (sandbox.CommandResult, error) {
	if err := i.valid(); err != nil {
		return sandbox.CommandResult{}, err
	}
	if strings.TrimSpace(spec.Name) == "" {
		return sandbox.CommandResult{}, fmt.Errorf("invalid sandbox command: name is required")
	}
	args := []string{"exec", i.idOrName, spec.Name}
	args = append(args, spec.Args...)
	result, err := i.runtime.run(ctx, args, spec.Stdout, spec.Stderr)
	out := sandbox.CommandResult{ExitCode: result.ExitCode}
	if err != nil {
		return out, wrapRunError("container exec", result, err)
	}
	return out, nil
}

func (i *Instance) Close() error {
	return nil
}

func (i *Instance) valid() error {
	if i == nil || i.runtime == nil {
		return fmt.Errorf("invalid container instance")
	}
	if err := i.runtime.valid(); err != nil {
		return err
	}
	if strings.TrimSpace(i.idOrName) == "" {
		return fmt.Errorf("container id or name is required")
	}
	return nil
}
