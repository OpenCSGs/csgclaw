// Package boxlitecli adapts the boxlite CLI to the generic sandbox interfaces.
package boxlitecli

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/sandbox"
)

const providerName = "boxlite"

type Provider struct {
	path       string
	config     string
	registries []string
	runner     Runner
}

func NewProvider(opts ...ProviderOption) Provider {
	p := Provider{
		path:   defaultCLIPath,
		runner: execRunner{},
	}
	for _, opt := range opts {
		opt(&p)
	}
	p.path = ResolvePath(p.path)
	if p.runner == nil {
		p.runner = execRunner{}
	}
	return p
}

func (Provider) Name() string {
	return providerName
}

func (p Provider) Open(_ context.Context, homeDir string) (sandbox.Runtime, error) {
	if strings.TrimSpace(homeDir) == "" {
		return nil, fmt.Errorf("boxlite cli home dir is required")
	}
	return &Runtime{
		path:       p.path,
		homeDir:    homeDir,
		config:     p.config,
		registries: append([]string(nil), p.registries...),
		runner:     p.runner,
	}, nil
}

type Runtime struct {
	path       string
	homeDir    string
	config     string
	registries []string
	runner     Runner
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
		return nil, wrapRunError("run boxlite cli box", result, err)
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
		return fmt.Errorf("boxlite cli box id or name is required")
	}
	args := []string{"rm"}
	if opts.Force {
		args = append(args, "-f")
	}
	args = append(args, idOrName)
	result, err := r.run(ctx, args, nil, nil)
	return wrapRunError("remove boxlite cli box", result, err)
}

func (r *Runtime) Close() error {
	return nil
}

func (r *Runtime) valid() error {
	if r == nil || r.runner == nil {
		return fmt.Errorf("invalid boxlite cli runtime")
	}
	if strings.TrimSpace(r.path) == "" {
		return fmt.Errorf("boxlite cli path is required")
	}
	if strings.TrimSpace(r.homeDir) == "" {
		return fmt.Errorf("boxlite cli home dir is required")
	}
	return nil
}

func (r *Runtime) inspect(ctx context.Context, idOrName string) (sandbox.Info, error) {
	if strings.TrimSpace(idOrName) == "" {
		return sandbox.Info{}, fmt.Errorf("boxlite cli box id or name is required")
	}
	result, err := r.run(ctx, []string{"inspect", "--format", "json", idOrName}, nil, nil)
	if err != nil {
		return sandbox.Info{}, wrapRunError("inspect boxlite cli box", result, err)
	}
	info, err := parseInspect(result.Stdout)
	if err != nil {
		if sandbox.IsNotFound(err) {
			return sandbox.Info{}, fmt.Errorf("inspect boxlite cli box: %w", err)
		}
		return sandbox.Info{}, err
	}
	return info, nil
}

func (r *Runtime) run(ctx context.Context, args []string, stdout, stderr interface{ Write([]byte) (int, error) }) (CommandResult, error) {
	release, err := acquireBoxliteHomeLock(ctx, r.homeDir)
	if err != nil {
		return CommandResult{ExitCode: -1}, err
	}
	defer release()

	req := CommandRequest{
		Path:   r.path,
		Args:   r.baseArgs(args),
		Stdout: stdout,
		Stderr: stderr,
	}
	return r.runner.Run(ctx, req)
}

func (r *Runtime) baseArgs(args []string) []string {
	out := []string{"--home", r.homeDir}
	if strings.TrimSpace(r.config) != "" {
		out = append(out, "--config", r.config)
	}
	for _, registry := range r.registries {
		if strings.TrimSpace(registry) != "" {
			out = append(out, "--registry", registry)
		}
	}
	out = append(out, args...)
	return out
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
	return wrapRunError("start boxlite cli box", result, err)
}

func (i *Instance) Stop(ctx context.Context, opts sandbox.StopOptions) error {
	if err := i.valid(); err != nil {
		return err
	}
	if opts.Force {
		return fmt.Errorf("unsupported sandbox option: force stop")
	}
	if opts.Timeout != 0 {
		return fmt.Errorf("unsupported sandbox option: stop timeout")
	}
	result, err := i.runtime.run(ctx, []string{"stop", i.idOrName}, nil, nil)
	return wrapRunError("stop boxlite cli box", result, err)
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
	args := []string{"exec", i.idOrName, "--", spec.Name}
	args = append(args, spec.Args...)
	result, err := i.runtime.run(ctx, args, spec.Stdout, spec.Stderr)
	out := sandbox.CommandResult{ExitCode: result.ExitCode}
	if err != nil {
		return out, wrapRunError("run boxlite cli command", result, err)
	}
	return out, nil
}

func (i *Instance) Close() error {
	return nil
}

func (i *Instance) valid() error {
	if i == nil || i.runtime == nil {
		return fmt.Errorf("invalid boxlite cli box")
	}
	if err := i.runtime.valid(); err != nil {
		return err
	}
	if strings.TrimSpace(i.idOrName) == "" {
		return fmt.Errorf("boxlite cli box id or name is required")
	}
	return nil
}
