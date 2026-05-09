package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestClientCheckUpdateAvailable(t *testing.T) {
	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want GET", req.Method)
			}
			if req.URL.String() != "https://example.test/releases/latest" {
				t.Fatalf("url = %q, want latest endpoint", req.URL.String())
			}
			return jsonResponse(http.StatusOK, `{
				"name":"v0.2.7",
				"assets":[
					{"name":"csgclaw-cli_v0.2.7_darwin_arm64.tar.gz"},
					{"name":"csgclaw_v0.2.7_darwin_arm64.tar.gz","browser_download_url":"http://csgclaw.opencsg.com/releases/v0.2.7/csgclaw_v0.2.7_darwin_arm64.tar.gz","size":123,"sha256":"abc"}
				]
			}`), nil
		}),
		LatestURL: "https://example.test/releases/latest",
		GOOS:      "darwin",
		GOARCH:    "arm64",
	}

	result, err := client.Check(context.Background(), "v0.2.5")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !result.UpdateAvailable {
		t.Fatal("Check().UpdateAvailable = false, want true")
	}
	if got, want := result.CurrentVersion, "v0.2.5"; got != want {
		t.Fatalf("CurrentVersion = %q, want %q", got, want)
	}
	if got, want := result.LatestVersion, "v0.2.7"; got != want {
		t.Fatalf("LatestVersion = %q, want %q", got, want)
	}
	if result.Asset == nil {
		t.Fatal("Asset = nil, want matched asset")
	}
	if got, want := result.Asset.Name, "csgclaw_v0.2.7_darwin_arm64.tar.gz"; got != want {
		t.Fatalf("Asset.Name = %q, want %q", got, want)
	}
	if got, want := result.Asset.DownloadURL, "http://csgclaw.opencsg.com/releases/v0.2.7/csgclaw_v0.2.7_darwin_arm64.tar.gz"; got != want {
		t.Fatalf("Asset.DownloadURL = %q, want %q", got, want)
	}
}

func TestClientCheckRequiresNameField(t *testing.T) {
	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"version":"v0.2.7","assets":[]}`), nil
		}),
		LatestURL: "https://example.test/releases/latest",
	}

	_, err := client.Check(context.Background(), "v0.2.7")
	if err == nil || !strings.Contains(err.Error(), `latest version "" is not a valid semver release`) {
		t.Fatalf("Check() error = %v, want missing name validation error", err)
	}
}
func TestClientCheckNoUpdate(t *testing.T) {
	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"name":"v0.2.7","assets":[]}`), nil
		}),
		LatestURL: "https://example.test/releases/latest",
		GOOS:      "darwin",
		GOARCH:    "arm64",
	}

	result, err := client.Check(context.Background(), "v0.2.7")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.UpdateAvailable {
		t.Fatal("Check().UpdateAvailable = true, want false")
	}
	if result.Asset != nil {
		t.Fatalf("Asset = %#v, want nil", result.Asset)
	}
}

func TestClientCheckRejectsInvalidCurrentVersion(t *testing.T) {
	client := Client{LatestURL: "https://example.test/releases/latest"}
	_, err := client.Check(context.Background(), "dev")
	if err == nil || !strings.Contains(err.Error(), "not a valid semver release") {
		t.Fatalf("Check() error = %v, want semver validation error", err)
	}
}

func TestClientCheckErrorsWhenAssetMissing(t *testing.T) {
	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"name":"v0.2.7","assets":[{"name":"csgclaw_v0.2.7_linux_amd64.tar.gz"}]}`), nil
		}),
		LatestURL: "https://example.test/releases/latest",
		GOOS:      "darwin",
		GOARCH:    "arm64",
	}

	_, err := client.Check(context.Background(), "v0.2.5")
	if err == nil || !strings.Contains(err.Error(), "no release asset for darwin/arm64") {
		t.Fatalf("Check() error = %v, want asset error", err)
	}
}

func TestClientPrepareReleaseDownloadsVerifiesAndExtracts(t *testing.T) {
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n",
		"csgclaw/bin/boxlite": "#!/bin/sh\n",
		"csgclaw/README.md":   "bundle\n",
	})
	sum := sha256.Sum256(archive)

	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %q, want GET", req.Method)
			}
			if req.URL.String() != "https://downloads.example.test/csgclaw.tar.gz" {
				t.Fatalf("url = %q, want asset download URL", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		}),
	}

	prepared, err := client.PrepareRelease(context.Background(), ReleaseAsset{
		Name:        "csgclaw_v0.2.7_darwin_arm64.tar.gz",
		DownloadURL: "https://downloads.example.test/csgclaw.tar.gz",
		Size:        int64(len(archive)),
		SHA256:      hex.EncodeToString(sum[:]),
	}, t.TempDir())
	if err != nil {
		t.Fatalf("PrepareRelease() error = %v", err)
	}

	if prepared.ArchivePath == "" || prepared.BundleDir == "" || prepared.WorkDir == "" {
		t.Fatalf("PrepareRelease() = %#v, want populated paths", prepared)
	}
	if _, err := os.Stat(prepared.ArchivePath); err != nil {
		t.Fatalf("Stat(%q) error = %v", prepared.ArchivePath, err)
	}
	for _, path := range []string{
		filepath.Join(prepared.BundleDir, "bin", "csgclaw"),
		filepath.Join(prepared.BundleDir, "bin", "boxlite"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Stat(%q) error = %v", path, err)
		}
	}
}

func TestClientPrepareReleaseRejectsSizeMismatch(t *testing.T) {
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n",
		"csgclaw/bin/boxlite": "#!/bin/sh\n",
	})
	sum := sha256.Sum256(archive)

	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		}),
	}

	_, err := client.PrepareRelease(context.Background(), ReleaseAsset{
		Name:        "csgclaw_v0.2.7_darwin_arm64.tar.gz",
		DownloadURL: "https://downloads.example.test/csgclaw.tar.gz",
		Size:        int64(len(archive) - 1),
		SHA256:      hex.EncodeToString(sum[:]),
	}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "downloaded size mismatch") {
		t.Fatalf("PrepareRelease() error = %v, want size mismatch", err)
	}
}

func TestClientPrepareReleaseAllowsMissingSHA256MetadataTemporarily(t *testing.T) {
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n",
		"csgclaw/bin/boxlite": "#!/bin/sh\n",
	})

	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		}),
	}

	prepared, err := client.PrepareRelease(context.Background(), ReleaseAsset{
		Name:        "csgclaw_v0.2.7_darwin_arm64.tar.gz",
		DownloadURL: "https://downloads.example.test/csgclaw.tar.gz",
		Size:        int64(len(archive)),
	}, t.TempDir())
	if err != nil {
		t.Fatalf("PrepareRelease() error = %v, want temporary success without sha256 metadata", err)
	}
	if prepared.BundleDir == "" {
		t.Fatalf("PrepareRelease() = %#v, want extracted bundle", prepared)
	}
}

func TestClientPrepareReleaseRejectsInvalidBundle(t *testing.T) {
	archive := releaseTarball(t, map[string]string{
		"csgclaw/bin/csgclaw": "#!/bin/sh\n",
	})
	sum := sha256.Sum256(archive)

	client := Client{
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(archive)),
			}, nil
		}),
	}

	_, err := client.PrepareRelease(context.Background(), ReleaseAsset{
		Name:        "csgclaw_v0.2.7_darwin_arm64.tar.gz",
		DownloadURL: "https://downloads.example.test/csgclaw.tar.gz",
		Size:        int64(len(archive)),
		SHA256:      hex.EncodeToString(sum[:]),
	}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "release bundle is missing bin/boxlite") {
		t.Fatalf("PrepareRelease() error = %v, want bundle validation error", err)
	}
}

func TestClientInstallPreparedRejectsNonBundleInstall(t *testing.T) {
	client := Client{
		ExecutablePath: func() (string, error) {
			return filepath.Join(t.TempDir(), "csgclaw"), nil
		},
	}

	_, err := client.InstallPrepared(PreparedBundle{BundleDir: writeBundleDir(t, t.TempDir(), "new")})
	if err == nil || !strings.Contains(err.Error(), "not installed from an official csgclaw bundle") {
		t.Fatalf("InstallPrepared() error = %v, want non-bundle install error", err)
	}
}

func TestClientInstallPreparedReplacesBundleFromSymlinkedExecutable(t *testing.T) {
	installParent := t.TempDir()
	installRoot := writeBundleDir(t, installParent, "old")
	linkDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(linkDir) error = %v", err)
	}
	linkPath := filepath.Join(linkDir, "csgclaw")
	if err := os.Symlink(filepath.Join(installRoot, "bin", "csgclaw"), linkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	preparedRoot := writeBundleDir(t, t.TempDir(), "new")
	client := Client{
		ExecutablePath: func() (string, error) {
			return linkPath, nil
		},
	}

	installed, err := client.InstallPrepared(PreparedBundle{BundleDir: preparedRoot})
	if err != nil {
		t.Fatalf("InstallPrepared() error = %v", err)
	}
	gotRoot, err := filepath.EvalSymlinks(installed.InstallRoot)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", installed.InstallRoot, err)
	}
	wantRoot, err := filepath.EvalSymlinks(installRoot)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", installRoot, err)
	}
	if got, want := gotRoot, wantRoot; got != want {
		t.Fatalf("InstallRoot = %q, want %q", got, want)
	}
	assertFileContent(t, filepath.Join(installRoot, "README.md"), "new")
	assertFileContent(t, filepath.Join(installRoot, "bin", "boxlite"), "#!/bin/sh\n# new boxlite\n")
}

func TestClientInstallPreparedRollsBackOnRenameFailure(t *testing.T) {
	installParent := t.TempDir()
	installRoot := writeBundleDir(t, installParent, "old")
	preparedRoot := writeBundleDir(t, t.TempDir(), "new")

	originalRename := renamePath
	renameCount := 0
	renamePath = func(oldPath, newPath string) error {
		renameCount++
		if renameCount == 2 {
			return fmt.Errorf("boom")
		}
		return originalRename(oldPath, newPath)
	}
	t.Cleanup(func() { renamePath = originalRename })

	client := Client{
		ExecutablePath: func() (string, error) {
			return filepath.Join(installRoot, "bin", "csgclaw"), nil
		},
	}

	_, err := client.InstallPrepared(PreparedBundle{BundleDir: preparedRoot})
	if err == nil || !strings.Contains(err.Error(), "install new bundle") {
		t.Fatalf("InstallPrepared() error = %v, want install failure", err)
	}
	assertFileContent(t, filepath.Join(installRoot, "README.md"), "old")
	assertFileContent(t, filepath.Join(installRoot, "bin", "boxlite"), "#!/bin/sh\n# old boxlite\n")
}

func TestClientRestartIfRunningSkipsWhenPIDFileMissing(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)

	client := Client{}
	result, err := client.RestartIfRunning(context.Background(), InstalledBundle{InstallRoot: t.TempDir()}, RestartOptions{})
	if err != nil {
		t.Fatalf("RestartIfRunning() error = %v", err)
	}
	if result.DaemonWasRunning || result.Restarted {
		t.Fatalf("RestartIfRunning() = %#v, want no daemon and no restart", result)
	}
}

func TestClientRestartIfRunningRemovesStalePID(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	pidPath := filepath.Join(configHome, ".csgclaw", "server.pid")
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(pidPath), err)
	}
	if err := os.WriteFile(pidPath, []byte("424242\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", pidPath, err)
	}

	originalFindProcess := findProcessByPID
	findProcessByPID = func(int) (*os.Process, error) {
		return &os.Process{Pid: 424242}, nil
	}
	t.Cleanup(func() { findProcessByPID = originalFindProcess })

	originalExec := execCommandContext
	execCommandContext = func(context.Context, string, ...string) *exec.Cmd {
		t.Fatal("execCommandContext should not be called for stale pid")
		return nil
	}
	t.Cleanup(func() { execCommandContext = originalExec })

	result, err := Client{}.RestartIfRunning(context.Background(), InstalledBundle{InstallRoot: t.TempDir()}, RestartOptions{})
	if err != nil {
		t.Fatalf("RestartIfRunning() error = %v", err)
	}
	if result.DaemonWasRunning || result.Restarted {
		t.Fatalf("RestartIfRunning() = %#v, want no daemon and no restart", result)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("pid file still exists, stat err = %v", err)
	}
}

func TestClientRestartIfRunningStopsAndStartsDaemon(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	pidPath := filepath.Join(configHome, ".csgclaw", "server.pid")
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(pidPath), err)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", pidPath, err)
	}

	installRoot := t.TempDir()
	var calls [][]string
	originalExec := execCommandContext
	execCommandContext = func(_ context.Context, name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		return exec.Command("sh", "-c", "exit 0")
	}
	t.Cleanup(func() { execCommandContext = originalExec })

	result, err := Client{}.RestartIfRunning(context.Background(), InstalledBundle{InstallRoot: installRoot}, RestartOptions{
		ConfigPath: "/tmp/custom.toml",
	})
	if err != nil {
		t.Fatalf("RestartIfRunning() error = %v", err)
	}
	if !result.DaemonWasRunning || !result.Restarted {
		t.Fatalf("RestartIfRunning() = %#v, want restarted daemon", result)
	}
	want := [][]string{
		{filepath.Join(installRoot, "bin", "csgclaw"), "stop"},
		{filepath.Join(installRoot, "bin", "csgclaw"), "serve", "--daemon", "--config", "/tmp/custom.toml"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("exec calls = %#v, want %#v", calls, want)
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{a: "v0.2.5", b: "v0.2.7", want: -1},
		{a: "v0.2.7", b: "v0.2.7", want: 0},
		{a: "v0.3.0", b: "v0.2.9", want: 1},
		{a: "v0.2.7-rc.1", b: "v0.2.7", want: -1},
		{a: "v0.2.7", b: "v0.2.7-rc.1", want: 1},
	}

	for _, tt := range tests {
		if got := compareSemver(tt.a, tt.b); got != tt.want {
			t.Fatalf("compareSemver(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
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

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}
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

func writeBundleDir(t *testing.T, parentDir, marker string) string {
	t.Helper()

	root := filepath.Join(parentDir, "csgclaw")
	for path, content := range map[string]string{
		filepath.Join(root, "bin", "csgclaw"): "#!/bin/sh\n# " + marker + "\n",
		filepath.Join(root, "bin", "boxlite"): "#!/bin/sh\n# " + marker + " boxlite\n",
		filepath.Join(root, "README.md"):      marker,
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
