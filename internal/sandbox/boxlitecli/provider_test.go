package boxlitecli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"csgclaw/internal/sandbox"
)

func TestProviderImplementsSandboxProvider(t *testing.T) {
	var _ sandbox.Provider = NewProvider()
	if got, want := NewProvider().Name(), "boxlite"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
}

func TestCreateBuildsRunCLIArgs(t *testing.T) {
	runner := &fakeRunner{
		results: []fakeResult{{result: CommandResult{Stdout: []byte("box-id\n")}}},
	}
	rt, err := NewProvider(
		WithPath("/usr/local/bin/boxlite"),
		WithConfig("/tmp/config.toml"),
		WithRegistry("registry.local"),
		WithRunner(runner),
	).Open(context.Background(), "/tmp/boxlite-home")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	inst, err := rt.Create(context.Background(), sandbox.CreateSpec{
		Image:      "alpine",
		Name:       "agent",
		Detach:     true,
		AutoRemove: true,
		Env:        map[string]string{"B": "two", "A": "one"},
		Cmd:        []string{"sh", "-lc", "echo ok"},
		Mounts: []sandbox.Mount{
			{HostPath: "/host/rw", GuestPath: "/guest/rw"},
			{HostPath: "/host/ro", GuestPath: "/guest/ro", ReadOnly: true},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if inst == nil {
		t.Fatal("Create() instance = nil")
	}

	want := []string{
		"--home", "/tmp/boxlite-home",
		"--config", "/tmp/config.toml",
		"--registry", "registry.local",
		"run",
		"--name", "agent",
		"--detach",
		"--rm",
		"-e", "A=one",
		"-e", "B=two",
		"-v", "/host/rw:/guest/rw",
		"-v", "/host/ro:/guest/ro:ro",
		"alpine",
		"sh",
		"-lc",
		"echo ok",
	}
	if got := runner.requests[0].Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("Create() args = %#v, want %#v", got, want)
	}
	if got, want := runner.requests[0].Path, "/usr/local/bin/boxlite"; got != want {
		t.Fatalf("Create() path = %q, want %q", got, want)
	}
}

func TestListImagesBuildsCLIArgsAndParsesJSON(t *testing.T) {
	runner := &fakeRunner{
		results: []fakeResult{{result: CommandResult{Stdout: []byte(`[
			{"Repository":"registry.example/picoclaw","Tag":"2026.5.27"},
			{"repository":"registry.example/picoclaw","tag":"2026.5.22"},
			{"Reference":"registry.example/custom:dev"},
			{"repository":"<none>","tag":"<none>"},
			{"Repository":"registry.example/picoclaw","Tag":"2026.5.27"}
		]`)}}},
	}

	got, err := ListImages(
		context.Background(),
		"/tmp/boxlite-home",
		WithPath("/usr/local/bin/boxlite"),
		WithConfig("/tmp/config.toml"),
		WithRegistry("registry.local"),
		WithRunner(runner),
	)
	if err != nil {
		t.Fatalf("ListImages() error = %v", err)
	}
	wantImages := []string{
		"registry.example/picoclaw:2026.5.27",
		"registry.example/picoclaw:2026.5.22",
		"registry.example/custom:dev",
	}
	if !reflect.DeepEqual(got, wantImages) {
		t.Fatalf("ListImages() = %#v, want %#v", got, wantImages)
	}

	wantArgs := []string{
		"--home", "/tmp/boxlite-home",
		"--config", "/tmp/config.toml",
		"--registry", "registry.local",
		"images",
		"--format", "json",
	}
	if got := runner.requests[0].Args; !reflect.DeepEqual(got, wantArgs) {
		t.Fatalf("ListImages() args = %#v, want %#v", got, wantArgs)
	}
	if got, want := runner.requests[0].Path, "/usr/local/bin/boxlite"; got != want {
		t.Fatalf("ListImages() path = %q, want %q", got, want)
	}
}

func TestCreateRejectsUnsupportedOptions(t *testing.T) {
	tests := []struct {
		name string
		spec sandbox.CreateSpec
		want string
	}{
		{name: "image", spec: sandbox.CreateSpec{}, want: "image is required"},
		{name: "entrypoint", spec: sandbox.CreateSpec{Image: "alpine", Entrypoint: []string{"sh"}}, want: "entrypoint"},
		{name: "env", spec: sandbox.CreateSpec{Image: "alpine", Env: map[string]string{"": "x"}}, want: "env"},
		{name: "mount host", spec: sandbox.CreateSpec{Image: "alpine", Mounts: []sandbox.Mount{{GuestPath: "/guest"}}}, want: "host path"},
		{name: "mount guest", spec: sandbox.CreateSpec{Image: "alpine", Mounts: []sandbox.Mount{{HostPath: "/host"}}}, want: "guest path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runArgs(tt.spec)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("runArgs() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestInstanceMethodsBuildCLIArgs(t *testing.T) {
	runner := &fakeRunner{
		results: []fakeResult{
			{result: CommandResult{}},
			{result: CommandResult{}},
			{result: CommandResult{Stdout: []byte(`[{"Id":"box-id","Name":"agent","Created":"2026-04-18T07:31:25.471080+00:00","Status":"running"}]`)}},
			{result: CommandResult{}},
		},
	}
	rt, err := NewProvider(WithRunner(runner)).Open(context.Background(), "/tmp/boxlite-home")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	inst := &Instance{runtime: rt.(*Runtime), idOrName: "box-id"}

	if err := inst.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := inst.Stop(context.Background(), sandbox.StopOptions{}); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if _, err := inst.Info(context.Background()); err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if _, err := inst.Run(context.Background(), sandbox.CommandSpec{Name: "sh", Args: []string{"-lc", "echo ok"}}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wants := [][]string{
		{"--home", "/tmp/boxlite-home", "start", "box-id"},
		{"--home", "/tmp/boxlite-home", "stop", "box-id"},
		{"--home", "/tmp/boxlite-home", "inspect", "--format", "json", "box-id"},
		{"--home", "/tmp/boxlite-home", "exec", "box-id", "--", "sh", "-lc", "echo ok"},
	}
	for idx, want := range wants {
		if got := runner.requests[idx].Args; !reflect.DeepEqual(got, want) {
			t.Fatalf("request %d args = %#v, want %#v", idx, got, want)
		}
	}
}

func TestRunForwardsOutputAndPreservesExitCode(t *testing.T) {
	runner := &fakeRunner{
		results: []fakeResult{{
			result:      CommandResult{Stdout: []byte("out"), Stderr: []byte("err"), ExitCode: 7},
			err:         &exec.ExitError{},
			writeStdout: []byte("out"),
			writeStderr: []byte("err"),
		}},
	}
	rt, err := NewProvider(WithRunner(runner)).Open(context.Background(), "/tmp/boxlite-home")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	inst := &Instance{runtime: rt.(*Runtime), idOrName: "box-id"}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	result, err := inst.Run(context.Background(), sandbox.CommandSpec{
		Name:   "sh",
		Args:   []string{"-lc", "echo out; echo err >&2; exit 7"},
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err == nil {
		t.Fatal("Run() error = nil, want exit error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %T, want *ExitError", err)
	}
	if result.ExitCode != 7 || exitErr.ExitCode != 7 {
		t.Fatalf("exit code = result %d error %d, want 7", result.ExitCode, exitErr.ExitCode)
	}
	if stdout.String() != "out" || stderr.String() != "err" {
		t.Fatalf("forwarded output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestNotFoundErrorsMapToSandboxNotFound(t *testing.T) {
	runner := &fakeRunner{
		results: []fakeResult{{
			result: CommandResult{Stdout: []byte("[]"), Stderr: []byte("Error: no such box: missing\n"), ExitCode: 1},
			err:    &exec.ExitError{},
		}},
	}
	rt, err := NewProvider(WithRunner(runner)).Open(context.Background(), "/tmp/boxlite-home")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	_, err = rt.Get(context.Background(), "missing")
	if !sandbox.IsNotFound(err) {
		t.Fatalf("Get() error = %v, want sandbox not found", err)
	}
}

func TestUnsupportedStopOptions(t *testing.T) {
	inst := &Instance{runtime: &Runtime{path: "boxlite", homeDir: "/tmp/home", runner: &fakeRunner{}}, idOrName: "box-id"}
	if err := inst.Stop(context.Background(), sandbox.StopOptions{Force: true}); err == nil || !strings.Contains(err.Error(), "force stop") {
		t.Fatalf("Stop(force) error = %v, want unsupported force", err)
	}
}

func TestNilHandles(t *testing.T) {
	ctx := context.Background()
	rt := (*Runtime)(nil)
	if _, err := rt.Create(ctx, sandbox.CreateSpec{}); err == nil {
		t.Fatal("nil runtime Create should fail")
	}
	if _, err := rt.Get(ctx, "box"); err == nil {
		t.Fatal("nil runtime Get should fail")
	}
	if err := rt.Remove(ctx, "box", sandbox.RemoveOptions{}); err == nil {
		t.Fatal("nil runtime Remove should fail")
	}

	inst := (*Instance)(nil)
	if err := inst.Start(ctx); err == nil {
		t.Fatal("nil instance Start should fail")
	}
	if err := inst.Stop(ctx, sandbox.StopOptions{}); err == nil {
		t.Fatal("nil instance Stop should fail")
	}
	if _, err := inst.Info(ctx); err == nil {
		t.Fatal("nil instance Info should fail")
	}
	if _, err := inst.Run(ctx, sandbox.CommandSpec{Name: "true"}); err == nil {
		t.Fatal("nil instance Run should fail")
	}
}

func TestIntegrationCreateStartExecRemove(t *testing.T) {
	if os.Getenv("CSGCLAW_BOXLITE_CLI_INTEGRATION") != "1" {
		t.Skip("set CSGCLAW_BOXLITE_CLI_INTEGRATION=1 to run boxlite CLI integration test")
	}
	path := strings.TrimSpace(os.Getenv("CSGCLAW_BOXLITE_CLI_PATH"))
	if path == "" {
		path = "boxlite"
	}

	ctx := context.Background()
	rt, err := NewProvider(WithPath(path)).Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer rt.Close()

	name := "csgclaw-boxlite-cli-it"
	inst, err := rt.Create(ctx, sandbox.CreateSpec{
		Image:  "alpine",
		Name:   name,
		Detach: true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer func() {
		_ = rt.Remove(context.Background(), name, sandbox.RemoveOptions{Force: true})
	}()
	if _, err := inst.Info(ctx); err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	var stdout bytes.Buffer
	result, err := inst.Run(ctx, sandbox.CommandSpec{
		Name:   "sh",
		Args:   []string{"-lc", "echo ok"},
		Stdout: &stdout,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(stdout.String()) != "ok" {
		t.Fatalf("Run() result exit=%d stdout=%q, want exit 0 stdout ok", result.ExitCode, stdout.String())
	}
	if err := rt.Remove(ctx, name, sandbox.RemoveOptions{Force: true}); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
}

type fakeRunner struct {
	requests []CommandRequest
	results  []fakeResult
}

type fakeResult struct {
	result      CommandResult
	err         error
	writeStdout []byte
	writeStderr []byte
}

func (r *fakeRunner) Run(_ context.Context, req CommandRequest) (CommandResult, error) {
	r.requests = append(r.requests, req)
	if len(r.results) == 0 {
		return CommandResult{}, nil
	}
	result := r.results[0]
	r.results = r.results[1:]
	if req.Stdout != nil && len(result.writeStdout) > 0 {
		_, _ = req.Stdout.Write(result.writeStdout)
	}
	if req.Stderr != nil && len(result.writeStderr) > 0 {
		_, _ = req.Stderr.Write(result.writeStderr)
	}
	return result.result, result.err
}
