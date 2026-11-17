package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	"csgclaw/internal/worklease"
)

func TestAutonomousUnreadToolsAndThreadReply(t *testing.T) {
	h := newAppTaskTestHandler(t)
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "autonomous", CreatorID: "admin", MemberIDs: []string{"dev", "qa"}, Type: apitypes.RoomTypeAutonomous})
	if err != nil {
		t.Fatal(err)
	}
	root, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "Root"})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "Thread work", RelatesTo: &im.MessageRelation{RelType: im.RelationTypeThread, EventID: root.ID}})
	if err != nil {
		t.Fatal(err)
	}
	h.registerAppRoomTools("agent-dev")
	h.registerAppRoomTools("agent-qa")
	h.registerAppRoomTools("agent-outsider")
	dev := taskMCPClient(t, h, "agent-dev")
	qa := taskMCPClient(t, h, "agent-qa")
	outsider := taskMCPClient(t, h, "agent-outsider")
	args := map[string]any{"room_id": room.ID, "thread_root_id": root.ID, "limit": 1}
	var page apitypes.UnreadPage
	callTaskTool(t, dev, "messages_read", args, &page)
	if len(page.Messages) != 1 || page.Messages[0].ID != reply.ID || page.Root == nil || page.Root.ID != root.ID || page.ParticipantID != "pt-dev" {
		t.Fatalf("page=%+v", page)
	}
	ack := map[string]any{"room_id": room.ID, "thread_root_id": root.ID, "ack_token": page.Token}
	denyTaskTool(t, qa, "messages_ack", ack)
	denyTaskTool(t, outsider, "messages_read", args)
	callTaskTool(t, dev, "messages_ack", ack, nil)
	var empty apitypes.UnreadPage
	callTaskTool(t, dev, "messages_read", args, &empty)
	if len(empty.Messages) != 0 {
		t.Fatal("ack not applied")
	}
	conversation := "csgclaw-im:pt-dev:room:" + url.QueryEscape(room.ID) + ":thread:" + url.QueryEscape(root.ID)
	send := map[string]any{"room_id": room.ID, "content": "Result", "client_message_id": "retry-key"}
	if err := h.scopeAppPlatformRoomConversation("agent-dev", "message_send", conversation, "", send); err != nil {
		t.Fatal(err)
	}
	var sent, retried im.Message
	callTaskTool(t, dev, "message_send", send, &sent)
	callTaskTool(t, dev, "message_send", send, &retried)
	if sent.RelatesTo == nil || sent.RelatesTo.EventID != root.ID || sent.ID != retried.ID {
		t.Fatalf("reply lost thread/idempotency: %+v %+v", sent, retried)
	}
	for _, target := range []map[string]any{
		{"room_id": "other"},
		{"room_id": room.ID, "thread_root_id": "other"},
	} {
		if err := h.scopeAppPlatformRoomConversation("agent-dev", "message_send", conversation, "", target); err == nil {
			t.Fatalf("native send escaped the conversation: %v", target)
		}
	}
	for _, name := range []string{"messages_read", "messages_ack"} {
		scoped := map[string]any{"room_id": room.ID}
		if err := h.scopeAppPlatformRoomConversation("agent-dev", name, conversation, "", scoped); err != nil || scoped["thread_root_id"] != root.ID {
			t.Fatalf("scope=%v %v", scoped, err)
		}
		if err := h.scopeAppPlatformRoomConversation("agent-dev", name, conversation, "", map[string]any{"room_id": "other"}); err == nil {
			t.Fatal("cross-room consumption allowed")
		}
		if err := h.scopeAppPlatformRoomConversation("agent-dev", name, conversation, "", map[string]any{"room_id": room.ID, "thread_root_id": "other"}); err == nil {
			t.Fatal("cross-thread consumption allowed")
		}
	}
}

func TestAutonomousPlanMessageReachesPeerWithoutMentionOrTask(t *testing.T) {
	h := newAppTaskTestHandler(t)
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Writing", CreatorID: "admin", MemberIDs: []string{"dev", "qa"}, Type: apitypes.RoomTypeAutonomous})
	if err != nil {
		t.Fatal(err)
	}
	h.registerAppRoomTools("agent-dev")
	h.registerAppRoomTools("agent-qa")
	dev := taskMCPClient(t, h, "agent-dev")
	qa := taskMCPClient(t, h, "agent-qa")
	events, closeEvents := h.participantBridge.Subscribe("pt-qa")
	defer closeEvents()

	const plan = "I'll draft the introduction in intro.md."
	var sent im.Message
	callTaskTool(t, dev, "message_send", map[string]any{"room_id": room.ID, "content": plan}, &sent)
	sender, _ := h.im.User(sent.SenderID)
	h.PublishParticipantEvent(im.Event{Type: im.EventTypeMessageCreated, RoomID: room.ID, Message: &sent, Sender: &sender})
	select {
	case event := <-events:
		// The bridge marks autonomous broadcasts as eligible for dispatch.
		// Actual mentions still come only from the stored message.
		if event.MessageID != sent.ID || !event.NotificationOnly || len(event.Mentions) != 0 || event.Text != "" || event.TaskID != "" {
			t.Fatalf("unexpected peer notification: %+v", event)
		}
	default:
		t.Fatal("peer received no unread notice")
	}
	if len(sent.Mentions) != 0 || len(h.autonomousRoomTaskViews(room.ID, nil)) != 0 {
		t.Fatal("the public announcement added a mention or a tracked task")
	}
	var page apitypes.UnreadPage
	callTaskTool(t, qa, "messages_read", map[string]any{"room_id": room.ID}, &page)
	for _, message := range page.Messages {
		if message.ID == sent.ID && message.Content == plan && message.Sender.ID == sent.SenderID {
			return
		}
	}
	t.Fatalf("peer could not read %s from %s in unread page %+v", sent.ID, sent.SenderID, page)
}

func TestAutonomousAgentMessageHTTPUsesCurrentConversation(t *testing.T) {
	h := newAppTaskTestHandler(t)
	first, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "first", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeAutonomous})
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "second", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeAutonomous})
	if err != nil {
		t.Fatal(err)
	}
	root, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: first.ID, SenderID: "admin", Content: "Root"})
	if err != nil {
		t.Fatal(err)
	}
	h.participant = participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID: "pt-dev", Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent,
		ChannelUserRef: "user-dev", LifecycleStatus: participant.LifecycleStatusActive,
	}}))
	registry := worklease.NewRegistry(h.participant, h.im, worklease.NewBus())
	h.participantWork = registry
	start := func(roomID, threadRootID string) {
		t.Helper()
		_, err := registry.StartOrRenew(context.Background(), worklease.ParticipantWorkLease{
			ParticipantID: "pt-dev", LeaseID: worklease.NewID(), RoomID: roomID,
			ThreadRootID: threadRootID, Kind: apitypes.ParticipantWorkKindAgentTurn,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	start(first.ID, root.ID)
	post := func(path string, payload map[string]any) (map[string]any, bool) {
		t.Helper()
		raw, _ := json.Marshal(payload)
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
		if !h.authorizeAgentPlatformRoute(r, "agent-dev") {
			return nil, false
		}
		return platformRequestBody(r)
	}
	read := func(roomID, threadRootID string) (url.Values, bool) {
		query := url.Values{"room_id": {roomID}, "thread_root_id": {threadRootID}}
		r := httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/messages/unread?"+query.Encode(), nil)
		ok := h.authorizeAgentPlatformRoute(r, "agent-dev")
		return r.URL.Query(), ok
	}
	const messagesPath = "/api/v1/channels/csgclaw/messages"
	const ackPath = messagesPath + "/unread/ack"
	if _, ok := post(messagesPath, map[string]any{"room_id": second.ID, "content": "Wrong room"}); ok {
		t.Fatal("CLI can send to another room it belongs to")
	}
	if _, ok := post(messagesPath, map[string]any{"room_id": first.ID, "content": "Wrong thread", "relates_to": map[string]any{"rel_type": "m.thread", "event_id": "elsewhere"}}); ok {
		t.Fatal("CLI can send to another thread")
	}
	payload, ok := post(messagesPath, map[string]any{"room_id": first.ID, "content": "Reply"})
	relation, _ := payload["relates_to"].(map[string]any)
	if !ok || relation["event_id"] != root.ID {
		t.Fatalf("CLI did not inherit the active thread: %v", payload)
	}
	for _, target := range [][2]string{{second.ID, ""}, {first.ID, "elsewhere"}} {
		if _, ok := read(target[0], target[1]); ok {
			t.Fatalf("CLI can read unread messages outside the active conversation: %v", target)
		}
		if _, ok := post(ackPath, map[string]any{"room_id": target[0], "thread_root_id": target[1]}); ok {
			t.Fatalf("CLI can acknowledge unread messages outside the active conversation: %v", target)
		}
	}
	if query, ok := read(first.ID, ""); !ok || query.Get("thread_root_id") != root.ID {
		t.Fatalf("unread read did not inherit the active thread: %v", query)
	}
	if ack, ok := post(ackPath, map[string]any{"room_id": first.ID}); !ok || ack["thread_root_id"] != root.ID {
		t.Fatalf("unread ack did not inherit the active thread: %v", ack)
	}
	start(second.ID, "")
	if _, ok := post(messagesPath, map[string]any{"room_id": first.ID, "content": "Ambiguous origin"}); ok {
		t.Fatal("CLI inferred the origin from the requested destination")
	}
	if _, ok := read(first.ID, root.ID); ok {
		t.Fatal("CLI inferred the unread conversation from the requested scope")
	}
	if _, ok := post(ackPath, map[string]any{"room_id": first.ID, "thread_root_id": root.ID}); ok {
		t.Fatal("CLI inferred the acknowledgement conversation from the requested scope")
	}
	h.registerAppRoomTools("agent-dev")
	dev := taskMCPClient(t, h, "agent-dev")
	denyTaskTool(t, dev, "messages_read", map[string]any{"room_id": first.ID, "thread_root_id": root.ID})
	for _, roomID := range []string{first.ID, second.ID} {
		conversation := "csgclaw-im:pt-dev:room:" + url.QueryEscape(roomID)
		var thread string
		if roomID == first.ID {
			thread = root.ID
			conversation += ":thread:" + url.QueryEscape(thread)
		}
		for _, name := range []string{"message_send", "messages_read", "messages_ack"} {
			args := map[string]any{"room_id": roomID}
			if err := h.scopeAppPlatformRoomConversation("agent-dev", name, conversation, "", args); err != nil || args["thread_root_id"] != thread {
				t.Fatalf("native %s did not preserve the current conversation with two active rooms: %v (%v)", name, args, err)
			}
		}
	}
}

func TestUnreadHTTPBindsAuthenticatedIdentity(t *testing.T) {
	h := newAppTaskTestHandler(t)
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "r", CreatorID: "admin", MemberIDs: []string{"dev", "qa"}, Type: apitypes.RoomTypeAutonomous})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "Read"}); err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string, handler http.HandlerFunc) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), appAgentContextKey{}, "agent-dev"))
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec
	}
	rec := request("GET", "/api/v1/messages/unread?room_id="+room.ID+"&participant_id=qa", "", h.readUnreadMessages)
	if rec.Code != 403 {
		t.Fatalf("spoof=%d", rec.Code)
	}
	rec = request("GET", "/api/v1/messages/unread?room_id="+room.ID, "", h.readUnreadMessages)
	var page apitypes.UnreadPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || rec.Code != 200 || page.ParticipantID != "pt-dev" {
		t.Fatalf("read=%d %s", rec.Code, rec.Body.String())
	}
	raw, _ := json.Marshal(apitypes.UnreadAck{RoomID: room.ID, Token: page.Token})
	rec = request("POST", "/api/v1/messages/unread/ack", string(raw), h.acknowledgeUnreadMessages)
	if rec.Code != 200 {
		t.Fatalf("ack=%s", rec.Body.String())
	}
}
