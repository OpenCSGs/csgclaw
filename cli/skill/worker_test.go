package skill

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/cli/command"
)

func TestWorkerRegistryInstallEnablesCanonicalNameOnlyInOwnRoot(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, err := z.Create("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "# Demo"); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	const name = "canonical-demo"
	var enables int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/skills/alias":
			fmt.Fprint(w, `{"skill":{"slug":"canonical-demo"},"latestVersion":{"version":"1.0.0"}}`)
		case "/api/v1/download/canonical-demo", "/api/v1/download":
			_, _ = w.Write(buf.Bytes())
		case "/api/v1/agents/agent-test/skills/" + name + "/ensure-enabled":
			if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test" {
				t.Errorf("bad enable request")
			}
			enables++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CSGCLAW_CALLER_AGENT_ID", "agent-test")
	t.Setenv("DSH_HOME", "")
	t.Setenv("CODEX_HOME", home)
	t.Setenv("CSGCLAW_SKILLS_DIR", "")
	config := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("[skill]\nbase_url = %q\n", srv.URL)), 0600); err != nil {
		t.Fatal(err)
	}
	globals := command.GlobalOptions{Endpoint: srv.URL, Token: "test", Config: config}
	run := &command.Context{Program: "csgclaw-cli", Stdout: io.Discard, Stderr: io.Discard, HTTPClient: srv.Client()}
	for _, extra := range [][]string{nil, {"--force"}, {"--skills-dir", t.TempDir()}} {
		if err := (cmd{}).runInstall(context.Background(), run, append([]string{"alias", "--registry", "opencsg"}, extra...), globals); err != nil {
			t.Fatal(err)
		}
	}
	if enables != 2 {
		t.Fatalf("enable calls = %d, want 2", enables)
	}
}
