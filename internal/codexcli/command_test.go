package codexcli

import (
	"context"
	"reflect"
	"testing"
)

func TestAppServerCommandContextUsesBundledNativeBinary(t *testing.T) {
	binaryPath := "/opt/csgclaw/bin/codex"
	cmd, err := AppServerCommandContext(context.Background(), binaryPath)
	if err != nil {
		t.Fatalf("AppServerCommandContext() error = %v", err)
	}
	if got, want := cmd.Path, binaryPath; got != want {
		t.Fatalf("command path = %q, want %q", got, want)
	}
	if got, want := cmd.Args, []string{binaryPath, "app-server", "--disable", "plugins", "--listen", "stdio://"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("command args = %q, want %q", got, want)
	}
}

func TestAppServerCommandContextWithOverrides(t *testing.T) {
	binaryPath := "/opt/csgclaw/bin/codex"
	cmd, err := AppServerCommandContextWithOverrides(context.Background(), binaryPath, []string{"--disable", "shell_tool"})
	if err != nil {
		t.Fatalf("AppServerCommandContextWithOverrides() error = %v", err)
	}
	want := []string{binaryPath, "app-server", "--disable", "plugins", "--disable", "shell_tool", "--listen", "stdio://"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("command args = %q, want %q", cmd.Args, want)
	}
}
