package csghub

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/csghub/csghubsdk"
)

const providerName = "csghub"

const (
	defaultPVCMountPath = "/opt/csgclaw"
	defaultReadyTimeout = 5 * time.Minute
	defaultPollInterval = 3 * time.Second
	defaultHealthReqTO  = 5 * time.Second
	minReadyTimeout     = 5 * time.Second
	minPollInterval     = 500 * time.Millisecond
	maxPollInterval     = 30 * time.Second
)

type runtimeConfig struct {
	clientCfg    csghubsdk.Config
	clusterID    string
	resourceID   int
	port         int
	timeout      int
	pvcMountPath string
	readyTimeout time.Duration
	pollInterval time.Duration
}

// Provider is the sandbox.Provider implementation for [sandbox].provider = csghub.
type Provider struct{}

func NewProvider() Provider {
	return Provider{}
}

func (Provider) Name() string {
	return providerName
}

func (Provider) Open(_ context.Context, _ string) (sandbox.Runtime, error) {
	cfg, err := loadRuntimeConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return &Runtime{
		cfg:    cfg,
		client: csghubsdk.New(cfg.clientCfg),
	}, nil
}

var _ sandbox.Provider = Provider{}

// Runtime adapts the csghub lifecycle API to sandbox.Runtime.
type Runtime struct {
	cfg    runtimeConfig
	client *csghubsdk.Client
}

var _ sandbox.Runtime = (*Runtime)(nil)

func (r *Runtime) Create(ctx context.Context, spec sandbox.CreateSpec) (sandbox.Instance, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("invalid csghub runtime")
	}
	req, err := r.createRequest(spec)
	if err != nil {
		return nil, err
	}

	resp, err := r.client.Create(ctx, req)
	if err != nil {
		if !csghubsdk.IsConflict(err) {
			return nil, wrapError("create csghub sandbox", err)
		}
		// Existing sandbox: align desired spec then read latest state.
		if _, applyErr := r.client.Apply(ctx, req); applyErr != nil {
			return nil, wrapError("apply csghub sandbox", applyErr)
		}
		resp, err = r.client.Get(ctx, req.SandboxName)
		if err != nil {
			return nil, wrapError("get csghub sandbox after apply", err)
		}
	}

	started, startErr := r.startSandboxIdempotent(ctx, req.SandboxName)
	if startErr != nil {
		return nil, startErr
	}
	if started != nil {
		resp = started
	}

	if !isSandboxRunning(resp.State.Status) {
		resp, err = r.waitForRunning(ctx, req.SandboxName)
		if err != nil {
			return nil, err
		}
	}
	if err := r.waitForRuntimeHealth(ctx, req.SandboxName); err != nil {
		return nil, err
	}
	return &Instance{
		runtime: r,
		name:    req.SandboxName,
	}, nil
}

func (r *Runtime) Get(ctx context.Context, idOrName string) (sandbox.Instance, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("invalid csghub runtime")
	}
	name := strings.TrimSpace(idOrName)
	if name == "" {
		return nil, fmt.Errorf("csghub sandbox id or name is required")
	}
	if _, err := r.client.Get(ctx, name); err != nil {
		return nil, wrapError("get csghub sandbox", err)
	}
	return &Instance{
		runtime: r,
		name:    name,
	}, nil
}

func (r *Runtime) Remove(ctx context.Context, idOrName string, _ sandbox.RemoveOptions) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("invalid csghub runtime")
	}
	name := strings.TrimSpace(idOrName)
	if name == "" {
		return fmt.Errorf("csghub sandbox id or name is required")
	}
	return wrapError("remove csghub sandbox", r.client.Delete(ctx, name))
}

func (r *Runtime) Close() error {
	return nil
}

func (r *Runtime) createRequest(spec sandbox.CreateSpec) (csghubsdk.CreateRequest, error) {
	image := strings.TrimSpace(spec.Image)
	if image == "" {
		return csghubsdk.CreateRequest{}, fmt.Errorf("invalid sandbox image: image is required")
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return csghubsdk.CreateRequest{}, fmt.Errorf("invalid sandbox name: name is required")
	}

	volumes, err := r.volumeSpecs(spec.Mounts)
	if err != nil {
		return csghubsdk.CreateRequest{}, err
	}

	req := csghubsdk.CreateRequest{
		Image:        image,
		SandboxName:  name,
		Environments: cloneStringMap(spec.Env),
		Volumes:      volumes,
		ClusterID:    strings.TrimSpace(r.cfg.clusterID),
		ResourceID:   r.cfg.resourceID,
		Port:         r.cfg.port,
		Timeout:      r.cfg.timeout,
	}
	return req, nil
}

func (r *Runtime) volumeSpecs(mounts []sandbox.Mount) ([]csghubsdk.VolumeSpec, error) {
	if len(mounts) == 0 {
		return nil, nil
	}
	root := strings.TrimSpace(r.cfg.pvcMountPath)
	rootClean := filepath.Clean(root)
	volumes := make([]csghubsdk.VolumeSpec, 0, len(mounts))
	for _, mount := range mounts {
		host := strings.TrimSpace(mount.HostPath)
		guest := strings.TrimSpace(mount.GuestPath)
		if host == "" {
			return nil, fmt.Errorf("invalid sandbox mount: host path is required")
		}
		if guest == "" {
			return nil, fmt.Errorf("invalid sandbox mount: guest path is required")
		}
		hostClean := filepath.Clean(host)
		rel, err := filepath.Rel(rootClean, hostClean)
		if err != nil {
			return nil, fmt.Errorf("resolve mount relative path %q: %w", host, err)
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || strings.HasPrefix(rel, "../") || rel == ".." {
			return nil, fmt.Errorf("invalid sandbox mount: host path %q must be under %q", host, root)
		}
		volumes = append(volumes, csghubsdk.VolumeSpec{
			SandboxMountSubpath: rel,
			SandboxMountPath:    guest,
			ReadOnly:            mount.ReadOnly,
		})
	}
	return volumes, nil
}

func (r *Runtime) waitForRunning(ctx context.Context, sandboxName string) (*csghubsdk.Response, error) {
	waitCtx, cancel := context.WithTimeout(ctx, r.cfg.readyTimeout)
	defer cancel()

	ticker := time.NewTicker(r.cfg.pollInterval)
	defer ticker.Stop()

	var attempt int
	var lastErr error
	var lastStatus string
	for {
		attempt++
		resp, err := r.client.Get(waitCtx, sandboxName)
		if err == nil {
			lastStatus = strings.TrimSpace(resp.State.Status)
			if isSandboxRunning(lastStatus) {
				return resp, nil
			}
		} else if shouldRaiseImmediately(err) {
			return nil, wrapError("poll csghub sandbox status", err)
		} else {
			lastErr = err
		}
		select {
		case <-waitCtx.Done():
			if lastErr != nil {
				return nil, wrapError("wait csghub sandbox running", lastErr)
			}
			if strings.TrimSpace(lastStatus) == "" {
				return nil, fmt.Errorf("wait csghub sandbox running exceeded deadline (%s): %w", r.cfg.readyTimeout, waitCtx.Err())
			}
			return nil, fmt.Errorf(
				"wait csghub sandbox running exceeded deadline (%s), last status=%q: %w",
				r.cfg.readyTimeout,
				lastStatus,
				waitCtx.Err(),
			)
		case <-ticker.C:
		}
	}
}

func (r *Runtime) waitForRuntimeHealth(ctx context.Context, sandboxName string) error {
	waitCtx, cancel := context.WithTimeout(ctx, r.cfg.readyTimeout)
	defer cancel()

	ticker := time.NewTicker(r.cfg.pollInterval)
	defer ticker.Stop()

	var lastErr error
	for {
		perReqTimeout := defaultHealthReqTO
		if ddl, ok := waitCtx.Deadline(); ok {
			remaining := time.Until(ddl)
			if remaining <= 0 {
				if lastErr != nil {
					return wrapError("wait csghub runtime health", lastErr)
				}
				return fmt.Errorf("wait csghub runtime health exceeded deadline (%s): %w", r.cfg.readyTimeout, waitCtx.Err())
			}
			if perReqTimeout > remaining {
				perReqTimeout = remaining
			}
		}

		reqCtx, cancelReq := context.WithTimeout(waitCtx, perReqTimeout)
		err := r.client.RuntimeHealth(reqCtx, sandboxName)
		cancelReq()
		if err == nil {
			return nil
		}
		if shouldRaiseImmediately(err) {
			return wrapError("wait csghub runtime health", err)
		}
		lastErr = err

		select {
		case <-waitCtx.Done():
			return wrapError("wait csghub runtime health", lastErr)
		case <-ticker.C:
		}
	}
}

func (r *Runtime) startSandboxIdempotent(ctx context.Context, sandboxName string) (*csghubsdk.Response, error) {
	started, err := r.client.Start(ctx, sandboxName)
	if err == nil {
		return started, nil
	}
	if isStartUnsupported(err) {
		return started, nil
	}
	if !isDuplicateStartError(err) {
		return nil, wrapError("start csghub sandbox", err)
	}
	resp, getErr := r.client.Get(ctx, sandboxName)
	if getErr != nil {
		return nil, wrapError("get csghub sandbox after duplicate start", getErr)
	}
	if statusOKAfterDuplicateStart(resp.State.Status) {
		return resp, nil
	}
	return nil, wrapError("start csghub sandbox", err)
}

// Instance adapts one csghub sandbox to sandbox.Instance.
type Instance struct {
	runtime *Runtime
	name    string
}

var _ sandbox.Instance = (*Instance)(nil)

func (i *Instance) Start(ctx context.Context) error {
	if err := i.valid(); err != nil {
		return err
	}
	resp, err := i.runtime.startSandboxIdempotent(ctx, i.name)
	if err != nil {
		return err
	}
	if resp != nil && !isSandboxRunning(resp.State.Status) {
		_, err = i.runtime.waitForRunning(ctx, i.name)
		if err != nil {
			return err
		}
	}
	return i.runtime.waitForRuntimeHealth(ctx, i.name)
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
	return wrapError("stop csghub sandbox", i.runtime.client.Stop(ctx, i.name))
}

func (i *Instance) Info(ctx context.Context) (sandbox.Info, error) {
	if err := i.valid(); err != nil {
		return sandbox.Info{}, err
	}
	resp, err := i.runtime.client.Get(ctx, i.name)
	if err != nil {
		return sandbox.Info{}, wrapError("read csghub sandbox info", err)
	}
	return sandboxInfo(resp), nil
}

func (i *Instance) Run(ctx context.Context, spec sandbox.CommandSpec) (sandbox.CommandResult, error) {
	if err := i.valid(); err != nil {
		return sandbox.CommandResult{}, err
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return sandbox.CommandResult{}, fmt.Errorf("invalid sandbox command: name is required")
	}
	command := shellCommand(name, spec.Args)
	writer := firstWriter(spec.Stdout, spec.Stderr)

	var firstStreamError string
	emit := func(line string) error {
		if strings.HasPrefix(line, "ERROR:") && firstStreamError == "" {
			firstStreamError = line
		}
		if writer != nil {
			if _, err := io.WriteString(writer, line+"\n"); err != nil {
				return err
			}
		}
		return nil
	}
	if err := i.runtime.client.StreamExecute(ctx, i.name, command, emit); err != nil {
		return sandbox.CommandResult{}, fmt.Errorf("run csghub command: %w", err)
	}
	if firstStreamError != "" {
		return sandbox.CommandResult{ExitCode: 1}, fmt.Errorf("run csghub command: %s", firstStreamError)
	}
	// Gateway stream API does not expose process exit codes; treat successful
	// streaming completion as zero for the generic sandbox contract.
	return sandbox.CommandResult{ExitCode: 0}, nil
}

func (i *Instance) Close() error {
	return nil
}

func (i *Instance) valid() error {
	if i == nil || i.runtime == nil || i.runtime.client == nil {
		return fmt.Errorf("invalid csghub sandbox")
	}
	if strings.TrimSpace(i.name) == "" {
		return fmt.Errorf("csghub sandbox id or name is required")
	}
	return nil
}

func loadRuntimeConfigFromEnv() (runtimeConfig, error) {
	baseURL := strings.TrimSpace(os.Getenv("CSGHUB_API_BASE_URL"))
	if baseURL == "" {
		return runtimeConfig{}, fmt.Errorf("CSGHUB_API_BASE_URL is required")
	}
	token := strings.TrimSpace(os.Getenv("CSGHUB_USER_TOKEN"))
	if token == "" {
		return runtimeConfig{}, fmt.Errorf("CSGHUB_USER_TOKEN is required")
	}
	resourceID, err := parseOptionalIntEnv("CSGCLAW_RESOURCE_ID")
	if err != nil {
		return runtimeConfig{}, err
	}
	port, err := parseOptionalIntEnv("CSGCLAW_SANDBOX_PORT")
	if err != nil {
		return runtimeConfig{}, err
	}
	timeout, err := parseOptionalIntEnv("CSGCLAW_SANDBOX_TIMEOUT")
	if err != nil {
		return runtimeConfig{}, err
	}
	readyTimeout, err := parseDurationEnv("CSGCLAW_SANDBOX_READY_TIMEOUT", defaultReadyTimeout)
	if err != nil {
		return runtimeConfig{}, err
	}
	if readyTimeout < minReadyTimeout {
		readyTimeout = minReadyTimeout
	}
	pollInterval, err := parseDurationEnv("CSGCLAW_SANDBOX_POLL_INTERVAL", defaultPollInterval)
	if err != nil {
		return runtimeConfig{}, err
	}
	if pollInterval < minPollInterval {
		pollInterval = minPollInterval
	}
	if pollInterval > maxPollInterval {
		pollInterval = maxPollInterval
	}

	pvcMountPath := strings.TrimSpace(os.Getenv("CSGCLAW_PVC_MOUNT_PATH"))
	if pvcMountPath == "" {
		pvcMountPath = defaultPVCMountPath
	}

	return runtimeConfig{
		clientCfg: csghubsdk.Config{
			BaseURL:      baseURL,
			AIGatewayURL: strings.TrimSpace(os.Getenv("CSGHUB_AIGATEWAY_URL")),
			Token:        token,
		},
		clusterID:    strings.TrimSpace(os.Getenv("CSGCLAW_CLUSTER_ID")),
		resourceID:   resourceID,
		port:         port,
		timeout:      timeout,
		pvcMountPath: pvcMountPath,
		readyTimeout: readyTimeout,
		pollInterval: pollInterval,
	}, nil
}

func parseOptionalIntEnv(key string) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	if v < 0 {
		return 0, fmt.Errorf("%s must be >= 0", key)
	}
	return v, nil
}

func parseDurationEnv(key string, defaultValue time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue, nil
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs < 0 {
			return 0, fmt.Errorf("%s must be >= 0", key)
		}
		return time.Duration(secs) * time.Second, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration (e.g. 90s) or integer seconds: %w", key, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("%s must be >= 0", key)
	}
	return d, nil
}

func wrapError(op string, err error) error {
	if err == nil {
		return nil
	}
	if csghubsdk.IsNotFound(err) {
		return fmt.Errorf("%s: %w: %w", op, sandbox.ErrNotFound, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}

func shouldRaiseImmediately(err error) bool {
	var httpErr *csghubsdk.HTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	return httpErr.StatusCode == 401 || httpErr.StatusCode == 403
}

func isDuplicateStartError(err error) bool {
	var httpErr *csghubsdk.HTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	return httpErr.StatusCode == 400
}

func statusOKAfterDuplicateStart(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "deploying", "running", "starting", "creating", "created", "pending":
		return true
	}
	return false
}

func sandboxInfo(resp *csghubsdk.Response) sandbox.Info {
	if resp == nil {
		return sandbox.Info{}
	}
	name := strings.TrimSpace(resp.Spec.SandboxName)
	return sandbox.Info{
		ID:        name,
		Name:      name,
		State:     sandboxState(resp.State.Status),
		CreatedAt: resp.State.CreatedAt,
	}
}

func sandboxState(status string) sandbox.State {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "ready":
		return sandbox.StateRunning
	case "deploying", "starting", "creating", "created", "configured", "pending":
		return sandbox.StateCreated
	case "stopped", "terminated":
		return sandbox.StateStopped
	case "failed", "error", "errored", "crashed", "dead", "exited":
		return sandbox.StateExited
	default:
		return sandbox.StateUnknown
	}
}

func shellCommand(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteShell(name))
	for _, arg := range args {
		parts = append(parts, quoteShell(arg))
	}
	return strings.Join(parts, " ")
}

func quoteShell(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func firstWriter(writers ...io.Writer) io.Writer {
	for _, w := range writers {
		if w != nil {
			return w
		}
	}
	return nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
