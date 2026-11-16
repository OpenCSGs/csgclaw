package taskcore

import (
	"fmt"
	"reflect"
	"slices"
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
	return s.mutateSnapshotLocked(s.snapshotLocked(rootID), change)
}

// MutateAutonomousRequest serializes creation and the first decision for the
// same source message. The initial root and any children commit together.
func (s *Service) MutateAutonomousRequest(source Task, change func(*Snapshot, func() (string, error)) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if source.RoomID == "" || source.SourceMessageID == "" || source.Title == "" || source.CreatedBy == "" {
		return fmt.Errorf("room, source message, title and requester are required")
	}
	for _, root := range s.roots {
		if root.RoomMode == RoomModeAutonomous && root.RoomID == source.RoomID && root.SourceMessageID == source.SourceMessageID {
			if root.ThreadRootID != source.ThreadRootID {
				return ErrTaskNotFound
			}
			return s.mutateSnapshotLocked(s.snapshotLocked(root.ID), change)
		}
	}
	id, err := s.nextTaskIdentifier()
	if err != nil {
		return err
	}
	now := s.now()
	root := Task{
		ID: id, RoomID: source.RoomID, AssignmentType: AssignmentTypeRoom, AssignmentID: source.RoomID,
		RoomMode: RoomModeAutonomous, ThreadRootID: source.ThreadRootID, ExecutionChannel: "csgclaw",
		SourceMessageID: source.SourceMessageID, Title: source.Title, Body: source.Body,
		CreatedBy: source.CreatedBy, Status: StatusPending, CreatedAt: now, UpdatedAt: now,
	}
	return s.mutateSnapshotLocked(Snapshot{Root: root}, func(next *Snapshot, allocate func() (string, error)) error {
		next.Events = append(next.Events, TaskEvent{Type: EventTaskCreated, TaskID: id, ActorID: root.CreatedBy, Summary: root.Title})
		return change(next, allocate)
	})
}

func (s *Service) mutateSnapshotLocked(before Snapshot, change func(*Snapshot, func() (string, error)) error) error {
	root := before.Root
	rootID := root.ID
	next := before
	next.Children = slices.Clone(before.Children)
	for i := range next.Children {
		next.Children[i] = cloneTask(next.Children[i])
	}
	next.Root = cloneTask(before.Root)
	next.Events = cloneEvents(before.Events)
	next.Approvals = slices.Clone(before.Approvals)
	for i := range next.Approvals {
		next.Approvals[i] = cloneApproval(next.Approvals[i])
	}
	next.Presence = slices.Clone(before.Presence)
	start := len(next.Events)
	if err := change(&next, s.nextTaskIdentifier); err != nil {
		return err
	}
	if next.Root.ID != rootID || next.Root.ParentID != "" || !sameTaskScope(next.Root, root) || next.Root.RoomMode == RoomModeAutonomous && next.Root.SourceMessageID != root.SourceMessageID {
		return fmt.Errorf("task aggregate identity cannot change")
	}
	seen := map[string]bool{rootID: true}
	parents := map[string]string{}
	for _, child := range next.Children {
		if child.ID == "" || seen[child.ID] || child.ParentID == "" || !sameTaskScope(child, root) {
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
	if len(next.Events) < start || !slices.Equal(next.Events[:start], before.Events) {
		return fmt.Errorf("task events are append-only")
	}
	if !reflect.DeepEqual(next.Approvals, before.Approvals) || !reflect.DeepEqual(next.Presence, before.Presence) {
		return fmt.Errorf("use the approval and presence operations to change those records")
	}
	if s.roots[rootID] != nil && reflect.DeepEqual(next, before) {
		return nil
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

func sameTaskScope(left, right Task) bool {
	return left.AssignmentType == right.AssignmentType && left.AssignmentID == right.AssignmentID &&
		left.RoomID == right.RoomID && left.RoomMode == right.RoomMode && left.ThreadRootID == right.ThreadRootID
}
