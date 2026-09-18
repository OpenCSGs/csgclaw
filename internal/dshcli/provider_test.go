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

func TestProviderResolvePrefersExplicitPath(t *testing.T) {
	root := t.TempDir()
	explicit := filepath.Join(root, "explicit-dsh")
	environment := filepath.Join(root, "environment-dsh")
	for _, path := range []string{explicit, environment} {
		if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(PathEnv, environment)
	var invoked string
	info, err := (Provider{
		ExplicitPath: explicit,
		LookPath: func(string) (string, error) {
			t.Fatal("PATH lookup must not run when an explicit path is configured")
			return "", nil
		},
		Run: func(_ context.Context, path string, _ ...string) ([]byte, error) {
			invoked = path
			return []byte("dsh 0.1.5-rc.2"), nil
		},
	}).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if info.Path != explicit || invoked != explicit || info.Version != "0.1.5-rc.2" {
		t.Fatalf("Resolve() = %+v, invoked %q", info, invoked)
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
				ExplicitPath: path,
				Run:          func(context.Context, string, ...string) ([]byte, error) { return []byte(version), nil },
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
		ExplicitPath: path,
		Run:          func(context.Context, string, ...string) ([]byte, error) { return []byte("development build"), nil },
	}).Resolve(context.Background())
	if err != nil || info.Path != path || info.Version != "" {
		t.Fatalf("Resolve() = %+v, error = %v", info, err)
	}
}

func TestValidateExecutableRequiresAbsolutePath(t *testing.T) {
	if _, err := validateExecutable("dsh"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("validateExecutable() error = %v", err)
	}
}
