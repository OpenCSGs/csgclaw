package installstate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSameNameConflictRequiresFreshApprovalForSelectedOrigin(t *testing.T) {
	root := t.TempDir()
	name := "demo"
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "SKILL.md")
	stage := t.TempDir()
	newFile := filepath.Join(stage, "SKILL.md")
	if err := os.WriteFile(newFile, []byte("after"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}
	old := Record{Source: SourceLocal, SourceURL: "https://skills.sh/one/repo/demo"}
	next := Record{Source: SourceLocal, SourceURL: "https://skills.sh/two/repo/demo"}
	if err := Update(root, name, &old, nil); err != nil {
		t.Fatal(err)
	}
	count := 0
	apply := func() error { count++; return os.WriteFile(file, []byte("after"), 0644) }
	request := WorkerInstall{Replace: true}
	getConflict := func(next Record) *SourceConflict {
		t.Helper()
		err := request.Commit(root, name, stage, &next, apply)
		var conflict *SourceConflict
		if !errors.As(err, &conflict) || conflict.Installed != old.SourceURL || conflict.Requested != next.SourceURL || count != 0 {
			t.Fatalf("conflict=%v writes=%d", err, count)
		}
		return conflict
	}
	request.Confirmation = getConflict(next).Confirmation
	if err := os.WriteFile(file, []byte("new user edit"), 0644); err != nil {
		t.Fatal(err)
	}
	fresh := getConflict(next)
	request.Confirmation = fresh.Confirmation
	third := next
	third.SourceURL = "https://skills.sh/three/repo/demo"
	getConflict(third)
	if err := os.WriteFile(newFile, []byte("new download"), 0644); err != nil {
		t.Fatal(err)
	}
	request.Confirmation = getConflict(next).Confirmation
	if err := request.Commit(root, name, stage, &next, apply); err != nil || count != 1 {
		t.Fatalf("confirmed replacement: %v, writes=%d", err, count)
	}
	stored, err := Lookup(root, name)
	if err != nil || stored.SourceURL != next.SourceURL {
		t.Fatalf("wrong origin: %+v %v", stored, err)
	}
}
