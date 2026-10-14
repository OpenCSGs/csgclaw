package api

import (
	"bytes"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/roomtask"
	"csgclaw/internal/taskcore"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRoomTaskDispatchSurvivesFullIMBusOverHTTP(t *testing.T) {
	for _, online := range []bool{true, false} {
		t.Run(fmt.Sprintf("worker_subscribed=%t", online), func(t *testing.T) {
			bus := im.NewBus()
			messages := im.NewServiceWithBus(bus)
			_, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "agent-dev", Name: "Dev", Role: "worker"})
			if err != nil {
				t.Fatal(err)
			}
			room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Work", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand})
			if err != nil {
				t.Fatal(err)
			}
			bridge := im.NewParticipantBridge("")
			var events <-chan im.ParticipantEvent
			if online {
				var cancel func()
				events, cancel = bridge.Subscribe("pt-dev")
				defer cancel()
			}
			h := &Handler{im: messages, participantBridge: bridge}
			h.SetRoomTaskCore(taskcore.NewService())
			parent, err := h.roomTaskSvc.Create(room.ID, "source", "admin", "Build", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := h.roomTaskSvc.Plan(room.ID, parent.ID, "Build", []roomtask.PlanItem{{IDRef: "dev", Title: "Build", AssignedTo: "pt-dev"}}, false); err != nil {
				t.Fatal(err)
			}
			if err := h.roomTaskSvc.Start(room.ID, parent.ID); err != nil {
				t.Fatal(err)
			}
			childID := ""
			for _, task := range h.roomTaskSvc.List(room.ID) {
				if task.ParentID == parent.ID {
					childID = task.ID
				}
			}
			updates, closeUpdates := bus.Subscribe()
			defer closeUpdates()
			// Temporarily slow the dispatch consumer and fill it with real IM messages.
			for i := 0; i < 16; i++ {
				if _, err := messages.DeliverMessage(im.DeliverMessageRequest{RoomID: room.ID, SenderID: room.ManagerID, MessageID: fmt.Sprintf("status-%d", i), Content: "Manager progress update"}); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(h.Routes())
			defer server.Close()
			post := func(path string) {
				req, _ := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(`{}`))
				req.Header.Set("X-CSGClaw-Caller-Agent", "manager")
				res, err := server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(res.Body)
				res.Body.Close()
				if res.StatusCode != http.StatusOK {
					t.Fatalf("%s: %d %s", path, res.StatusCode, body)
				}
			}
			post("/api/v1/rooms/" + room.ID + "/tasks/" + childID + "/dispatch")
			saved, _ := h.roomTaskSvc.Get(room.ID, childID)
			if saved.Status != taskcore.StatusAssigned || saved.ClaimedBy != "" {
				t.Fatalf("unexpected task %+v", saved)
			}
			dispatchMessageID := ""
			stored, _ := messages.ListMessages(room.ID)
			for _, message := range stored {
				if message.Event != nil && message.Event.Key == "task_assigned" {
					dispatchMessageID = message.ID
				}
			}
			if dispatchMessageID == "" {
				t.Fatal("no persisted dispatch")
			}
			drain := func() {
				for {
					select {
					case evt := <-updates:
						h.PublishParticipantEvent(evt)
					default:
						return
					}
				}
			}
			drain()
			if !online {
				var cancel func()
				events, cancel = bridge.Subscribe("pt-dev")
				defer cancel()
			}
			select {
			case evt := <-events:
				if evt.MessageID != dispatchMessageID || evt.TaskID != childID {
					t.Fatalf("wrong delivery %+v", evt)
				}
				bridge.Ack("pt-dev", evt.MessageID)
			default:
				t.Fatal("dispatch returned success but Worker got no task notification")
			}

			post("/api/v1/rooms/" + room.ID + "/tasks/retry-delivery")
			drain()
			select {
			case evt := <-events:
				t.Fatalf("retry duplicated acknowledged execution: %+v", evt)
			default:
			}
		})
	}
}

func TestRoomTaskClaimIgnoresProjectionFailureOverHTTP(t *testing.T) {
	messages := im.NewService()
	_, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "agent-dev", Name: "Dev", Role: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Work", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: messages, participantBridge: im.NewParticipantBridge("")}
	core := taskcore.NewService()
	h.SetRoomTaskCore(core)
	parent, err := h.roomTaskSvc.Create(room.ID, "source", "admin", "Build", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.roomTaskSvc.Plan(room.ID, parent.ID, "Build", []roomtask.PlanItem{{IDRef: "dev", Title: "Build", AssignedTo: "pt-dev"}}, false); err != nil {
		t.Fatal(err)
	}
	if err := h.roomTaskSvc.Start(room.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	childID := ""
	for _, task := range h.roomTaskSvc.List(room.ID) {
		if task.ParentID == parent.ID {
			childID = task.ID
		}
	}
	if err := h.roomTaskSvc.Dispatch(room.ID, childID, ""); err != nil {
		t.Fatal(err)
	}
	assigned, _ := h.roomTaskSvc.Get(room.ID, childID)
	// Fault injection: IM projection persistence fails after task commit.
	var projectionCalls atomic.Int32
	h.roomTaskSvc = roomtask.NewService(core, h.roomSchedulingContext, func(roomtask.Projection) error {
		projectionCalls.Add(1)
		return fmt.Errorf("IM projection write failed")
	})
	server := httptest.NewServer(h.Routes())
	defer server.Close()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/rooms/"+room.ID+"/tasks/"+childID+"/claim", strings.NewReader(fmt.Sprintf(`{"attempt":%d}`, assigned.Attempt)))
	req.Header.Set("X-CSGClaw-Caller-Agent", "pt-dev")
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	saved, _ := h.roomTaskSvc.Get(room.ID, childID)
	if res.StatusCode != http.StatusOK || saved.Status != taskcore.StatusInProgress || saved.ClaimedBy != "pt-dev" {
		t.Fatalf("claim=%d body=%s task=%+v", res.StatusCode, body, saved)
	}
	if projectionCalls.Load() != 0 {
		t.Fatal("claim replayed historical projections")
	}

}

// A delivery failure must identify the saved work and its failed notification,
// preserve the cause, and give a retry action without exposing credentials.
func TestRoomTaskDeliveryErrorAndClaimDiagnostics(t *testing.T) {
	previous := slog.Default()
	var logs bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	h := newAppTaskTestHandler(t)
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Work", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := h.roomTaskSvc.Create(room.ID, "source", "admin", "Build", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.roomTaskSvc.Plan(room.ID, parent.ID, "Build", []roomtask.PlanItem{{IDRef: "dev", Title: "Build", AssignedTo: "pt-dev"}}, true); err != nil {
		t.Fatal(err)
	}
	childID := ""
	for _, task := range h.roomTaskSvc.List(room.ID) {
		if task.ParentID == parent.ID {
			childID = task.ID
		}
	}
	routes := h.Routes()
	call := func(method, path, actor, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(payload))
		req.Header.Set("X-CSGClaw-Caller-Agent", actor)
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)
		return rec
	}
	dispatch := call(http.MethodPost, "/api/v1/rooms/"+room.ID+"/tasks/"+childID+"/dispatch", "manager", `{}`)
	if dispatch.Code != 200 {
		t.Fatal(dispatch.Body.String())
	}
	// A stale attempt is a rejected claim, not a projection failure.
	denied := call(http.MethodPost, "/api/v1/rooms/"+room.ID+"/tasks/"+childID+"/claim", "pt-dev", `{"attempt":2}`)
	if denied.Code != 409 || !strings.Contains(denied.Body.String(), "current attempt 1") || !strings.Contains(denied.Body.String(), childID) {
		t.Fatalf("unclear rejection: %d %s", denied.Code, denied.Body.String())
	}
	saved, _ := h.roomTaskSvc.Get(room.ID, childID)
	if saved.Status != taskcore.StatusAssigned || saved.ClaimedBy != "" {
		t.Fatal("rejected claim mutated execution")
	}
	claimed := call(http.MethodPost, "/api/v1/rooms/"+room.ID+"/tasks/"+childID+"/claim", "pt-dev", `{"attempt":1}`)
	if claimed.Code != 200 {
		t.Fatal(claimed.Body.String())
	}
	// Task metadata continues to be authoritative, including after an IM failure.
	core := taskcore.NewService()
	h.SetRoomTaskCore(core)
	parent, err = h.roomTaskSvc.Create(room.ID, "projection-source", "admin", "Projection", "")
	if err != nil {
		t.Fatal(err)
	}
	h.roomTaskSvc = roomtask.NewService(core, h.roomSchedulingContext, func(roomtask.Projection) error {
		return fmt.Errorf("write failed Authorization: Bearer fixture-log-secret")
	})
	failed := call(http.MethodPost, "/api/v1/rooms/"+room.ID+"/tasks/"+parent.ID+"/plan", "manager", `{"tasks":[{"id_ref":"dev","title":"Build","assigned_to":"pt-dev"}]}`)
	if failed.Code != 503 {
		t.Fatalf("projection failure = %d %s", failed.Code, failed.Body.String())
	}
	var result struct {
		Error struct{ Code, Message string }
	}
	if err := json.Unmarshal(failed.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error.Code != "room_task_delivery_failed" || !strings.Contains(result.Error.Message, "task state saved") || !strings.Contains(result.Error.Message, "retry-delivery --room "+room.ID) {
		t.Fatalf("unclear delivery error: %+v", result)
	}
	if strings.Contains(failed.Body.String()+logs.String(), "fixture-log-secret") {
		t.Fatal("credential leaked in delivery error or logs")
	}
	for _, field := range []string{`"room_id":"` + room.ID + `"`, `"task_id":"` + childID + `"`, `"participant_id":"pt-dev"`, `"requested_attempt":2`, `"current_attempt":1`, `"phase":"task_commit"`, `"phase":"message_projection"`, `"task_state_saved":true`, `"message_id":`} {
		if !strings.Contains(logs.String(), field) {
			t.Fatalf("missing diagnostic field %s: %s", field, logs.String())
		}
	}
}
