package roomtask

import (
	"errors"
	"sync"
	"testing"

	"csgclaw/internal/taskcore"
)

func fixture(t *testing.T) (*Service, *taskcore.Service, *Roster, *[]Projection) {
	t.Helper()
	store, err := taskcore.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core := taskcore.NewService(taskcore.WithStore(store))
	r := &Roster{RoomID: "room-a", ManagerID: "manager", WorkerIDs: []string{"dev", "qa"}}
	projections := []Projection{}
	s := NewService(core, func(room string) (Roster, bool) { return *r, room == r.RoomID }, func(p Projection) error {
		// A synchronous sink may read task state without deadlocking.
		if _, ok := core.Get(p.TaskID); !ok {
			t.Fatal("projection before state commit")
		}
		projections = append(projections, p)
		return nil
	})
	return s, core, r, &projections
}
func rootTask(t *testing.T, s *Service) taskcore.Task {
	t.Helper()
	r, err := s.Create("room-a", "source-1", "admin", "Build and test", "Deliver page and QA results")
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func standardPlan() []PlanItem {
	return []PlanItem{{IDRef: "dev", Title: "Build page", Body: "Deliver an HTML file", AssignedTo: "dev"}, {IDRef: "qa", Title: "Test page", AssignedTo: "qa", DependsOnRefs: []string{"dev"}}}
}
func childFor(t *testing.T, s *Service, actor string) taskcore.Task {
	t.Helper()
	for _, c := range s.List("room-a") {
		if c.ParentID != "" && c.AssignedTo == actor {
			return c
		}
	}
	t.Fatal("child missing")
	return taskcore.Task{}
}
func mustUpdate(t *testing.T, s *Service, id, actor, status, result, reason string) {
	t.Helper()
	if _, err := s.Update("room-a", id, actor, status, result, reason); err != nil {
		t.Fatal(err)
	}
}
func feedbackCount(core *taskcore.Service, id string) int {
	n := 0
	for _, e := range core.Events(id) {
		if e.Type == EventFeedback {
			n++
		}
	}
	return n
}

func TestManagerReviewsEveryChildBeforeExplicitDispatch(t *testing.T) {
	s, core, _, _ := fixture(t)
	root := rootTask(t, s)
	if err := s.Plan("room-a", root.ID, "Build then QA", standardPlan(), true); err != nil {
		t.Fatal(err)
	}
	dev, qa := childFor(t, s, "dev"), childFor(t, s, "qa")
	if dev.DispatchedAt != nil || qa.DispatchedAt != nil {
		t.Fatal("plan automatically dispatched")
	}
	if err := s.Dispatch("room-a", qa.ID, ""); err == nil {
		t.Fatal("ignored predecessor")
	}
	if err := s.Dispatch("room-a", dev.ID, ""); err != nil {
		t.Fatal(err)
	}
	mustUpdate(t, s, dev.ID, "dev", taskcore.StatusInProgress, "", "")
	mustUpdate(t, s, dev.ID, "dev", taskcore.StatusCompleted, "page.html", "")
	mustUpdate(t, s, dev.ID, "dev", taskcore.StatusCompleted, "page.html", "")
	if feedbackCount(core, root.ID) != 1 {
		t.Fatal("each unique result must notify manager once")
	}
	got, _ := s.Get("room-a", dev.ID)
	if got.Status != taskcore.StatusReview {
		t.Fatal("worker self-accepted")
	}
	if err := s.Dispatch("room-a", qa.ID, ""); err == nil {
		t.Fatal("submitted is not accepted")
	}
	if err := s.Review("room-a", dev.ID, 1, true, "Page verified"); err != nil {
		t.Fatal(err)
	}
	if childFor(t, s, "qa").DispatchedAt != nil {
		t.Fatal("review automatically dispatched")
	}
	if err := s.Dispatch("room-a", qa.ID, ""); err != nil {
		t.Fatal(err)
	}
	mustUpdate(t, s, qa.ID, "qa", taskcore.StatusInProgress, "", "")
	mustUpdate(t, s, qa.ID, "qa", taskcore.StatusCompleted, "P1 issue found", "")
	if feedbackCount(core, root.ID) != 2 {
		t.Fatal("missing QA feedback")
	}
	if err := s.Review("room-a", qa.ID, 1, true, "Test report received; defect remains"); err != nil {
		t.Fatal(err)
	}
	result, err := s.Report("room-a", root.ID, "issues", "Page delivered; P1 remains")
	if err != nil || result.ReportStatus != "delivered" || result.GoalOutcome != "issues" {
		t.Fatal(result, err)
	}
}

func TestPlanValidationAndTaskBoundaries(t *testing.T) {
	for _, kind := range []string{"empty", "outsider", "manager", "duplicate", "cycle", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _, _ := fixture(t)
			root := rootTask(t, s)
			p := standardPlan()
			switch kind {
			case "empty":
				p = nil
			case "outsider":
				p[0].AssignedTo = "external"
			case "manager":
				p[0].AssignedTo = "manager"
			case "duplicate":
				p[1].IDRef = "dev"
			case "cycle":
				p[0].DependsOnRefs = []string{"qa"}
			case "unknown":
				p[1].DependsOnRefs = []string{"missing"}
			}
			if err := s.Plan("room-a", root.ID, "bad", p, true); err == nil {
				t.Fatal("accepted invalid plan")
			}
			if len(s.List("room-a")) != 1 {
				t.Fatal("partial plan persisted")
			}
		})
	}
	s, _, _, _ := fixture(t)
	root := rootTask(t, s)
	if err := s.Plan("room-a", root.ID, "plan", standardPlan(), true); err != nil {
		t.Fatal(err)
	}
	dev := childFor(t, s, "dev")
	if _, ok := s.Get("room-b", dev.ID); ok {
		t.Fatal("cross-room read")
	}
	if _, err := s.Update("room-b", dev.ID, "dev", taskcore.StatusInProgress, "", ""); err == nil {
		t.Fatal("cross-room claim")
	}
	if _, err := s.Update("room-a", dev.ID, "qa", taskcore.StatusInProgress, "", ""); err == nil {
		t.Fatal("wrong worker claimed")
	}
	if _, err := s.Update("room-a", root.ID, "manager", taskcore.StatusCompleted, "done", ""); err == nil {
		t.Fatal("manager bypassed children")
	}
	if err := s.Plan("room-a", root.ID, "replacement", standardPlan(), true); err == nil {
		t.Fatal("silently replanned")
	}
	if len(s.List("room-a")) != 3 {
		t.Fatal("duplicate children")
	}
}

func TestRootSubmissionConcurrentAndPersistent(t *testing.T) {
	store, err := taskcore.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(room string) (Roster, bool) {
		return Roster{RoomID: room, ManagerID: "manager", WorkerIDs: []string{"dev"}}, true
	}
	s := NewService(taskcore.NewService(taskcore.WithStore(store)), resolve, nil)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Create("room-a", "source", "admin", "Build", ""); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(s.List("room-a")) != 1 {
		t.Fatal("duplicate root")
	}
	reloaded := NewService(taskcore.NewService(taskcore.WithStore(store)), resolve, nil)
	r, err := reloaded.Create("room-a", "source", "admin", "Build", "")
	if err != nil || r.ID != s.List("room-a")[0].ID {
		t.Fatal("lost dedupe on reload", err)
	}
	if _, err := reloaded.Create("room-a", "other-source", "manager", "Other", ""); err != nil {
		t.Fatal("manager could not create queued parent", err)
	}
	if len(reloaded.List("room-a")) != 2 {
		t.Fatal("second parent missing")
	}
}

func TestFailureStopAndMembership(t *testing.T) {
	for _, status := range []string{taskcore.StatusFailed, taskcore.StatusCancelled, taskcore.StatusBlocked} {
		t.Run(status, func(t *testing.T) {
			s, core, _, _ := fixture(t)
			root := rootTask(t, s)
			if err := s.Plan("room-a", root.ID, "", standardPlan(), true); err != nil {
				t.Fatal(err)
			}
			dev := childFor(t, s, "dev")
			if err := s.Dispatch("room-a", dev.ID, ""); err != nil {
				t.Fatal(err)
			}
			mustUpdate(t, s, dev.ID, "dev", taskcore.StatusInProgress, "", "")
			if err := s.WorkerStopped("room-a", "", "dev", 0); err != nil {
				t.Fatal(err)
			}
			got, _ := s.Get("room-a", dev.ID)
			if got.Status != taskcore.StatusInProgress {
				t.Fatal("unscoped stop cancelled task")
			}
			mustUpdate(t, s, dev.ID, "dev", status, "", "blocked or failed")
			if feedbackCount(core, root.ID) != 1 {
				t.Fatal("missing exception notification")
			}
			qa := childFor(t, s, "qa")
			if qa.DispatchedAt != nil {
				t.Fatal("continued after failure")
			}
			if qa.Status != taskcore.StatusPending {
				t.Fatal("failure cancelled work without Manager decision")
			}
		})
	}
	s, core, roster, _ := fixture(t)
	root := rootTask(t, s)
	if err := s.Plan("room-a", root.ID, "", standardPlan(), true); err != nil {
		t.Fatal(err)
	}
	roster.WorkerIDs = []string{"dev"}
	if err := s.MembersChanged("room-a"); err != nil {
		t.Fatal(err)
	}
	if childFor(t, s, "qa").Status != taskcore.StatusBlocked || feedbackCount(core, root.ID) != 1 {
		t.Fatal("removed worker not blocked")
	}
	if err := s.MembersChanged("room-a"); err != nil {
		t.Fatal(err)
	}
	if feedbackCount(core, root.ID) != 1 {
		t.Fatal("duplicate member alert")
	}

	s, _, roster, _ = fixture(t)
	root = rootTask(t, s)
	if err := s.Plan("room-a", root.ID, "", standardPlan(), true); err != nil {
		t.Fatal(err)
	}
	dev := childFor(t, s, "dev")
	if err := s.Dispatch("room-a", dev.ID, ""); err != nil {
		t.Fatal(err)
	}
	mustUpdate(t, s, dev.ID, "dev", taskcore.StatusInProgress, "", "")
	mustUpdate(t, s, dev.ID, "dev", taskcore.StatusCompleted, "page.html", "")
	roster.WorkerIDs = []string{"qa"}
	if err := s.MembersChanged("room-a"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get("room-a", dev.ID); got.Status != taskcore.StatusReview || got.Result != "page.html" {
		t.Fatalf("submitted task changed after member removal: %+v", got)
	}
	if err := s.Review("room-a", dev.ID, 1, true, "submission remains reviewable"); err != nil {
		t.Fatal(err)
	}
}

func TestRoomDeletionRequiresTerminalTasksAndBlocksMutations(t *testing.T) {
	s, _, _, _ := fixture(t)
	root := rootTask(t, s)
	if release, err := s.BeginRoomDeletion("room-a"); err == nil {
		release()
		t.Fatal("deleted room with unfinished task")
	}
	if _, err := s.Report("room-a", root.ID, "stopped", "No work was started"); err != nil {
		t.Fatal(err)
	}
	release, err := s.BeginRoomDeletion("room-a")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := s.Create("room-a", "source-2", "manager", "More work", ""); err == nil {
		t.Fatal("created task while room deletion was in progress")
	}
}

func TestProjectionAndMentionIdentity(t *testing.T) {
	s, _, _, sent := fixture(t)
	root := rootTask(t, s)
	if err := s.Plan("room-a", root.ID, "", standardPlan(), true); err != nil {
		t.Fatal(err)
	}
	dev := childFor(t, s, "dev")
	if err := s.Dispatch("room-a", dev.ID, ""); err != nil {
		t.Fatal(err)
	}
	p := (*sent)[len(*sent)-1]
	if id, ok := s.ResolveProjection("room-a", p.ID, "task_assigned", "dev"); !ok || id != dev.ID {
		t.Fatal("dispatch lost identity")
	}
	if _, ok := s.ResolveProjection("room-a", "forged", "task_assigned", "dev"); ok {
		t.Fatal("forged dispatch")
	}
	if _, ok := s.ResolveMention("room-a", dev.ID, "qa"); ok {
		t.Fatal("wrong task session")
	}
	if id, ok := s.ResolveMention("room-a", dev.ID, "dev"); !ok || id != dev.ID {
		t.Fatal("same task did not reuse identity")
	}
	if id, ok := s.ResolveMention("room-a", dev.ID, "manager"); !ok || id != dev.ID {
		t.Fatal("manager lost related child context")
	}
	if _, ok := s.ResolveMention("room-a", "", "dev"); ok {
		t.Fatal("guessed task")
	}
	mustUpdate(t, s, dev.ID, "dev", taskcore.StatusInProgress, "", "")
	if s.SourceTaskID("room-a", p.ID) != dev.ID {
		t.Fatal("stop cannot resolve claimed dispatch")
	}
}

func TestDeliveryFailureRetriesWithoutRecreatingWork(t *testing.T) {
	s, _, _, _ := fixture(t)
	root := rootTask(t, s)
	s.send = func(Projection) error { return errors.New("offline") }
	if err := s.Plan("room-a", root.ID, "", standardPlan(), true); err == nil {
		t.Fatal("lost delivery failure")
	}
	if len(s.List("room-a")) != 3 {
		t.Fatal("lost saved plan")
	}
	dev := childFor(t, s, "dev")
	ids := []string{}
	s.send = func(p Projection) error { ids = append(ids, p.ID); return nil }
	if err := s.RetryDelivery("room-a"); err != nil {
		t.Fatal(err)
	}
	if err := s.RetryDelivery("room-a"); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != ids[1] || childFor(t, s, "dev").ID != dev.ID {
		t.Fatal("retry recreated dispatch")
	}
}

func TestCorrectionsAndReassignmentRejectOldAttempts(t *testing.T) {
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
	mustUpdate(t, s, dev.ID, "dev", taskcore.StatusCompleted, "incomplete", "")
	if err := s.Review("room-a", dev.ID, 1, false, "Missing entrypoint"); err != nil {
		t.Fatal(err)
	}
	if err := s.Dispatch("room-a", dev.ID, "qa"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateExecution("room-a", dev.ID, "dev", taskcore.StatusCompleted, "late", "", 1); err == nil {
		t.Fatal("accepted stale old assignee")
	}
	if _, err := s.UpdateExecution("room-a", dev.ID, "qa", taskcore.StatusInProgress, "", "", 1); err == nil {
		t.Fatal("accepted stale attempt")
	}
	if _, err := s.UpdateExecution("room-a", dev.ID, "qa", taskcore.StatusInProgress, "", "", 2); err != nil {
		t.Fatal(err)
	}
}

func TestParentsQueueAndIndependentWorkersRunConcurrently(t *testing.T) {
	s, _, _, _ := fixture(t)
	a := rootTask(t, s)
	b, err := s.Create("room-a", "manager-goal", "manager", "Next", "")
	if err != nil {
		t.Fatal(err)
	}
	plan := standardPlan()
	plan[1].DependsOnRefs = nil
	if err := s.Plan("room-a", a.ID, "", plan, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Plan("room-a", b.ID, "", plan, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Start("room-a", b.ID); err == nil {
		t.Fatal("second parent ran early")
	}
	tasks := s.List("room-a")
	for _, c := range tasks {
		if c.ParentID == a.ID {
			if err := s.Dispatch("room-a", c.ID, ""); err != nil {
				t.Fatal(err)
			}
			mustUpdate(t, s, c.ID, c.AssignedTo, taskcore.StatusInProgress, "", "")
		}
	}
	for _, c := range tasks {
		if c.ParentID == a.ID {
			mustUpdate(t, s, c.ID, c.AssignedTo, taskcore.StatusCompleted, "delivered", "")
			if err := s.Review("room-a", c.ID, 1, true, "accepted"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := s.Report("room-a", a.ID, "succeeded", "all done"); err != nil {
		t.Fatal(err)
	}
	if err := s.Start("room-a", b.ID); err != nil {
		t.Fatal(err)
	}
	for _, c := range tasks {
		if c.ParentID == b.ID {
			if err := s.Dispatch("room-a", c.ID, ""); err != nil {
				t.Fatal(err)
			}
		}
	}
}
