// Package roomtask coordinates room work over the shared task aggregate.
// It owns no teams or member store: every decision reads the current room roster.
package roomtask

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/taskcore"
)

const (
	EventDispatched = "task.dispatched"
	EventFeedback   = "task.feedback"
	EventReported   = "task.reported"
	EventPlanned    = "task.planned"
	EventExtended   = "task.plan_extended"
	EventReviewed   = "task.reviewed"
)

type Roster struct {
	RoomID, ManagerID string
	WorkerIDs         []string
}
type Projection struct {
	ID, RoomID, TaskID, SenderID, TargetID, Kind, Title, Content string
	Attempt                                                      int
}
type Service struct {
	mu     sync.Mutex
	core   *taskcore.Service
	roster func(string) (Roster, bool)
	send   func(Projection) error
}

func NewService(core *taskcore.Service, roster func(string) (Roster, bool), send func(Projection) error) *Service {
	return &Service{core: core, roster: roster, send: send}
}

func (s *Service) List(roomID string) []taskcore.Task {
	out := []taskcore.Task{}
	for _, t := range s.core.ListGlobal() {
		if t.AssignmentType == taskcore.AssignmentTypeRoom && (roomID == "" || t.RoomID == roomID) {
			out = append(out, t)
		}
	}
	return out
}
func (s *Service) Get(roomID, id string) (taskcore.Task, bool) {
	t, ok := s.core.Get(id)
	return t, ok && t.AssignmentType == taskcore.AssignmentTypeRoom && t.RoomID == roomID
}

// Resolve looks up a room task by its globally unique task id. Room-scoped
// mutations still receive the resolved room id and validate it in Get/change.
func (s *Service) Resolve(id string) (taskcore.Task, bool) {
	if s == nil || s.core == nil {
		return taskcore.Task{}, false
	}
	t, ok := s.core.Get(id)
	return t, ok && t.AssignmentType == taskcore.AssignmentTypeRoom
}
func (s *Service) members(room string) (Roster, error) {
	if s.roster == nil {
		return Roster{}, fmt.Errorf("room roster unavailable")
	}
	r, ok := s.roster(room)
	if !ok || r.RoomID != room || r.ManagerID == "" {
		return Roster{}, fmt.Errorf("collaboration room or manager unavailable")
	}
	return r, nil
}
func worker(r Roster, id string) bool {
	if id == "" || id == r.ManagerID {
		return false
	}
	for _, member := range r.WorkerIDs {
		if id == member {
			return true
		}
	}
	return false
}
func Terminal(status string) bool {
	return status == taskcore.StatusCompleted || status == taskcore.StatusFailed || status == taskcore.StatusCancelled
}

func (s *Service) Create(room, source, requester, title, body string) (taskcore.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.members(room)
	if err != nil {
		return taskcore.Task{}, err
	}
	if worker(r, requester) {
		return taskcore.Task{}, fmt.Errorf("workers cannot create parent tasks")
	}
	if strings.TrimSpace(source) == "" {
		return taskcore.Task{}, fmt.Errorf("source message is required")
	}
	for _, t := range s.List(room) {
		if t.ParentID != "" {
			continue
		}
		if t.SourceMessageID == source {
			return t, nil
		}
	}
	if len(r.WorkerIDs) == 0 {
		return taskcore.Task{}, fmt.Errorf("add an available room worker before starting")
	}
	return s.core.CreateRoot(taskcore.CreateRootInput{Status: taskcore.StatusQueued, AssignmentType: taskcore.AssignmentTypeRoom, AssignmentID: room, RoomID: room, ExecutionChannel: "csgclaw", AssignedTo: r.ManagerID, CreatedBy: requester, SourceMessageID: source, Title: title, Body: body})
}

// The manager supplies the plan it has reasoned about. Planning does not start
// a second hidden model turn, and never guesses assignees from global agents.
type PlanItem struct {
	IDRef         string   `json:"id_ref"`
	Title         string   `json:"title"`
	Body          string   `json:"body,omitempty"`
	AssignedTo    string   `json:"assigned_to"`
	DependsOnRefs []string `json:"depends_on_refs,omitempty"`
}

func validatePlan(items []PlanItem, roster Roster, existing ...taskcore.Task) error {
	if len(items) == 0 {
		return fmt.Errorf("tasks must contain at least one concrete worker subtask")
	}
	refs := map[string]PlanItem{}
	for _, task := range existing {
		refs[task.ID] = PlanItem{IDRef: task.ID}
	}
	for _, item := range items {
		if strings.TrimSpace(item.IDRef) == "" || strings.TrimSpace(item.Title) == "" || !worker(roster, item.AssignedTo) {
			return fmt.Errorf("each task needs id_ref, title and an assigned_to from current room workers")
		}
		if _, exists := refs[item.IDRef]; exists {
			return fmt.Errorf("duplicate id_ref %q", item.IDRef)
		}
		refs[item.IDRef] = item
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(ref string) error {
		if visiting[ref] {
			return fmt.Errorf("cyclic task dependency %q", ref)
		}
		if done[ref] {
			return nil
		}
		item, ok := refs[ref]
		if !ok {
			return fmt.Errorf("unknown dependency %q", ref)
		}
		visiting[ref] = true
		for _, dep := range item.DependsOnRefs {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[ref], done[ref] = false, true
		return nil
	}
	for ref := range refs {
		if err := visit(ref); err != nil {
			return err
		}
	}
	return nil
}

// change serializes admission across parents and workers, not model execution.
// Delivery occurs after releasing the coordinator lock.
func (s *Service) change(room, id string, fn func(Roster, []taskcore.Task, *taskcore.Snapshot, func() (string, error)) error) error {
	s.mu.Lock()
	roster, err := s.members(room)
	if err == nil {
		if _, ok := s.Get(room, id); !ok {
			err = fmt.Errorf("room task not found")
		}
	}
	if err == nil {
		tasks := s.List("")
		err = s.core.Mutate(id, func(a *taskcore.Snapshot, allocate func() (string, error)) error {
			return fn(roster, tasks, a, allocate)
		})
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.RetryDelivery(room)
}

func firstParent(tasks []taskcore.Task, room string) string {
	parents := []taskcore.Task{}
	for _, t := range tasks {
		if t.RoomID == room && t.ParentID == "" && !Terminal(t.Status) {
			parents = append(parents, t)
		}
	}
	sort.Slice(parents, func(i, j int) bool {
		if parents[i].CreatedAt.Equal(parents[j].CreatedAt) {
			return parents[i].ID < parents[j].ID
		}
		return parents[i].CreatedAt.Before(parents[j].CreatedAt)
	})
	if len(parents) == 0 {
		return ""
	}
	return parents[0].ID
}

func (s *Service) Plan(room, id, summary string, items []PlanItem, autoStart bool) error {
	return s.change(room, id, func(r Roster, tasks []taskcore.Task, a *taskcore.Snapshot, allocate func() (string, error)) error {
		if a.Root.ID != id || Terminal(a.Root.Status) || a.Root.Status == taskcore.StatusStopping {
			return fmt.Errorf("only a manager parent task can be planned")
		}
		if err := validatePlan(items, r); err != nil {
			return err
		}
		if len(a.Children) > 0 {
			return fmt.Errorf("task already planned; inspect existing tasks")
		}
		ids := map[string]string{}
		for _, item := range items {
			value, err := allocate()
			if err != nil {
				return err
			}
			ids[item.IDRef] = value
		}
		now := time.Now().UTC()
		for i, item := range items {
			t := taskcore.Task{ID: ids[item.IDRef], ParentID: id, AssignmentType: a.Root.AssignmentType, AssignmentID: a.Root.AssignmentID, RoomID: room, ExecutionChannel: "csgclaw", Title: strings.TrimSpace(item.Title), Body: item.Body, AssignedTo: item.AssignedTo, CreatedBy: r.ManagerID, Status: taskcore.StatusPending, Priority: len(items) - i, CreatedAt: now, UpdatedAt: now}
			for _, dep := range item.DependsOnRefs {
				t.DependsOn = append(t.DependsOn, ids[dep])
			}
			a.Children = append(a.Children, t)
			appendEvent(a, taskcore.EventTaskCreated, &t, r.ManagerID, t.Title)
		}
		a.Root.PlanSummary, a.Root.UpdatedAt = summary, now
		if autoStart && firstParent(tasks, room) == id {
			a.Root.Status = taskcore.StatusInProgress
		}
		appendEvent(a, EventPlanned, &a.Root, r.ManagerID, first(summary, a.Root.Title))
		return nil
	})
}

// Start activates a queued parent. Only explicit Manager Dispatch starts workers.
func (s *Service) Start(room, id string) error {
	return s.change(room, id, func(r Roster, tasks []taskcore.Task, a *taskcore.Snapshot, _ func() (string, error)) error {
		if a.Root.ID != id || Terminal(a.Root.Status) || a.Root.Status == taskcore.StatusStopping || len(a.Children) == 0 {
			return fmt.Errorf("a planned parent is required")
		}
		if firstParent(tasks, room) != id {
			return fmt.Errorf("another parent task is ahead in the room queue")
		}
		if a.Root.Status == taskcore.StatusQueued {
			a.Root.Status = taskcore.StatusInProgress
		}
		return nil
	})
}

func appendEvent(a *taskcore.Snapshot, kind string, t *taskcore.Task, actor, summary string) {
	a.Events = append(a.Events, taskcore.TaskEvent{Type: kind, TaskID: t.ID, TargetID: t.AssignedTo, ActorID: actor, Summary: summary, Attempt: t.Attempt, CreatedAt: time.Now().UTC()})
}
func feedback(a *taskcore.Snapshot, r Roster, t *taskcore.Task, summary string) {
	a.Events = append(a.Events, taskcore.TaskEvent{Type: EventFeedback, TaskID: t.ID, TargetID: r.ManagerID, ActorID: t.AssignedTo, Summary: summary, Attempt: t.Attempt, CreatedAt: time.Now().UTC()})
}
func child(a *taskcore.Snapshot, id string) *taskcore.Task {
	for i := range a.Children {
		if a.Children[i].ID == id {
			return &a.Children[i]
		}
	}
	return nil
}
func dependenciesReady(a *taskcore.Snapshot, t *taskcore.Task) bool {
	for _, id := range t.DependsOn {
		dep := child(a, id)
		if dep == nil || dep.Status != taskcore.StatusCompleted {
			return false
		}
	}
	return true
}
func running(t taskcore.Task) bool {
	return t.Status == taskcore.StatusInProgress || t.Status == taskcore.StatusAssigned && t.DispatchedAt != nil
}

// Dispatch is a Manager decision. A dependency becoming ready never calls it.
func (s *Service) Dispatch(room, id, assignee string) error {
	return s.change(room, id, func(r Roster, tasks []taskcore.Task, a *taskcore.Snapshot, _ func() (string, error)) error {
		t := child(a, id)
		if t == nil || t.ParentID != a.Root.ID {
			return fmt.Errorf("first-version dispatch requires a direct worker child")
		}
		if firstParent(tasks, room) != a.Root.ID || Terminal(a.Root.Status) || a.Root.Status == taskcore.StatusStopping {
			return fmt.Errorf("parent task is not active in the room queue")
		}
		if t.RecoveryRequired {
			return fmt.Errorf("verify the previous execution and resolve recovery before dispatch")
		}
		if assignee == "" {
			assignee = t.AssignedTo
		}
		if !worker(r, assignee) {
			return fmt.Errorf("assignee must be a current room worker")
		}
		if running(*t) {
			if assignee == t.AssignedTo {
				return nil
			}
			return fmt.Errorf("stop the active execution before reassignment")
		}
		if t.Status == taskcore.StatusCompleted || t.Status == taskcore.StatusReview {
			return fmt.Errorf("review the submitted result before dispatching again")
		}
		if !dependenciesReady(a, t) {
			return fmt.Errorf("waiting for predecessor acceptance by the manager")
		}
		for _, other := range tasks {
			if other.ID != id && other.AssignedTo == assignee && (running(other) || other.RecoveryRequired) {
				t.AssignedTo, t.Status = assignee, taskcore.StatusBlocked
				t.WaitingOnTaskID = other.ID
				t.Error = "worker is busy on task " + other.ID + "; waiting for capacity"
				appendEvent(a, taskcore.EventTaskBlocked, t, r.ManagerID, t.Error)
				return nil
			}
		}
		now := time.Now().UTC()
		t.AssignedTo, t.ClaimedBy, t.Status = assignee, "", taskcore.StatusAssigned
		t.Attempt++
		t.DispatchedAt, t.CompletedAt, t.UpdatedAt = &now, nil, now
		t.Error, t.Review, t.ReviewedBy, t.WaitingOnTaskID = "", "", "", ""
		a.Root.Status = taskcore.StatusInProgress
		appendEvent(a, EventDispatched, t, r.ManagerID, t.Title)
		return nil
	})
}

// Update is used only by trusted local stop handling and tests. Remote callers
// must supply the attempt from their dispatch through UpdateExecution.
func (s *Service) Update(room, id, actor, status, result, reason string) (taskcore.Task, error) {
	t, ok := s.Get(room, id)
	if !ok {
		return taskcore.Task{}, fmt.Errorf("task not found")
	}
	return s.UpdateExecution(room, id, actor, status, result, reason, t.Attempt)
}
func (s *Service) UpdateExecution(room, id, actor, status, result, reason string, attempt int) (taskcore.Task, error) {
	if status == taskcore.StatusCompleted {
		status = taskcore.StatusReview
	}
	err := s.change(room, id, func(r Roster, _ []taskcore.Task, a *taskcore.Snapshot, _ func() (string, error)) error {
		t := child(a, id)
		if t == nil || !worker(r, actor) || actor != t.AssignedTo {
			return fmt.Errorf("only the assigned current room worker can update a subtask")
		}
		if attempt < 1 || attempt != t.Attempt {
			return fmt.Errorf("stale or missing execution attempt")
		}
		if (t.Status == status || t.Status == taskcore.StatusCompleted && status == taskcore.StatusReview) && t.Result == result && t.Error == reason {
			return nil
		}
		if Terminal(a.Root.Status) || t.RecoveryRequired {
			return fmt.Errorf("parent execution has ended or execution requires recovery")
		}
		if status == taskcore.StatusInProgress {
			if a.Root.Status == taskcore.StatusStopping || t.Status != taskcore.StatusAssigned || t.DispatchedAt == nil || !dependenciesReady(a, t) {
				return fmt.Errorf("only a valid dispatched task with accepted dependencies can be claimed")
			}
			t.ClaimedBy = actor
		} else {
			if t.Status != taskcore.StatusInProgress {
				return fmt.Errorf("claim the current dispatch before reporting")
			}
			switch status {
			case taskcore.StatusReview:
				if strings.TrimSpace(result) == "" {
					return fmt.Errorf("result is required")
				}
			case taskcore.StatusFailed, taskcore.StatusBlocked, taskcore.StatusCancelled:
				if strings.TrimSpace(reason) == "" {
					return fmt.Errorf("reason is required")
				}
			default:
				return fmt.Errorf("unsupported worker status")
			}
		}
		t.Status, t.Result, t.Error, t.UpdatedAt = status, result, reason, time.Now().UTC()
		appendEvent(a, "task."+status, t, actor, first(result, reason))
		if status != taskcore.StatusInProgress {
			feedback(a, r, t, fmt.Sprintf("%s: %s. %s", id, status, first(result, reason)))
		}
		return nil
	})
	t, _ := s.Get(room, id)
	return t, err
}

func (s *Service) Review(room, id string, attempt int, accept bool, summary string) error {
	return s.change(room, id, func(r Roster, _ []taskcore.Task, a *taskcore.Snapshot, _ func() (string, error)) error {
		t := child(a, id)
		if t == nil || attempt != t.Attempt || attempt < 1 || strings.TrimSpace(summary) == "" {
			return fmt.Errorf("task, current attempt and review summary are required")
		}
		if t.Review == summary && t.ReviewedBy == r.ManagerID && ((accept && t.Status == taskcore.StatusCompleted) || (!accept && t.Status == taskcore.StatusBlocked)) {
			return nil
		}
		// A Worker may classify failing tests as failed even though it delivered
		// a complete defect report. Manager can explicitly accept that work.
		reportedFailure := t.Status == taskcore.StatusFailed && strings.TrimSpace(t.Result) != ""
		if Terminal(a.Root.Status) || a.Root.Status == taskcore.StatusStopping || t.RecoveryRequired || (t.Status != taskcore.StatusReview && !reportedFailure) {
			return fmt.Errorf("only submitted results can be reviewed")
		}
		t.Review, t.ReviewedBy = summary, r.ManagerID
		now := time.Now().UTC()
		t.UpdatedAt = now
		if accept {
			t.Status, t.CompletedAt = taskcore.StatusCompleted, &now
			t.Error = ""
		} else {
			t.Status, t.Error = taskcore.StatusBlocked, summary
		}
		appendEvent(a, EventReviewed, t, r.ManagerID, summary)
		all := len(a.Children) > 0
		for _, c := range a.Children {
			all = all && c.Status == taskcore.StatusCompleted
		}
		if all {
			a.Root.Status = taskcore.StatusReview
		}
		return nil
	})
}

func (s *Service) Report(room, id, outcome, summary string) (taskcore.Task, error) {
	err := s.change(room, id, func(r Roster, _ []taskcore.Task, a *taskcore.Snapshot, _ func() (string, error)) error {
		if a.Root.ID != id || strings.TrimSpace(summary) == "" {
			return fmt.Errorf("parent task and summary are required")
		}
		if a.Root.Report != "" {
			if a.Root.Report != summary || a.Root.GoalOutcome != outcome {
				return fmt.Errorf("parent already has a different final report")
			}
			return nil
		}
		if a.Root.Status == taskcore.StatusStopping && outcome != "stopped" {
			return fmt.Errorf("stopping parent requires a stopped outcome")
		}
		switch outcome {
		case "succeeded", "issues", "failed", "stopped":
		default:
			return fmt.Errorf("invalid goal outcome")
		}
		for _, c := range a.Children {
			if running(c) || c.RecoveryRequired {
				return fmt.Errorf("stop or verify previous workers before closing the parent")
			}
			if (outcome == "succeeded" || outcome == "issues") && c.Status != taskcore.StatusCompleted {
				return fmt.Errorf("all child tasks must be accepted before closing with succeeded/issues; review submitted work and append repair/regression tasks to this parent while work remains")
			}
		}
		if outcome == "succeeded" && len(a.Children) == 0 {
			return fmt.Errorf("a worker plan is required before success")
		}
		now := time.Now().UTC()
		for i := range a.Children {
			c := &a.Children[i]
			if !Terminal(c.Status) {
				c.Status, c.Error = taskcore.StatusCancelled, "parent closed by manager"
				appendEvent(a, taskcore.EventTaskCancelled, c, r.ManagerID, c.Error)
			}
		}
		a.Root.Status = taskcore.StatusCompleted
		if outcome == "failed" {
			a.Root.Status = taskcore.StatusFailed
		}
		if outcome == "stopped" {
			a.Root.Status = taskcore.StatusCancelled
		}
		a.Root.CompletedAt, a.Root.UpdatedAt = &now, now
		a.Root.GoalOutcome, a.Root.Report, a.Root.ReportStatus = outcome, summary, "pending"
		appendEvent(a, EventReported, &a.Root, r.ManagerID, summary)
		return nil
	})
	t, _ := s.Get(room, id)
	return t, err
}

// recordNext is replay-safe, including recovery after a saved report was delivered.
// It only wakes the Manager; it never executes the queued parent.
func (s *Service) recordNext(room string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks := s.List(room)
	delivered := false
	for _, t := range tasks {
		delivered = delivered || t.ParentID == "" && t.ReportStatus == "delivered"
	}
	id := firstParent(tasks, room)
	if !delivered || id == "" {
		return false, nil
	}
	r, err := s.members(room)
	if err != nil {
		return false, err
	}
	created := false
	err = s.core.Mutate(id, func(a *taskcore.Snapshot, _ func() (string, error)) error {
		if a.Root.Status != taskcore.StatusQueued {
			return nil
		}
		for _, e := range a.Events {
			if e.Type == EventFeedback && e.TaskID == id {
				return nil
			}
		}
		feedback(a, r, &a.Root, "Previous parent ended. Review this queued parent, plan/start it and explicitly dispatch eligible worker tasks.")
		created = true
		return nil
	})
	return created, err
}
func (s *Service) MembersChanged(room string) error {
	for _, root := range s.List(room) {
		if root.ParentID != "" || Terminal(root.Status) {
			continue
		}
		if err := s.change(room, root.ID, func(r Roster, _ []taskcore.Task, a *taskcore.Snapshot, _ func() (string, error)) error {
			for i := range a.Children {
				t := &a.Children[i]
				if !Terminal(t.Status) && t.Status != taskcore.StatusBlocked && !worker(r, t.AssignedTo) {
					t.Status, t.Error = taskcore.StatusBlocked, "assigned worker is no longer a room member"
					feedback(a, r, t, t.ID+": "+t.Error)
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) WorkerStopped(room, id, actor string, attempt int) error {
	if id == "" {
		return nil
	}
	return s.change(room, id, func(r Roster, _ []taskcore.Task, a *taskcore.Snapshot, _ func() (string, error)) error {
		t := child(a, id)
		if t == nil || t.AssignedTo != actor || t.Attempt != attempt || attempt < 1 {
			return fmt.Errorf("stale stop confirmation")
		}
		if !running(*t) {
			return nil
		}
		t.Status, t.Error = taskcore.StatusCancelled, "stopped by user"
		appendEvent(a, taskcore.EventTaskCancelled, t, actor, t.Error)
		feedback(a, r, t, t.ID+": stopped; inspect preserved results and decide whether to continue or report the parent")
		return nil
	})
}
func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
