package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	skill "csgclaw/internal/skill/state"
)

func TestSkillEnablementNativeCodexE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY to run native Codex skill discovery")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	skillPath := filepath.Join(home, "skills", "reviewer", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("---\nname: reviewer\ndescription: Review local code changes.\n---\nReview the supplied code.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rt := New(Dependencies{})
	t.Cleanup(func() { _ = rt.Close() })
	for _, step := range []struct {
		name    string
		enabled bool
	}{{"enabled", true}, {"disabled", false}, {"reenabled", true}} {
		t.Run(step.name, func(t *testing.T) {
			if err := rt.writeSkillStates(home, map[string]skill.State{"reviewer": {Enabled: step.enabled}}); err != nil {
				t.Fatal(err)
			}
			assertNativeCodexSkillState(t, binary, home, step.enabled)
		})
	}
}

func assertNativeCodexSkillState(t *testing.T, binary, home string, enabled bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "app-server")
	cmd.Dir = home
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "CODEX_HOME=") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "CODEX_HOME="+home)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	client := newAppServerClient(stdin, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 4<<20)
		for scanner.Scan() {
			client.handleLine(scanner.Text())
		}
	}()
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		<-done
	}()
	if _, err := client.request(ctx, "initialize", map[string]any{"clientInfo": map[string]any{"name": "skill-enablement-test", "version": "1"}}); err != nil {
		t.Fatal(err)
	}
	client.notify("initialized")
	raw, err := client.request(ctx, "skills/list", map[string]any{"cwds": []string{home}, "forceReload": true})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data []struct {
			Skills []struct {
				Path    string `json:"path"`
				Enabled bool   `json:"enabled"`
			} `json:"skills"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	wantPath, err := filepath.EvalSymlinks(filepath.Join(home, "skills", "reviewer", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range result.Data {
		for _, item := range entry.Skills {
			if item.Path == wantPath {
				if item.Enabled != enabled {
					t.Fatalf("native skill enabled = %v, want %v", item.Enabled, enabled)
				}
				return
			}
		}
	}
	t.Fatal("native Codex skill list is missing reviewer")
}
