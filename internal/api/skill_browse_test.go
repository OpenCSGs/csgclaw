package api

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/auth"
)

func TestSkillBrowseRoutesUseRemoteRepositoryAndLogin(t *testing.T) {
	authenticated := false
	restore := stubAuthStatus(func(*http.Request) (auth.Status, error) {
		return auth.Status{Authenticated: authenticated, BaseURL: auth.StageCSGHubBaseURL, OpenCSGBaseURL: auth.StageOpenCSGBaseURL}, nil
	})
	defer restore()
	previous := remoteSkillsHubAccessToken
	remoteSkillsHubAccessToken = func() (string, error) { return "preview-token", nil }
	t.Cleanup(func() { remoteSkillsHubAccessToken = previous })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantToken := ""
		if authenticated {
			wantToken = "Bearer preview-token"
		}
		if r.Header.Get("Authorization") != wantToken {
			t.Error("wrong authentication for remote preview")
		}
		switch r.URL.Path {
		case "/api/v1/skills/team/demo/refs/dev/tree/":
			io.WriteString(w, `{"data":{"Files":[{"path":"SKILL.md","type":"file"}]}}`)
		case "/api/v1/skills/team/demo/blob/SKILL.md":
			if r.URL.Query().Get("ref") != "dev" {
				t.Error("wrong ref")
			}
			io.WriteString(w, `{"data":{"content":"`+base64.StdEncoding.EncodeToString([]byte("Remote instructions"))+`"}}`)
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	stubHubTransport(t, server.URL)
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte(`[server]
listen_addr = "127.0.0.1:18080"
[models]
default = "default.model"
[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "test"
models = ["model"]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	h := &Handler{}
	h.SetConfigPath(configPath)
	for _, signedIn := range []bool{false, true} {
		authenticated = signedIn
		for _, test := range []struct{ endpoint, want string }{
			{"/api/v1/skills/tree?remote_path=team%2Fdemo&ref=dev", `"name":"SKILL.md"`},
			{"/api/v1/skills/file?remote_path=team%2Fdemo&ref=dev&path=SKILL.md", "Remote instructions"},
		} {
			rec := httptest.NewRecorder()
			h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, test.endpoint, nil))
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), test.want) {
				t.Fatalf("GET %s = %d %s", test.endpoint, rec.Code, rec.Body.String())
			}
		}
	}
	for _, endpoint := range []string{
		"/api/v1/skills/tree?remote_path=..%2Fdemo",
		"/api/v1/skills/file?remote_path=team%2Fdemo&path=..%2FSKILL.md",
	} {
		rec := httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, endpoint, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("GET %s = %d, want 400", endpoint, rec.Code)
		}
	}
}
