package mcpschema

import (
	"path/filepath"
	"testing"
)

func TestResolveWorkspaceArg(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	for _, placeholder := range []string{"${workspace}", "${workspaceDir}", "{workspace}", "{workspaceDir}"} {
		if got := ResolveWorkspaceArg(placeholder, workspace); got != workspace {
			t.Fatalf("exact placeholder %s = %s", placeholder, got)
		}
		arg := placeholder + "/../boxlite/server.py"
		want := filepath.Join(filepath.Dir(workspace), "boxlite", "server.py")
		if got := ResolveWorkspaceArg(arg, workspace); got != want {
			t.Fatalf("relative placeholder %s = %s; want %s", placeholder, got, want)
		}
		if got := ResolveWorkspaceArg(arg, ""); got != arg {
			t.Fatalf("empty workspace changed %q", arg)
		}
	}
	for _, arg := range []string{"--root=${workspace}", "${workspace}-suffix", "/unchanged/path"} {
		if got := ResolveWorkspaceArg(arg, workspace); got != arg {
			t.Fatalf("unrecognized argument changed: %s", got)
		}
	}
}
