package dshcli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	for input, want := range map[string]string{
		"0.1.6-alpha.2\n":      "0.1.6-alpha.2",
		"dsh v0.2.0 (build)\n": "0.2.0",
	} {
		got, err := ParseVersion(input)
		if err != nil {
			t.Fatalf("ParseVersion(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestProviderResolvePrefersPATH(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "dsh")
	if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	var invoked string
	info, err := (Provider{
		LookPath: func(string) (string, error) {
			return path, nil
		},
		Run: func(_ context.Context, path string, _ ...string) ([]byte, error) {
			invoked = path
			return []byte("dsh 0.1.5-rc.2"), nil
		},
	}).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if info.Path != path || invoked != path || info.Version != "0.1.5-rc.2" {
		t.Fatalf("Resolve() = %+v, invoked %q", info, invoked)
	}
}

func TestProviderResolveIgnoresLegacyPathEnvironment(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "path-dsh")
	legacy := filepath.Join(root, "legacy-dsh")
	for _, candidate := range []string{path, legacy} {
		if err := os.WriteFile(candidate, []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CSGCLAW_DSH_PATH", legacy)
	info, err := (Provider{
		LookPath: func(string) (string, error) { return path, nil },
		Run:      func(context.Context, string, ...string) ([]byte, error) { return []byte("0.1.5-rc.2"), nil },
	}).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if info.Path != path {
		t.Fatalf("Resolve() path = %q, want PATH result %q", info.Path, path)
	}
}

func TestProviderResolveDoesNotGateByVersion(t *testing.T) {
	for _, version := range []string{"0.1.3-alpha.1", "99.0.0"} {
		t.Run(version, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "dsh")
			if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
				t.Fatal(err)
			}
			info, err := (Provider{
				LookPath: func(string) (string, error) { return path, nil },
				Run:      func(context.Context, string, ...string) ([]byte, error) { return []byte(version), nil },
			}).Resolve(context.Background())
			if err != nil || info.Version != version {
				t.Fatalf("Resolve() = %+v, error = %v", info, err)
			}
		})
	}
}

func TestProviderResolveAllowsUnknownVersionOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dsh")
	if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := (Provider{
		LookPath: func(string) (string, error) { return path, nil },
		Run:      func(context.Context, string, ...string) ([]byte, error) { return []byte("development build"), nil },
	}).Resolve(context.Background())
	if err != nil || info.Path != path || info.Version != "" {
		t.Fatalf("Resolve() = %+v, error = %v", info, err)
	}
}

func TestProviderResolveMissingIncludesDomesticInstallCommand(t *testing.T) {
	_, err := (Provider{
		LookPath:    func(string) (string, error) { return "", os.ErrNotExist },
		UserHomeDir: func() (string, error) { return t.TempDir(), nil },
	}).Resolve(context.Background())
	if err == nil {
		t.Fatal("Resolve() error = nil, want missing DSH guidance")
	}
	for _, want := range []string{InstallCommand, "registry.npmmirror.com"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Resolve() error = %q, want %q", err, want)
		}
	}
}

func TestProviderResolveFindsManagedUserInstallOutsidePATH(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".local", "share", "deepseek-harness", "bin", "dsh")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}

	info, err := (Provider{
		LookPath:    func(string) (string, error) { return "", os.ErrNotExist },
		UserHomeDir: func() (string, error) { return home, nil },
		Run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("dsh 0.1.5-rc.2"), nil
		},
	}).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if info.Path != path || info.Version != "0.1.5-rc.2" {
		t.Fatalf("Resolve() = %+v, want managed user install", info)
	}
}

func TestProviderResolvePrefersCommonUserBinBeforeManagedInstall(t *testing.T) {
	home := t.TempDir()
	common := filepath.Join(home, "bin", "dsh")
	managed := filepath.Join(home, ".local", "share", "deepseek-harness", "bin", "dsh")
	for _, path := range []string{common, managed} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	info, err := (Provider{
		LookPath:    func(string) (string, error) { return "", os.ErrNotExist },
		UserHomeDir: func() (string, error) { return home, nil },
		Run:         func(context.Context, string, ...string) ([]byte, error) { return []byte("0.1.5-rc.2"), nil },
	}).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if info.Path != common {
		t.Fatalf("Resolve() path = %q, want common user install %q", info.Path, common)
	}
}

func TestValidateExecutableRequiresAbsolutePath(t *testing.T) {
	if _, err := validateExecutable("dsh"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("validateExecutable() error = %v", err)
	}
}
