package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/enginetest"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/team"
)

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
