package dshcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestInstallerUsesUserPrefixDomesticRegistryAndVerifies(t *testing.T) {
	if DefaultPackage != "@deepseek-ai/dsh@0.1.5-rc.2" {
		t.Fatalf("DefaultPackage = %q, want pinned DSH release candidate", DefaultPackage)
	}
	home := t.TempDir()
	installRoot := filepath.Join(home, ".local", "share", "deepseek-harness")
	t.Setenv(InstallRootEnv, installRoot)
	t.Setenv(InstallPackageEnv, "")
	t.Setenv("NPM_CONFIG_REGISTRY", "")
	t.Setenv("SHELL", "/bin/zsh")
	var npmArgs []string
	var stages []string

	info, err := (Installer{
		LookPath:    func(name string) (string, error) { return "/test/bin/" + name, nil },
		UserHomeDir: func() (string, error) { return home, nil },
		GOOS:        "linux",
		Run: func(_ context.Context, path string, args ...string) ([]byte, error) {
			if path == "/test/bin/node" {
				return []byte("v24.21.0"), nil
			}
			if path == "/test/bin/npm" {
				npmArgs = append([]string(nil), args...)
				actual := filepath.Join(installRoot, "bin", "dsh")
				if err := os.MkdirAll(filepath.Dir(actual), 0o755); err != nil {
					return nil, err
				}
				if err := os.WriteFile(actual, []byte("test"), 0o755); err != nil {
					return nil, err
				}
				return []byte("installed"), nil
			}
			return []byte("dsh 0.1.5-rc.2"), nil
		},
	}).InstallWithProgress(context.Background(), func(progress InstallProgress) {
		stages = append(stages, progress.Stage)
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	wantArgs := []string{
		"install",
		"-g",
		"--prefix=" + installRoot,
		"--registry=" + DefaultRegistry,
		"--loglevel=http",
		"--no-progress",
		"--no-audit",
		"--no-fund",
		"--prefer-offline",
		DefaultPackage,
	}
	if !slices.Equal(npmArgs, wantArgs) {
		t.Fatalf("npm args = %q, want %q", npmArgs, wantArgs)
	}
	launcher := filepath.Join(home, ".local", "bin", "dsh")
	if info.Path != launcher || info.Version != "0.1.5-rc.2" {
		t.Fatalf("Install() info = %+v", info)
	}
	target, err := os.Readlink(launcher)
	if err != nil || target != filepath.Join(installRoot, "bin", "dsh") {
		t.Fatalf("launcher target = %q, error = %v", target, err)
	}
	profile, err := os.ReadFile(filepath.Join(home, ".zprofile"))
	if err != nil || !strings.Contains(string(profile), `export PATH="$HOME/.local/bin:$PATH"`) {
		t.Fatalf("zprofile = %q, error = %v", profile, err)
	}
	wantStages := []string{
		InstallStageChecking,
		InstallStageInstalling,
		InstallStageLinking,
		InstallStageVerifying,
		InstallStageConfiguring,
		InstallStageCompleted,
	}
	if !slices.Equal(stages, wantStages) {
		t.Fatalf("progress stages = %q, want %q", stages, wantStages)
	}
}

func TestInstallOutputCollectorReportsRealNPMActivity(t *testing.T) {
	var reports []InstallProgress
	collector := &installOutputCollector{report: func(progress InstallProgress) {
		reports = append(reports, progress)
	}}
	for index := 0; index < 12; index++ {
		if _, err := collector.Write([]byte("npm http fetch GET 200 package\n")); err != nil {
			t.Fatal(err)
		}
	}
	collector.flushProgress()
	if len(reports) != 3 {
		t.Fatalf("progress reports = %+v, want counts 1, 11, and 12", reports)
	}
	if reports[0].ActivityCount != 1 || reports[1].ActivityCount != 11 || reports[2].ActivityCount != 12 {
		t.Fatalf("progress reports = %+v, want counts 1, 11, and 12", reports)
	}
}

func TestInstallerRequiresNode(t *testing.T) {
	_, err := (Installer{
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}).Install(context.Background())
	if err == nil || !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("Install() error = %v, want ErrNodeNotFound", err)
	}
}

func TestInstallerRequiresNPM(t *testing.T) {
	_, err := (Installer{
		LookPath: func(name string) (string, error) {
			if name == "node" {
				return "/test/bin/node", nil
			}
			return "", os.ErrNotExist
		},
		Run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("v24.21.0"), nil
		},
	}).Install(context.Background())
	if err == nil || !errors.Is(err, ErrNPMNotFound) {
		t.Fatalf("Install() error = %v, want ErrNPMNotFound", err)
	}
}

func TestInstallerRejectsUnsupportedNodeVersion(t *testing.T) {
	_, err := (Installer{
		LookPath: func(name string) (string, error) { return "/test/bin/" + name, nil },
		Run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("v22.18.0"), nil
		},
	}).Install(context.Background())
	if err == nil || !errors.Is(err, ErrNodeUnsupported) {
		t.Fatalf("Install() error = %v, want ErrNodeUnsupported", err)
	}
}

func TestSupportedNodeVersion(t *testing.T) {
	for _, test := range []struct {
		version string
		want    bool
	}{
		{version: "v22.18.0", want: false},
		{version: "v22.19.0", want: true},
		{version: "v23.11.1", want: false},
		{version: "v24.0.0", want: true},
		{version: "v25.1.0", want: true},
		{version: "unknown", want: false},
	} {
		if got := supportedNodeVersion(test.version); got != test.want {
			t.Fatalf("supportedNodeVersion(%q) = %v, want %v", test.version, got, test.want)
		}
	}
}
