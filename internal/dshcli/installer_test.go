package dshcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestInstallerFallsBackToManagedNode(t *testing.T) {
	for _, test := range []struct {
		name     string
		lookPath func(string) (string, error)
		nodeOut  string
	}{
		{
			name:     "Node missing",
			lookPath: func(string) (string, error) { return "", os.ErrNotExist },
		},
		{
			name: "npm missing",
			lookPath: func(name string) (string, error) {
				if name == "node" {
					return "/system/node", nil
				}
				return "", os.ErrNotExist
			},
			nodeOut: "v24.21.0",
		},
		{
			name:     "Node incompatible",
			lookPath: func(name string) (string, error) { return "/system/" + name, nil },
			nodeOut:  "v22.18.0",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			installRoot := filepath.Join(home, "dsh")
			t.Setenv(InstallRootEnv, installRoot)
			nodeRoot := filepath.Join(installRoot, "node-v24.21.0-linux-amd64")
			nodePath := filepath.Join(nodeRoot, "bin", "node")
			npmCLI := filepath.Join(nodeRoot, "lib", "node_modules", "npm", "bin", "npm-cli.js")
			managedCalls := 0
			var npmInvocation []string
			var stages []string
			info, err := (Installer{
				LookPath:    test.lookPath,
				UserHomeDir: func() (string, error) { return home, nil },
				GOOS:        "linux",
				GOARCH:      "amd64",
				InstallManagedNode: func(context.Context, string, ProgressReporter) (nodeRuntime, error) {
					managedCalls++
					for _, path := range []string{nodePath, npmCLI} {
						if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
							return nodeRuntime{}, err
						}
						if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
							return nodeRuntime{}, err
						}
					}
					return managedNodeRuntimeAt(nodeRoot, "linux"), nil
				},
				Run: func(_ context.Context, path string, args ...string) ([]byte, error) {
					if path == "/system/node" {
						return []byte(test.nodeOut), nil
					}
					if path == nodePath && len(args) == 1 && args[0] == "--version" {
						return []byte("v24.21.0"), nil
					}
					if path == nodePath && len(args) > 1 && args[0] == npmCLI {
						npmInvocation = append([]string(nil), args...)
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
			if managedCalls != 1 || info.Version != DefaultVersion {
				t.Fatalf("managed calls = %d, info = %+v", managedCalls, info)
			}
			if len(npmInvocation) == 0 || npmInvocation[0] != npmCLI {
				t.Fatalf("npm invocation = %q, want managed npm CLI prefix", npmInvocation)
			}
			if !slices.Contains(stages, InstallStageNode) {
				t.Fatalf("stages = %q, want %q", stages, InstallStageNode)
			}
			launcher, readErr := os.ReadFile(filepath.Join(home, ".local", "bin", "dsh"))
			if readErr != nil || !strings.Contains(string(launcher), nodeRoot) || !strings.Contains(string(launcher), managedLauncherMarker) {
				t.Fatalf("launcher = %q, error = %v", launcher, readErr)
			}
		})
	}
}

func TestManagedNodeIsAvailableToNPMInstallScripts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	home := t.TempDir()
	installRoot := filepath.Join(home, "dsh")
	nodeRoot := filepath.Join(installRoot, "node-v24.21.0-linux-amd64")
	nodePath := filepath.Join(nodeRoot, "bin", "node")
	npmCLI := filepath.Join(nodeRoot, "lib", "node_modules", "npm", "bin", "npm-cli.js")
	actualDSH := filepath.Join(installRoot, "bin", "dsh")
	for _, path := range []string{nodePath, npmCLI} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	nodeScript := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo v24.21.0; exit 0; fi\nexec /bin/sh \"$@\"\n"
	if err := os.WriteFile(nodePath, []byte(nodeScript), 0o755); err != nil {
		t.Fatal(err)
	}
	npmScript := "#!/bin/sh\n[ \"$(command -v node)\" = " + shellQuote(nodePath) + " ] || exit 42\nmkdir -p " + shellQuote(filepath.Dir(actualDSH)) + "\nprintf '#!/bin/sh\\necho dsh 0.1.5-rc.2\\n' > " + shellQuote(actualDSH) + "\nchmod +x " + shellQuote(actualDSH) + "\n"
	if err := os.WriteFile(npmCLI, []byte(npmScript), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(InstallRootEnv, installRoot)
	info, err := (Installer{
		LookPath:    func(string) (string, error) { return "", os.ErrNotExist },
		UserHomeDir: func() (string, error) { return home, nil },
		GOOS:        "linux",
		GOARCH:      "amd64",
		InstallManagedNode: func(context.Context, string, ProgressReporter) (nodeRuntime, error) {
			return managedNodeRuntimeAt(nodeRoot, "linux"), nil
		},
	}).Install(context.Background())
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if info.Version != DefaultVersion {
		t.Fatalf("Install() version = %q, want %q", info.Version, DefaultVersion)
	}
}

func TestInstallerReportsManagedNodeFailure(t *testing.T) {
	home := t.TempDir()
	_, err := (Installer{
		LookPath:    func(string) (string, error) { return "", os.ErrNotExist },
		UserHomeDir: func() (string, error) { return home, nil },
		InstallManagedNode: func(context.Context, string, ProgressReporter) (nodeRuntime, error) {
			return nodeRuntime{}, errors.New("download failed")
		},
	}).Install(context.Background())
	if err == nil || !errors.Is(err, ErrNodeInstallFailed) {
		t.Fatalf("Install() error = %v, want ErrNodeInstallFailed", err)
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

func TestSelectNodeReleaseUsesStableNode24Asset(t *testing.T) {
	checksum := strings.Repeat("a", 64)
	manifest := checksum + "  node-v24.21.0-darwin-arm64.tar.gz\n" +
		checksum + "  node-v24.21.0-linux-x64.tar.gz\n" +
		checksum + "  node-v24.21.0-win-x64.zip\n"
	for _, test := range []struct {
		goos     string
		goarch   string
		filename string
	}{
		{goos: "darwin", goarch: "arm64", filename: "node-v24.21.0-darwin-arm64.tar.gz"},
		{goos: "linux", goarch: "amd64", filename: "node-v24.21.0-linux-x64.tar.gz"},
		{goos: "windows", goarch: "amd64", filename: "node-v24.21.0-win-x64.zip"},
	} {
		release, err := selectNodeRelease(manifest, test.goos, test.goarch)
		if err != nil || release.Version != "24.21.0" || release.Filename != test.filename {
			t.Fatalf("selectNodeRelease(%s/%s) = %+v, %v", test.goos, test.goarch, release, err)
		}
	}
}

func TestCreateLauncherCanReturnFromManagedNodeToSystemNode(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(root, "share", "dsh")
	launcher := filepath.Join(root, "bin", "dsh")
	if err := os.MkdirAll(filepath.Dir(actual), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actual, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, managedLauncherContents(actual, filepath.Join(root, "node", "bin"), "linux"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := createLauncher(actual, launcher, "", "linux"); err != nil {
		t.Fatalf("createLauncher() error = %v", err)
	}
	target, err := os.Readlink(launcher)
	if err != nil || target != actual {
		t.Fatalf("launcher target = %q, error = %v", target, err)
	}
}
