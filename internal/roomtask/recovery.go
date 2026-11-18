package roomtask

import (
	"csgclaw/internal/taskcore"
	"fmt"
	"slices"
	"strings"
	"time"
)

// RequestStop revokes work not yet claimed. Running work remains running until
// the execution controller confirms its exact attempt stopped (or it reports).
func (s *Service) RequestStop(room, id string) error {
	return s.change(room, id, func(r Roster, a *taskcore.Snapshot, _ func() (string, error)) error {
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
	rooms, err := s.recoverUnfinished()
	if err != nil {
		return err
	}
	for room := range rooms {
		if err := s.RetryDelivery(room); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) recoverUnfinished() (map[string]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rooms := map[string]bool{}
	for _, root := range s.List("") {
		if root.ParentID != "" || Terminal(root.Status) {
			continue
		}
		r, err := s.members(root.RoomID)
		if err != nil {
			continue
		}
		if err := s.core.Mutate(root.ID, func(a *taskcore.Snapshot, _ func() (string, error)) error {
			// Older versions queued goals behind the oldest parent in the room.
			if a.Root.Status == taskcore.StatusQueued {
				a.Root.Status, a.Root.UpdatedAt = taskcore.StatusAssigned, time.Now().UTC()
				feedback(a, r, &a.Root, "Review this goal and explicitly dispatch ready work independently of other room goals.")
			}
			for i := range a.Children {
				c := &a.Children[i]
				// A legacy wait for another task has no outstanding execution to replay.
				if c.Status == taskcore.StatusBlocked && c.WaitingOnTaskID != "" && !c.RecoveryRequired && a.Root.Status != taskcore.StatusStopping {
					c.Status, c.WaitingOnTaskID, c.Error, c.UpdatedAt = taskcore.StatusPending, "", "", time.Now().UTC()
					feedback(a, r, c, c.ID+": previous worker capacity wait cleared; check dependencies and explicitly dispatch if appropriate")
				}
				if !running(*c) {
					continue
				}
				c.Status, c.RecoveryRequired = taskcore.StatusBlocked, true
				c.Error = "service restarted; verify the previous execution and saved artifacts before resuming"
				c.UpdatedAt = time.Now().UTC()
				feedback(a, r, c, c.ID+": "+c.Error)
			}
			return nil
		}); err != nil {
			return nil, err
		}
		// Replay stable event IDs even if a previous startup saved the recovery
		// feedback and exited before delivering it.
		rooms[root.RoomID] = true
	}
	return rooms, nil
}

func (s *Service) ResolveRecovery(room, id string, attempt int, assessment string) error {
	return s.change(room, id, func(r Roster, a *taskcore.Snapshot, _ func() (string, error)) error {
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

// CancelAgentTasks runs after the Agent Engine has drained the archived agent.
// Other workers and the parent task keep their existing progress.
func (s *Service) CancelAgentTasks(actors []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, task := range s.List("") {
		if task.ParentID == "" || !slices.Contains(actors, task.AssignedTo) || Terminal(task.Status) {
			continue
		}
		if err := s.core.Mutate(task.ID, func(a *taskcore.Snapshot, _ func() (string, error)) error {
			t := child(a, task.ID)
			if t == nil || Terminal(t.Status) {
				return nil
			}
			t.Status, t.Error, t.WaitingOnTaskID = taskcore.StatusCancelled, "agent archived", ""
			t.RecoveryRequired = false
			t.UpdatedAt = time.Now().UTC()
			appendEvent(a, taskcore.EventTaskCancelled, t, task.AssignedTo, t.Error)
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
