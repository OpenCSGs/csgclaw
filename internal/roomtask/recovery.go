package roomtask

import (
	"csgclaw/internal/taskcore"
	"fmt"
	"strings"
	"time"
)

// releaseCapacity records a new coordination event in each affected room. A
// released slot is not permission to execute: the Manager must dispatch again.
func (s *Service) releaseCapacity() error {
	rooms := map[string]bool{}
	s.mu.Lock()
	tasks := s.List("")
	byID := map[string]taskcore.Task{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	var failure error
	for _, t := range tasks {
		if t.WaitingOnTaskID == "" || t.Status != taskcore.StatusBlocked {
			continue
		}
		occupied, found := byID[t.WaitingOnTaskID]
		if found && (running(occupied) || occupied.RecoveryRequired) {
			continue
		}
		r, err := s.members(t.RoomID)
		if err != nil {
			continue
		}
		err = s.core.Mutate(t.ID, func(a *taskcore.Snapshot, _ func() (string, error)) error {
			c := child(a, t.ID)
			if c == nil || Terminal(a.Root.Status) || a.Root.Status == taskcore.StatusStopping {
				return nil
			}
			c.Status, c.WaitingOnTaskID, c.Error = taskcore.StatusPending, "", ""
			feedback(a, r, c, c.ID+": worker capacity released; inspect current dependencies and explicitly dispatch if still appropriate")
			rooms[t.RoomID] = true
			return nil
		})
		if err != nil {
			failure = err
			break
		}
	}
	s.mu.Unlock()
	if failure != nil {
		return failure
	}
	for room := range rooms {
		if err := s.RetryDelivery(room); err != nil {
			return err
		}
	}
	return nil
}

// RequestStop revokes work not yet claimed. Running work remains running until
// the execution controller confirms its exact attempt stopped (or it reports).
func (s *Service) RequestStop(room, id string) error {
	return s.change(room, id, func(r Roster, _ []taskcore.Task, a *taskcore.Snapshot, _ func() (string, error)) error {
		if a.Root.ID != id {
			return fmt.Errorf("stop requires a parent task")
		}
		if Terminal(a.Root.Status) || a.Root.Status == taskcore.StatusStopping {
			return nil
		}
		a.Root.Status = taskcore.StatusStopping
		a.Root.UpdatedAt = time.Now().UTC()
		for i := range a.Children {
			c := &a.Children[i]
			if c.Status == taskcore.StatusInProgress || c.RecoveryRequired || Terminal(c.Status) {
				continue
			}
			c.Status, c.Error, c.WaitingOnTaskID = taskcore.StatusCancelled, "parent stop requested", ""
			appendEvent(a, taskcore.EventTaskCancelled, c, r.ManagerID, c.Error)
		}
		feedback(a, r, &a.Root, "Parent stop requested. Do not dispatch more work. Await exact execution stop confirmations and preserve all existing results, then report the parent with outcome stopped.")
		return nil
	})
}

// Recover is called once when the host opens its persisted task store. It never
// replays an uncertain execution. Manager verification is required before retry.
func (s *Service) Recover() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, root := range s.List("") {
		if root.ParentID != "" || Terminal(root.Status) {
			continue
		}
		r, err := s.members(root.RoomID)
		if err != nil {
			continue
		}
		if err := s.core.Mutate(root.ID, func(a *taskcore.Snapshot, _ func() (string, error)) error {
			for i := range a.Children {
				c := &a.Children[i]
				if !running(*c) {
					continue
				}
				c.Status, c.RecoveryRequired = taskcore.StatusBlocked, true
				c.Error = "service restarted; verify the previous execution and saved artifacts before resuming"
				feedback(a, r, c, c.ID+": "+c.Error)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ResolveRecovery(room, id string, attempt int, assessment string) error {
	return s.change(room, id, func(r Roster, _ []taskcore.Task, a *taskcore.Snapshot, _ func() (string, error)) error {
		c := child(a, id)
		if c == nil || c.Attempt != attempt || attempt < 1 || strings.TrimSpace(assessment) == "" {
			return fmt.Errorf("current attempt and recovery assessment are required")
		}
		if !c.RecoveryRequired {
			return fmt.Errorf("task is not awaiting recovery")
		}
		c.RecoveryRequired = false
		c.Error = assessment
		if a.Root.Status == taskcore.StatusStopping {
			c.Status = taskcore.StatusCancelled
		}
		appendEvent(a, "task.recovered", c, r.ManagerID, assessment)
		return nil
	})
}

func (s *Service) Events(room, id string) []taskcore.TaskEvent {
	t, ok := s.Get(room, id)
	if !ok {
		return nil
	}
	for t.ParentID != "" {
		t, ok = s.Get(room, t.ParentID)
		if !ok {
			return nil
		}
	}
	return s.core.Events(t.ID)
}

func (s *Service) SourceAttempt(room, source string) int {
	for _, t := range s.List(room) {
		if t.ParentID != "" {
			continue
		}
		for _, e := range s.core.Events(t.ID) {
			if eventID(t.ID, e.Seq) == source {
				return e.Attempt
			}
		}
	}
	return 0
}
