package taskcore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAggregateMutationRollback(t *testing.T) {
	s := NewService()
	root, err := s.CreateRoot(CreateRootInput{AssignmentType: AssignmentTypeRoom, AssignmentID: "r", RoomID: "r", CreatedBy: "admin", Title: "root"})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("reject plan")
	err = s.Mutate(root.ID, func(a *Snapshot, next func() (string, error)) error {
		a.Root.Title = "changed"
		id, err := next()
		if err != nil {
			return err
		}
		a.Children = append(a.Children, Task{ID: id})
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	got, _ := s.Get(root.ID)
	if got.Title != "root" || len(s.ListGlobal()) != 1 {
		t.Fatal("partial aggregate committed")
	}
	err = s.Mutate(root.ID, func(a *Snapshot, _ func() (string, error)) error { a.Root.RoomID = "other"; return nil })
	if err == nil {
		t.Fatal("allowed identity mutation")
	}
}

func TestRoomAggregateReloadAndFailedCommit(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(WithStore(store))
	root, err := s.CreateRoot(CreateRootInput{AssignmentType: AssignmentTypeRoom, AssignmentID: "r", RoomID: "r", CreatedBy: "admin", Title: "root"})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Mutate(root.ID, func(a *Snapshot, next func() (string, error)) error {
		id, err := next()
		if err != nil {
			return err
		}
		child := a.Root
		child.ID, child.ParentID = id, a.Root.ID
		a.Children = append(a.Children, child)
		a.Root.Status = StatusInProgress
		a.Events = append(a.Events, TaskEvent{Type: EventTaskCreated, TaskID: id})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Sequence metadata never acts as an index and cannot hide a committed room aggregate.
	if err := writeTaskSequence(filepath.Join(dir, sequenceFileName), taskSequenceState{}); err != nil {
		t.Fatal(err)
	}
	reloaded := NewService(WithStore(store))
	if len(reloaded.ListGlobal()) != 2 || len(reloaded.Events(root.ID)) != 2 {
		t.Fatal("aggregate or events missing on reload")
	}
	rootDir := filepath.Join(dir, root.ID)
	backup := rootDir + "-test-backup"
	if err := os.Rename(rootDir, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rootDir, []byte("force mkdir failure"), 0600); err != nil {
		t.Fatal(err)
	}
	err = s.Mutate(root.ID, func(a *Snapshot, _ func() (string, error)) error { a.Root.Title = "must not commit"; return nil })
	if err == nil {
		t.Fatal("expected storage failure")
	}
	got, _ := s.Get(root.ID)
	if got.Title != "root" {
		t.Fatal("failed write changed memory")
	}
	if err := os.Remove(rootDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, rootDir); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.LoadRoot(root.ID)
	if err != nil || snapshot.Root.Title != "root" || len(snapshot.Children) != 1 {
		t.Fatal("failed write changed durable aggregate", err)
	}
}

func TestRecursiveTaskRelationsPersistAndRejectCycles(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(WithStore(store))
	parent, err := s.CreateRoot(CreateRootInput{AssignmentType: AssignmentTypeRoom, AssignmentID: "r", RoomID: "r", CreatedBy: "manager", Title: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateChild(CreateChildInput{ParentID: parent.ID, CreatedBy: "manager", Title: "child"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.CreateChild(CreateChildInput{ParentID: c.ID, CreatedBy: "manager", Title: "grandchild"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Mutate(g.ID, func(a *Snapshot, _ func() (string, error)) error {
		for i := range a.Children {
			if a.Children[i].ID == g.ID {
				a.Children[i].Result = "nested result"
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	loaded := NewService(WithStore(store))
	got, ok := loaded.Get(g.ID)
	if !ok || got.ParentID != c.ID || got.RoomID != "r" || got.Result != "nested result" {
		t.Fatal(got)
	}
	if err := loaded.Mutate(c.ID, func(a *Snapshot, _ func() (string, error)) error {
		for i := range a.Children {
			if a.Children[i].ID == c.ID {
				a.Children[i].ParentID = g.ID
			}
		}
		return nil
	}); err == nil {
		t.Fatal("ancestor cycle accepted")
	}
	if err := loaded.Mutate(g.ID, func(a *Snapshot, _ func() (string, error)) error {
		for i := range a.Children {
			if a.Children[i].ID == g.ID {
				a.Children[i].ParentID = "missing"
			}
		}
		return nil
	}); err == nil {
		t.Fatal("missing parent accepted")
	}
}
