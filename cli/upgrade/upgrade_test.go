package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"csgclaw/cli/command"
	internalupgrade "csgclaw/internal/upgrade"
	appversion "csgclaw/internal/version"
)

func TestRunNoRestartInstallsBundle(t *testing.T) {
	originalVersion := appversion.Version
	appversion.Version = "v0.2.5"
	t.Cleanup(func() { appversion.Version = originalVersion })

	installRoot := writeInstalledBundle(t, t.TempDir(), "old")
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n# new\n",
		"csgclaw/bin/boxlite": "#!/bin/sh\n# new boxlite\n",
		"csgclaw/README.md":   "new",
	})
	sum := sha256.Sum256(archive)

	originalClientFactory := newUpgradeClient
	newUpgradeClient = func(run *command.Context) internalupgrade.Client {
		return internalupgrade.Client{
			HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.String() {
				case "https://example.test/releases/latest":
					return jsonResponse(http.StatusOK, `{
						"name":"v0.2.7",
						"assets":[{"name":"csgclaw_v0.2.7_darwin_arm64.tar.gz","browser_download_url":"https://downloads.example.test/csgclaw.tar.gz","size":`+strconv.Itoa(len(archive))+`,"sha256":"`+hex.EncodeToString(sum[:])+`"}]
					}`), nil
				case "https://downloads.example.test/csgclaw.tar.gz":
					return &http.Response{
						StatusCode: http.StatusOK,
						Status:     http.StatusText(http.StatusOK),
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewReader(archive)),
					}, nil
				default:
					t.Fatalf("unexpected URL %q", req.URL.String())
					return nil, nil
				}
			}),
			LatestURL: "https://example.test/releases/latest",
			GOOS:      "darwin",
			GOARCH:    "arm64",
			ExecutablePath: func() (string, error) {
				return filepath.Join(installRoot, "bin", "csgclaw"), nil
			},
		}
	}
	t.Cleanup(func() { newUpgradeClient = originalClientFactory })

	var stdout bytes.Buffer
	err := NewCmd().Run(context.Background(), &command.Context{
		Program: "csgclaw",
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	}, []string{"--no-restart"}, command.GlobalOptions{Output: "table"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{
		"Current version: v0.2.5",
		"Latest version:  v0.2.7",
		"Installing new bundle",
		"Upgrade completed: v0.2.7",
		"Restart skipped",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
	assertFileContent(t, filepath.Join(installRoot, "README.md"), "new")
}

func TestRunWindowsNoRestartDoesNotStopDaemon(t *testing.T) {
	originalVersion := appversion.Version
	appversion.Version = "v0.2.5"
	t.Cleanup(func() { appversion.Version = originalVersion })

	originalGOOS := currentGOOS
	originalStop := stopDaemonFromExecutable
	originalStart := startDaemonFromExecutable
	currentGOOS = "windows"
	stopDaemonFromExecutable = func(context.Context) (internalupgrade.RestartResult, error) {
		t.Fatal("stopDaemonFromExecutable should not be called with --no-restart")
		return internalupgrade.RestartResult{}, nil
	}
	startDaemonFromExecutable = func(context.Context, internalupgrade.RestartOptions) error {
		t.Fatal("startDaemonFromExecutable should not be called with --no-restart")
		return nil
	}
	t.Cleanup(func() {
		currentGOOS = originalGOOS
		stopDaemonFromExecutable = originalStop
		startDaemonFromExecutable = originalStart
	})

	installRoot := writeInstalledBundle(t, t.TempDir(), "old")
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n# new\n",
		"csgclaw/bin/boxlite": "#!/bin/sh\n# new boxlite\n",
		"csgclaw/README.md":   "new",
	})
	sum := sha256.Sum256(archive)

	originalClientFactory := newUpgradeClient
	newUpgradeClient = func(run *command.Context) internalupgrade.Client {
		return internalupgrade.Client{
			HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.String() {
				case "https://example.test/releases/latest":
					return jsonResponse(http.StatusOK, `{
						"name":"v0.2.7",
						"assets":[{"name":"csgclaw_v0.2.7_darwin_arm64.tar.gz","browser_download_url":"https://downloads.example.test/csgclaw.tar.gz","size":`+strconv.Itoa(len(archive))+`,"sha256":"`+hex.EncodeToString(sum[:])+`"}]
					}`), nil
				case "https://downloads.example.test/csgclaw.tar.gz":
					return &http.Response{
						StatusCode: http.StatusOK,
						Status:     http.StatusText(http.StatusOK),
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewReader(archive)),
					}, nil
				default:
					t.Fatalf("unexpected URL %q", req.URL.String())
					return nil, nil
				}
			}),
			LatestURL: "https://example.test/releases/latest",
			GOOS:      "darwin",
			GOARCH:    "arm64",
			ExecutablePath: func() (string, error) {
				return filepath.Join(installRoot, "bin", "csgclaw"), nil
			},
		}
	}
	t.Cleanup(func() { newUpgradeClient = originalClientFactory })

	var stdout bytes.Buffer
	err := NewCmd().Run(context.Background(), &command.Context{
		Program: "csgclaw",
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	}, []string{"--no-restart"}, command.GlobalOptions{Output: "table"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Restart skipped") {
		t.Fatalf("stdout = %q, want restart skipped", stdout.String())
	}
	assertFileContent(t, filepath.Join(installRoot, "README.md"), "new")
}

func TestRunRestartsRunningDaemonAfterInstall(t *testing.T) {
	originalVersion := appversion.Version
	appversion.Version = "v0.2.5"
	t.Cleanup(func() { appversion.Version = originalVersion })

	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	pidPath := filepath.Join(configHome, ".csgclaw", "server.pid")
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(pidPath), err)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", pidPath, err)
	}

	installRoot := writeInstalledBundle(t, t.TempDir(), "old")
	restartLog := filepath.Join(t.TempDir(), "restart.log")
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": restartScript(logPathLiteral(restartLog)),
		"csgclaw/bin/boxlite": "#!/bin/sh\n# new boxlite\n",
		"csgclaw/README.md":   "new",
	})
	sum := sha256.Sum256(archive)

	originalClientFactory := newUpgradeClient
	newUpgradeClient = func(run *command.Context) internalupgrade.Client {
		return internalupgrade.Client{
			HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.String() {
				case "https://example.test/releases/latest":
					return jsonResponse(http.StatusOK, `{
						"name":"v0.2.7",
						"assets":[{"name":"csgclaw_v0.2.7_darwin_arm64.tar.gz","browser_download_url":"https://downloads.example.test/csgclaw.tar.gz","size":`+strconv.Itoa(len(archive))+`,"sha256":"`+hex.EncodeToString(sum[:])+`"}]
					}`), nil
				case "https://downloads.example.test/csgclaw.tar.gz":
					return &http.Response{
						StatusCode: http.StatusOK,
						Status:     http.StatusText(http.StatusOK),
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewReader(archive)),
					}, nil
				default:
					t.Fatalf("unexpected URL %q", req.URL.String())
					return nil, nil
				}
			}),
			LatestURL: "https://example.test/releases/latest",
			GOOS:      "darwin",
			GOARCH:    "arm64",
			ExecutablePath: func() (string, error) {
				return filepath.Join(installRoot, "bin", "csgclaw"), nil
			},
		}
	}
	t.Cleanup(func() { newUpgradeClient = originalClientFactory })

	var stdout bytes.Buffer
	err := NewCmd().Run(context.Background(), &command.Context{
		Program: "csgclaw",
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
	}, nil, command.GlobalOptions{Output: "table"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{
		"Installing new bundle",
		"Restarting service",
		"Upgrade completed: v0.2.7",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
	logData, err := os.ReadFile(restartLog)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", restartLog, err)
	}
	if got := string(logData); got != "stop\nserve --daemon\n" {
		t.Fatalf("restart log = %q, want stop then serve", got)
	}
}

func TestRunWindowsRestartsStoppedDaemonWhenInstallFails(t *testing.T) {
	originalVersion := appversion.Version
	appversion.Version = "v0.2.5"
	t.Cleanup(func() { appversion.Version = originalVersion })

	originalGOOS := currentGOOS
	originalStop := stopDaemonFromExecutable
	originalStart := startDaemonFromExecutable
	originalInstall := installPrepared
	currentGOOS = "windows"
	stopCalls := 0
	startCalls := 0
	stopDaemonFromExecutable = func(context.Context) (internalupgrade.RestartResult, error) {
		stopCalls++
		return internalupgrade.RestartResult{DaemonWasRunning: true}, nil
	}
	startDaemonFromExecutable = func(_ context.Context, opts internalupgrade.RestartOptions) error {
		startCalls++
		if opts.ConfigPath != "/tmp/csgclaw-test-config.toml" {
			t.Fatalf("ConfigPath = %q, want test config path", opts.ConfigPath)
		}
		return nil
	}
	installPrepared = func(internalupgrade.Client, internalupgrade.PreparedBundle) (internalupgrade.InstalledBundle, error) {
		return internalupgrade.InstalledBundle{}, errors.New("install exploded")
	}
	t.Cleanup(func() {
		currentGOOS = originalGOOS
		stopDaemonFromExecutable = originalStop
		startDaemonFromExecutable = originalStart
		installPrepared = originalInstall
	})

	installRoot := writeInstalledBundle(t, t.TempDir(), "old")
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n# new\n",
		"csgclaw/bin/boxlite": "#!/bin/sh\n# new boxlite\n",
		"csgclaw/README.md":   "new",
	})
	sum := sha256.Sum256(archive)

	originalClientFactory := newUpgradeClient
	newUpgradeClient = func(run *command.Context) internalupgrade.Client {
		return internalupgrade.Client{
			HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.String() {
				case "https://example.test/releases/latest":
					return jsonResponse(http.StatusOK, `{
						"name":"v0.2.7",
						"assets":[{"name":"csgclaw_v0.2.7_darwin_arm64.tar.gz","browser_download_url":"https://downloads.example.test/csgclaw.tar.gz","size":`+strconv.Itoa(len(archive))+`,"sha256":"`+hex.EncodeToString(sum[:])+`"}]
					}`), nil
				case "https://downloads.example.test/csgclaw.tar.gz":
					return &http.Response{
						StatusCode: http.StatusOK,
						Status:     http.StatusText(http.StatusOK),
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewReader(archive)),
					}, nil
				default:
					t.Fatalf("unexpected URL %q", req.URL.String())
					return nil, nil
				}
			}),
			LatestURL: "https://example.test/releases/latest",
			GOOS:      "darwin",
			GOARCH:    "arm64",
			ExecutablePath: func() (string, error) {
				return filepath.Join(installRoot, "bin", "csgclaw"), nil
			},
		}
	}
	t.Cleanup(func() { newUpgradeClient = originalClientFactory })

	err := NewCmd().Run(context.Background(), &command.Context{
		Program: "csgclaw",
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}, nil, command.GlobalOptions{Output: "table", Config: "/tmp/csgclaw-test-config.toml"})
	if err == nil || !strings.Contains(err.Error(), "install exploded") {
		t.Fatalf("Run() error = %v, want install failure", err)
	}
	if stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", stopCalls)
	}
	if startCalls != 1 {
		t.Fatalf("start calls = %d, want 1", startCalls)
	}
}

func TestRunHelpIncludesUpgradeGuidance(t *testing.T) {
	var stderr bytes.Buffer
	err := NewCmd().Run(context.Background(), &command.Context{
		Program: "csgclaw",
		Stdout:  &bytes.Buffer{},
		Stderr:  &stderr,
	}, []string{"-h"}, command.GlobalOptions{Output: "table"})
	if err != flag.ErrHelp {
		t.Fatalf("Run() error = %v, want %v", err, flag.ErrHelp)
	}
	for _, want := range []string{
		"Usage:",
		"csgclaw upgrade [flags]",
		"csgclaw upgrade --check",
		"csgclaw upgrade --no-restart",
		"Automatic install requires an official release bundle marker and layout",
		"Automatic restart only supports the default PID path",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("help = %q, want substring %q", stderr.String(), want)
		}
	}
}

func TestRunInstallErrorExplainsBundleRequirement(t *testing.T) {
	originalVersion := appversion.Version
	appversion.Version = "v0.2.5"
	t.Cleanup(func() { appversion.Version = originalVersion })

	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n# new\n",
		"csgclaw/bin/boxlite": "#!/bin/sh\n# new boxlite\n",
	})
	sum := sha256.Sum256(archive)

	originalClientFactory := newUpgradeClient
	newUpgradeClient = func(run *command.Context) internalupgrade.Client {
		return internalupgrade.Client{
			HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.String() {
				case "https://example.test/releases/latest":
					return jsonResponse(http.StatusOK, `{
						"name":"v0.2.7",
						"assets":[{"name":"csgclaw_v0.2.7_darwin_arm64.tar.gz","browser_download_url":"https://downloads.example.test/csgclaw.tar.gz","size":`+strconv.Itoa(len(archive))+`,"sha256":"`+hex.EncodeToString(sum[:])+`"}]
					}`), nil
				case "https://downloads.example.test/csgclaw.tar.gz":
					return &http.Response{
						StatusCode: http.StatusOK,
						Status:     http.StatusText(http.StatusOK),
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewReader(archive)),
					}, nil
				default:
					t.Fatalf("unexpected URL %q", req.URL.String())
					return nil, nil
				}
			}),
			LatestURL: "https://example.test/releases/latest",
			GOOS:      "darwin",
			GOARCH:    "arm64",
			ExecutablePath: func() (string, error) {
				return filepath.Join(t.TempDir(), "csgclaw"), nil
			},
		}
	}
	t.Cleanup(func() { newUpgradeClient = originalClientFactory })

	err := NewCmd().Run(context.Background(), &command.Context{
		Program: "csgclaw",
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}, nil, command.GlobalOptions{Output: "table"})
	if err == nil {
		t.Fatal("Run() error = nil, want bundle install guidance")
	}
	for _, want := range []string{
		"not installed from an official csgclaw bundle",
		"curl -fsSL https://csgclaw.opencsg.com/install.sh | bash",
		"replaces only ~/.local/bin/csgclaw with a symlink",
		"csgclaw upgrade --check",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err.Error(), want)
		}
	}
}

func TestRunRestartErrorExplainsManualRecovery(t *testing.T) {
	originalVersion := appversion.Version
	appversion.Version = "v0.2.5"
	t.Cleanup(func() { appversion.Version = originalVersion })

	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	pidPath := filepath.Join(configHome, ".csgclaw", "server.pid")
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(pidPath), err)
	}
	if err := os.WriteFile(pidPath, []byte("not-a-pid\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", pidPath, err)
	}

	installRoot := writeInstalledBundle(t, t.TempDir(), "old")
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n# new\n",
		"csgclaw/bin/boxlite": "#!/bin/sh\n# new boxlite\n",
		"csgclaw/README.md":   "new",
	})
	sum := sha256.Sum256(archive)

	originalClientFactory := newUpgradeClient
	newUpgradeClient = func(run *command.Context) internalupgrade.Client {
		return internalupgrade.Client{
			HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.String() {
				case "https://example.test/releases/latest":
					return jsonResponse(http.StatusOK, `{
						"name":"v0.2.7",
						"assets":[{"name":"csgclaw_v0.2.7_darwin_arm64.tar.gz","browser_download_url":"https://downloads.example.test/csgclaw.tar.gz","size":`+strconv.Itoa(len(archive))+`,"sha256":"`+hex.EncodeToString(sum[:])+`"}]
					}`), nil
				case "https://downloads.example.test/csgclaw.tar.gz":
					return &http.Response{
						StatusCode: http.StatusOK,
						Status:     http.StatusText(http.StatusOK),
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewReader(archive)),
					}, nil
				default:
					t.Fatalf("unexpected URL %q", req.URL.String())
					return nil, nil
				}
			}),
			LatestURL: "https://example.test/releases/latest",
			GOOS:      "darwin",
			GOARCH:    "arm64",
			ExecutablePath: func() (string, error) {
				return filepath.Join(installRoot, "bin", "csgclaw"), nil
			},
		}
	}
	t.Cleanup(func() { newUpgradeClient = originalClientFactory })

	err := NewCmd().Run(context.Background(), &command.Context{
		Program: "csgclaw",
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}, nil, command.GlobalOptions{Output: "table"})
	if err == nil {
		t.Fatal("Run() error = nil, want manual restart guidance")
	}
	for _, want := range []string{
		"parse pid file",
		"csgclaw upgrade --no-restart",
		"csgclaw stop",
		"csgclaw serve --daemon",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err.Error(), want)
		}
	}
}

func restartScript(logPath string) string {
	return "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + logPath + "\n"
}

func logPathLiteral(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func releaseTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()

	files = withOfficialBundleMarker(files)
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%q) error = %v", name, err)
		}
		if _, err := io.WriteString(tw, content); err != nil {
			t.Fatalf("WriteString(%q) error = %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close error = %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("gzip close error = %v", err)
	}
	return buf.Bytes()
}

func withOfficialBundleMarker(files map[string]string) map[string]string {
	out := make(map[string]string, len(files)+1)
	for name, content := range files {
		out[name] = content
	}
	if _, ok := out["csgclaw/.csgclaw-bundle.json"]; !ok {
		out["csgclaw/.csgclaw-bundle.json"] = `{"app":"csgclaw","layout":"official-bundle","version":"test"}`
	}
	return out
}

func writeInstalledBundle(t *testing.T, parentDir, marker string) string {
	t.Helper()

	root := filepath.Join(parentDir, "csgclaw")
	for path, content := range map[string]string{
		filepath.Join(root, ".csgclaw-bundle.json"): `{"app":"csgclaw","layout":"official-bundle","version":"test"}`,
		filepath.Join(root, "bin", "csgclaw"):       "#!/bin/sh\n# " + marker + "\n",
		filepath.Join(root, "bin", "boxlite"):       "#!/bin/sh\n# " + marker + " boxlite\n",
		filepath.Join(root, "README.md"):            marker,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}
	return root
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("file %q = %q, want %q", path, string(got), want)
	}
}
