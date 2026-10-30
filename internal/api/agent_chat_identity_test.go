package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/enginetest"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

func independentChatIdentityHandler(t *testing.T) (*Handler, string, string) {
	t.Helper()
	engine := enginetest.NewMemoryClient(agentengine.Agent{ID: "agent-executor", Spec: agentengine.AgentSpec{Name: "worker", Role: agentengine.AgentRoleWorker, Runtime: agentengine.RuntimeSpec{Adapter: "codex"}}})
	dir := t.TempDir()
	imPath, participantPath := filepath.Join(dir, "im", "state.json"), filepath.Join(dir, "participants.json")
	err := im.SaveBootstrap(imPath, im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{
		{ID: im.AdminUserID, Name: "admin", Role: "admin"},
		{ID: "user-chat", Name: "worker", Role: "worker", Avatar: "avatar/3D-1.png"},
		{ID: "user-second", Name: "second", Role: "worker"},
		{ID: "user-executor", Name: "unrelated", Role: "worker"},
	}, Rooms: []im.Room{
		{ID: "group", Members: []string{im.AdminUserID, "user-chat", "user-second"}, Messages: []im.Message{{ID: "old", SenderID: "user-chat", Content: "keep history"}}},
		{ID: "direct", IsDirect: true, Members: []string{im.AdminUserID, "user-chat"}},
		{ID: "unrelated", Members: []string{im.AdminUserID, "user-executor"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := im.NewServiceFromPath(imPath)
	if err != nil {
		t.Fatal(err)
	}
	store, err := participant.NewStore(participantPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []participant.Participant{
		{ID: "pt-entry", Channel: "csgclaw", Type: "agent", Name: "worker", AgentID: "agent-executor", ChannelUserRef: "user-chat", ChannelUserKind: "local_user_id"},
		{ID: "pt-secondary", Channel: "csgclaw", Type: "agent", Name: "second", AgentID: "agent-executor", ChannelUserRef: "user-second", ChannelUserKind: "local_user_id"},
	} {
		if err := store.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(AgentServices{}, engine, chat, nil, nil, nil, nil)
	h.participant = participant.NewService(store, participant.WithAgentEngine(engine), participant.WithIMService(chat))
	return h, imPath, participantPath
}

func chatIdentityRequest(t *testing.T, h *Handler, method, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	if rec.Code != want {
		t.Fatalf("%s %s = %d, want %d: %s", method, path, rec.Code, want, rec.Body)
	}
	return rec
}

func TestIndependentChatIdentityLifecycleHTTP(t *testing.T) {
	h, imPath, participantPath := independentChatIdentityHandler(t)
	var impact agentArchiveImpact
	rec := chatIdentityRequest(t, h, http.MethodGet, "/api/v1/agents/agent-executor/archive-impact", "", 200)
	if err := json.Unmarshal(rec.Body.Bytes(), &impact); err != nil {
		t.Fatal(err)
	}
	if impact.Rooms != 2 {
		t.Fatalf("rooms=%d, want distinct group + direct", impact.Rooms)
	}
	for _, action := range []string{"archive", "restore", "archive"} {
		chatIdentityRequest(t, h, http.MethodPost, "/api/v1/agents/agent-executor/"+action, "", 200)
		// Repeated requests must not duplicate system notices.
		chatIdentityRequest(t, h, http.MethodPost, "/api/v1/agents/agent-executor/"+action, "", 200)
		want := "archived"
		if action == "restore" {
			want = ""
		}
		for _, id := range []string{"user-chat", "user-second"} {
			u, _ := h.im.User(id)
			if u.AgentState != want {
				t.Fatalf("%s state=%s, want %s", id, u.AgentState, want)
			}
		}
		unrelated, _ := h.im.User("user-executor")
		if unrelated.AgentState != "" {
			t.Fatalf("unrelated state=%s", unrelated.AgentState)
		}
		room, _ := h.im.Room("unrelated")
		if len(room.Messages) != 0 || !slices.Contains(room.Members, "user-executor") {
			t.Fatalf("unrelated room changed: %+v", room)
		}
		for _, room := range h.im.ListRooms() {
			if room.ID == "direct" && action == "archive" {
				t.Fatal("archived DM remains visible")
			}
		}
	}
	chatIdentityRequest(t, h, http.MethodDelete, "/api/v1/agents/agent-executor", `{"name":"worker"}`, 204)
	group, _ := h.im.Room("group")
	if group.Messages[0].SenderID != "user-chat" || len(group.Messages) != 9 {
		t.Fatalf("history/notices=%+v", group.Messages)
	}
	for _, msg := range group.Messages[1:] {
		if msg.Event == nil || len(msg.Event.TargetIDs) != 1 || !slices.Contains([]string{"user-chat", "user-second"}, msg.Event.TargetIDs[0]) {
			t.Fatalf("wrong event target: %+v", msg)
		}
	}
	reloaded, err := im.NewServiceFromPath(imPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"user-chat", "user-second"} {
		u, _ := reloaded.User(id)
		if u.AgentState != "deleted" || u.AgentID != "agent-executor" || u.DeletedAt == nil {
			t.Fatalf("lost tombstone: %+v", u)
		}
	}
	old, _ := reloaded.User("user-chat")
	if old.Name != "worker" || old.Avatar != "avatar/3D-1.png" {
		t.Fatalf("lost attribution: %+v", old)
	}
	other, _ := reloaded.User("user-executor")
	room, _ := reloaded.Room("unrelated")
	if other.AgentState != "" || !slices.Contains(room.Members, "user-executor") || len(room.Messages) != 0 {
		t.Fatalf("unrelated identity damaged: %+v %+v", other, room)
	}
	store, err := participant.NewStore(participantPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.List(participant.ListOptions{AgentID: "agent-executor"})) != 0 {
		t.Fatal("bindings survived deletion")
	}
	chatIdentityRequest(t, h, http.MethodPost, "/api/v1/agents", `{"id":"agent-executor","name":"worker","runtime_name":"codex"}`, 409)
	chatIdentityRequest(t, h, http.MethodPost, "/api/v1/channels/csgclaw/participants", `{"id":"replacement","type":"agent","name":"worker","agent_binding":{"mode":"create","agent":{"runtime_name":"codex"}}}`, 201)
}

func TestIndependentChatIdentityReplaceHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"same name", `{"id":"agent-executor","name":"worker","replace":true,"field_mask":["name"]}`, 201},
		{"another own identity", `{"id":"agent-executor","name":"second","replace":true,"field_mask":["name"]}`, 201},
		{"another user's name", `{"id":"agent-executor","name":"unrelated","replace":true,"field_mask":["name"]}`, 400},
		{"masked out name", `{"id":"agent-executor","name":"unrelated","description":"changed","replace":true,"field_mask":["description"]}`, 201},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _ := independentChatIdentityHandler(t)
			chatIdentityRequest(t, h, http.MethodPost, "/api/v1/agents", tc.body, tc.status)
			if tc.status == 400 {
				a, err := h.agentEngine.Agents().Get(context.Background(), "agent-executor", agentengine.AgentGetOptions{})
				if err != nil || a.Spec.Name != "worker" {
					t.Fatalf("rejected replace changed runtime: %+v %v", a, err)
				}
			}
		})
	}
}

func TestDeletedUnrelatedSuffixDoesNotBlockReplacement(t *testing.T) {
	h, _, _ := independentChatIdentityHandler(t)
	if err := h.im.RetireAgentUser("user-executor", "agent-other"); err != nil {
		t.Fatal(err)
	}
	chatIdentityRequest(t, h, http.MethodPost, "/api/v1/agents", `{"id":"agent-executor","name":"worker","replace":true,"field_mask":["name"]}`, 201)
}

func TestLegacyChatIdentityRequiresMatchingProfileAndNoBinding(t *testing.T) {
	for _, tc := range []struct {
		name, userName, role string
		claimed              bool
		want                 bool
	}{
		{"matching legacy", "worker", "worker", false, true},
		{"different name", "someone else", "worker", false, false},
		{"human", "worker", "member", false, false},
		{"claimed elsewhere", "worker", "worker", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := enginetest.NewMemoryClient(agentengine.Agent{ID: "agent-legacy", Spec: agentengine.AgentSpec{Name: "worker", Role: agentengine.AgentRoleWorker}})
			chat := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Name: "admin", Role: "admin"}, {ID: "user-legacy", Name: tc.userName, Role: tc.role}}})
			h := NewHandler(AgentServices{}, engine, chat, nil, nil, nil, nil)
			var ps []participant.Participant
			if tc.claimed {
				ps = append(ps, participant.Participant{ID: "pt-other", Channel: "csgclaw", Type: "agent", AgentID: "agent-other", ChannelUserRef: "user-legacy", ChannelUserKind: "local_user_id"})
			}
			h.participant = participant.NewService(participant.NewMemoryStore(ps), participant.WithAgentEngine(engine), participant.WithIMService(chat))
			chatIdentityRequest(t, h, http.MethodPost, "/api/v1/agents/agent-legacy/archive", "", 200)
			chatIdentityRequest(t, h, http.MethodDelete, "/api/v1/agents/agent-legacy", `{"name":"worker"}`, 204)
			u, _ := chat.User("user-legacy")
			if (u.AgentState == "deleted") != tc.want {
				t.Fatalf("user=%+v, should retire=%v", u, tc.want)
			}
		})
	}
}

func TestMissingBoundUserDoesNotRetireSuffixOrNameMatch(t *testing.T) {
	h, _, _ := independentChatIdentityHandler(t)
	// A dangling binding is still authoritative. It must not fall back to the
	// unrelated user-executor or a member whose display name is user-chat.
	if err := h.im.DeleteUser("user-chat"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.im.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "user-name-match", Name: "user-chat", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	chatIdentityRequest(t, h, http.MethodPost, "/api/v1/agents/agent-executor/archive", "", 200)
	chatIdentityRequest(t, h, http.MethodDelete, "/api/v1/agents/agent-executor", `{"name":"worker"}`, 204)
	for _, id := range []string{"user-executor", "user-name-match"} {
		u, _ := h.im.User(id)
		if u.AgentState != "" {
			t.Fatalf("unrelated user retired: %+v", u)
		}
	}
}

func TestDeletingOneAgentPreservesSharedChatIdentity(t *testing.T) {
	h, _, _ := independentChatIdentityHandler(t)
	other, err := h.agentEngine.Agents().Create(context.Background(), agentengine.AgentCreateRequest{ID: "agent-other", Spec: agentengine.AgentSpec{Name: "other", Role: agentengine.AgentRoleWorker, Runtime: agentengine.RuntimeSpec{Adapter: "codex"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.participant.Create(context.Background(), participant.CreateRequest{
		ID: "shared", Channel: "csgclaw", Type: "agent", Name: "worker",
		ChannelUser:  participant.ChannelUserSpec{Ref: "user-chat", Kind: "local_user_id"},
		AgentBinding: participant.AgentBindingSpec{Mode: "reuse", AgentID: other.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	chatIdentityRequest(t, h, http.MethodPost, "/api/v1/agents/agent-executor/archive", "", 200)
	u, _ := h.im.User("user-chat")
	if u.AgentState != "" {
		t.Fatalf("active shared identity hidden: %+v", u)
	}
	chatIdentityRequest(t, h, http.MethodDelete, "/api/v1/agents/agent-executor", `{"name":"worker"}`, 204)
	u, _ = h.im.User("user-chat")
	group, _ := h.im.Room("group")
	if u.AgentState != "" || !slices.Contains(group.Members, "user-chat") {
		t.Fatalf("shared identity retired: %+v %+v", u, group)
	}
	for _, message := range group.Messages {
		if message.Event != nil && slices.Contains(message.Event.TargetIDs, "user-chat") {
			t.Fatalf("shared active identity received a removal notice: %+v", message)
		}
	}
}
