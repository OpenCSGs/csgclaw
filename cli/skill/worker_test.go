package skill

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/cli/command"
)

func TestWorkerRegistryInstallReturnsCanonicalNameWithoutWorkerToken(t *testing.T) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/skills/alias":
			fmt.Fprint(w, `{"skill":{"slug":"canonical-demo"},"latestVersion":{"version":"1.0.0"}}`)
		case "/api/v1/download/canonical-demo", "/api/v1/download":
			_, _ = w.Write(buf.Bytes())
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
	globals := command.GlobalOptions{Endpoint: srv.URL, Output: "json", Config: config}
	var output bytes.Buffer
	run := &command.Context{Program: "csgclaw-cli", Stdout: &output, Stderr: io.Discard, HTTPClient: srv.Client()}
	for _, extra := range [][]string{nil, {"--force"}, {"--skills-dir", t.TempDir()}} {
		output.Reset()
		if err := (cmd{}).runInstall(context.Background(), run, append([]string{"alias", "--registry", "opencsg"}, extra...), globals); err != nil {
			t.Fatal(err)
		}
		var installed struct{ Slug, SkillsDir string }
		if err := json.Unmarshal(output.Bytes(), &installed); err != nil || installed.Slug != name {
			t.Fatalf("install result = %s, error = %v", output.Bytes(), err)
		}
		if _, err := os.Stat(filepath.Join(installed.SkillsDir, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
}
