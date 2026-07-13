package codexcli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestResolveDownloadPlatform(t *testing.T) {
	tests := []struct {
		goos     string
		goarch   string
		wantOS   string
		wantArch string
		wantBin  string
		wantErr  bool
	}{
		{goos: "linux", goarch: "amd64", wantOS: "linux", wantArch: "amd64", wantBin: "codex-x86_64-unknown-linux-musl"},
		{goos: "linux", goarch: "arm64", wantOS: "linux", wantArch: "arm64", wantBin: "codex-aarch64-unknown-linux-musl"},
		{goos: "darwin", goarch: "amd64", wantOS: "macos", wantArch: "x64", wantBin: "codex-x86_64-apple-darwin"},
		{goos: "darwin", goarch: "arm64", wantOS: "macos", wantArch: "arm64", wantBin: "codex-aarch64-apple-darwin"},
		{goos: "windows", goarch: "amd64", wantOS: "windows", wantArch: "amd64"},
		{goos: "windows", goarch: "arm64", wantOS: "windows", wantArch: "arm64"},
		{goos: "freebsd", goarch: "amd64", wantErr: true},
		{goos: "linux", goarch: "386", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.goos+"_"+tt.goarch, func(t *testing.T) {
			got, err := resolveDownloadPlatform(tt.goos, tt.goarch)
			if tt.wantErr {
				if err == nil {
					t.Fatal("resolveDownloadPlatform() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveDownloadPlatform() error = %v", err)
			}
			if got.os != tt.wantOS || got.arch != tt.wantArch || got.archiveBinary != tt.wantBin {
				t.Fatalf("resolveDownloadPlatform() = %+v, want %s/%s binary %q", got, tt.wantOS, tt.wantArch, tt.wantBin)
			}
		})
	}
}

func TestInstallerEnsureSkipsDownloadWhenEnvBinaryExists(t *testing.T) {
	target := writeExecutable(t, filepath.Join(t.TempDir(), "codex-existing"), "existing")
	t.Setenv(EnvBinaryPath, target)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	status, err := NewInstaller(InstallerOptions{BaseURL: server.URL}).Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !status.Installed || status.Path != target {
		t.Fatalf("Ensure() status = %+v, want installed at %q", status, target)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("download requests = %d, want 0", got)
	}
}

func TestInstallerEnsureInstallsUnixArchiveAtEnvPath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "custom", "codex")
	t.Setenv(EnvBinaryPath, target)
	binaryPayload := testMachOBinary("arm64")
	payload := unixCodexArchive(t, []archiveEntry{{name: "codex-aarch64-apple-darwin", body: string(binaryPayload)}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/macos/arm64" || r.URL.Query().Get("package") != "codex-cli" {
			t.Errorf("request URL = %s, want /macos/arm64?package=codex-cli", r.URL.String())
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	installer := NewInstaller(InstallerOptions{BaseURL: server.URL, GOOS: "darwin", GOARCH: "arm64"})
	status, err := installer.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !status.Installed || status.Path != target {
		t.Fatalf("Ensure() status = %+v, want installed at %q", status, target)
	}
	assertFileContent(t, target, string(binaryPayload))
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", target, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("installed mode = %v, want executable", info.Mode())
	}
}

func TestInstallerEnsureInstallsWindowsExecutable(t *testing.T) {
	target := filepath.Join(t.TempDir(), "codex.exe")
	t.Setenv(EnvBinaryPath, target)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/windows/arm64" || r.URL.Query().Get("package") != "codex-cli" {
			t.Errorf("request URL = %s, want /windows/arm64?package=codex-cli", r.URL.String())
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testPEBinary("arm64"))
	}))
	defer server.Close()

	status, err := NewInstaller(InstallerOptions{BaseURL: server.URL, GOOS: "windows", GOARCH: "arm64"}).Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !status.Installed || status.Path != target {
		t.Fatalf("Ensure() status = %+v, want installed at %q", status, target)
	}
	assertFileContent(t, target, string(testPEBinary("arm64")))
}

func TestInstallerEnsureUsesExistingWindowsCommandShim(t *testing.T) {
	dir := t.TempDir()
	shimPath := writeExecutable(t, filepath.Join(dir, "npm", "codex.cmd"), "@echo off\n")
	managedPath := filepath.Join(dir, "managed", "codex.exe")
	t.Setenv(EnvBinaryPath, shimPath)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/windows/amd64" || r.URL.Query().Get("package") != "codex-cli" {
			t.Errorf("request URL = %s, want /windows/amd64?package=codex-cli", r.URL.String())
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testPEBinary("amd64"))
	}))
	defer server.Close()

	installer := NewInstaller(InstallerOptions{
		Locator: Locator{
			ManagedPath: managedPath,
			LookPath: func(string) (string, error) {
				return "", os.ErrNotExist
			},
		},
		BaseURL: server.URL,
		GOOS:    "windows",
		GOARCH:  "amd64",
	})
	status, err := installer.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !status.Installed || status.Path != shimPath {
		t.Fatalf("Ensure() status = %+v, want installed command shim at %q", status, shimPath)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("download requests = %d, want 0", got)
	}
	if _, statErr := os.Stat(managedPath); !os.IsNotExist(statErr) {
		t.Fatalf("Stat(%q) error = %v, want no managed executable", managedPath, statErr)
	}
	assertFileContent(t, shimPath, "@echo off\n")
}

func TestInstallerEnsureUsesManagedExecutableForMissingWindowsCommandShim(t *testing.T) {
	dir := t.TempDir()
	shimPath := filepath.Join(dir, "npm", "codex.cmd")
	managedPath := filepath.Join(dir, "managed", "codex.exe")
	t.Setenv(EnvBinaryPath, shimPath)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/windows/amd64" || r.URL.Query().Get("package") != "codex-cli" {
			t.Errorf("request URL = %s, want /windows/amd64?package=codex-cli", r.URL.String())
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testPEBinary("amd64"))
	}))
	defer server.Close()

	installer := NewInstaller(InstallerOptions{
		Locator: Locator{
			ManagedPath: managedPath,
			LookPath: func(string) (string, error) {
				return "", os.ErrNotExist
			},
		},
		BaseURL: server.URL,
		GOOS:    "windows",
		GOARCH:  "amd64",
	})
	status, err := installer.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !status.Installed || status.Path != managedPath {
		t.Fatalf("Ensure() status = %+v, want managed executable at %q", status, managedPath)
	}
	assertFileContent(t, managedPath, string(testPEBinary("amd64")))
}

func TestInstallerEnsureCoalescesConcurrentDownloads(t *testing.T) {
	target := filepath.Join(t.TempDir(), "codex")
	t.Setenv(EnvBinaryPath, target)
	payload := unixCodexArchive(t, []archiveEntry{{name: "codex-x86_64-unknown-linux-musl", body: string(testELFBinary("amd64"))}})
	started := make(chan struct{})
	release := make(chan struct{})
	var requests atomic.Int32
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		once.Do(func() { close(started) })
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	installer := NewInstaller(InstallerOptions{BaseURL: server.URL, GOOS: "linux", GOARCH: "amd64"})

	type result struct {
		status InstallStatus
		err    error
	}
	results := make(chan result, 2)
	go func() {
		status, err := installer.Ensure(context.Background())
		results <- result{status: status, err: err}
	}()
	<-started
	if status := installer.Status(); status.State != InstallStateInstalling || status.Installed {
		t.Fatalf("Status() during download = %+v, want installing", status)
	}
	go func() {
		status, err := installer.Ensure(context.Background())
		results <- result{status: status, err: err}
	}()
	close(release)
	for range 2 {
		got := <-results
		if got.err != nil || !got.status.Installed {
			t.Fatalf("Ensure() = %+v, %v; want installed", got.status, got.err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("download requests = %d, want 1", got)
	}
}

func TestInstallerEnsureAutomaticallyRetriesTemporaryDownloadFailure(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	target := filepath.Join(t.TempDir(), "codex")
	t.Setenv(EnvBinaryPath, target)
	payload := unixCodexArchive(t, []archiveEntry{{name: "codex-x86_64-unknown-linux-musl", body: string(testELFBinary("amd64"))}})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	installer := NewInstaller(InstallerOptions{BaseURL: server.URL, GOOS: "linux", GOARCH: "amd64"})

	status, err := installer.Ensure(context.Background())
	if err != nil || !status.Installed {
		t.Fatalf("Ensure() = %+v, %v; want installed after automatic retry", status, err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("download requests = %d, want 2", got)
	}
	for _, expected := range []string{
		"Codex CLI download started",
		"Codex CLI download attempt started",
		"Codex CLI download response received",
		"status=502",
		"Codex CLI download attempt failed; retrying",
		"retry_in=100ms",
		"Codex CLI download body completed",
		"Codex CLI download completed",
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("download logs missing %q:\n%s", expected, logs.String())
		}
	}
}

func TestInstallerEnsureCanRetryAfterAutomaticRetriesAreExhausted(t *testing.T) {
	target := filepath.Join(t.TempDir(), "codex")
	t.Setenv(EnvBinaryPath, target)
	payload := unixCodexArchive(t, []archiveEntry{{name: "codex-x86_64-unknown-linux-musl", body: string(testELFBinary("amd64"))}})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) <= defaultInstallAttempts {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	installer := NewInstaller(InstallerOptions{BaseURL: server.URL, GOOS: "linux", GOARCH: "amd64"})

	if status, err := installer.Ensure(context.Background()); err == nil || status.State != InstallStateFailed {
		t.Fatalf("first Ensure() = %+v, %v; want failed after automatic retries", status, err)
	}
	if status := installer.Status(); status.State != InstallStateFailed || status.Message == "" {
		t.Fatalf("Status() = %+v, want retryable failure", status)
	}
	status, err := installer.Ensure(context.Background())
	if err != nil || !status.Installed {
		t.Fatalf("second Ensure() = %+v, %v; want installed", status, err)
	}
	if got := requests.Load(); got != defaultInstallAttempts+1 {
		t.Fatalf("download requests = %d, want %d", got, defaultInstallAttempts+1)
	}
}

func TestInstallerDefaultHTTPClientNegotiatesHTTP2OnEveryPlatform(t *testing.T) {
	for _, test := range []struct {
		goos         string
		wantProtocol string
	}{
		{goos: "windows", wantProtocol: "HTTP/2.0"},
		{goos: "darwin", wantProtocol: "HTTP/2.0"},
		{goos: "linux", wantProtocol: "HTTP/2.0"},
	} {
		t.Run(test.goos, func(t *testing.T) {
			negotiatedProtocol := make(chan string, 1)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				negotiatedProtocol <- r.Proto
				w.WriteHeader(http.StatusOK)
			}))
			server.EnableHTTP2 = true
			server.StartTLS()
			defer server.Close()

			installer := NewInstaller(InstallerOptions{GOOS: test.goos})
			transport, ok := installer.httpClient.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("HTTP transport = %T, want *http.Transport", installer.httpClient.Transport)
			}
			serverTransport := server.Client().Transport.(*http.Transport)
			transport.TLSClientConfig = serverTransport.TLSClientConfig.Clone()
			response, err := installer.httpClient.Get(server.URL)
			if err != nil {
				t.Fatalf("GET error = %v", err)
			}
			_ = response.Body.Close()
			if got := <-negotiatedProtocol; got != test.wantProtocol {
				t.Fatalf("negotiated protocol = %q, want %s", got, test.wantProtocol)
			}
		})
	}
}

func TestDownloadLogURLRedactsCredentialsAndQuery(t *testing.T) {
	got := downloadLogURL("https://user:secret@example.test/codex-cli/latest/windows/amd64?package=codex-cli&token=secret#fragment")
	want := "https://example.test/codex-cli/latest/windows/amd64"
	if got != want {
		t.Fatalf("downloadLogURL() = %q, want %q", got, want)
	}
}

func TestInstallerEnsureRejectsInvalidPackageWithoutPartialBinary(t *testing.T) {
	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "codex")
	t.Setenv(EnvBinaryPath, target)
	payload := unixCodexArchive(t, []archiveEntry{{name: "README.md", body: "not a binary"}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	status, err := NewInstaller(InstallerOptions{BaseURL: server.URL, GOOS: "linux", GOARCH: "amd64"}).Ensure(context.Background())
	if err == nil || status.State != InstallStateFailed {
		t.Fatalf("Ensure() = %+v, %v; want failed", status, err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("Stat(%q) error = %v, want not exist", target, statErr)
	}
	matches, globErr := filepath.Glob(filepath.Join(targetDir, ".codex-install-*"))
	if globErr != nil {
		t.Fatalf("Glob() error = %v", globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary install files = %v, want none", matches)
	}
}

func TestInstallerEnsureRejectsWrongArchitectureWithoutPartialBinary(t *testing.T) {
	target := filepath.Join(t.TempDir(), "codex")
	t.Setenv(EnvBinaryPath, target)
	payload := unixCodexArchive(t, []archiveEntry{{
		name: "codex-x86_64-apple-darwin",
		body: string(testMachOBinary("arm64")),
	}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	status, err := NewInstaller(InstallerOptions{BaseURL: server.URL, GOOS: "darwin", GOARCH: "amd64"}).Ensure(context.Background())
	if err == nil || status.State != InstallStateFailed {
		t.Fatalf("Ensure() = %+v, %v; want architecture validation failure", status, err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("Stat(%q) error = %v, want not exist", target, statErr)
	}
}

func TestInstallerEnsureRejectsUnsupportedPlatformBeforeDownload(t *testing.T) {
	t.Setenv(EnvBinaryPath, filepath.Join(t.TempDir(), "codex"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	_, err := NewInstaller(InstallerOptions{BaseURL: server.URL, GOOS: "freebsd", GOARCH: "amd64"}).Ensure(context.Background())
	if err == nil || !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Ensure() error = %v, want ErrUnsupportedPlatform", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("download requests = %d, want 0", got)
	}
}

type archiveEntry struct {
	name     string
	body     string
	typeflag byte
}

func unixCodexArchive(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		header := &tar.Header{Name: entry.name, Mode: 0o755, Size: int64(len(entry.body)), Typeflag: typeflag}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader(%q) error = %v", entry.name, err)
		}
		if typeflag == tar.TypeReg || typeflag == tar.TypeRegA {
			if _, err := tarWriter.Write([]byte(entry.body)); err != nil {
				t.Fatalf("Write(%q) error = %v", entry.name, err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("tar Close() error = %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
	return compressed.Bytes()
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("ReadFile(%q) = %q, want %q", path, data, want)
	}
}

func testELFBinary(arch string) []byte {
	data := make([]byte, 64)
	copy(data[:4], []byte{0x7f, 'E', 'L', 'F'})
	data[4] = 2
	data[5] = 1
	machine := uint16(62)
	if arch == "arm64" {
		machine = 183
	}
	binary.LittleEndian.PutUint16(data[18:20], machine)
	return data
}

func testMachOBinary(arch string) []byte {
	data := make([]byte, 32)
	copy(data[:4], []byte{0xcf, 0xfa, 0xed, 0xfe})
	cpuType := uint32(0x01000007)
	if arch == "arm64" {
		cpuType = 0x0100000c
	}
	binary.LittleEndian.PutUint32(data[4:8], cpuType)
	return data
}

func testPEBinary(arch string) []byte {
	data := make([]byte, 128)
	copy(data[:2], "MZ")
	binary.LittleEndian.PutUint32(data[0x3c:0x40], 0x40)
	copy(data[0x40:0x44], "PE\x00\x00")
	machine := uint16(0x8664)
	if arch == "arm64" {
		machine = 0xaa64
	}
	binary.LittleEndian.PutUint16(data[0x44:0x46], machine)
	return data
}

func ExampleInstaller_downloadURL() {
	installer := NewInstaller(InstallerOptions{BaseURL: "https://example.test/codex-cli/latest"})
	value, _ := installer.downloadURL(downloadPlatform{os: "linux", arch: "amd64"})
	fmt.Println(value)
	// Output: https://example.test/codex-cli/latest/linux/amd64?package=codex-cli
}
