package codex

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestIsCodexAppServerCommand(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{name: "codex app server", args: []string{"/usr/local/bin/codex", "app-server", "--listen", "stdio://"}, want: true},
		{name: "versioned codex app server", args: []string{"/opt/codex/codex-x86_64-linux", "app-server"}, want: true},
		{name: "other codex command", args: []string{"codex", "exec"}, want: false},
		{name: "app server argument to other command", args: []string{"codex", "exec", "app-server"}, want: false},
		{name: "unrelated app server", args: []string{"node", "codex", "app-server"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCodexAppServerCommand(tc.args); got != tc.want {
				t.Fatalf("isCodexAppServerCommand(%q) = %t, want %t", tc.args, got, tc.want)
			}
		})
	}
}

func TestPathWithinRuntimeDir(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "agent-manager", ".codex")
	if pathWithinRuntimeDir("", filepath.Join(runtimeDir, "home")) {
		t.Fatal("empty runtime directory must not match")
	}
	if !pathWithinRuntimeDir(runtimeDir, filepath.Join(runtimeDir, "home")) {
		t.Fatal("runtime home should be within runtime directory")
	}
	if pathWithinRuntimeDir(runtimeDir, runtimeDir+"-other/home") {
		t.Fatal("sibling runtime must not match runtime directory")
	}
	if pathWithinRuntimeDir(runtimeDir, filepath.Join(runtimeDir, "..", "worker", ".codex", "home")) {
		t.Fatal("worker runtime must not match manager runtime directory")
	}
}

func TestSplitNullTerminated(t *testing.T) {
	got := splitNullTerminated([]byte("codex\x00app-server\x00\x00"))
	want := []string{"codex", "app-server"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitNullTerminated() = %q, want %q", got, want)
	}
}
