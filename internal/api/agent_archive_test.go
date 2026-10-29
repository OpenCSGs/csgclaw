package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/enginetest"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	"csgclaw/internal/scheduledtask"
	"csgclaw/internal/team"
)

func TestDeletedAgentCanBeRecreatedByNameWithoutReusingHistoryHTTP(t *testing.T) {
	engine := enginetest.NewMemoryClient(agentengine.Agent{ID: "agent-old", Spec: agentengine.AgentSpec{
		Name: "worker", Role: agentengine.AgentRoleWorker, Runtime: agentengine.RuntimeSpec{Adapter: "codex"},
	}})
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{
		{ID: im.AdminUserID, Name: "admin", Role: "admin"},
		{ID: "user-old", Name: "worker", Role: "worker", Avatar: "avatar/3D-1.png"},
	}, Rooms: []im.Room{{ID: "group", Members: []string{im.AdminUserID, "user-old"},
		Messages: []im.Message{{ID: "old", SenderID: "user-old", Content: "keep old attribution"}},
	}}})
	h := NewHandler(AgentServices{}, engine, imSvc, nil, nil, nil, nil)
	h.participant = participant.NewService(participant.NewMemoryStore([]participant.Participant{{
		ID: "pt-old", Channel: "csgclaw", Type: "agent", Name: "worker", AgentID: "agent-old",
		ChannelUserRef: "user-old", ChannelUserKind: "local_user_id",
	}}), participant.WithAgentEngine(engine), participant.WithIMService(imSvc))
	store, err := scheduledtask.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	schedules, err := scheduledtask.NewService(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.SetScheduledTaskService(schedules)
	schedule, err := schedules.Create(scheduledtask.CreateInput{Title: "old schedule", AgentID: "agent-old", Prompt: "work", Recurrence: scheduledtask.RecurrenceDaily, FirstRunAt: time.Now().Add(time.Hour), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	routes := h.Routes()
	request := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code != want {
			t.Fatalf("%s %s = %d: %s", method, path, rec.Code, rec.Body.String())
		}
		return rec
	}
	const createNew = `{"id":"new","type":"agent","name":"worker","avatar":"avatar/3D-5.png","agent_binding":{"mode":"create","agent":{"name":"worker","role":"worker","runtime_name":"codex"}}}`
	request(http.MethodPost, "/api/v1/agents/agent-old/archive", "", 200)
	request(http.MethodPost, "/api/v1/channels/csgclaw/participants", createNew, 409)
	if _, err := engine.Agents().Get(context.Background(), "agent-new", agentengine.AgentGetOptions{}); err == nil {
		t.Fatal("rejected creation left an orphan runtime")
	}
	request(http.MethodDelete, "/api/v1/agents/agent-old", `{"name":"worker"}`, 204)
	for _, tc := range []struct{ path, body string }{
		{"/api/v1/channels/csgclaw/participants", `{"id":"old","type":"agent","name":"worker","agent_binding":{"mode":"create","agent":{"runtime_name":"codex"}}}`},
		{"/api/v1/channels/csgclaw/users", `{"id":"old","name":"worker","role":"worker"}`},
		{"/api/v1/agents", `{"id":"agent-old","name":"worker","runtime_name":"codex"}`},
	} {
		response := request(http.MethodPost, tc.path, tc.body, 409)
		if !strings.Contains(response.Body.String(), `"code":"agent_identity_deleted"`) {
			t.Fatalf("missing identity error: %s", response.Body)
		}
	}
	if _, err := engine.Agents().Get(context.Background(), "agent-old", agentengine.AgentGetOptions{}); err == nil {
		t.Fatal("rejected recreation left an orphan runtime")
	}
	response := request(http.MethodPost, "/api/v1/channels/csgclaw/participants", createNew, 201)
	var created apitypes.Participant
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.AgentID != "agent-new" {
		t.Fatalf("replacement agent = %+v", created)
	}
	request(http.MethodPatch, "/api/v1/channels/csgclaw/users/user-new", `{"avatar":"avatar/3D-5.png"}`, 200)
	old, _ := imSvc.User("user-old")
	current, _ := imSvc.User("user-new")
	if old.AgentState != "deleted" || old.Avatar != "avatar/3D-1.png" || old.Name != "worker" || old.AgentID != "agent-old" || old.DeletedAt == nil || current.AgentState != "" || current.Avatar != "avatar/3D-5.png" {
		t.Fatalf("mixed identities: old=%+v new=%+v", old, current)
	}
	group, _ := imSvc.Room("group")
	if group.Messages[0].SenderID != "user-old" || slices.Contains(group.Members, "user-new") {
		t.Fatal("replacement inherited old collaboration history")
	}
	direct, err := imSvc.CreateRoom(im.CreateRoomRequest{IsDirect: true, Title: "New chat", CreatorID: im.AdminUserID, MemberIDs: []string{"user-new"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := imSvc.CreateMessage(im.CreateMessageRequest{RoomID: direct.ID, SenderID: im.AdminUserID, Content: "new chat"}); err != nil {
		t.Fatal(err)
	}
	var tasks []apitypes.ScheduledTask
	if err := json.Unmarshal(request(http.MethodGet, "/api/v1/scheduled-tasks", "", 200).Body.Bytes(), &tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].AgentID != "agent-old" || tasks[0].AgentName != "worker" || tasks[0].AgentState != "deleted" || tasks[0].Enabled {
		t.Fatalf("old schedule changed identity: %+v", tasks)
	}
	request(http.MethodPatch, "/api/v1/scheduled-tasks/"+schedule.ID, `{"enabled":true}`, 400)
	request(http.MethodPatch, "/api/v1/scheduled-tasks/"+schedule.ID, `{"agent_id":"agent-new","enabled":true}`, 200)
}

func TestAgentArchiveRestoreAndConfirmedDeletionHTTP(t *testing.T) {
	engine := enginetest.NewMemoryClient(agentengine.Agent{ID: "agent-worker", Spec: agentengine.AgentSpec{Name: "worker", Role: agentengine.AgentRoleWorker, Runtime: agentengine.RuntimeSpec{Adapter: "codex"}}})
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Name: "admin", Role: "admin"}, {ID: "user-worker", Name: "worker", Role: "worker"}}, Rooms: []im.Room{{ID: "direct", IsDirect: true, Members: []string{im.AdminUserID, "user-worker"}}, {ID: "group", Members: []string{im.AdminUserID, "user-worker"}, Messages: []im.Message{{ID: "old", SenderID: "user-worker", Content: "keep me"}}}}})
	h := NewHandler(AgentServices{}, engine, imSvc, nil, nil, nil, nil)
	teams := team.NewService()
	meta, err := teams.CreateTeam(team.CreateTeamInput{Title: "team", LeadAgentID: "agent-manager", MemberAgentIDs: []string{"agent-worker"}})
	if err != nil {
		t.Fatal(err)
	}
	h.SetTeamService(teams)
	routes := h.Routes()
	request := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code != want {
			t.Fatalf("%s %s = %d: %s", method, path, rec.Code, rec.Body.String())
		}
		return rec
	}
	request(http.MethodDelete, "/api/v1/agents/agent-worker", `{"name":"worker"}`, 400)
	request(http.MethodPost, "/api/v1/agents/agent-worker/archive", "", 200)
	request(http.MethodPost, "/api/v1/agents/agent-worker/archive", "", 200)
	group, _ := imSvc.Room("group")
	if len(group.Messages) != 2 || group.Messages[1].Event.Key != im.AgentArchivedEventKey {
		t.Fatalf("archive notice missing or duplicated: %+v", group.Messages)
	}
	var active []agentResponse
	if err = json.Unmarshal(request(http.MethodGet, "/api/v1/agents", "", 200).Body.Bytes(), &active); err != nil || len(active) != 0 {
		t.Fatalf("active=%v %v", active, err)
	}
	var archived []agentResponse
	if err = json.Unmarshal(request(http.MethodGet, "/api/v1/agents?archived=true", "", 200).Body.Bytes(), &archived); err != nil || len(archived) != 1 || archived[0].ArchivedAt == nil {
		t.Fatalf("archived=%v %v", archived, err)
	}
	if len(imSvc.Bootstrap().Rooms) != 1 {
		t.Fatal("archived DM still visible")
	}
	request(http.MethodPost, "/api/v1/agents/agent-worker/start", "", 409)
	request(http.MethodPost, "/api/v1/agents/agent-worker/llm/v1/responses", `{}`, 409)
	if h.presentParticipant(apitypes.Participant{AgentID: "agent-worker", Mentionable: true}).Mentionable {
		t.Fatal("archived channel participant is still mentionable")
	}
	request(http.MethodPatch, "/api/v1/teams/"+meta.ID, `{"title":"edited while archived","member_agent_ids":[]}`, 200)
	var visible apitypes.Team
	if err := json.Unmarshal(request(http.MethodGet, "/api/v1/teams/"+meta.ID, "", 200).Body.Bytes(), &visible); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(visible.MemberAgentIDs, "agent-worker") {
		t.Fatal("archived member visible in normal team response")
	}
	request(http.MethodDelete, "/api/v1/agents/agent-worker", `{"name":"wrong"}`, 400)
	removedTeam, err := teams.CreateTeam(team.CreateTeamInput{Title: "removed while archived", LeadAgentID: "agent-manager", MemberAgentIDs: []string{"agent-worker"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := teams.DeleteTeam(removedTeam.ID); err != nil {
		t.Fatal(err)
	}
	if err := imSvc.DeleteRoom("direct"); err != nil {
		t.Fatal(err)
	}
	request(http.MethodPost, "/api/v1/agents/agent-worker/restore", "", 200)
	request(http.MethodPost, "/api/v1/agents/agent-worker/restore", "", 200)
	group, _ = imSvc.Room("group")
	if len(group.Messages) != 3 || group.Messages[2].Event.Key != im.AgentRestoredEventKey {
		t.Fatalf("restore notice missing or duplicated: %+v", group.Messages)
	}
	if !slices.Contains(group.Members, "user-worker") {
		t.Fatal("restore lost group membership")
	}
	if len(imSvc.Bootstrap().Rooms) != 1 {
		t.Fatal("restore recreated a deleted conversation")
	}
	if _, exists := teams.GetTeam(removedTeam.ID); exists {
		t.Fatal("restore recreated a deleted team")
	}
	kept, _ := teams.GetTeam(meta.ID)
	if !slices.Contains(kept.MemberAgentIDs, "agent-worker") {
		t.Fatal("archive removed team membership")
	}
	request(http.MethodPost, "/api/v1/agents/agent-worker/archive", "", 200)
	request(http.MethodDelete, "/api/v1/agents/agent-worker", `{"name":"worker"}`, 204)
	group, _ = imSvc.Room("group")
	if len(group.Messages) != 5 || group.Messages[0].Content != "keep me" || group.Messages[4].Event.Key != im.AgentDeletedEventKey {
		t.Fatal("delete erased group history")
	}
	user, _ := imSvc.User("user-worker")
	if user.AgentState != "deleted" {
		t.Fatal("missing deleted history label")
	}
	kept, exists := teams.GetTeam(meta.ID)
	if !exists || slices.Contains(kept.MemberAgentIDs, "agent-worker") {
		t.Fatal("delete should retain team and remove membership")
	}
}
