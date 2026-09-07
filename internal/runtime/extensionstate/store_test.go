package extensionstate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/runtime"
)

func stage(t *testing.T, store *Store, name, value string) *Change {
	t.Helper()
	change, err := store.Stage(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(change.Directory(), "data"), []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	change.SetProjection(runtime.ExtensionProjection{Name: name, Kind: "test", Generation: 1, SourceRevision: value, Environment: map[string]string{"TOOL": value}, Instructions: value})
	return change
}

func TestActivationRollbackPreservesPreviousGeneration(t *testing.T) {
	ctx := context.Background()
	store, _ := New(t.TempDir())
	first := stage(t, store, "tool", "old")
	if err := first.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := first.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	next := stage(t, store, "tool", "new")
	before, _, _ := store.Load("tool")
	if before.SourceRevision != "old" {
		t.Fatal("staging changed active state")
	}
	if err := next.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	if current, _, _ := store.Load("tool"); current.SourceRevision != "new" {
		t.Fatal("activation did not switch")
	}
	if err := next.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := next.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	current, found, err := store.Load("tool")
	if err != nil || !found || current.SourceRevision != "old" {
		t.Fatalf("rollback=(%+v,%v,%v)", current, found, err)
	}
	if data, err := os.ReadFile(filepath.Join(first.Directory(), "data")); err != nil || string(data) != "old" {
		t.Fatalf("previous data=%q %v", data, err)
	}
	if _, err := os.Stat(next.Directory()); !os.IsNotExist(err) {
		t.Fatalf("staging remains: %v", err)
	}
}

func TestDeleteDoesNotCleanAnotherNamesTombstone(t *testing.T) {
	ctx := context.Background()
	store, _ := New(t.TempDir())
	for _, name := range []string{"a", "a-b"} {
		item := stage(t, store, name, name)
		if err := item.Activate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := store.Delete("a")
	b, _ := store.Delete("a-b")
	if err := a.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.Rollback(ctx); err != nil {
		t.Fatalf("other extension tombstone was deleted: %v", err)
	}
	if _, found, err := store.Load("a-b"); err != nil || !found {
		t.Fatalf("other extension missing: %v", err)
	}
}

func TestMetadataRevisionUsesEffectiveProjectionDigest(t *testing.T) {
	ctx := context.Background()
	store, _ := New(t.TempDir())
	first := stage(t, store, "tool", "same")
	if err := first.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	item := first.Projection()
	digest := item.Digest
	item.Generation++
	next, err := store.Revise(item)
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := next.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	current, _, err := store.Load("tool")
	if err != nil || current.Generation != 2 || current.Digest != digest {
		t.Fatalf("projection=%+v error=%v", current, err)
	}
	copy := next.Projection()
	copy.Environment["TOOL"] = "changed"
	if next.Projection().Environment["TOOL"] != "same" {
		t.Fatal("projection aliases mutable state")
	}
}

func TestNamesAndSymlinksCannotEscapePrivateRoot(t *testing.T) {
	root := t.TempDir()
	store, _ := New(root)
	for _, name := range []string{"../other", ".", "..", "a/b", "A", ""} {
		if _, err := store.Stage(name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "tool")); err != nil {
		t.Skip(err)
	}
	if _, err := store.Stage("tool"); err == nil {
		t.Fatal("followed a symlink outside the managed root")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("wrote outside managed root")
	}
}
