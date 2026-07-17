package completion

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompleteFullTopLevel(t *testing.T) {
	got := Complete(FullSpec(), "csgclaw", []string{"csgclaw", ""})

	assertContainsAll(t, got, "serve", "upgrade", "agent", "template", "skill", "model", "participant", "pt", "task", "completion", "--endpoint", "--config", "-V")
	assertContainsNone(t, got, "bot", "channel", "_serve", "__complete")
}

func TestCompleteLiteTopLevel(t *testing.T) {
	got := Complete(LiteSpec(), "csgclaw-cli", []string{"csgclaw-cli", ""})

	assertContainsAll(t, got, "participant", "pt", "room", "member", "message", "task", "completion", "--endpoint", "-V")
	assertContainsNone(t, got, "bot", "channel", "serve", "agent", "model", "user", "_serve", "__complete")
}

func TestCompleteSubcommandsAndFlags(t *testing.T) {
	got := Complete(FullSpec(), "csgclaw", []string{"csgclaw", "agent", ""})
	assertContainsAll(t, got, "list", "create", "start", "stop", "delete", "logs", "--help")

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "serve", "--"})
	assertContainsAll(t, got, "--daemon", "--no-auth-detect", "--no-browser", "--no-codex-auto-install", "--log-level", "--log", "--pid")

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "upgrade", "--"})
	assertContainsAll(t, got, "--check", "--no-restart")

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "agent", "create", "--"})
	assertContainsAll(t, got, "--replace", "--force", "--id", "--name", "--description", "--image", "--profile")

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "template", ""})
	assertContainsAll(t, got, "list", "get", "publish", "--help")

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "template", "publish", "--"})
	assertContainsAll(t, got, "--agent", "--registry")

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "team", "task", ""})
	assertContainsAll(t, got, "list", "create-batch", "plan", "start", "assign", "claim", "claim-next", "update", "--help")

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "team", "task", "plan", "--"})
	assertContainsAll(t, got, "--team", "--task", "--start")
	assertContainsNone(t, got, "--actor-id")

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "team", "task", "start", "--"})
	assertContainsAll(t, got, "--team", "--task")
	assertContainsNone(t, got, "--actor-id")

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "task", ""})
	assertContainsAll(t, got, "list", "create", "claim", "update", "--help")

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "task", "create", "--"})
	assertContainsAll(t, got, "--agent-id", "--title", "--body", "--created-by")

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "task", "update", "--"})
	assertContainsAll(t, got, "--task", "--actor-id", "--status", "--result", "--error", "--reason")

	got = Complete(LiteSpec(), "csgclaw-cli", []string{"csgclaw-cli", "participant", ""})
	assertContainsAll(t, got, "list", "create", "delete", "bind")

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "pt", ""})
	assertContainsAll(t, got, "list", "create", "delete", "bind")

	got = Complete(LiteSpec(), "csgclaw-cli", []string{"csgclaw-cli", "participant", "create", "--"})
	assertContainsAll(t, got, "--channel", "--name", "--type", "--bind", "--agent-id")

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "bot", ""})
	assertContainsNone(t, got, "list", "create", "delete", "config")

	got = Complete(LiteSpec(), "csgclaw-cli", []string{"csgclaw-cli", "pt", "bind", "--"})
	assertContainsAll(t, got, "--channel", "--feishu-kind", "--admin", "--open-id", "--agent", "--app-id", "--app-secret-file", "--app-secret-env", "--app-secret-stdin", "--restart")
}

func TestCompleteFlagValues(t *testing.T) {
	got := Complete(FullSpec(), "csgclaw", []string{"csgclaw", "participant", "list", "--channel", ""})
	assertEqual(t, got, []string{"csgclaw", "feishu"})

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "participant", "list", "--channel=f"})
	assertEqual(t, got, []string{"--channel=feishu"})

	got = Complete(FullSpec(), "csgclaw", []string{"csgclaw", "model", "auth", "login", "c"})
	assertEqual(t, got, []string{"codex", "claude-code"})
}

func TestGenerateScripts(t *testing.T) {
	tests := []struct {
		name    string
		program string
		shell   string
		want    []string
	}{
		{
			name:    "bash",
			program: "csgclaw",
			shell:   "bash",
			want:    []string{`"${COMP_WORDS[0]}" __complete`, "complete -F _csgclaw_completion csgclaw"},
		},
		{
			name:    "zsh",
			program: "csgclaw-cli",
			shell:   "zsh",
			want:    []string{"#compdef csgclaw-cli", "compdef _csgclaw_cli_completion csgclaw-cli"},
		},
		{
			name:    "fish",
			program: "csgclaw-cli",
			shell:   "fish",
			want:    []string{"command csgclaw-cli __complete", "complete -c csgclaw-cli"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := Generate(&out, tt.program, tt.shell); err != nil {
				t.Fatalf("Generate() error = %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("script = %q, want substring %q", out.String(), want)
				}
			}
		})
	}
}

func assertContainsAll(t *testing.T, got []string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		found := false
		for _, item := range got {
			if item == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("suggestions = %#v, want %q", got, want)
		}
	}
}

func assertContainsNone(t *testing.T, got []string, notWants ...string) {
	t.Helper()
	for _, notWant := range notWants {
		for _, item := range got {
			if item == notWant {
				t.Fatalf("suggestions = %#v, should not include %q", got, notWant)
			}
		}
	}
}

func assertEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("suggestions = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("suggestions = %#v, want %#v", got, want)
		}
	}
}
