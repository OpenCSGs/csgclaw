package roomtask

import (
	"fmt"
	"log/slog"
	"time"

	"csgclaw/internal/diagnostics"
	"csgclaw/internal/taskcore"
)

// DeliveryError means the ledger is saved but an IM projection could not be
// delivered. Retrying delivery preserves task IDs and execution attempts.
type DeliveryError struct {
	Projection Projection
	Err        error
}

func (e *DeliveryError) Error() string {
	p := e.Projection
	return fmt.Sprintf("task state saved; notification delivery failed (room=%s task=%s message=%s participant=%s attempt=%d kind=%s); run task retry-delivery --room %s without rerunning work or restarting Serve: %v", p.RoomID, p.TaskID, p.ID, p.TargetID, p.Attempt, p.Kind, p.RoomID, diagnostics.Redact(e.Err.Error()))
}
func (e *DeliveryError) Unwrap() error { return e.Err }

func eventID(root string, seq int64) string { return fmt.Sprintf("task:%s:event:%d", root, seq) }

func (s *Service) SourceTaskID(room, source string) string {
	for _, root := range s.List(room) {
		if root.ParentID != "" {
			continue
		}
		for _, e := range s.core.Events(root.ID) {
			if (e.Type == EventDispatched || e.Type == EventFeedback) && eventID(root.ID, e.Seq) == source {
				return e.TaskID
			}
		}
	}
	return ""
}

func (s *Service) projection(root taskcore.Task, e taskcore.TaskEvent, r Roster) (Projection, bool) {
	t, ok := s.Get(r.RoomID, e.TaskID)
	if !ok {
		return Projection{}, false
	}
	p := Projection{ID: eventID(root.ID, e.Seq), RoomID: r.RoomID, TaskID: e.TaskID, SenderID: r.ManagerID, TargetID: e.TargetID, Title: e.Summary, Content: e.Summary, Attempt: e.Attempt}
	switch e.Type {
	case EventDispatched:
		if t.Status != taskcore.StatusAssigned || t.DispatchedAt == nil || e.Attempt != t.Attempt || !worker(r, t.AssignedTo) {
			return p, false
		}
		p.Kind = "task_assigned"
		p.Title = t.ID + " · " + t.Title
		p.Content = "已分配任务：" + t.Title
	case EventFeedback:
		if root.ReportStatus == "delivered" || (t.ParentID != "" && (t.Attempt != e.Attempt || t.Status == taskcore.StatusCompleted)) {
			return p, false
		}
		p.Kind, p.TargetID, p.SenderID = "task_feedback", r.ManagerID, e.ActorID
		p.Title = t.ID + " · " + t.Title
		p.Content = "任务状态已更新，等待 Manager 处理：" + t.Title
	case EventPlanned:
		p.Kind, p.TargetID = "task_planned", ""
		p.Content = "已记录任务计划：" + first(root.PlanSummary, root.Title)
	case EventExtended:
		p.Kind, p.TargetID = "task_plan_updated", ""
		p.Content = "已补充子任务：" + e.Summary
	case EventReported:
		if root.ReportStatus == "delivered" {
			return p, false
		}
		p.Kind, p.TargetID = "task_reported", ""
	default:
		return p, false
	}
	return p, true
}

// Retry replays stable event IDs, not executions. IM and the participant bridge
// deduplicate those IDs. No task lock is held while invoking the delivery sink.
func (s *Service) RetryDelivery(room string) (deliveryErr error) {
	start := time.Now()
	count := 0
	defer func() {
		if elapsed := time.Since(start); elapsed >= time.Second {
			slog.Warn("room task delivery replay slow", "phase", "message_projection", "room_id", room, "projection_count", count, "duration", elapsed, "failed", deliveryErr != nil)
		}
	}()
	r, err := s.members(room)
	if err != nil {
		return err
	}
	if s.send == nil {
		return nil
	}
	for _, root := range s.List(room) {
		if root.ParentID != "" {
			continue
		}
		for _, e := range s.core.Events(root.ID) {
			p, ok := s.projection(root, e, r)
			if !ok {
				continue
			}
			count++
			if err := s.send(p); err != nil {
				failure := &DeliveryError{Projection: p, Err: err}
				slog.Error("room task notification delivery failed", "phase", "message_projection", "room_id", p.RoomID, "task_id", p.TaskID, "message_id", p.ID, "participant_id", p.TargetID, "attempt", p.Attempt, "kind", p.Kind, "task_state_saved", true, "duration", time.Since(start), "error", diagnostics.Redact(err.Error()), "action", "retry task delivery; do not rerun work or restart Serve")
				return failure
			}
			if e.Type == EventReported {
				if err := s.core.Mutate(root.ID, func(a *taskcore.Snapshot, _ func() (string, error)) error {
					a.Root.ReportStatus = "delivered"
					return nil
				}); err != nil {
					return err
				}
			}
		}
	}
	created, err := s.recordNext(room)
	if err != nil {
		return err
	}
	if created {
		return s.RetryDelivery(room)
	}
	if err := s.releaseCapacity(); err != nil {
		return err
	}
	return nil
}

// ResolveProjection derives identity from the durable ledger, never event-shaped
// text or a model-selected session ID.
func (s *Service) ResolveProjection(room, messageID, kind, target string) (string, bool) {
	r, err := s.members(room)
	if err != nil {
		return "", false
	}
	for _, root := range s.List(room) {
		if root.ParentID != "" {
			continue
		}
		for _, e := range s.core.Events(root.ID) {
			if eventID(root.ID, e.Seq) != messageID {
				continue
			}
			p, ok := s.projection(root, e, r)
			return p.TaskID, ok && p.Kind == kind && p.TargetID == target
		}
	}
	return "", false
}

// ResolveMention validates an explicitly selected task. A room message with no
// task selection remains ordinary chat; never guess the agent's newest task.
func (s *Service) ResolveMention(room, id, target string) (string, bool) {
	r, err := s.members(room)
	if err != nil {
		return "", false
	}
	t, ok := s.Get(room, id)
	if !ok {
		return "", false
	}
	if target == r.ManagerID {
		return t.ID, true
	}
	return t.ID, t.ParentID != "" && worker(r, target) && t.AssignedTo == target && t.DispatchedAt != nil && running(t)
}
