package api

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	"csgclaw/internal/codexcli"
	"csgclaw/internal/runtimecatalog"
)

func TestAgentRuntimesListUsesCodexPathEnvironment(t *testing.T) {
	target := filepath.Join(t.TempDir(), "missing-codex")
	t.Setenv(codexcli.EnvBinaryPath, target)
	handler := &Handler{}
	handler.SetAgentRuntimeService(runtimecatalog.NewService())

	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/agent-runtimes", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var runtimes []runtimecatalog.Runtime
	if err := json.NewDecoder(recorder.Body).Decode(&runtimes); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(runtimes) != 2 {
		t.Fatalf("runtimes = %+v, want Codex and Claude Code", runtimes)
	}
	if got := runtimes[0]; got.Name != runtimecatalog.RuntimeCodex || got.Installed || !got.Installable || got.Status != string(codexcli.InstallStateNotInstalled) {
		t.Fatalf("Codex runtime = %+v, want not installed and installable", got)
	}
	if got := runtimes[1]; got.Name != runtimecatalog.RuntimeClaudeCode || got.Installable || got.Status != runtimecatalog.StatusComingSoon {
		t.Fatalf("Claude Code runtime = %+v, want coming soon", got)
	}
}

func TestAgentRuntimeInstallE2ERetriesInterruptedWindowsDownload(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bin", "codex.exe")
	t.Setenv(codexcli.EnvBinaryPath, target)
	binaryPayload := apiTestPEBinary("amd64")
	var downloads atomic.Int32
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/windows/amd64" || r.URL.Query().Get("package") != "codex-cli" {
			t.Errorf("download URL = %s, want /windows/amd64?package=codex-cli", r.URL.String())
		}
		if downloads.Add(1) == 1 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("download response does not support connection hijacking")
			}
			connection, buffered, err := hijacker.Hijack()
			if err != nil {
				t.Errorf("Hijack() error = %v", err)
				return
			}
			defer connection.Close()
			_, _ = buffered.WriteString("HTTP/1.1 200 OK\r\nContent-Length: " + strconv.Itoa(len(binaryPayload)) + "\r\n\r\n")
			_, _ = buffered.Write(binaryPayload[:len(binaryPayload)/2])
			_ = buffered.Flush()
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(binaryPayload)
	}))
	defer downloadServer.Close()
	installer := codexcli.NewInstaller(codexcli.InstallerOptions{
		BaseURL: downloadServer.URL,
		GOOS:    "windows",
		GOARCH:  "amd64",
	})
	handler := &Handler{}
	handler.SetAgentRuntimeService(runtimecatalog.NewService(
		runtimecatalog.WithCodexInstaller(installer),
		runtimecatalog.WithPlatform("windows", "amd64"),
	))
	server := httptest.NewServer(handler.Routes())
	defer server.Close()

	response := agentRuntimeRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/agent-runtimes/codex/install")
	var installed runtimecatalog.Runtime
	decodeTestJSON(t, response, &installed)
	if !installed.Installed || installed.Path != target || installed.Status != string(codexcli.InstallStateInstalled) {
		t.Fatalf("installed runtime = %+v, want installed at %q", installed, target)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", target, err)
	}
	if !bytes.Equal(data, binaryPayload) {
		t.Fatalf("installed binary does not match the validated fixture")
	}
	if info, err := os.Stat(target); err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("installed binary mode = %v, %v; want executable", info, err)
	}

	response = agentRuntimeRequest(t, server.Client(), http.MethodGet, server.URL+"/api/v1/agent-runtimes")
	var afterInstall []runtimecatalog.Runtime
	decodeTestJSON(t, response, &afterInstall)
	if !afterInstall[0].Installed || afterInstall[0].Path != target {
		t.Fatalf("Codex after install = %+v, want persisted installed status", afterInstall[0])
	}
	if got := downloads.Load(); got != 2 {
		t.Fatalf("download requests = %d, want 2", got)
	}
}

func TestAgentRuntimeInstallRejectsClaudeCodeAndUnknownRuntime(t *testing.T) {
	handler := &Handler{}
	handler.SetAgentRuntimeService(runtimecatalog.NewService())

	for _, test := range []struct {
		path string
		want int
	}{
		{path: "/api/v1/agent-runtimes/claude_code/install", want: http.StatusConflict},
		{path: "/api/v1/agent-runtimes/unknown/install", want: http.StatusNotFound},
	} {
		recorder := httptest.NewRecorder()
		handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, test.path, nil))
		if recorder.Code != test.want {
			t.Fatalf("POST %s status = %d, want %d; body=%s", test.path, recorder.Code, test.want, recorder.Body.String())
		}
	}
}

func apiTestPEBinary(arch string) []byte {
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

func agentRuntimeRequest(t *testing.T, client *http.Client, method, url string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	return response
}

func decodeTestJSON(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("response status = %d, want 200; body=%s", response.StatusCode, readTestResponse(t, response))
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func readTestResponse(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	var body bytes.Buffer
	_, _ = body.ReadFrom(response.Body)
	return body.String()
}
