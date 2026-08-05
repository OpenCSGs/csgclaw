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
	if got, want := cmd.Args, []string{binaryPath, "app-server", "--listen", "stdio://"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("command args = %q, want %q", got, want)
	}
}
