package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/roomtask"
	"csgclaw/internal/taskcore"
)

func TestPrivateRoomContextTracksDurableWorkWithoutDiscovery(t *testing.T) {
	messages := im.NewService()
	for _, id := range []string{"dev", "qa"} {
		if _, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: id, Name: id, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Room", CreatorID: "admin", MemberIDs: []string{"dev", "qa"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	store, err := taskcore.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: messages, participantBridge: im.NewParticipantBridge("")}
	h.SetRoomTaskCore(taskcore.NewService(taskcore.WithStore(store)))
	projector := roomtask.NewContextProjector()
	contextFor := func(actor, taskID string) string {
		t.Helper()
		key := room.ID + ":" + actor
		if actor != "manager" {
			key += ":" + taskID
		}
		current, err := h.participantBridge.RoomContext(roomtask.TurnContextRequest{
			ConversationID: key, RoomID: room.ID, ParticipantID: actor, SourceID: "source", TaskID: taskID,
		})
		if err != nil {
			t.Fatal(err)
		}
		text, err := projector.Project(key, current)
		if err != nil {
			t.Fatal(err)
		}
		return text
	}
	initial := contextFor("manager", "")
	if !strings.Contains(initial, `mode="full"`) || !strings.Contains(initial, `room_type="on_demand"`) || !strings.Contains(initial, `role="manager"`) || !strings.Contains(initial, `policy_id="on-demand-manager/v1"`) || !strings.Contains(initial, `"tasks":[]`) || !strings.Contains(initial, `"has_tasks":false`) || !strings.Contains(initial, "pt-dev") {
		t.Fatal(initial)
	}
	if strings.Contains(initial, "task claim --task") || !strings.Contains(initial, "task submit, task plan, and task dispatch") || !strings.Contains(initial, "not the direct executor") {
		t.Fatal("dynamic context did not carry the compact Manager action gate", initial)
	}
	if strings.Contains(initial, "task_policy") || strings.Contains(initial, "as_of") {
		t.Fatal("volatile or duplicated policy facts remain in projected context", initial)
	}
	unchanged := contextFor("manager", "")
	if !strings.Contains(unchanged, `mode="steady"`) || !strings.Contains(unchanged, "task submit, task plan, and task dispatch") || strings.Contains(unchanged, `kind="room-scope"`) || strings.Contains(unchanged, `kind="task-snapshot"`) {
		t.Fatal("unchanged Manager context omitted the action gate or repeated stable facts", unchanged)
	}
	freeRoom, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Free", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeFree})
	if err != nil {
		t.Fatal(err)
	}
	freeContext, err := h.participantBridge.RoomContext(roomtask.TurnContextRequest{ConversationID: "free", RoomID: freeRoom.ID, ParticipantID: "manager", SourceID: "source-free"})
	if got, projectErr := projector.Project("free", freeContext); err != nil || projectErr != nil || got != "" {
		t.Fatalf("free room context = %q, provider error %v, projection error %v; want no room workflow instructions", got, err, projectErr)
	}
	root, err := h.roomTaskSvc.Create(room.ID, "source", "admin", "Build", "Parent goal")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.roomTaskSvc.Plan(room.ID, root.ID, "Build then test", []roomtask.PlanItem{
		{IDRef: "dev", Title: "Build", Body: "Deliver page.html", AssignedTo: "pt-dev"},
		{IDRef: "qa", Title: "Test", Body: "Verify the page", AssignedTo: "pt-qa", DependsOnRefs: []string{"dev"}},
	}, true); err != nil {
		t.Fatal(err)
	}
	children := map[string]taskcore.Task{}
	for _, task := range h.roomTaskSvc.List(room.ID) {
		if task.ParentID != "" {
			children[task.AssignedTo] = task
		}
	}
	dev, qa := children["pt-dev"], children["pt-qa"]
	if err := h.roomTaskSvc.Dispatch(room.ID, dev.ID, ""); err != nil {
		t.Fatal(err)
	}
	worker := contextFor("pt-dev", dev.ID)
	if !strings.Contains(worker, `role="worker"`) || !strings.Contains(worker, `policy_id="on-demand-worker/v1"`) || !strings.Contains(worker, "Deliver page.html") || strings.Contains(worker, "Verify the page") || strings.Contains(worker, "task claim --task") || strings.Contains(worker, "task submit") {
		t.Fatal(worker)
	}
	if _, err := h.participantBridge.RoomContext(roomtask.TurnContextRequest{ConversationID: "qa-wrong", RoomID: room.ID, ParticipantID: "pt-qa", SourceID: "source", TaskID: dev.ID}); err == nil {
		t.Fatal("worker read another assignment")
	}
	for _, child := range []taskcore.Task{dev, qa} {
		if child.ID == qa.ID {
			if err := h.roomTaskSvc.Dispatch(room.ID, qa.ID, ""); err != nil {
				t.Fatal(err)
			}
			if got := contextFor("pt-qa", qa.ID); !strings.Contains(got, "page.html accepted output") {
				t.Fatal(got)
			}
		}
		if _, err := h.roomTaskSvc.Update(room.ID, child.ID, child.AssignedTo, taskcore.StatusInProgress, "", ""); err != nil {
			t.Fatal(err)
		}
		if _, err := h.roomTaskSvc.Update(room.ID, child.ID, child.AssignedTo, taskcore.StatusCompleted, "page.html accepted output", ""); err != nil {
			t.Fatal(err)
		}
		feedback := contextFor("manager", child.ID)
		if !strings.Contains(feedback, `"status":"pending_review"`) || !strings.Contains(feedback, "page.html accepted output") {
			t.Fatal(feedback)
		}
		if err := h.roomTaskSvc.Review(room.ID, child.ID, 1, true, "Verified"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.roomTaskSvc.Report(room.ID, root.ID, "succeeded", "Delivered"); err != nil {
		t.Fatal(err)
	}
	for _, message := range func() []im.Message { list, _ := messages.ListMessages(room.ID); return list }() {
		if message.Event != nil && strings.HasPrefix(message.Event.Key, "task_") {
			if strings.Contains(message.Content, "room-task") || strings.Contains(message.Content, "You coordinate") || strings.Contains(message.Content, "page.html accepted output") {
				t.Fatal("private context leaked", message.Content)
			}
		}
	}
	// Both the live read and a fresh service over the persisted aggregate must
	// return 2/2 completed, regardless of old assignment messages in the room.
	for pass := range 2 {
		if pass == 1 {
			h.SetRoomTaskCore(taskcore.NewService(taskcore.WithStore(store)))
		}
		out := httptest.NewRecorder()
		h.Routes().ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+room.ID+"/tasks", nil))
		var tasks []taskcore.Task
		if out.Code != http.StatusOK || json.Unmarshal(out.Body.Bytes(), &tasks) != nil || len(tasks) != 3 {
			t.Fatal(out.Code, out.Body.String())
		}
		for _, task := range tasks {
			if task.Status != taskcore.StatusCompleted {
				t.Fatal(task)
			}
		}
		if out.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("task state may be cached")
		}
	}
	if got := contextFor("manager", ""); !strings.Contains(got, `"tasks":[]`) || strings.Contains(got, "page.html accepted output") {
		t.Fatal("closed history injected on unrelated wake", got)
	}
}
