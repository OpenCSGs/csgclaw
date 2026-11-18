package roomtask

import (
	"csgclaw/internal/taskcore"
	"errors"
	"path/filepath"
	"testing"
)

func TestReportRetryDoesNotWakeIndependentParent(t *testing.T) {
	s, core, _, _ := fixture(t)
	a := rootTask(t, s)
	b, err := s.Create("room-a", "next", "manager", "Next goal", "")
	if err != nil {
		t.Fatal(err)
	}
	s.send = func(Projection) error { return errors.New("offline") }
	if _, err := s.Report("room-a", a.ID, "stopped", "No work needed"); err == nil {
		t.Fatal("missing delivery error")
	}
	if feedbackCount(core, b.ID) != 0 {
		t.Fatal("unexpected feedback for an independent parent")
	}
	s.send = func(Projection) error { return nil }
	for range 2 {
		if err := s.RetryDelivery("room-a"); err != nil {
			t.Fatal(err)
		}
	}
	if feedbackCount(core, b.ID) != 0 {
		t.Fatal("report retry changed an unrelated goal")
	}
	next, _ := s.Get("room-a", b.ID)
	if next.Status != taskcore.StatusAssigned {
		t.Fatal("unplanned goal should remain with its manager")
	}
}

func TestWorkerCanRunIndependentChildrenInAndAcrossRooms(t *testing.T) {
	core := taskcore.NewService()
	s := NewService(core, func(room string) (Roster, bool) {
		return Roster{RoomID: room, ManagerID: "manager", WorkerIDs: []string{"dev"}}, true
	}, func(Projection) error { return nil })
	titles := [][]string{{"Build", "Document"}, {"Separate build"}}
	children := map[string]taskcore.Task{}
	for i, room := range []string{"a", "b"} {
		root, err := s.Create(room, "source", "admin", "Build", "")
		if err != nil {
			t.Fatal(err)
		}
		var plan []PlanItem
		for _, title := range titles[i] {
			plan = append(plan, PlanItem{IDRef: title, Title: title, AssignedTo: "dev"})
		}
		if err := s.Plan(room, root.ID, "", plan, true); err != nil {
			t.Fatal(err)
		}
		for _, c := range s.List(room) {
			if c.ParentID != "" {
				children[c.Title] = c
			}
		}
		if err := s.Dispatch(room, children[titles[i][0]].ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	build, doc, separate := children["Build"], children["Document"], children["Separate build"]
	if task, _ := s.Get("b", separate.ID); task.Attempt != 1 || !running(task) {
		t.Fatalf("another room reserved the worker: %+v", task)
	}
	if err := s.Dispatch("a", doc.ID, ""); err != nil {
		t.Fatal(err)
	}
	for _, c := range []taskcore.Task{build, doc, separate} {
		task, ok := s.Get(c.RoomID, c.ID)
		if !ok || !running(task) || task.Attempt != 1 || task.WaitingOnTaskID != "" {
			t.Fatalf("independent task was not dispatched: %+v", task)
		}
	}
	if feedbackCount(core, doc.ParentID) != 0 {
		t.Fatal("independent work generated a capacity notification")
	}
}

func TestRecoveryReleasesLegacyRoomQueueAndSameWorkerWait(t *testing.T) {
	s, core, _, _ := fixture(t)
	first := rootTask(t, s)
	if err := s.Plan("room-a", first.ID, "", standardPlan(), true); err != nil {
		t.Fatal(err)
	}
	busy := childFor(t, s, "dev")
	if err := s.Dispatch("room-a", busy.ID, ""); err != nil {
		t.Fatal(err)
	}
	next, err := s.Create("room-a", "next", "admin", "Next goal", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Plan("room-a", next.ID, "", standardPlan(), false); err != nil {
		t.Fatal(err)
	}
	var waitingID string
	if err := core.Mutate(next.ID, func(a *taskcore.Snapshot, _ func() (string, error)) error {
		a.Root.Status = taskcore.StatusQueued
		c := &a.Children[0]
		waitingID = c.ID
		c.Status, c.WaitingOnTaskID, c.Error = taskcore.StatusBlocked, busy.ID, "waiting for capacity"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.Recover(); err != nil {
			t.Fatal(err)
		}
	}
	parent, _ := s.Get("room-a", next.ID)
	waiting, _ := s.Get("room-a", waitingID)
	if parent.Status != taskcore.StatusAssigned || waiting.Status != taskcore.StatusPending || waiting.Attempt != 0 || waiting.WaitingOnTaskID != "" || feedbackCount(core, next.ID) != 2 {
		t.Fatalf("legacy waits were not released exactly once: parent=%+v child=%+v", parent, waiting)
	}
	if err := s.Dispatch("room-a", waitingID, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get("room-a", busy.ID); !got.RecoveryRequired {
		t.Fatal("dispatch cleared another session's recovery flag")
	}
}

func TestRoomStopWaitsForExactAttemptAndPreservesResults(t *testing.T) {
	s, _, _, _ := fixture(t)
	root := rootTask(t, s)
	if err := s.Plan("room-a", root.ID, "", standardPlan(), true); err != nil {
		t.Fatal(err)
	}
	dev, qa := childFor(t, s, "dev"), childFor(t, s, "qa")
	if err := s.Dispatch("room-a", dev.ID, ""); err != nil {
		t.Fatal(err)
	}
	mustUpdate(t, s, dev.ID, "dev", taskcore.StatusInProgress, "", "")
	if err := s.RequestStop("room-a", root.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get("room-a", qa.ID); got.Status != taskcore.StatusCancelled {
		t.Fatal("pending task not revoked")
	}
	if got, _ := s.Get("room-a", dev.ID); got.Status != taskcore.StatusInProgress {
		t.Fatal("pretended execution stopped")
	}
	if _, err := s.Report("room-a", root.ID, "stopped", "stopped"); err == nil {
		t.Fatal("closed running task")
	}
	if err := s.WorkerStopped("room-a", dev.ID, "dev", 2); err == nil {
		t.Fatal("stale stop accepted")
	}
	if err := s.WorkerStopped("room-a", dev.ID, "dev", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Dispatch("room-a", qa.ID, ""); err == nil {
		t.Fatal("dispatch during stop")
	}
	if _, err := s.Report("room-a", root.ID, "stopped", "Stopped, preserve saved work"); err != nil {
		t.Fatal(err)
	}
}

func TestRoomRestartReleasesOtherRoomWaitingForInterruptedWorker(t *testing.T) {
	store, err := taskcore.NewStore(filepath.Join(t.TempDir(), "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	roster := func(room string) (Roster, bool) {
		return Roster{RoomID: room, ManagerID: "manager", WorkerIDs: []string{"dev"}}, true
	}
	core := taskcore.NewService(taskcore.WithStore(store))
	s := NewService(core, roster, nil)
	children := map[string]taskcore.Task{}
	for _, room := range []string{"old", "waiting"} {
		root, err := s.Create(room, "source", "admin", "Build", "")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Plan(room, root.ID, "", []PlanItem{{IDRef: "dev", Title: "Build", AssignedTo: "dev"}}, true); err != nil {
			t.Fatal(err)
		}
		for _, task := range s.List(room) {
			if task.ParentID != "" {
				children[room] = task
			}
		}
	}
	old, waiting := children["old"], children["waiting"]
	if err := s.Dispatch("old", old.ID, ""); err != nil {
		t.Fatal(err)
	}
	// Older builds persisted a cross-room blocker instead of dispatching.
	if err := core.Mutate(waiting.ID, func(a *taskcore.Snapshot, _ func() (string, error)) error {
		c := child(a, waiting.ID)
		c.Status, c.WaitingOnTaskID, c.Error = taskcore.StatusBlocked, old.ID, "worker is busy in another room"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	core = taskcore.NewService(taskcore.WithStore(store))
	delivered := map[string]Projection{}
	s = NewService(core, roster, func(p Projection) error {
		delivered[p.ID] = p
		return nil
	})
	for range 2 {
		if err := s.Recover(); err != nil {
			t.Fatal(err)
		}
	}
	if task, _ := s.Get("old", old.ID); !task.RecoveryRequired || task.Status != taskcore.StatusBlocked {
		t.Fatalf("interrupted attempt lost its recovery requirement: %+v", task)
	}
	if err := s.Dispatch("old", old.ID, ""); err == nil {
		t.Fatal("interrupted work was eligible to restart without recovery")
	}
	if task, _ := s.Get("waiting", waiting.ID); task.Status != taskcore.StatusPending || task.WaitingOnTaskID != "" || task.Attempt != 0 {
		t.Fatalf("waiting task did not return to the Manager: %+v", task)
	}
	wakes := map[string]int{}
	for _, p := range delivered {
		if p.Kind == "task_feedback" {
			wakes[p.TaskID]++
		}
	}
	if wakes[waiting.ID] != 1 || wakes[old.ID] != 1 {
		t.Fatalf("Manager feedback after recovery: %v", wakes)
	}
	if err := s.Dispatch("waiting", waiting.ID, ""); err != nil {
		t.Fatal(err)
	}
	if task, _ := s.Get("waiting", waiting.ID); task.Attempt != 1 || !running(task) {
		t.Fatalf("Manager could not dispatch the other room's task: %+v", task)
	}
}

func TestRoomRestartRequiresExplicitRecoveryBeforeNewAttempt(t *testing.T) {
	s, _, _, _ := fixture(t)
	root := rootTask(t, s)
	if err := s.Plan("room-a", root.ID, "", standardPlan(), true); err != nil {
		t.Fatal(err)
	}
	dev := childFor(t, s, "dev")
	if err := s.Dispatch("room-a", dev.ID, ""); err != nil {
		t.Fatal(err)
	}
	mustUpdate(t, s, dev.ID, "dev", taskcore.StatusInProgress, "", "")
	if err := s.Recover(); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get("room-a", dev.ID)
	if !got.RecoveryRequired || got.Status != taskcore.StatusBlocked {
		t.Fatal(got)
	}
	if err := s.Dispatch("room-a", dev.ID, ""); err == nil {
		t.Fatal("blind rerun after restart")
	}
	if _, err := s.UpdateExecution("room-a", dev.ID, "dev", taskcore.StatusCompleted, "old result", "", 1); err == nil {
		t.Fatal("uncertain old result accepted")
	}
	if err := s.ResolveRecovery("room-a", dev.ID, 1, "Verified old process ended and inspected saved artifact"); err != nil {
		t.Fatal(err)
	}
	if err := s.Dispatch("room-a", dev.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.WorkerStopped("room-a", dev.ID, "dev", 1); err == nil {
		t.Fatal("old stop cancelled new attempt")
	}
}
