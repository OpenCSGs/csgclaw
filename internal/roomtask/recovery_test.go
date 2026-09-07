package roomtask

import (
	"csgclaw/internal/taskcore"
	"errors"
	"testing"
)

func TestRoomQueuedParentWakesAfterReportDeliveryRetry(t *testing.T) {
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
		t.Fatal("next parent woke before summary delivery")
	}
	s.send = func(Projection) error { return nil }
	for range 2 {
		if err := s.RetryDelivery("room-a"); err != nil {
			t.Fatal(err)
		}
	}
	if feedbackCount(core, b.ID) != 1 {
		t.Fatal("lost or repeated queue wake")
	}
	next, _ := s.Get("room-a", b.ID)
	if next.Status != taskcore.StatusQueued {
		t.Fatal("queue wake executed parent")
	}
}

func TestRoomWorkerCapacityWaitPersistsAndWakesOtherManager(t *testing.T) {
	core := taskcore.NewService()
	s := NewService(core, func(room string) (Roster, bool) {
		return Roster{RoomID: room, ManagerID: "manager", WorkerIDs: []string{"dev"}}, true
	}, func(Projection) error { return nil })
	children := map[string]taskcore.Task{}
	for _, room := range []string{"a", "b"} {
		root, err := s.Create(room, "source", "admin", "Build", "")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Plan(room, root.ID, "", []PlanItem{{IDRef: "dev", Title: "Build", AssignedTo: "dev"}}, true); err != nil {
			t.Fatal(err)
		}
		for _, c := range s.List(room) {
			if c.ParentID != "" {
				children[room] = c
			}
		}
		if err := s.Dispatch(room, children[room].ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	a, b := children["a"], children["b"]
	waiting, _ := s.Get("b", b.ID)
	if waiting.Status != taskcore.StatusBlocked || waiting.WaitingOnTaskID != a.ID || waiting.Attempt != 0 {
		t.Fatal(waiting)
	}
	for _, status := range []string{taskcore.StatusInProgress, taskcore.StatusCompleted} {
		result := ""
		if status == taskcore.StatusCompleted {
			result = "artifact"
		}
		if _, err := s.UpdateExecution("a", a.ID, "dev", status, result, "", 1); err != nil {
			t.Fatal(err)
		}
	}
	ready, _ := s.Get("b", b.ID)
	if ready.Status != taskcore.StatusPending || ready.DispatchedAt != nil || ready.WaitingOnTaskID != "" {
		t.Fatal("capacity release auto-dispatched or remained stuck", ready)
	}
	if feedbackCount(core, b.ParentID) != 1 {
		t.Fatal("waiting room manager not notified")
	}
	if err := s.Dispatch("b", b.ID, ""); err != nil {
		t.Fatal(err)
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
