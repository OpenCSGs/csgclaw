package roomtask

import (
	"csgclaw/internal/taskcore"
	"reflect"
	"testing"
)

func finishChild(t *testing.T, s *Service, task taskcore.Task, status, result, reason string) {
	t.Helper()
	if err := s.Dispatch("room-a", task.ID, ""); err != nil {
		t.Fatal(err)
	}
	mustUpdate(t, s, task.ID, task.AssignedTo, taskcore.StatusInProgress, "", "")
	mustUpdate(t, s, task.ID, task.AssignedTo, status, result, reason)
	current, _ := s.Get("room-a", task.ID)
	if err := s.Review("room-a", task.ID, current.Attempt, true, "Verified the delivered work: "+result); err != nil {
		t.Fatal(err)
	}
}

func TestRepairAndRegressionStayUnderOriginalParent(t *testing.T) {
	store, err := taskcore.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	roster := func(room string) (Roster, bool) {
		return Roster{RoomID: "room-a", ManagerID: "manager", WorkerIDs: []string{"dev", "qa"}}, room == "room-a"
	}
	core := taskcore.NewService(taskcore.WithStore(store))
	s := NewService(core, roster, nil)
	root := rootTask(t, s)
	if err := s.Plan("room-a", root.ID, "Build then test", standardPlan(), true); err != nil {
		t.Fatal(err)
	}
	dev, qa := childFor(t, s, "dev"), childFor(t, s, "qa")
	finishChild(t, s, dev, taskcore.StatusCompleted, "page.html", "")
	finishChild(t, s, qa, taskcore.StatusFailed, "Complete test report: image field mismatch", "Product tests failed")
	before, _ := s.Get("room-a", qa.ID)
	items := []PlanItem{
		{IDRef: "fix", Title: "Repair image field", AssignedTo: "dev", DependsOnRefs: []string{qa.ID}},
		{IDRef: "regression", Title: "Regression test", AssignedTo: "qa", DependsOnRefs: []string{"fix"}},
	}
	if err := s.AppendPlan("room-a", root.ID, "qa-feedback-1", "Repair then retest", items); err != nil {
		t.Fatal(err)
	}
	parent, _ := s.Get("room-a", root.ID)
	if parent.Status != taskcore.StatusInProgress {
		t.Fatal(parent.Status)
	}
	after, _ := s.Get("room-a", qa.ID)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("extension rewrote historical testing")
	}
	var repair, regression taskcore.Task
	for _, c := range s.List("room-a") {
		if c.ParentID == "" && c.ID != root.ID {
			t.Fatal("created another parent")
		}
		if c.Title == "Repair image field" {
			repair = c
		}
		if c.Title == "Regression test" {
			regression = c
		}
	}
	if repair.ParentID != root.ID || regression.ParentID != root.ID || !reflect.DeepEqual(regression.DependsOn, []string{repair.ID}) {
		t.Fatal(repair, regression)
	}
	if repair.DispatchedAt != nil || regression.DispatchedAt != nil {
		t.Fatal("append dispatched work")
	}
	if err := s.Dispatch("room-a", regression.ID, ""); err == nil {
		t.Fatal("regression bypassed repair")
	}
	for _, outcome := range []string{"succeeded", "issues"} {
		if _, err := s.Report("room-a", root.ID, outcome, "Premature closure"); err == nil {
			t.Fatal("closed unfinished repair", outcome)
		}
	}
	// Rebuild the service from persisted events; retries cannot duplicate tasks.
	core = taskcore.NewService(taskcore.WithStore(store))
	s = NewService(core, roster, nil)
	if err := s.AppendPlan("room-a", root.ID, "qa-feedback-1", "Repair then retest", items); err != nil {
		t.Fatal(err)
	}
	if len(s.List("room-a")) != 5 {
		t.Fatal("restart retry duplicated work")
	}
	finishChild(t, s, repair, taskcore.StatusCompleted, "Fixed page with diff and checks", "")
	finishChild(t, s, regression, taskcore.StatusCompleted, "Regression passed", "")
	if _, err := s.Report("room-a", root.ID, "succeeded", "Delivered and tested"); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendPlan("room-a", root.ID, "qa-feedback-1", "Repair then retest", items); err != nil {
		t.Fatal("retry after closure", err)
	}
	if len(s.List("room-a")) != 5 {
		t.Fatal("duplicated children")
	}
	if err := s.AppendPlan("room-a", root.ID, "qa-feedback-2", "New work", items); err == nil {
		t.Fatal("extended closed parent")
	}
}

func TestAppendValidationAndIdempotency(t *testing.T) {
	s, core, r, _ := fixture(t)
	root := rootTask(t, s)
	if err := s.Plan("room-a", root.ID, "Original", standardPlan(), true); err != nil {
		t.Fatal(err)
	}
	qa := childFor(t, s, "qa")
	good := []PlanItem{{IDRef: "repair", Title: "Repair", AssignedTo: "dev", DependsOnRefs: []string{qa.ID}}}
	for name, items := range map[string][]PlanItem{
		"foreign dependency": {{IDRef: "fix", Title: "Repair", AssignedTo: "dev", DependsOnRefs: []string{"other-room-task"}}},
		"cycle":              {{IDRef: "fix", Title: "Repair", AssignedTo: "dev", DependsOnRefs: []string{"fix"}}},
		"ref collision":      {{IDRef: qa.ID, Title: "Repair", AssignedTo: "dev"}},
		"nonmember":          {{IDRef: "fix", Title: "Repair", AssignedTo: "outsider"}},
	} {
		if err := s.AppendPlan("room-a", root.ID, name, "invalid", items); err == nil {
			t.Fatal(name)
		}
		if len(s.List("room-a")) != 3 {
			t.Fatal("partial mutation", name)
		}
	}
	if err := s.AppendPlan("room-a", qa.ID, "nested", "invalid", good); err == nil {
		t.Fatal("nested append")
	}
	if err := s.AppendPlan("other-room", root.ID, "foreign", "invalid", good); err == nil {
		t.Fatal("foreign room append")
	}
	if err := s.AppendPlan("room-a", root.ID, "", "invalid", good); err == nil {
		t.Fatal("missing request ID")
	}
	if err := s.AppendPlan("room-a", root.ID, "stable", "Repair", good); err != nil {
		t.Fatal(err)
	}
	// Replace the coordinator to test durable request identity independently of its memory.
	s = NewService(core, func(room string) (Roster, bool) { return *r, room == r.RoomID }, nil)
	if err := s.AppendPlan("room-a", root.ID, "stable", "Repair", good); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendPlan("room-a", root.ID, "stable", "Changed payload", good); err == nil {
		t.Fatal("key reused with conflicting content")
	}
	if len(s.List("room-a")) != 4 {
		t.Fatal("duplicated children")
	}
}
