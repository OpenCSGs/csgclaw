package sandboxproviders

import (
	"reflect"
	"testing"
	"unsafe"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/dockercli"
)

func TestDockerProviderFactoryUsesConfiguredPath(t *testing.T) {
	factory, ok := factories[config.DockerProvider]
	if !ok {
		t.Fatalf("docker provider factory not registered")
	}

	opt, err := factory(config.SandboxConfig{
		Provider:      config.DockerProvider,
		DockerCLIPath: "/opt/homebrew/bin/docker",
	})
	if err != nil {
		t.Fatalf("factory() error = %v", err)
	}

	provider := sandboxProviderFromOption(t, opt)
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
	factory := factories[config.DockerProvider]
	opt, err := factory(config.SandboxConfig{Provider: config.DockerProvider})
	if err != nil {
		t.Fatalf("factory() error = %v", err)
	}
	provider := sandboxProviderFromOption(t, opt)
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
