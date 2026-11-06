package relay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlatformClientIDSurvivesSandboxRecreation(t *testing.T) {
	t.Setenv("OPENCSG_SANDBOX_TYPE", "csgclaw")
	t.Setenv("CSGCLAW_NAME", "sandbox-one")
	first, err := StableClientID(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	recreated, err := StableClientID(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if first != recreated {
		t.Fatalf("client ID changed across empty state directories: %q vs %q", first, recreated)
	}
	t.Setenv("CSGCLAW_NAME", "sandbox-two")
	other, err := StableClientID(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if first == other {
		t.Fatal("different sandbox instances shared a client ID")
	}
}

func TestPlatformClientIDPreservesExistingIdentity(t *testing.T) {
	t.Setenv("OPENCSG_SANDBOX_TYPE", "csgclaw-workspace")
	t.Setenv("CSGCLAW_NAME", "sandbox-one")
	root := t.TempDir()
	const original = "e72848c2-88b8-46ac-b6ea-caf2978300d1"
	if err := os.WriteFile(filepath.Join(root, "client-id"), []byte(original+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := StableClientID(root)
	if err != nil {
		t.Fatal(err)
	}
	if id != original {
		t.Fatalf("existing client ID replaced: %q", id)
	}
}
