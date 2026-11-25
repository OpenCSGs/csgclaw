package installstate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkerReinstallLegacyHubOrigin(t *testing.T) {
	const page = "https://hub.example/community/skills/team/demo"
	for _, tc := range []struct {
		name, oldURL, oldPath, nextURL, nextPath string
		allow                                    bool
	}{
		{"same legacy repository", page, "", page, "team/demo", true},
		{"escaped repository", "https://hub.example/skills/%E5%9B%A2%E9%98%9F/demo", "", "https://hub.example/skills/%E5%9B%A2%E9%98%9F/demo", "团队/demo", true},
		{"encoded slash is not a namespace", "https://hub.example/skills/team%2Fdemo", "", "https://hub.example/skills/team%2Fdemo", "team/demo", false},
		{"different site", "https://other.example/community/skills/team/demo", "", page, "team/demo", false},
		{"different repository", "https://hub.example/community/skills/other/demo", "", page, "team/demo", false},
		{"conflicting stored paths", page, "other/demo", page, "team/demo", false},
		{"URL disagrees with requested path", page, "", page, "other/demo", false},
		{"not a Hub page", "https://registry.example", "", "https://registry.example", "team/demo", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, stage := t.TempDir(), t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "demo"), 0755); err != nil {
				t.Fatal(err)
			}
			old := Record{SourceURL: tc.oldURL, RemotePath: tc.oldPath}
			if err := Update(root, "demo", &old, nil); err != nil {
				t.Fatal(err)
			}
			next := Record{Source: SourceMarket, SourceURL: tc.nextURL, RemotePath: tc.nextPath}
			applied := false
			err := (WorkerInstall{Replace: true}).Commit(root, "demo", stage, &next, func() error { applied = true; return nil })
			var conflict *SourceConflict
			if applied != tc.allow || (tc.allow && err != nil) || (!tc.allow && !errors.As(err, &conflict)) {
				t.Fatalf("applied=%v, error=%v", applied, err)
			}
			saved, err := Lookup(root, "demo")
			want := old
			if tc.allow {
				want = next
			}
			if err != nil || saved.Source != want.Source || saved.SourceURL != want.SourceURL || saved.RemotePath != want.RemotePath {
				t.Fatalf("unexpected persisted origin: %+v (%v)", saved, err)
			}
		})
	}
}

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
