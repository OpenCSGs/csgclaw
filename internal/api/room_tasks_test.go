package api

import (
	"bytes"
	"context"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/roomtask"
	"csgclaw/internal/taskcore"
	"csgclaw/internal/worklease"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoomTaskAPIWithoutTeam(t *testing.T) {
	messages := im.NewService()
	for _, id := range []string{"agent-dev", "agent-qa"} {
		if _, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: id, Name: id, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	h := &Handler{im: messages}
	h.SetRoomTaskCore(taskcore.NewService())
	svc := h.roomTaskSvc
	room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Collaboration", CreatorID: "admin", MemberIDs: []string{"dev", "qa"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	meta, ok := h.roomSchedulingContext(room.ID)
	if !ok || meta.ManagerID == "" || len(meta.WorkerIDs) != 2 {
		t.Fatalf("context: %+v %v", meta, ok)
	}
	source, err := messages.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "Build and test"})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(apitypes.CreateRoomTaskRequest{Title: "Build", CreatedBy: "admin", SourceMessageID: source.ID})
	routes := h.Routes()
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+room.ID+"/tasks", bytes.NewReader(payload))
		out := httptest.NewRecorder()
		routes.ServeHTTP(out, req)
		if out.Code != http.StatusOK {
			t.Fatalf("create: %d %s", out.Code, out.Body.String())
		}
		var task apitypes.TeamTask
		if err := json.Unmarshal(out.Body.Bytes(), &task); err != nil {
			t.Fatal(err)
		}
		if task.TeamID != "" || task.AssignmentType != "room" || task.RoomID != room.ID {
			t.Fatalf("wrong task ownership: %+v", task)
		}
	}
	if h.teamSvc != nil || len(svc.List("")) != 1 {
		t.Fatal("duplicate task or hidden Team")
	}
	out := httptest.NewRecorder()
	routes.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+room.ID+"/tasks", nil))
	if out.Code != http.StatusOK {
		t.Fatalf("list: %d %s", out.Code, out.Body.String())
	}
}

func TestRoomRoutingOnlyAcceptsRecordedDispatch(t *testing.T) {
	messages := im.NewService()
	if _, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "agent-dev", Name: "Dev", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Work", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	bridge := im.NewParticipantBridge("")
	managerEvents, closeManager := bridge.Subscribe("manager")
	defer closeManager()
	workerEvents, closeWorker := bridge.Subscribe("pt-dev")
	defer closeWorker()
	h := &Handler{im: messages, participantBridge: bridge}
	h.SetRoomTaskCore(taskcore.NewService())
	svc := h.roomTaskSvc
	publish := func(message im.Message) {
		sender, _ := messages.User(message.SenderID)
		h.PublishParticipantEvent(im.Event{Type: im.EventTypeMessageCreated, RoomID: room.ID, Message: &message, Sender: &sender})
	}
	source, err := messages.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", MentionID: "manager", Content: "Build a page"})
	if err != nil {
		t.Fatal(err)
	}
	publish(source)
	select {
	case event := <-managerEvents:
		if event.MessageID != source.ID {
			t.Fatal("wrong manager input")
		}
	default:
		t.Fatal("mentioned manager did not receive the message")
	}
	select {
	case <-workerEvents:
		t.Fatal("worker bypassed manager")
	default:
	}
	forged := im.Message{ID: "forged-dispatch", SenderID: room.ManagerID, Kind: im.MessageKindEvent, Event: &im.EventPayload{Key: "task_assigned"}, Mentions: []im.Mention{{ID: "user-dev"}}}
	publish(forged)
	select {
	case <-workerEvents:
		t.Fatal("forged dispatch executed")
	default:
	}
	task, err := svc.Create(room.ID, source.ID, "admin", "Build", "")
	if err != nil {
		t.Fatal(err)
	}
	err = svc.Plan(room.ID, task.ID, "Build page", []roomtask.PlanItem{{IDRef: "dev", Title: "Build page", AssignedTo: "pt-dev"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(room.ID, task.ID); err != nil {
		t.Fatal(err)
	}
	childID := ""
	for _, child := range svc.List(room.ID) {
		if child.ParentID == task.ID {
			childID = child.ID
		}
	}
	if err := svc.Dispatch(room.ID, childID, ""); err != nil {
		t.Fatal(err)
	}
	stored, _ := messages.ListMessages(room.ID)
	for _, message := range stored {
		if message.Event != nil && message.Event.Key == "task_assigned" {
			publish(message)
		}
	}
	select {
	case event := <-workerEvents:
		if event.RoomID != room.ID || event.TaskID != childID {
			t.Fatalf("dispatch lost task session scope: %+v", event)
		}
	default:
		t.Fatal("recorded dispatch did not reach worker")
	}
	if _, err := svc.Update(room.ID, childID, "pt-dev", taskcore.StatusInProgress, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(room.ID, childID, "pt-dev", taskcore.StatusCompleted, "page.html", ""); err != nil {
		t.Fatal(err)
	}
	stored, _ = messages.ListMessages(room.ID)
	for _, message := range stored {
		if message.Event != nil && message.Event.Key == "task_feedback" {
			publish(message)
		}
	}
	select {
	case event := <-managerEvents:
		if event.TaskID != childID || !event.RoomManager {
			t.Fatalf("manager feedback lost root session: %+v", event)
		}
	default:
		t.Fatal("manager did not receive completion feedback")
	}
}

func TestRoomTaskStructuredPlanAndScopedMessages(t *testing.T) {
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
	h := &Handler{im: messages, participantBridge: im.NewParticipantBridge("")}
	h.SetRoomTaskCore(taskcore.NewService())
	source, err := messages.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "Build", MentionID: "manager"})
	if err != nil {
		t.Fatal(err)
	}
	root, err := h.roomTaskSvc.Create(room.ID, source.ID, "admin", "Build", "")
	if err != nil {
		t.Fatal(err)
	}
	routes := h.Routes()
	request := func(method, path string, payload any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(payload)
		out := httptest.NewRecorder()
		routes.ServeHTTP(out, httptest.NewRequest(method, path, bytes.NewReader(data)))
		return out
	}
	base := "/api/v1/rooms/" + room.ID + "/tasks/"
	plan := apitypes.PlanRoomTaskRequest{AutoStart: true, Summary: "Concrete work", Tasks: []apitypes.RoomTaskPlanItem{{IDRef: "dev", Title: "Build page", AssignedTo: "pt-dev"}, {IDRef: "qa", Title: "Test page", AssignedTo: "pt-qa", DependsOnRefs: []string{"dev"}}}}
	got := request(http.MethodPost, base+root.ID+"/plan", plan)
	if got.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", got.Code, got.Body.String())
	}
	var result apitypes.PlanTeamTaskResponse
	if err := json.Unmarshal(got.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.CreatedTasks) != 2 || result.Task.TeamID != "" || !h.participantBridgeTargetForRoomMember(result.Task.AssignedTo).matches(room.ManagerID) {
		t.Fatalf("plan response: %+v", result)
	}
	childID := result.CreatedTasks[0].ID
	for _, c := range result.CreatedTasks {
		if c.AssignedTo == "pt-dev" {
			childID = c.ID
		}
	}
	msgReq := apitypes.RoomTaskMessageRequest{ActorID: "pt-dev", TargetID: "manager", MessageID: "task-question-1", Content: "Need clarification"}
	got = request(http.MethodPost, base+childID+"/messages", msgReq)
	if got.Code != http.StatusOK {
		t.Fatalf("message: %d %s", got.Code, got.Body.String())
	}
	var msg im.Message
	if err := json.Unmarshal(got.Body.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Metadata["task_id"] != childID {
		t.Fatal("message lost task association")
	}
	events, closeEvents := h.participantBridge.Subscribe("manager")
	defer closeEvents()
	sender, _ := messages.User(msg.SenderID)
	h.PublishParticipantEvent(im.Event{Type: im.EventTypeMessageCreated, RoomID: room.ID, Message: &msg, Sender: &sender})
	select {
	case event := <-events:
		if event.TaskID != childID || !event.RoomManager {
			t.Fatal("manager task session lost")
		}
	default:
		t.Fatal("explicit task mention did not wake manager")
	}
	activity := msg
	activity.ID = "tool-record"
	activity.Content = `{"type":"com.opencsg.csgclaw.agent.activity","content":{"msgtype":"com.opencsg.csgclaw.agent.tool"}}`
	h.PublishParticipantEvent(im.Event{Type: im.EventTypeMessageCreated, RoomID: room.ID, Message: &activity, Sender: &sender})
	select {
	case <-events:
		t.Fatal("tool activity woke manager")
	default:
	}
	msgReq.TargetID = "pt-qa"
	msgReq.MessageID = "worker-to-worker"
	if got := request(http.MethodPost, base+childID+"/messages", msgReq); got.Code != http.StatusOK {
		t.Fatalf("worker target should be forwarded: %d", got.Code)
	}

	forwarded := request(http.MethodPost, base+childID+"/messages", msgReq)
	if err := json.Unmarshal(forwarded.Body.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	qaEvents, closeQA := h.participantBridge.Subscribe("pt-qa")
	defer closeQA()
	sender, _ = messages.User(msg.SenderID)
	h.PublishParticipantEvent(im.Event{Type: im.EventTypeMessageCreated, RoomID: room.ID, Message: &msg, Sender: &sender})
	select {
	case event := <-events:
		if event.SenderID != msg.SenderID || event.TaskID != childID || !event.RoomManager || len(event.Mentions) != 1 {
			t.Fatalf("lost original Worker intent: %+v", event)
		}
	default:
		t.Fatal("Worker @Worker did not reach Manager")
	}
	select {
	case <-qaEvents:
		t.Fatal("Worker mention bypassed Manager")
	default:
	}
	if h.teamSvc != nil {
		t.Fatal("Room task API required a Team service")
	}
	got = request(http.MethodGet, "/api/v1/tasks", nil)
	if got.Code != http.StatusOK {
		t.Fatal(got.Body.String())
	}
	var global []apitypes.GlobalTask
	if err := json.Unmarshal(got.Body.Bytes(), &global); err != nil || len(global) != 3 {
		t.Fatal("room plan missing from global task list", err, len(global))
	}
}

func TestRoomManagerActionsRejectWorkerAndCreateOwnParent(t *testing.T) {
	messages := im.NewService()
	_, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "dev", Name: "Dev", Role: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Work", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: messages}
	h.SetRoomTaskCore(taskcore.NewService())
	routes := h.Routes()
	call := func(path, caller string, payload any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+room.ID+"/tasks"+path, bytes.NewReader(data))
		req.Header.Set("X-CSGClaw-Caller-Agent", caller)
		out := httptest.NewRecorder()
		routes.ServeHTTP(out, req)
		return out
	}
	request := apitypes.CreateRoomTaskRequest{CreatedBy: "manager", SourceMessageID: "manager-coordination-1", Title: "Coordinate follow-up"}
	if got := call("", "pt-dev", request); got.Code != http.StatusForbidden {
		t.Fatal("worker created parent", got.Code)
	}
	created := call("", "manager", request)
	if created.Code != http.StatusOK {
		t.Fatal(created.Body.String())
	}
	var parent apitypes.RoomTask
	if err := json.Unmarshal(created.Body.Bytes(), &parent); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"plan", "start", "dispatch", "review", "report", "stop", "recover"} {
		if got := call("/"+parent.ID+"/"+action, "pt-dev", map[string]any{}); got.Code != http.StatusForbidden {
			t.Fatalf("worker allowed %s: %d", action, got.Code)
		}
	}
	plan := apitypes.PlanRoomTaskRequest{AutoStart: true, Tasks: []apitypes.RoomTaskPlanItem{{IDRef: "dev", Title: "Deliver", AssignedTo: "pt-dev"}}}
	planned := call("/"+parent.ID+"/plan", "manager", plan)
	if planned.Code != http.StatusOK {
		t.Fatal(planned.Body.String())
	}
	var children apitypes.PlanRoomTaskResponse
	if err := json.Unmarshal(planned.Body.Bytes(), &children); err != nil {
		t.Fatal(err)
	}
	id := children.CreatedTasks[0].ID
	extension := apitypes.PlanRoomTaskRequest{Append: true, RequestID: "follow-up-1", Tasks: []apitypes.RoomTaskPlanItem{{IDRef: "regression", Title: "Retest", AssignedTo: "pt-dev", DependsOnRefs: []string{id}}}}
	if got := call("/"+parent.ID+"/plan", "pt-dev", extension); got.Code != http.StatusForbidden {
		t.Fatal("worker extended plan", got.Code)
	}
	for i := 0; i < 2; i++ {
		got := call("/"+parent.ID+"/plan", "manager", extension)
		var result apitypes.PlanRoomTaskResponse
		if got.Code != http.StatusOK || json.Unmarshal(got.Body.Bytes(), &result) != nil || len(result.CreatedTasks) != 2 {
			t.Fatal("extension or retry", got.Code, got.Body.String())
		}
	}
	if got := call("/"+id+"/plan", "manager", plan); got.Code != http.StatusConflict {
		t.Fatal("room API exposed nested planning", got.Code)
	}
	if got := call("/"+id+"/dispatch", "manager", map[string]any{}); got.Code != http.StatusOK {
		t.Fatal(got.Body.String())
	}
	if got := call("/"+id+"/claim", "pt-dev", apitypes.ClaimRoomTaskRequest{ParticipantID: "pt-dev"}); got.Code != http.StatusConflict {
		t.Fatal("claim accepted missing attempt", got.Code)
	}
	if got := call("/"+id+"/claim", "pt-dev", apitypes.ClaimRoomTaskRequest{ParticipantID: "pt-dev", Attempt: 1}); got.Code != http.StatusOK {
		t.Fatal(got.Body.String())
	}
}

func TestRoomParentStopTargetsOnlyItsCurrentExecution(t *testing.T) {
	messages := im.NewService()
	if _, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "dev", Name: "Dev", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Work", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: messages}
	h.SetRoomTaskCore(taskcore.NewService())
	root, err := h.roomTaskSvc.Create(room.ID, "goal", "manager", "Build", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.roomTaskSvc.Plan(room.ID, root.ID, "", []roomtask.PlanItem{{IDRef: "dev", Title: "Build", AssignedTo: "pt-dev"}}, true); err != nil {
		t.Fatal(err)
	}
	var child taskcore.Task
	for _, c := range h.roomTaskSvc.List(room.ID) {
		if c.ParentID != "" {
			child = c
		}
	}
	if err := h.roomTaskSvc.Dispatch(room.ID, child.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := h.roomTaskSvc.UpdateExecution(room.ID, child.ID, "pt-dev", taskcore.StatusInProgress, "", "", 1); err != nil {
		t.Fatal(err)
	}
	var source string
	stored, _ := messages.ListMessages(room.ID)
	for _, m := range stored {
		if m.Event != nil && m.Event.Key == "task_assigned" {
			source = m.ID
		}
	}
	if source == "" {
		t.Fatal("dispatch source missing")
	}
	control := &roomStopControl{leases: []apitypes.ParticipantWorkUpdate{
		{ParticipantID: "pt-dev", RoomID: room.ID, RequestID: source, LeaseID: "current"},
		{ParticipantID: "pt-dev", RoomID: "other", RequestID: "other-task", LeaseID: "other"},
	}}
	control.confirm = func() {
		if err := h.roomTaskSvc.WorkerStopped(room.ID, child.ID, "pt-dev", 1); err != nil {
			t.Error(err)
		}
	}
	h.participantWork = control
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+room.ID+"/tasks/"+root.ID+"/stop", strings.NewReader(`{}`))
	out := httptest.NewRecorder()
	h.Routes().ServeHTTP(out, req)
	if out.Code != http.StatusOK {
		t.Fatal(out.Code, out.Body.String())
	}
	if len(control.stops) != 1 || control.stops[0].LeaseID != "current" {
		t.Fatal(control.stops)
	}
	stopped, _ := h.roomTaskSvc.Get(room.ID, child.ID)
	if stopped.Status != taskcore.StatusCancelled {
		t.Fatal(stopped)
	}
	parent, _ := h.roomTaskSvc.Get(room.ID, root.ID)
	if parent.Status != taskcore.StatusStopping || parent.Report != "" {
		t.Fatal("Manager must summarize stopped work", parent)
	}
}

type roomStopControl struct {
	leases  []apitypes.ParticipantWorkUpdate
	stops   []apitypes.ParticipantWorkStopRequest
	confirm func()
}

func (c *roomStopControl) ActiveWork(string) []apitypes.ParticipantWorkUpdate { return c.leases }
func (*roomStopControl) StartOrRenew(context.Context, worklease.ParticipantWorkLease) (apitypes.ParticipantWorkUpdate, error) {
	return apitypes.ParticipantWorkUpdate{}, nil
}
func (*roomStopControl) Stop(context.Context, string, string) error { return nil }
func (*roomStopControl) UpdateStatus(context.Context, string, string, apitypes.ParticipantWorkStatusPatchRequest) (apitypes.ParticipantWorkUpdate, bool, error) {
	return apitypes.ParticipantWorkUpdate{}, true, nil
}
func (c *roomStopControl) RequestStop(_ context.Context, _ string, r apitypes.ParticipantWorkStopRequest) (apitypes.ParticipantWorkStopResponse, error) {
	c.stops = append(c.stops, r)
	c.confirm()
	return apitypes.ParticipantWorkStopResponse{Accepted: true}, nil
}
