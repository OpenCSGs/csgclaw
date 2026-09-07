package roomtask

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"csgclaw/internal/taskcore"
)

// AppendPlan adds a batch without rewriting earlier work or starting workers.
// Dependencies may refer to existing siblings by ID or new batch-local refs.
// The durable request key makes retries safe even after delivery or restart.
func (s *Service) AppendPlan(room, id, requestID, summary string, items []PlanItem) error {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return fmt.Errorf("a stable request_id is required when appending tasks")
	}
	data, err := json.Marshal(struct {
		Summary string
		Items   []PlanItem
	}{summary, items})
	if err != nil {
		return err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	return s.change(room, id, func(r Roster, _ []taskcore.Task, a *taskcore.Snapshot, allocate func() (string, error)) error {
		if a.Root.ID != id {
			return fmt.Errorf("append requires the existing parent task")
		}
		for _, event := range a.Events {
			if event.Type == EventExtended && event.RequestID == requestID {
				if event.RequestHash != hash {
					return fmt.Errorf("request_id already used for a different plan extension")
				}
				return nil
			}
		}
		if Terminal(a.Root.Status) || a.Root.Status == taskcore.StatusStopping || len(a.Children) == 0 {
			return fmt.Errorf("append requires an unfinished, already planned parent")
		}
		if err := validatePlan(items, r, a.Children...); err != nil {
			return err
		}
		ids := make(map[string]string)
		for _, task := range a.Children {
			ids[task.ID] = task.ID
		}
		for _, item := range items {
			value, err := allocate()
			if err != nil {
				return err
			}
			ids[item.IDRef] = value
		}
		now := time.Now().UTC()
		for i, item := range items {
			task := taskcore.Task{ID: ids[item.IDRef], ParentID: id, AssignmentType: a.Root.AssignmentType,
				AssignmentID: a.Root.AssignmentID, RoomID: room, ExecutionChannel: "csgclaw",
				Title: strings.TrimSpace(item.Title), Body: item.Body, AssignedTo: item.AssignedTo,
				CreatedBy: r.ManagerID, Status: taskcore.StatusPending, Priority: len(items) - i, CreatedAt: now, UpdatedAt: now}
			for _, dep := range item.DependsOnRefs {
				task.DependsOn = append(task.DependsOn, ids[dep])
			}
			a.Children = append(a.Children, task)
			appendEvent(a, taskcore.EventTaskCreated, &task, r.ManagerID, task.Title)
		}
		if a.Root.Status == taskcore.StatusReview {
			a.Root.Status = taskcore.StatusInProgress
		}
		a.Root.UpdatedAt = now
		a.Events = append(a.Events, taskcore.TaskEvent{Type: EventExtended, TaskID: id, ActorID: r.ManagerID,
			Summary: first(summary, "已补充后续子任务"), RequestID: requestID, RequestHash: hash, CreatedAt: now})
		return nil
	})
}
