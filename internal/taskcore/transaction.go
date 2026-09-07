package taskcore

import (
	"fmt"
	"reflect"
)

// Mutate applies an aggregate change under the task lock. The callback must not
// call Service methods or external systems. IDs may be consumed on rollback;
// state and events become visible only after persistence succeeds.
func (s *Service) Mutate(taskID string, change func(*Snapshot, func() (string, error)) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rootID, _, err := s.requireTaskLocked(taskID)
	if err != nil {
		return err
	}
	next := s.snapshotLocked(rootID)
	root := next.Root
	before := s.snapshotLocked(rootID)
	start := len(next.Events)
	if err := change(&next, s.nextTaskIdentifier); err != nil {
		return err
	}
	if next.Root.ID != rootID || next.Root.ParentID != "" || next.Root.AssignmentType != root.AssignmentType || next.Root.AssignmentID != root.AssignmentID || next.Root.RoomID != root.RoomID {
		return fmt.Errorf("task aggregate identity cannot change")
	}
	seen := map[string]bool{rootID: true}
	parents := map[string]string{}
	for _, child := range next.Children {
		if child.ID == "" || seen[child.ID] || child.ParentID == "" || child.AssignmentType != root.AssignmentType || child.AssignmentID != root.AssignmentID || child.RoomID != root.RoomID {
			return fmt.Errorf("invalid child task identity")
		}
		if owner, _, err := s.requireTaskLocked(child.ID); err == nil && owner != rootID {
			return fmt.Errorf("child ID belongs to another aggregate")
		}
		seen[child.ID] = true
		parents[child.ID] = child.ParentID
	}
	for id := range parents {
		path := map[string]bool{}
		for current := id; current != rootID; current = parents[current] {
			if !seen[current] || path[current] {
				return fmt.Errorf("task parent is missing or cyclic")
			}
			path[current] = true
		}
	}
	if len(next.Events) < start || !reflect.DeepEqual(next.Events[:start], before.Events) {
		return fmt.Errorf("task events are append-only")
	}
	if !reflect.DeepEqual(next.Approvals, before.Approvals) || !reflect.DeepEqual(next.Presence, before.Presence) {
		return fmt.Errorf("use the approval and presence operations to change those records")
	}
	seq := s.nextSeq
	for i := start; i < len(next.Events); i++ {
		seq++
		e := &next.Events[i]
		e.Seq, e.AssignmentType, e.AssignmentID = seq, root.AssignmentType, root.AssignmentID
		e.RoomID, e.Channel = root.RoomID, root.ExecutionChannel
		if e.CreatedAt.IsZero() {
			e.CreatedAt = s.now()
		}
	}
	if s.store != nil {
		if err := s.store.SaveSnapshot(next, next.Events[start:]); err != nil {
			return err
		}
	}
	s.nextSeq = seq
	copyRoot := cloneTask(next.Root)
	s.roots[rootID] = &copyRoot
	children := make(map[string]*Task, len(next.Children))
	for _, child := range next.Children {
		c := cloneTask(child)
		children[c.ID] = &c
	}
	s.children[rootID] = children
	s.events[rootID] = cloneEvents(next.Events)
	return nil
}
