package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"csgclaw/internal/agenttask"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	"csgclaw/internal/roomtask"
	"csgclaw/internal/taskcore"
	"csgclaw/internal/worklease"
)

func createAutonomousTaskFixture(t *testing.T, h *Handler) (im.Room, im.Message) {
	t.Helper()
	room, err := h.im.CreateRoom(im.CreateRoomRequest{
		Title: "Gomoku", CreatorID: "admin", MemberIDs: []string{"dev", "qa"}, Type: apitypes.RoomTypeAutonomous,
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "Build a Gomoku game"})
	if err != nil {
		t.Fatal(err)
	}
	return room, source
}

func TestAutonomousTaskWholeClaimRacesWithThreadSplit(t *testing.T) {
	h := newAppTaskTestHandler(t)
	store, err := taskcore.NewStore(filepath.Join(t.TempDir(), "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	core := taskcore.NewService(taskcore.WithStore(store))
	h.SetRoomTaskCore(core)
	room, source := createAutonomousTaskFixture(t, h)
	type outcome struct {
		result autonomousTaskResponse
		err    error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	run := func(actor, tool string, args map[string]any) {
		<-start
		value, err := h.runAutonomousTaskTool(actor, tool, args)
		results <- outcome{value.(autonomousTaskResponse), err}
	}
	go run("agent-dev", "autonomous_task_claim", map[string]any{"room_id": room.ID, "source_message_id": source.ID})
	go run("agent-qa", "autonomous_task_split", map[string]any{"room_id": room.ID, "thread_root_id": source.ID, "source_message_id": source.ID,
		"parts": []map[string]string{{"title": "Board", "scope": "board.ts"}, {"title": "Strategy", "scope": "strategy.ts"}}})
	close(start)
	wins := 0
	for range 2 {
		got := <-results
		if got.err != nil || len(got.result.Tasks) == 0 || got.result.NotificationPending {
			t.Fatalf("unexpected outcome: %+v", got)
		}
		if got.result.Accepted {
			wins++
		} else if got.result.Reason == "" {
			t.Fatal("conflict did not explain the current decision")
		}
	}
	reopened := taskcore.NewService(taskcore.WithStore(store))
	tasks := reopened.ListByAssignment(taskcore.AssignmentTypeRoom, room.ID)
	var roots int
	for _, task := range tasks {
		if task.ParentID == "" {
			roots++
			if task.SourceMessageID != source.ID || task.AssignedTo == "" {
				t.Fatalf("invalid persisted owner: %+v", task)
			}
		}
	}
	if wins != 1 || roots != 1 || (len(tasks) != 1 && len(tasks) != 3) || len(h.roomTaskSvc.List("")) != 0 {
		t.Fatalf("wins=%d roots=%d tasks=%+v", wins, roots, tasks)
	}
	topic, err := h.im.GetThread(room.ID, source.ID)
	if err != nil || len(topic.Replies) != 1 {
		t.Fatalf("either winner should post one update in the original topic: %+v, %v", topic.Replies, err)
	}
	conversation := roomToolConversation(t, room.ID, "")
	for _, tc := range []struct{ tool, requested, want string }{
		{"message_send", "", source.ID},
		{"messages_read", "", ""},
		{"messages_read", source.ID, source.ID},
		{"messages_ack", "", ""},
		{"messages_ack", source.ID, source.ID},
	} {
		args := map[string]any{"room_id": room.ID, "thread_root_id": tc.requested}
		if err := h.scopeAppPlatformRoomConversation("agent-dev", tc.tool, conversation, source.ID, args); err != nil || args["thread_root_id"] != tc.want {
			t.Fatalf("%s with topic %q: %+v, %v", tc.tool, tc.requested, args, err)
		}
	}
}

func TestAutonomousCoalescedTurnCanReadAndAckConfirmedTaskTopic(t *testing.T) {
	h := newAppTaskTestHandler(t)
	room, first := createAutonomousTaskFixture(t, h)
	second, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "Add a score display"})
	if err != nil {
		t.Fatal(err)
	}
	if n, admitted, err := h.im.AdmitAutonomousNotification(room.ID, first.ID, "pt-dev"); err != nil || !admitted || n.Count != 2 {
		t.Fatalf("notification: %+v %v %v", n, admitted, err)
	}
	claim, err := h.runAutonomousTaskTool("agent-dev", "autonomous_task_claim", map[string]any{"room_id": room.ID, "source_message_id": second.ID})
	if err != nil || !claim.(autonomousTaskResponse).Accepted {
		t.Fatalf("claim: %+v %v", claim, err)
	}
	topic := claim.(autonomousTaskResponse).ThreadRootID
	if topic != second.ID {
		t.Fatalf("task topic = %s, want %s", topic, second.ID)
	}
	conversation := roomToolConversation(t, room.ID, "")
	for _, tool := range []string{"message_send", "messages_read", "messages_ack"} {
		args := map[string]any{"thread_root_id": topic}
		if err := h.scopeAppPlatformRoomConversation("agent-dev", tool, conversation, first.ID, args); err != nil || args["thread_root_id"] != topic || args["room_id"] != room.ID {
			t.Fatalf("%s could not use the coalesced task topic: %+v %v", tool, args, err)
		}
		if err := h.scopeAppPlatformRoomConversation("agent-dev", tool, conversation, first.ID, map[string]any{"thread_root_id": first.ID}); err == nil {
			t.Fatalf("%s accepted a topic without a confirmed task", tool)
		}
	}
}

func TestAutonomousTaskToolsRecordIndependentPartsAndStayInScope(t *testing.T) {
	h := newAppTaskTestHandler(t)
	h.serverNoAuth = true
	room, source := createAutonomousTaskFixture(t, h)
	other, _ := createAutonomousTaskFixture(t, h)
	h.participant = participant.NewService(participant.NewMemoryStore([]apitypes.Participant{
		{ID: "pt-dev", Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent, ChannelUserRef: "user-dev", LifecycleStatus: participant.LifecycleStatusActive},
		{ID: "pt-qa", Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent, ChannelUserRef: "user-qa", LifecycleStatus: participant.LifecycleStatusActive},
	}))
	registry := worklease.NewRegistry(h.participant, h.im, worklease.NewBus())
	h.participantWork = registry
	for _, actor := range []string{"pt-dev", "pt-qa"} {
		if _, err := registry.StartOrRenew(context.Background(), worklease.ParticipantWorkLease{
			ParticipantID: actor, LeaseID: worklease.NewID(), RoomID: room.ID, ThreadRootID: source.ID, Kind: apitypes.ParticipantWorkKindAgentTurn,
		}); err != nil {
			t.Fatal(err)
		}
	}
	dev, qa := taskMCPClient(t, h, "agent-dev"), taskMCPClient(t, h, "agent-qa")
	var split autonomousTaskResponse
	callTaskTool(t, qa, "autonomous_task_split", map[string]any{"source_message_id": source.ID,
		"parts": []map[string]string{{"title": "Board", "scope": "board.ts"}, {"title": "Strategy", "scope": "strategy.ts"}}}, &split)
	if !split.Accepted || split.NotificationPending || len(split.Tasks) != 3 {
		t.Fatalf("split: %+v", split)
	}
	ids := make(map[string]string)
	for _, task := range split.Tasks {
		ids[task.Title] = task.ID
	}
	denyTaskTool(t, dev, "autonomous_task_claim", map[string]any{"task_id": ids["Board"], "room_id": other.ID})
	denyTaskTool(t, dev, "autonomous_tasks_list", map[string]any{"thread_root_id": "other-thread"})
	var claimed autonomousTaskResponse
	callTaskTool(t, dev, "autonomous_task_claim", map[string]any{"task_id": ids["Board"]}, &claimed)
	if !claimed.Accepted || claimed.NotificationPending {
		t.Fatalf("claim: %+v", claimed)
	}
	var conflict autonomousTaskResponse
	callTaskTool(t, qa, "autonomous_task_claim", map[string]any{"task_id": ids["Board"]}, &conflict)
	if conflict.Accepted || len(conflict.Tasks) != 3 {
		t.Fatalf("expected conflict with the existing assignment: %+v", conflict)
	}
	if _, err := h.agentTaskSvc.Claim(agenttask.ClaimInput{TaskID: ids["Strategy"], ParticipantID: "pt-dev"}); err == nil {
		t.Fatal("a personal task operation accepted a room task")
	}
	if _, err := h.agentTaskSvc.Update(agenttask.UpdateInput{TaskID: ids["Board"], ActorID: "pt-dev", Status: taskcore.StatusCompleted, Result: "wrong route"}); err == nil {
		t.Fatal("a personal task update accepted a room task")
	}
	var second autonomousTaskResponse
	callTaskTool(t, qa, "autonomous_task_claim", map[string]any{"task_id": ids["Strategy"]}, &second)
	if !second.Accepted {
		t.Fatalf("independent claim: %+v", second)
	}
	var handoff autonomousTaskResponse
	callTaskTool(t, dev, "autonomous_task_update", map[string]any{"task_id": ids["Board"], "status": "handed_off", "next_owner": "pt-qa", "summary": "Board is ready for integration"}, &handoff)
	if !handoff.Accepted || handoff.NotificationPending {
		t.Fatalf("handoff: %+v", handoff)
	}
	var takeover autonomousTaskResponse
	callTaskTool(t, qa, "autonomous_task_claim", map[string]any{"task_id": ids["Board"]}, &takeover)
	if !takeover.Accepted {
		t.Fatalf("takeover: %+v", takeover)
	}
	var premature autonomousTaskResponse
	callTaskTool(t, qa, "autonomous_task_update", map[string]any{"task_id": ids[source.Content], "status": "completed", "summary": "Integrated"}, &premature)
	if premature.Accepted {
		t.Fatal("parent completed before its parts")
	}
	for _, title := range []string{"Board", "Strategy", source.Content} {
		var completed autonomousTaskResponse
		callTaskTool(t, qa, "autonomous_task_update", map[string]any{"task_id": ids[title], "status": "completed", "summary": title + " verified"}, &completed)
		if !completed.Accepted || completed.NotificationPending {
			t.Fatalf("completion of %q: %+v", title, completed)
		}
	}
	view, err := h.im.GetThread(room.ID, source.ID)
	if err != nil || len(view.Replies) != 8 {
		t.Fatalf("want eight successful updates in original thread, got %d: %v", len(view.Replies), err)
	}
	unread, err := h.im.PrepareUnread(room.ID, "pt-dev", source.ID, 20, true)
	if err != nil || len(unread.Messages) != 6 {
		t.Fatalf("peer cannot read the task updates in the original thread: %+v, %v", unread, err)
	}
	out := httptest.NewRecorder()
	h.Routes().ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+room.ID+"/tasks", nil))
	if out.Code != http.StatusOK {
		t.Fatalf("room view: %d %s", out.Code, out.Body.String())
	}
}

func TestAutonomousVideoFeedbackDoesNotAdmitUnreadNotification(t *testing.T) {
	h := newAppTaskTestHandler(t)
	room, source := createAutonomousTaskFixture(t, h)
	request := roomtask.TurnContextRequest{RoomID: room.ID, ParticipantID: "pt-dev", SourceID: source.ID, VideoFeedback: true}
	feedback, err := h.roomExecutionContext(request)
	if err != nil || feedback.Skip || feedback.Role != roomtask.TurnRoleAutonomous || feedback.Notification != nil {
		t.Fatalf("video feedback context: %+v, %v", feedback, err)
	}
	request.VideoFeedback = false
	ordinary, err := h.roomExecutionContext(request)
	if err != nil || ordinary.Skip || ordinary.Notification == nil {
		t.Fatalf("ordinary notification context after feedback: %+v, %v", ordinary, err)
	}
}

func TestAutonomousVideoFeedbackHonorsRoomPause(t *testing.T) {
	h := newAppTaskTestHandler(t)
	room, source := createAutonomousTaskFixture(t, h)
	if err := h.im.PauseAutonomousRoom(room.ID, source.ID); err != nil {
		t.Fatal(err)
	}
	request := roomtask.TurnContextRequest{RoomID: room.ID, ParticipantID: "pt-dev", SourceID: source.ID, VideoFeedback: true}
	feedback, err := h.roomExecutionContext(request)
	if err != nil || !feedback.Skip || feedback.Notification != nil {
		t.Fatalf("video feedback after room pause: %+v, %v", feedback, err)
	}
	current, _ := h.im.Room(room.ID)
	if current.AutonomousState == nil || !current.AutonomousState.Paused {
		t.Fatal("video feedback changed the room pause state")
	}

	next, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "New request"})
	if err != nil {
		t.Fatal(err)
	}
	request.SourceID = next.ID
	request.VideoFeedback = false
	ordinary, err := h.roomExecutionContext(request)
	if err != nil || ordinary.Skip || ordinary.Notification == nil {
		t.Fatalf("new user message after room pause: %+v, %v", ordinary, err)
	}
	request.VideoFeedback = true
	feedback, err = h.roomExecutionContext(request)
	if err != nil || feedback.Skip || feedback.Role != roomtask.TurnRoleAutonomous || feedback.Notification != nil {
		t.Fatalf("video feedback after room resume: %+v, %v", feedback, err)
	}
}
