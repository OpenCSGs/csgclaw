package roomtask

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/taskcore"
)

type AutonomousService struct {
	core *taskcore.Service
	mu   sync.Mutex // orders task commits and room notices, and fences deletion
	send func(taskcore.Task, taskcore.TaskEvent) error
}

func NewAutonomousService(core *taskcore.Service, send func(taskcore.Task, taskcore.TaskEvent) error) *AutonomousService {
	return &AutonomousService{core: core, send: send}
}

type AutonomousDecision struct {
	Accepted            bool   `json:"accepted"`
	Reason              string `json:"reason,omitempty"`
	NotificationPending bool   `json:"notification_pending,omitempty"`
}

type AutonomousPart struct {
	Title string `json:"title"`
	Scope string `json:"scope"`
}

func (s *AutonomousService) List(roomID string) []taskcore.Task {
	items := make([]taskcore.Task, 0)
	for _, t := range s.core.ListGlobal() {
		if t.RoomMode == taskcore.RoomModeAutonomous && (roomID == "" || t.RoomID == roomID) {
			items = append(items, t)
		}
	}
	return items
}

func (s *AutonomousService) Get(id string) (taskcore.Task, bool) {
	t, ok := s.core.Get(id)
	return t, ok && t.RoomMode == taskcore.RoomModeAutonomous
}

func (s *AutonomousService) Events(task taskcore.Task) []taskcore.TaskEvent {
	root := task.ID
	if task.ParentID != "" {
		root = task.ParentID
	}
	return s.core.Events(root)
}

// WithRoomMutation fences removal of a room or member against admission of new
// work. Its callback must not invoke another autonomous mutation.
func (s *AutonomousService) WithRoomMutation(change func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return change()
}

var ErrAutonomousMemberHasWork = errors.New("ask the member to hand off their ongoing work before removing them")

// CheckMemberRemoval is called with the room mutation lock held.
func (s *AutonomousService) CheckMemberRemoval(roomID string, matches func(string) bool) error {
	for _, task := range s.List(roomID) {
		if task.AssignedTo != "" && matches(task.AssignedTo) && !Terminal(task.Status) {
			return ErrAutonomousMemberHasWork
		}
	}
	return nil
}

// ReleaseOwner makes unfinished work recoverable when a member is archived.
// The parent of a split stays a coordination record while its parts continue.
func (s *AutonomousService) ReleaseOwner(matches func(string) bool) error {
	return s.WithRoomMutation(func() error {
		for _, task := range s.List("") {
			if !matches(task.AssignedTo) || task.Status == taskcore.StatusCompleted || task.Status == taskcore.StatusCancelled {
				continue
			}
			if err := s.core.Mutate(task.ID, func(a *taskcore.Snapshot, _ func() (string, error)) error {
				target := &a.Root
				if task.ParentID != "" {
					target = child(a, task.ID)
				}
				target.AssignedTo, target.ClaimedBy, target.Error = "", "", ""
				target.Status, target.UpdatedAt = taskcore.StatusPending, time.Now().UTC()
				if target.ID == a.Root.ID && len(a.Children) != 0 {
					target.Status = taskcore.StatusBlocked
				}
				appendAutonomousEvent(a, *target, "task.released", task.AssignedTo, task.ThreadRootID, "previous owner is no longer available")
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *AutonomousService) Decide(source taskcore.Task, actor, thread string, parts []AutonomousPart) (string, AutonomousDecision, error) {
	if parts != nil {
		if err := validateAutonomousParts(parts); err != nil {
			return "", AutonomousDecision{}, err
		}
	}
	var rootID, eventType string
	err := s.core.MutateAutonomousRequest(source, func(a *taskcore.Snapshot, allocate func() (string, error)) error {
		root := &a.Root
		rootID = root.ID
		if parts == nil {
			eventType = taskcore.EventTaskClaimed
			if len(a.Children) != 0 {
				return fmt.Errorf("%w: this request is split; claim a child task by ID", taskcore.ErrTransitionInvalid)
			}
			return claimAutonomous(a, root, actor, thread)
		}
		eventType = EventPlanned
		if len(a.Children) != 0 && sameAutonomousPlan(a, actor, parts) {
			return nil
		}
		ownedHere := root.Status == taskcore.StatusInProgress && root.ClaimedBy == actor &&
			autonomousClaimInThread(a.Events, root.ID, actor, thread)
		if len(a.Children) != 0 || (root.Status != taskcore.StatusPending && !ownedHere) {
			return fmt.Errorf("%w: a whole-task owner or split is already recorded", taskcore.ErrTransitionInvalid)
		}
		now := time.Now().UTC()
		root.AssignedTo, root.Status, root.UpdatedAt = actor, taskcore.StatusInProgress, now
		root.ClaimedBy = ""
		for _, part := range parts {
			id, err := allocate()
			if err != nil {
				return err
			}
			child := taskcore.Task{
				ID: id, ParentID: root.ID, AssignmentType: root.AssignmentType, AssignmentID: root.AssignmentID,
				RoomID: root.RoomID, RoomMode: root.RoomMode, ThreadRootID: root.ThreadRootID, ExecutionChannel: root.ExecutionChannel,
				Title: strings.TrimSpace(part.Title), Body: strings.TrimSpace(part.Scope), CreatedBy: actor,
				Status: taskcore.StatusPending, CreatedAt: now, UpdatedAt: now,
			}
			a.Children = append(a.Children, child)
			appendAutonomousEvent(a, child, taskcore.EventTaskCreated, actor, thread, child.Title)
		}
		appendAutonomousEvent(a, *root, EventPlanned, actor, thread, root.Title)
		return nil
	})
	result, err := s.decision(rootID, rootID, actor, eventType, err)
	return rootID, result, err
}

func validateAutonomousParts(parts []AutonomousPart) error {
	if len(parts) < 2 {
		return fmt.Errorf("a split needs at least two independently verifiable parts")
	}
	titles := make(map[string]bool, len(parts))
	for _, part := range parts {
		title := strings.ToLower(strings.Join(strings.Fields(part.Title), " "))
		if title == "" || strings.TrimSpace(part.Scope) == "" || titles[title] {
			return fmt.Errorf("each part needs a unique title and a description of its deliverable and boundaries")
		}
		titles[title] = true
	}
	return nil
}

func sameAutonomousPlan(a *taskcore.Snapshot, actor string, parts []AutonomousPart) bool {
	if a.Root.AssignedTo != actor || len(a.Children) != len(parts) {
		return false
	}
	scopes := make(map[string]string, len(parts))
	for _, child := range a.Children {
		scopes[child.Title] = child.Body
	}
	for _, part := range parts {
		if scopes[strings.TrimSpace(part.Title)] != strings.TrimSpace(part.Scope) {
			return false
		}
	}
	return true
}

func (s *AutonomousService) Claim(task taskcore.Task, actor, thread string) (AutonomousDecision, error) {
	err := s.core.Mutate(task.ID, func(a *taskcore.Snapshot, _ func() (string, error)) error {
		if task.ParentID == "" {
			if len(a.Children) != 0 {
				if a.Root.AssignedTo == "" {
					a.Root.AssignedTo, a.Root.Status, a.Root.UpdatedAt = actor, taskcore.StatusInProgress, time.Now().UTC()
					appendAutonomousEvent(a, a.Root, taskcore.EventTaskClaimed, actor, thread, "coordinate and integrate the existing split")
					return nil
				}
				return fmt.Errorf("%w: this request is split; claim a child task by ID", taskcore.ErrTransitionInvalid)
			}
			return claimAutonomous(a, &a.Root, actor, thread)
		}
		return claimAutonomous(a, child(a, task.ID), actor, thread)
	})
	return s.decision(autonomousRootID(task), task.ID, actor, taskcore.EventTaskClaimed, err)
}

func claimAutonomous(a *taskcore.Snapshot, task *taskcore.Task, actor, thread string) error {
	if task == nil {
		return taskcore.ErrTaskNotFound
	}
	if task.Status == taskcore.StatusInProgress && task.ClaimedBy == actor {
		if autonomousClaimInThread(a.Events, task.ID, actor, thread) {
			return nil
		}
		return fmt.Errorf("%w: %s was already claimed in another conversation", taskcore.ErrTransitionInvalid, task.ID)
	}
	assignedHere := task.Status == taskcore.StatusAssigned && task.AssignedTo == actor
	resume := task.Status == taskcore.StatusBlocked && task.ClaimedBy == actor
	if task.Status != taskcore.StatusPending && !assignedHere && !resume {
		return fmt.Errorf("%w: %s is %s (owner: %s)", taskcore.ErrTransitionInvalid, task.ID, task.Status, task.AssignedTo)
	}
	task.AssignedTo, task.ClaimedBy = actor, actor
	task.Status, task.Error, task.UpdatedAt = taskcore.StatusInProgress, "", time.Now().UTC()
	appendAutonomousEvent(a, *task, taskcore.EventTaskClaimed, actor, thread, task.Title)
	return nil
}

func autonomousClaimInThread(events []taskcore.TaskEvent, id, actor, thread string) bool {
	for i := len(events) - 1; i >= 0; i-- {
		if e := events[i]; e.TaskID == id && e.Type == taskcore.EventTaskClaimed {
			return e.ActorID == actor && e.ThreadRootID == thread
		}
	}
	return false
}

func (s *AutonomousService) Update(task taskcore.Task, actor, thread, status, summary, nextOwner string) (AutonomousDecision, error) {
	eventType, ok := map[string]string{
		taskcore.StatusCompleted: taskcore.EventTaskCompleted,
		taskcore.StatusBlocked:   taskcore.EventTaskBlocked,
		taskcore.StatusFailed:    taskcore.EventTaskFailed,
		"released":               "task.released",
		"handed_off":             taskcore.EventTaskAssigned,
	}[status]
	if !ok || summary == "" || (status == "handed_off") != (nextOwner != "") || actor == nextOwner {
		return AutonomousDecision{}, fmt.Errorf("provide a valid status and summary; handoff also needs a different next_owner")
	}
	err := s.core.Mutate(task.ID, func(a *taskcore.Snapshot, _ func() (string, error)) error {
		target := &a.Root
		if task.ParentID != "" {
			target = child(a, task.ID)
			if target == nil {
				return taskcore.ErrTaskNotFound
			}
		}
		isCoordinator := target.ID == a.Root.ID && len(a.Children) != 0
		if target.AssignedTo != actor {
			released := status == "released" && target.Status == taskcore.StatusPending
			handedOff := status == "handed_off" && target.AssignedTo == nextOwner
			if (released || handedOff) && matchingAutonomousEvent(a.Events, task.ID, actor, eventType, summary) {
				return nil
			}
			return fmt.Errorf("%w: only the current owner may update %s", taskcore.ErrTransitionInvalid, task.ID)
		}
		if target.Status == status && matchingAutonomousEvent(a.Events, task.ID, actor, eventType, summary) {
			return nil
		}
		canRelinquish := status == "released" || status == "handed_off"
		if target.Status != taskcore.StatusInProgress && target.Status != taskcore.StatusBlocked &&
			!(canRelinquish && (target.Status == taskcore.StatusFailed || target.Status == taskcore.StatusAssigned)) {
			return fmt.Errorf("%w: %s is %s", taskcore.ErrTransitionInvalid, task.ID, target.Status)
		}
		if isCoordinator && status != taskcore.StatusBlocked && status != "handed_off" {
			if status != taskcore.StatusCompleted {
				return fmt.Errorf("%w: update the affected part of a split task", taskcore.ErrTransitionInvalid)
			}
			for _, part := range a.Children {
				if part.Status != taskcore.StatusCompleted {
					return fmt.Errorf("%w: %s has not been delivered", taskcore.ErrTransitionInvalid, part.ID)
				}
			}
		}
		now := time.Now().UTC()
		target.Status, target.UpdatedAt = status, now
		switch status {
		case taskcore.StatusCompleted:
			target.Result, target.Error, target.CompletedAt = summary, "", &now
		case "released":
			target.Status, target.AssignedTo, target.ClaimedBy, target.Error = taskcore.StatusPending, "", "", ""
		case "handed_off":
			target.Status, target.AssignedTo, target.ClaimedBy, target.Error = taskcore.StatusAssigned, nextOwner, "", ""
			if isCoordinator {
				target.Status = taskcore.StatusInProgress
			}
		default:
			target.Error = summary
		}
		appendAutonomousEvent(a, *target, eventType, actor, thread, summary)
		return nil
	})
	return s.decision(autonomousRootID(task), task.ID, actor, eventType, err)
}

func matchingAutonomousEvent(events []taskcore.TaskEvent, id, actor, kind, summary string) bool {
	for i := len(events) - 1; i >= 0; i-- {
		if e := events[i]; e.TaskID == id {
			return e.ActorID == actor && e.Type == kind && e.Summary == summary
		}
	}
	return false
}

func appendAutonomousEvent(a *taskcore.Snapshot, task taskcore.Task, kind, actor, thread, summary string) {
	a.Events = append(a.Events, taskcore.TaskEvent{
		Type: kind, TaskID: task.ID, ActorID: actor, TargetID: task.AssignedTo, ThreadRootID: thread, Summary: summary,
	})
}

func autonomousRootID(task taskcore.Task) string {
	if task.ParentID != "" {
		return task.ParentID
	}
	return task.ID
}

func (s *AutonomousService) decision(rootID, id, actor, kind string, err error) (AutonomousDecision, error) {
	if errors.Is(err, taskcore.ErrTransitionInvalid) {
		return AutonomousDecision{Reason: err.Error()}, nil
	}
	if err != nil {
		return AutonomousDecision{}, err
	}
	result := AutonomousDecision{Accepted: true}
	if s.send == nil {
		return result, nil
	}
	events := s.core.Events(rootID)
	for i := len(events) - 1; i >= 0; i-- {
		if e := events[i]; e.TaskID == id && e.ActorID == actor && e.Type == kind {
			task, _ := s.Get(id)
			result.NotificationPending = s.send(task, e) != nil
			break
		}
	}
	return result, nil
}
