package retiredclaws

import (
	"context"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/sandboxtest"
	"os"
	"path/filepath"
	"testing"
)

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}
func writeFile(t *testing.T, path, data string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func migrateAgentsWithMissingSandbox(t *testing.T, root string) error {
	t.Helper()
	return MigrateAgents(context.Background(), root, func(context.Context, string) (sandbox.Runtime, error) { return sandboxtest.NewRuntime(), nil })
}
