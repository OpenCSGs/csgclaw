package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallerDownloaderOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixtures use POSIX executable scripts")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to run the embedded installer")
	}
	const install = `/bin/mkdir -p .agents/skills/demo && echo '# demo' >.agents/skills/demo/SKILL.md`
	for _, tc := range []struct {
		name, skills, want string
		success            bool
	}{
		{"installed", install, "skills add org/repo --skill demo --agent codex --copy -y\nimport\n", true},
		{"missing", "", "npx --yes skills@1.9.0 add org/repo --skill demo --agent codex --copy -y\nimport\n", true},
		{"download failed", "exit 42", "skills add org/repo --skill demo --agent codex --copy -y\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "calls")
			write := func(name, body string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			if tc.skills != "" {
				write("skills", `echo "skills $*" >>"$CALL_LOG"`+"\n"+tc.skills)
			}
			write("npx", `echo "npx $*" >>"$CALL_LOG"`+"\n"+install)
			write("csgclaw-cli", `test -s "$5/SKILL.md" && echo import >>"$CALL_LOG"`)
			t.Setenv("PATH", dir)
			t.Setenv("CALL_LOG", log)
			t.Setenv("CSGCLAW_CLI", filepath.Join(dir, "csgclaw-cli"))
			t.Setenv("CSGCLAW_CALLER_AGENT_ID", "agent-test")
			t.Setenv("CODEX_HOME", dir)
			t.Setenv("CSGCLAW_DISABLE_PUBLIC_SKILLS", "")
			cmd := exec.Command(node, "embed/skill-installer/scripts/install-skills-sh.mjs", "org/repo", "--skill", "demo")
			output, err := cmd.CombinedOutput()
			if (err == nil) != tc.success {
				t.Fatalf("installer: %v: %s", err, output)
			}
			b, err := os.ReadFile(log)
			if err != nil || strings.TrimSpace(string(b)) != strings.TrimSpace(tc.want) {
				t.Fatalf("calls = %q, want %q (%v)", b, tc.want, err)
			}
		})
	}
}
