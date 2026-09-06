package sandboxproviders

import (
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/boxlitecli"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"strings"
	"testing"
	"unsafe"
)

func TestBoxLiteProviderFactoryUsesDefaultResolvedPath(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("boxlite is not supported on Windows")
	}
	restore := stubBoxLiteAvailability(t, func(path string) (string, error) {
		return path, nil
	}, func(path string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	})
	defer restore()

	factory, ok := factories[config.BoxLiteProvider]
	if !ok {
		t.Fatalf("boxlite provider factory not registered")
	}

	provider, err := factory(config.SandboxConfig{
		Provider:                 config.BoxLiteProvider,
		DebianRegistriesOverride: []string{"registry.a"},
	})
	if err != nil {
		t.Fatalf("factory() error = %v", err)
	}

	boxliteProvider, ok := provider.(boxlitecli.Provider)
	if !ok {
		t.Fatalf("provider = %T, want boxlitecli.Provider", provider)
	}
	if got, want := providerPath(t, boxliteProvider), boxlitecli.ResolvePath(""); got != want {
		t.Fatalf("provider path = %q, want %q", got, want)
	}
	if got, want := provider.Name(), config.BoxLiteProvider; got != want {
		t.Fatalf("provider.Name() = %q, want %q", got, want)
	}
}

func TestBoxLiteProviderDefersAvailabilityUntilOpen(t *testing.T) {
	restore := stubBoxLiteAvailability(t, func(string) (string, error) {
		return "", fmt.Errorf("not found")
	}, func(path string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	})
	defer restore()

	provider, err := Provider(config.SandboxConfig{Provider: config.BoxLiteProvider})
	if err != nil {
		t.Fatalf("Provider() error = %v, want deferred availability check", err)
	}
	_, err = provider.Open(t.Context(), t.TempDir())
	if err == nil {
		t.Fatal("provider.Open() error = nil, want actionable boxlite availability error")
	}
	wants := []string{
		`sandbox provider "boxlite"`,
		`is deprecated`,
		`"docker"`,
	}
	if goruntime.GOOS != "windows" {
		wants = append(wants,
			`no bundled boxlite binary was found`,
			`"boxlite" is not available on PATH`,
			`Switch [sandbox].provider to "docker"`,
			`legacy development use`,
		)
	}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("provider.Open() error = %q, want substring %q", err, want)
		}
	}
}

func TestBoxLiteProviderFactoryAcceptsBundledBinaryWithoutPATHLookup(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("boxlite is not supported on Windows")
	}
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	bundled := filepath.Join(binDir, "boxlite")
	if err := os.WriteFile(bundled, []byte(""), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	restoreExe := boxlitecli.StubExecutablePathForTest(filepath.Join(binDir, "csgclaw"))
	defer restoreExe()

	restore := stubBoxLiteAvailability(t, func(string) (string, error) {
		t.Fatal("lookPath() should not be called when bundled boxlite exists")
		return "", nil
	}, os.Stat)
	defer restore()

	factory, ok := factories[config.BoxLiteProvider]
	if !ok {
		t.Fatalf("boxlite provider factory not registered")
	}
	if _, err := factory(config.SandboxConfig{Provider: config.BoxLiteProvider}); err != nil {
		t.Fatalf("factory() error = %v", err)
	}
}

func sandboxProviderFromOption(t *testing.T, opt agent.ControllerOption) sandbox.Provider {
	t.Helper()
	svc := &agent.Controller{}
	if err := opt(svc); err != nil {
		t.Fatalf("ServiceOption() error = %v", err)
	}
	field := reflect.ValueOf(svc).Elem().FieldByName("sandbox")
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface().(sandbox.Provider)
}

func providerPath(t *testing.T, provider boxlitecli.Provider) string {
	t.Helper()
	value := reflect.ValueOf(&provider).Elem().FieldByName("path")
	return reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem().String()
}

func stubBoxLiteAvailability(t *testing.T, look func(string) (string, error), stat func(string) (os.FileInfo, error)) func() {
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
