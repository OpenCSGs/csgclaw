package template

import (
	"context"
	"csgclaw/internal/apitypes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func versionSpec(t *testing.T, name string) PublishSpec {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	return PublishSpec{Name: name, RuntimeKind: "codex", SourceAgentID: "agent-a", WorkspaceRef: WorkspaceRef{Kind: WorkspaceKindDir, Path: root}}
}

func TestSaveAgentVersionsPersistAndPrune(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewLocalStore(root)
	spec := versionSpec(t, "ReviewBot")
	first, err := store.SaveAgent(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	if first.UID == "" || first.Revision != 1 {
		t.Fatalf("first = %+v", first)
	}
	// A renamed save must follow the association, even across process restarts.
	spec.Name = "RenamedBot"
	spec.DisplayName = "Renamed"
	for i := 2; i <= 12; i++ {
		store = NewLocalStore(root)
		spec.CommitMessage = fmt.Sprintf("save %d", i)
		spec.Author = &VersionAuthor{ID: "alice", Name: "Alice"}
		item, err := store.SaveAgent(ctx, spec)
		if err != nil {
			t.Fatal(err)
		}
		if item.ID != first.ID || item.UID != first.UID || item.Revision != i {
			t.Fatalf("save %d = %+v", i, item)
		}
	}
	versions, err := store.Versions(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 10 || versions[0].Revision != 3 || versions[9].Revision != 12 {
		t.Fatalf("versions = %+v", versions)
	}
	if _, err := store.ReadVersionFile(ctx, first.ID, 1, "agent.toml"); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("pruned version error = %v", err)
	}
	snapshots, err := os.ReadDir(filepath.Join(store.templateRoot(first.ID), versionSnapshotsDir))
	if err != nil || len(snapshots) != 10 {
		t.Fatalf("snapshots = %d, err = %v", len(snapshots), err)
	}
}

func TestRestoreVersionPreservesIdentityAndAgentWorkspace(t *testing.T) {
	ctx := context.Background()
	store := NewLocalStore(t.TempDir())
	spec := versionSpec(t, "ReviewBot")
	first, err := store.SaveAgent(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(spec.WorkspaceRef.Path, "AGENTS.md")
	if err := os.WriteFile(source, []byte("updated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveAgent(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveAgent(ctx, spec); err != nil {
		t.Fatal(err)
	}
	restored, err := store.RestoreVersion(ctx, first.ID, 1, "restore initial", nil)
	if err != nil {
		t.Fatal(err)
	}
	if restored.UID != first.UID || restored.Revision != 1 || restored.LatestRevision != 3 {
		t.Fatalf("restored = %+v", restored)
	}
	file, err := store.ReadVersionFile(ctx, first.ID, 1, "instructions/AGENTS.md")
	if err != nil || file.Content != "original" {
		t.Fatalf("restored content = %q, err = %v", file.Content, err)
	}
	bytes, err := os.ReadFile(source)
	if err != nil || string(bytes) != "updated" {
		t.Fatalf("agent content = %q, err = %v", bytes, err)
	}
	versions, err := store.Versions(ctx, first.ID)
	if err != nil || len(versions) != 3 || versions[2].Revision != 3 {
		t.Fatalf("versions = %+v, err = %v", versions, err)
	}
	current, err := NewLocalStore(store.root).Get(ctx, first.ID)
	if err != nil || current.Revision != 1 || current.LatestRevision != 3 {
		t.Fatalf("persisted current = %+v, err = %v", current, err)
	}
	content, err := os.ReadFile(filepath.Join(store.templateRoot(first.ID), "instructions", "AGENTS.md"))
	if err != nil || string(content) != "original" {
		t.Fatalf("active content=%q err=%v", content, err)
	}
	if _, err := store.RestoreVersion(ctx, first.ID, 99, "", nil); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("invalid rollback: %v", err)
	}
	if _, err := store.RestoreVersion(ctx, first.ID, 1, "", nil); err != nil {
		t.Fatal(err)
	}
	retained, err := store.ReadVersionFile(ctx, first.ID, 3, "instructions/AGENTS.md")
	if err != nil || retained.Content != "updated" {
		t.Fatalf("retained V3=%+v err=%v", retained, err)
	}
	latest, err := store.SaveAgent(ctx, spec)
	if err != nil || latest.Revision != 4 || latest.ID != first.ID {
		t.Fatalf("next save = %+v, err = %v", latest, err)
	}
}

func TestSaveAgentClaimsExistingTemplate(t *testing.T) {
	ctx := context.Background()
	store := NewLocalStore(t.TempDir())
	spec := versionSpec(t, "ReviewBot")
	existing, err := store.Publish(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(spec.WorkspaceRef.Path, "AGENTS.md"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	item, err := store.SaveAgent(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	if item.UID != existing.UID || item.Revision != 2 {
		t.Fatalf("claimed = %+v", item)
	}
	old, err := store.ReadVersionFile(ctx, item.ID, 1, "instructions/AGENTS.md")
	if err != nil || old.Content != "original" {
		t.Fatalf("legacy snapshot = %+v, err = %v", old, err)
	}
}

func TestConcurrentSavesAllocateDistinctVersions(t *testing.T) {
	ctx := context.Background()
	store := NewLocalStore(t.TempDir())
	spec := versionSpec(t, "ReviewBot")
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.SaveAgent(ctx, spec); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	versions, err := store.Versions(ctx, "ReviewBot")
	if err != nil || len(versions) != 5 || versions[4].Revision != 5 {
		t.Fatalf("versions = %+v, err = %v", versions, err)
	}
}

func TestVersionSaveFailurePreservesLatestAndHistory(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	ctx := context.Background()
	spec := versionSpec(t, "Reviewer")
	first, err := store.SaveAgent(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/does-not-exist", filepath.Join(spec.WorkspaceRef.Path, "skills")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveAgent(ctx, spec); err == nil {
		t.Fatal("unsafe save succeeded")
	}
	current, err := store.Get(ctx, first.ID)
	if err != nil || current.Revision != 1 || current.UID != first.UID {
		t.Fatalf("current=%+v, err=%v", current, err)
	}
	versions, err := store.Versions(ctx, first.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("history=%+v err=%v", versions, err)
	}
}

func TestInstructionEditCreatesRetainedRevision(t *testing.T) {
	ctx := context.Background()
	store := NewLocalStore(t.TempDir())
	spec := versionSpec(t, "Edited")
	saved, err := store.SaveAgent(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteWorkspaceFile(ctx, saved.ID, "instructions/AGENTS.md", "new instructions"); err != nil {
		t.Fatal(err)
	}
	edited, err := store.Get(ctx, saved.ID)
	if err != nil || edited.Revision != 2 || edited.UID != saved.UID {
		t.Fatalf("edited=%+v err=%v", edited, err)
	}
	original, err := store.ReadVersionFile(ctx, saved.ID, 1, "instructions/AGENTS.md")
	if err != nil || original.Content == "new instructions\n" {
		t.Fatalf("original=%+v err=%v", original, err)
	}
	snapshot, err := store.ReadVersionFile(ctx, saved.ID, 2, "instructions/AGENTS.md")
	if err != nil || snapshot.Content != "new instructions\n" {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	if _, err := store.RestoreVersion(ctx, saved.ID, 1, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteWorkspaceFile(ctx, saved.ID, "instructions/AGENTS.md", "third"); err != nil {
		t.Fatal(err)
	}
	edited, err = store.Get(ctx, saved.ID)
	if err != nil || edited.Revision != 3 {
		t.Fatalf("edited=%+v err=%v", edited, err)
	}
}

func TestSaveAgentRejectsAnotherAgentsTemplate(t *testing.T) {
	for _, explicitID := range []bool{false, true} {
		t.Run(fmt.Sprint("explicit_id_", explicitID), func(t *testing.T) {
			ctx := context.Background()
			store := NewLocalStore(t.TempDir())
			first, err := store.SaveAgent(ctx, versionSpec(t, "SharedName"))
			if err != nil {
				t.Fatal(err)
			}
			publication := apitypes.TemplatePublication{Site: "https://example.invalid", Namespace: "alice", TemplateID: "alice/SharedName", Revision: 1}
			if err := store.recordPublication(ctx, first.ID, first.UID, publication); err != nil {
				t.Fatal(err)
			}
			before, err := store.Get(ctx, first.ID)
			if err != nil {
				t.Fatal(err)
			}
			second := versionSpec(t, "SharedName")
			second.SourceAgentID = "agent-b"
			if explicitID {
				second.ID = first.ID
				second.Name = "OtherName"
			}
			if err := os.WriteFile(filepath.Join(second.WorkspaceRef.Path, "AGENTS.md"), []byte("agent B"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SaveAgent(ctx, second); !errors.Is(err, ErrTemplateAlreadyExists) {
				t.Fatalf("collision error=%v", err)
			}
			after, err := store.Get(ctx, first.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("template mutated: before=%+v after=%+v err=%v", before, after, err)
			}
			versions, err := store.Versions(ctx, first.ID)
			if err != nil || len(versions) != 1 {
				t.Fatalf("versions=%+v err=%v", versions, err)
			}
			content, err := store.ReadWorkspaceFile(ctx, first.ID, "instructions/AGENTS.md")
			if err != nil || content.Content != "original" {
				t.Fatalf("content=%+v err=%v", content, err)
			}
			second.ID, second.Name = "", "OwnName"
			own, err := store.SaveAgent(ctx, second)
			if err != nil || own.UID == first.UID || own.SourceAgentID != "agent-b" || len(own.Publications) != 0 {
				t.Fatalf("independent template=%+v err=%v", own, err)
			}
		})
	}
}

func TestSaveAgentAssociationTakesPrecedenceOverAnotherAgentsName(t *testing.T) {
	ctx := context.Background()
	store := NewLocalStore(t.TempDir())
	a := versionSpec(t, "TemplateA")
	first, err := store.SaveAgent(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	b := versionSpec(t, "TemplateB")
	b.SourceAgentID = "agent-b"
	other, err := store.SaveAgent(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	a.Name, a.ID = other.Name, other.ID
	updated, err := store.SaveAgent(ctx, a)
	if err != nil || updated.ID != first.ID || updated.UID != first.UID || updated.Revision != 2 {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	untouched, err := store.Get(ctx, other.ID)
	if err != nil || !reflect.DeepEqual(untouched, other) {
		t.Fatalf("other template mutated: %+v err=%v", untouched, err)
	}
}
