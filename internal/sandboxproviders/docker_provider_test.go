package sandboxproviders

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/dockercli"
)

func TestDockerProviderFactoryUsesConfiguredPath(t *testing.T) {
	restore := stubDockerAvailability(t, func(path string) (string, error) {
		return path, nil
	}, func(path string) (os.FileInfo, error) {
		return fakeFileInfo{name: path}, nil
	})
	defer restore()

	factory, ok := factories[config.DockerProvider]
	if !ok {
		t.Fatalf("docker provider factory not registered")
	}

	provider, err := factory(config.SandboxConfig{
		Provider:      config.DockerProvider,
		DockerCLIPath: "/opt/homebrew/bin/docker",
	})
	if err != nil {
		t.Fatalf("factory() error = %v", err)
	}

	dockerProvider, ok := provider.(dockercli.Provider)
	if !ok {
		t.Fatalf("provider = %T, want dockercli.Provider", provider)
	}
	if got, want := dockerProviderPath(t, dockerProvider), "/opt/homebrew/bin/docker"; got != want {
		t.Fatalf("provider path = %q, want %q", got, want)
	}
	if got, want := provider.Name(), config.DockerProvider; got != want {
		t.Fatalf("provider.Name() = %q, want %q", got, want)
	}
}

func TestDockerProviderFactoryDefaultsPath(t *testing.T) {
	restore := stubDockerAvailability(t, func(path string) (string, error) {
		return path, nil
	}, func(path string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	})
	defer restore()

	factory := factories[config.DockerProvider]
	provider, err := factory(config.SandboxConfig{Provider: config.DockerProvider})
	if err != nil {
		t.Fatalf("factory() error = %v", err)
	}
	dockerProvider := provider.(dockercli.Provider)
	if got, want := dockerProviderPath(t, dockerProvider), "docker"; got != want {
		t.Fatalf("provider path = %q, want %q", got, want)
	}
}

func dockerProviderPath(t *testing.T, provider dockercli.Provider) string {
	t.Helper()
	value := reflect.ValueOf(&provider).Elem().FieldByName("path")
	return reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem().String()
}

func TestDockerServiceOptionWiresProvider(t *testing.T) {
	restore := stubDockerAvailability(t, func(path string) (string, error) {
		return path, nil
	}, func(path string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	})
	defer restore()

	opt, err := ServiceOptions(config.SandboxConfig{
		Provider: config.DockerProvider,
	})
	if err != nil {
		t.Fatalf("ServiceOptions() error = %v", err)
	}
	if len(opt) != 1 {
		t.Fatalf("len(opt) = %d, want 1", len(opt))
	}
	svc := &agent.Service{}
	if err := opt[0](svc); err != nil {
		t.Fatalf("sandbox option error = %v", err)
	}
	field := reflect.ValueOf(svc).Elem().FieldByName("sandbox")
	got := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface().(sandbox.Provider)
	if _, ok := got.(dockercli.Provider); !ok {
		t.Fatalf("sandbox provider = %T, want dockercli.Provider", got)
	}
}

func TestDockerProviderFactoryErrorsWhenCLIUnavailable(t *testing.T) {
	restore := stubDockerAvailability(t, func(string) (string, error) {
		return "", os.ErrNotExist
	}, func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	})
	defer restore()

	factory := factories[config.DockerProvider]
	_, err := factory(config.SandboxConfig{Provider: config.DockerProvider})
	if err == nil {
		t.Fatal("factory() error = nil, want docker CLI availability error")
	}
	if got := err.Error(); !strings.Contains(got, `sandbox provider "docker" is configured`) || !strings.Contains(got, `"docker" is not available`) {
		t.Fatalf("factory() error = %q, want docker availability warning", got)
	}
}

func TestDockerAvailabilityAcceptsConfiguredAbsolutePath(t *testing.T) {
	restore := stubDockerAvailability(t, func(string) (string, error) {
		t.Fatal("lookPath() should not be called for configured absolute docker path")
		return "", nil
	}, func(path string) (os.FileInfo, error) {
		return fakeFileInfo{name: path}, nil
	})
	defer restore()

	if err := Availability(config.SandboxConfig{
		Provider:      config.DockerProvider,
		DockerCLIPath: "/usr/local/bin/docker",
	}); err != nil {
		t.Fatalf("Availability() error = %v", err)
	}
}

func stubDockerAvailability(t *testing.T, look func(string) (string, error), stat func(string) (os.FileInfo, error)) func() {
	t.Helper()
	prevLookPath := lookPath
	prevStatPath := statPath
	lookPath = look
	statPath = stat
	return func() {
		lookPath = prevLookPath
		statPath = prevStatPath
	}
}

type fakeFileInfo struct {
	name string
}

func (f fakeFileInfo) Name() string     { return f.name }
func (fakeFileInfo) Size() int64        { return 1 }
func (fakeFileInfo) Mode() os.FileMode  { return 0o755 }
func (fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeFileInfo) IsDir() bool        { return false }
func (fakeFileInfo) Sys() any           { return nil }
