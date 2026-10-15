package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestWindowsTerminalWorkspacePathRemainsOneCommand(t *testing.T) {
	// Windows Terminal splits each argv with this delimiter rule before CLI parsing,
	// then Commandline::AddArg decodes each escaped semicolon once.
	// https://github.com/microsoft/terminal/blob/44b440bf043522ec06014b2cff30d0f52ca30b68/src/cascadia/TerminalApp/AppCommandlineArgs.cpp
	delimiter := regexp.MustCompile(`^;|[^\\];`)
	for _, directory := range []string{
		`C:\workspace\ordinary`,
		`C:\workspace\docs;calc.exe`,
		`C:\workspace\docs;;calc.exe;`,
		`C:\workspace\;leading`,
		`C:\workspace\;;`,
		`C:\workspace\with spaces\中文;notepad.exe`,
		`\\server\share\docs;calc.exe`,
		`\\?\C:\workspace\docs;calc.exe`,
	} {
		t.Run(directory, func(t *testing.T) {
			command, args, err := workspaceOpenCommand("windows", directory, "terminal", true)
			if err != nil || command != "wt.exe" || len(args) != 2 || args[0] != "-d" {
				t.Fatalf("terminal command: %s %v %v", command, args, err)
			}
			for _, arg := range args {
				if delimiter.MatchString(arg) {
					t.Errorf("Windows Terminal would split argument into commands: %q", arg)
				}
			}
			if decoded := strings.ReplaceAll(args[1], `\;`, ";"); decoded != directory {
				t.Errorf("decoded starting directory %q, want %q", decoded, directory)
			}
		})
	}
	parent := filepath.Join(t.TempDir(), "docs;calc.exe")
	command, args, err := workspaceOpenCommand("windows", filepath.Join(parent, "report.md"), "terminal", false)
	if err != nil || command != "wt.exe" || len(args) != 2 || delimiter.MatchString(args[1]) || strings.ReplaceAll(args[1], `\;`, ";") != parent {
		t.Fatalf("file preview terminal command: %s %v %v", command, args, err)
	}
}

func TestWorkspaceOpenCredentialsIndependentOfNoAuth(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("native system opener is unavailable without a desktop display")
	}
	for _, noAuth := range []bool{true, false} {
		t.Run(fmt.Sprintf("NoAuth=%t", noAuth), func(t *testing.T) {
			h, room, agent, root, _ := referenceTestHandler(t)
			h.serverNoAuth = noAuth
			h.serverAccessToken = "test-server-token"
			h.desktopSessionToken = "test-desktop-token"
			if err := os.Mkdir(filepath.Join(root, "docs"), 0700); err != nil {
				t.Fatal(err)
			}
			var launches atomic.Int32
			h.workspaceOpener = func(context.Context, string, string, bool) error { launches.Add(1); return nil }
			server := httptest.NewServer(h.Routes())
			defer server.Close()
			endpoint := server.URL + "/api/v1/agents/" + agent + "/workspace/open?room_id=" + room
			for _, tc := range []struct {
				name, origin, authorization string
				status                      int
			}{
				{"invalid token and malicious origin", "https://evil.example", "Bearer invalid", 403},
				{"invalid token without origin", "", "Bearer invalid", 403},
				{"invalid token with same origin", server.URL, "Bearer invalid", 403},
				{"malformed authorization", "https://evil.example", "Basic invalid", 403},
				{"empty bearer", "https://evil.example", "Bearer ", 403},
				{"agent credential", server.URL, "Bearer agent.invalid", 403},
				{"anonymous cross origin", "https://evil.example", "", 403},
				{"anonymous without origin", "", "", 403},
				{"same origin browser", server.URL, "", 204},
				{"real server token", "", "Bearer test-server-token", 204},
				{"real desktop token", "", "Bearer test-desktop-token", 204},
			} {
				t.Run(tc.name, func(t *testing.T) {
					before := launches.Load()
					req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"path":"docs","target":"system"}`))
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Content-Type", "application/json")
					if tc.origin != "" {
						req.Header.Set("Origin", tc.origin)
					}
					if tc.authorization != "" {
						req.Header.Set("Authorization", tc.authorization)
					}
					response, err := server.Client().Do(req)
					if err != nil {
						t.Fatal(err)
					}
					body, _ := io.ReadAll(response.Body)
					response.Body.Close()
					if response.StatusCode != tc.status {
						t.Errorf("HTTP status %d, want %d: %s", response.StatusCode, tc.status, body)
					}
					wantLaunches := before
					if tc.status == http.StatusNoContent {
						wantLaunches++
					}
					if launches.Load() != wantLaunches {
						t.Errorf("opener calls %d, want %d", launches.Load(), wantLaunches)
					}
					// Both metadata sources must reject an explicit invalid credential
					// even when NoAuth permits anonymous same-origin browser actions.
					for _, suffix := range []string{"?depth=1&", "/location?"} {
						metadataURL := server.URL + "/api/v1/agents/" + agent + "/workspace" + suffix + url.Values{"room_id": {room}, "path": {"docs"}}.Encode()
						metadataRequest, err := http.NewRequest(http.MethodGet, metadataURL, nil)
						if err != nil {
							t.Fatal(err)
						}
						if tc.authorization != "" {
							metadataRequest.Header.Set("Authorization", tc.authorization)
						}
						metadata, err := server.Client().Do(metadataRequest)
						if err != nil {
							t.Fatal(err)
						}
						var location workspaceLocation
						err = json.NewDecoder(metadata.Body).Decode(&location)
						metadata.Body.Close()
						if metadata.StatusCode != http.StatusOK || err != nil {
							t.Fatalf("metadata: %d, %v", metadata.StatusCode, err)
						}
						invalid := tc.authorization != "" && tc.authorization != "Bearer test-server-token" && tc.authorization != "Bearer test-desktop-token"
						if invalid != (len(location.OpenTargets) == 0) {
							t.Errorf("metadata %s targets %v for authorization %q", suffix, location.OpenTargets, tc.authorization)
						}
					}
				})
			}
		})
	}
}

func TestWorkspaceOpenCredentialRequiresConfiguredToken(t *testing.T) {
	for _, noAuth := range []bool{false, true} {
		h := &Handler{serverNoAuth: noAuth}
		for _, header := range []string{"", "Bearer ", "Bearer invalid", "Basic invalid"} {
			if h.validWorkspaceOpenCredential(header) {
				t.Errorf("NoAuth=%t accepted unset token %q", noAuth, header)
			}
		}
		h.serverAccessToken = " server-secret "
		h.desktopSessionToken = " desktop-secret "
		for _, header := range []string{"Bearer server-secret", "Bearer desktop-secret"} {
			if !h.validWorkspaceOpenCredential(header) {
				t.Errorf("NoAuth=%t denied configured token", noAuth)
			}
		}
		for _, header := range []string{"Bearer invalid", "Basic server-secret", "bearer server-secret", "Bearer server-secret ", " Bearer server-secret", "Bearer "} {
			if h.validWorkspaceOpenCredential(header) {
				t.Errorf("NoAuth=%t accepted malformed token %q", noAuth, header)
			}
		}
	}
}

func TestWorkspaceLocationAndLocalOpening(t *testing.T) {
	h, room, agent, root, _ := referenceTestHandler(t)
	_ = os.Mkdir(filepath.Join(root, "docs"), 0700)
	_ = os.WriteFile(filepath.Join(root, "docs", "file name.md"), []byte("text"), 0600)
	_ = os.Symlink(t.TempDir(), filepath.Join(root, "escape"))
	prefix := "/api/v1/agents/" + agent + "/workspace"
	query := url.Values{"room_id": {room}, "path": {"docs/file name.md"}}
	request := func(method, path, body, peer, host, origin, auth string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.RemoteAddr = peer
		r.Host = host
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		h.Routes().ServeHTTP(w, r)
		return w
	}
	w := request("GET", prefix+"/location?"+query.Encode(), "", "127.0.0.1:1234", "127.0.0.1:18080", "", "")
	var location workspaceLocation
	if err := json.Unmarshal(w.Body.Bytes(), &location); err != nil || w.Code != 200 || location.AbsolutePath != filepath.Join(root, "docs", "file name.md") || location.Directory {
		t.Fatalf("location: %d %s %v", w.Code, w.Body.String(), err)
	}
	// Metadata remains displayable to remote clients but cannot advertise native actions.
	w = request("GET", prefix+"/location?"+query.Encode(), "", "192.0.2.1:1234", "example.org", "", "")
	if err := json.Unmarshal(w.Body.Bytes(), &location); err != nil || len(location.OpenTargets) != 0 {
		t.Fatalf("remote targets: %s", w.Body.String())
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("native GUI opener availability is platform-dependent")
	}
	var opened []string
	h.workspaceOpener = func(_ context.Context, path, target string, directory bool) error {
		opened = append(opened, path)
		if target != "system" {
			t.Fatalf("target %s", target)
		}
		return nil
	}
	body := `{"path":"docs/file name.md","target":"system"}`
	for _, tc := range []struct {
		name, peer, host, origin, auth, path, body string
		status                                     int
	}{
		{"local", "127.0.0.1:1", "127.0.0.1:18080", "http://127.0.0.1:18080", "", prefix + "/open?room_id=" + room, body, 204},
		{"cross-site", "127.0.0.1:1", "127.0.0.1:18080", "https://evil.example", "", prefix + "/open", body, 403},
		{"remote-forged-origin", "192.0.2.1:1", "127.0.0.1:18080", "http://127.0.0.1:18080", "", prefix + "/open", body, 403},
		{"dns-rebinding", "127.0.0.1:1", "evil.example", "http://evil.example", "", prefix + "/open", body, 403},
		{"no-origin", "127.0.0.1:1", "127.0.0.1:18080", "", "", prefix + "/open", body, 403},
		{"invalid-credential", "127.0.0.1:1", "127.0.0.1:18080", "http://127.0.0.1:18080", "Bearer invalid", prefix + "/open", body, 403},
		{"traversal", "127.0.0.1:1", "127.0.0.1:18080", "http://127.0.0.1:18080", "", prefix + "/open", `{"path":"../outside","target":"system"}`, 400},
		{"symlink", "127.0.0.1:1", "127.0.0.1:18080", "http://127.0.0.1:18080", "", prefix + "/open", `{"path":"escape","target":"system"}`, 400},
		{"arbitrary-command", "127.0.0.1:1", "127.0.0.1:18080", "http://127.0.0.1:18080", "", prefix + "/open", `{"path":"docs","target":"sh -c bad"}`, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(http.MethodPost, tc.path, tc.body, tc.peer, tc.host, tc.origin, tc.auth)
			if w.Code != tc.status {
				t.Fatalf("%d want %d: %s", w.Code, tc.status, w.Body.String())
			}
		})
	}
	if !reflect.DeepEqual(opened, []string{filepath.Join(root, "docs", "file name.md")}) {
		t.Fatalf("unexpected launches: %v", opened)
	}
}

func TestWorkspaceOpenCommandKeepsPathsAsArguments(t *testing.T) {
	path := "/workspace/a 'quoted'; $(touch unsafe).md"
	for _, tc := range []struct {
		os, target, command string
		args                []string
	}{
		{"darwin", "system", "open", []string{"-R", path}},
		{"darwin", "vscode", "open", []string{"-a", "Visual Studio Code", path}},
		{"darwin", "terminal", "open", []string{"-a", "Terminal", "/workspace"}},
		{"windows", "system", "explorer.exe", []string{"/select," + path}},
		{"linux", "system", "xdg-open", []string{"/workspace"}},
	} {
		command, args, err := workspaceOpenCommand(tc.os, path, tc.target, false)
		if err != nil || command != tc.command || !reflect.DeepEqual(args, tc.args) {
			t.Fatalf("%s/%s: %s %v %v", tc.os, tc.target, command, args, err)
		}
	}
	if _, _, err := workspaceOpenCommand("darwin", path, "arbitrary", false); err == nil {
		t.Fatal("accepted arbitrary application")
	}
}
