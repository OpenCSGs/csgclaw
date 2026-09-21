package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agenttask"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/taskcore"
	"csgclaw/internal/team"
)

func TestScopedRuntimeCLITasksAndIdentityOverHTTP(t *testing.T) {
	h, alice, bob, aliceToken, bobToken := newAppPlatformAuthFixture(t)
	controller := h.svc.(*agent.Controller)
	manager, err := controller.EnsureManager(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := controller.PicoClawRuntimeHost().ResolveRuntimeProfile(agentruntime.Handle{RuntimeID: manager.RuntimeID})
	if err != nil {
		t.Fatal(err)
	}
	managerToken := profile.APIKey
	aliceActor := h.agentPlatformParticipant(alice.ID)
	bobActor := h.agentPlatformParticipant(bob.ID)
	h.im = im.NewService()
	h.participantBridge = im.NewParticipantBridge("")
	for _, item := range []agent.Agent{alice, bob, manager} {
		if _, _, err := h.im.EnsureAgentUser(im.EnsureAgentUserRequest{ID: item.ID, Name: item.Name, Role: item.Role}); err != nil {
			t.Fatal(err)
		}
	}
	core := taskcore.NewService()
	h.SetRoomTaskCore(core)
	h.agentTaskSvc = agenttask.NewService(core, h.im, nil, nil)
	adapter := team.NewCSGClawAdapter(h.im)
	h.teamSvc = team.NewService(team.WithProjector(newTestTeamProjector(adapter)))
	h.teamAdapters = newTestTeamAdapterRegistry(adapter)
	call := func(token, method, path string, payload any, status int, out any) {
		t.Helper()
		body := ""
		if payload != nil {
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			body = string(raw)
		}
		rec := appAuthRequest(t, h, method, path, body, token, nil)
		if rec.Code != status {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, path, rec.Code, status, rec.Body)
		}
		if out != nil {
			if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
	}
	var direct apitypes.TeamTask
	call(managerToken, http.MethodPost, "/api/v1/agent-tasks", map[string]any{"agent_id": alice.ID, "title": "Implement"}, 201, &direct)
	if direct.CreatedBy != agent.ManagerParticipantID {
		t.Fatal("direct task creator not bound to manager")
	}
	call(aliceToken, http.MethodPost, "/api/v1/agent-tasks", map[string]any{"agent_id": bob.ID, "title": "Unauthorized"}, 403, nil)
	call(bobToken, http.MethodPost, "/api/v1/agent-tasks/"+direct.ID+"/claim", map[string]any{"participant_id": aliceActor}, 403, nil)
	call(aliceToken, http.MethodPost, "/api/v1/agent-tasks/"+direct.ID+"/claim", map[string]any{"participant_id": bobActor}, 403, nil)
	call(aliceToken, http.MethodPost, "/api/v1/agent-tasks/"+direct.ID+"/claim", map[string]any{"participant_id": aliceActor}, 200, &direct)
	if direct.ClaimedBy != aliceActor {
		t.Fatal("CLI claim lost caller identity")
	}
	call(aliceToken, http.MethodPatch, "/api/v1/agent-tasks/"+direct.ID, map[string]any{"actor_id": aliceActor, "status": "completed", "result": "Done"}, 200, nil)
	call(managerToken, http.MethodPost, "/api/v1/agent-tasks", map[string]any{"agent_id": bob.ID, "title": "Private Bob task"}, 201, nil)
	var ownTasks []apitypes.TeamTask
	call(aliceToken, http.MethodGet, "/api/v1/agent-tasks?agent_id="+bob.ID, nil, 200, &ownTasks)
	if len(ownTasks) != 1 || ownTasks[0].ID != direct.ID {
		t.Fatal("direct task listing leaked another Agent's tasks")
	}

	meta, err := h.teamSvc.CreateTeam(team.CreateTeamInput{Title: "Shared", LeadAgentID: manager.ID, MemberAgentIDs: []string{alice.ID, bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	other, err := h.teamSvc.CreateTeam(team.CreateTeamInput{Title: "Bob only", LeadAgentID: manager.ID, MemberAgentIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	var ownTeams []apitypes.Team
	call(aliceToken, http.MethodGet, "/api/v1/teams", nil, 200, &ownTeams)
	if len(ownTeams) != 1 || ownTeams[0].ID != meta.ID {
		t.Fatal("team listing leaked an unrelated team")
	}
	call(aliceToken, http.MethodGet, "/api/v1/teams/"+other.ID+"/tasks", nil, 403, nil)
	var batch apitypes.CreateTeamTasksBatchResponse
	path := "/api/v1/teams/" + meta.ID
	payload := map[string]any{"tasks": []any{map[string]any{"title": "Team implementation", "assign_to": aliceActor}}}
	call(aliceToken, http.MethodPost, path+"/tasks/batch", payload, 403, nil)
	call(managerToken, http.MethodPost, path+"/tasks/batch", payload, 201, &batch)
	if len(batch.Tasks) != 1 {
		t.Fatal("missing team task")
	}
	task := batch.Tasks[0]
	call(bobToken, http.MethodPost, path+"/tasks/"+task.ID+"/claim", map[string]any{"participant_id": aliceActor}, 403, nil)
	call(aliceToken, http.MethodPost, path+"/tasks/"+task.ID+"/plan", map[string]any{}, 403, nil)
	call(aliceToken, http.MethodPost, path+"/tasks/"+task.ID+"/start", map[string]any{}, 403, nil)
	call(aliceToken, http.MethodPost, path+"/tasks/"+task.ID+"/claim", map[string]any{"participant_id": aliceActor}, 200, nil)
	call(bobToken, http.MethodPatch, path+"/tasks/"+task.ID, map[string]any{"actor_id": aliceActor, "status": "completed"}, 403, nil)
	var approval apitypes.TeamApproval
	request := map[string]any{"task_id": task.ID, "approver_id": agent.ManagerParticipantID, "kind": "decision", "summary": "Approve"}
	call(bobToken, http.MethodPost, path+"/approvals", request, 403, nil)
	call(aliceToken, http.MethodPost, path+"/approvals", request, 201, &approval)
	if approval.RequestedBy != aliceActor {
		t.Fatal("approval requester not injected")
	}
	call(bobToken, http.MethodPost, path+"/approvals/"+approval.ID+"/resolve", map[string]any{"status": "approved", "approver_id": agent.ManagerParticipantID}, 403, nil)
	call(managerToken, http.MethodPost, path+"/approvals/"+approval.ID+"/resolve", map[string]any{"status": "approved", "approver_id": agent.ManagerParticipantID}, 200, nil)
	call(aliceToken, http.MethodPost, "/api/v1/teams/tasks/claim-next", map[string]any{"participant_id": aliceActor}, 403, nil)
	before := len(h.teamSvc.ListApprovals(meta.ID))
	selfRequest := map[string]any{"task_id": task.ID, "approver_id": aliceActor, "kind": "decision", "summary": "Attempt self approval"}
	call(aliceToken, http.MethodPost, path+"/approvals", selfRequest, 403, nil)
	call(aliceToken, http.MethodPost, path+"/approvals/"+approval.ID+"/resolve", map[string]any{"status": "approved", "approver_id": aliceActor}, 403, nil)
	if len(h.teamSvc.ListApprovals(meta.ID)) != before {
		t.Fatal("scoped REST self-approval created a record")
	}
	for _, requester := range []string{aliceActor, agent.ManagerParticipantID} {
		bad, err := h.teamSvc.RequestApproval(team.RequestApprovalInput{TeamID: meta.ID, TaskID: task.ID, RequestedBy: requester, ApproverID: requester, Kind: "decision", Summary: "Existing invalid approval"})
		if err != nil {
			t.Fatal(err)
		}
		caller := aliceToken
		if requester == agent.ManagerParticipantID {
			caller = managerToken
		}
		call(caller, http.MethodPost, path+"/approvals/"+bad.ID+"/resolve", map[string]any{"status": "approved", "approver_id": requester}, 403, nil)
	}
	var human apitypes.TeamApproval
	call(managerToken, http.MethodPost, path+"/approvals", map[string]any{"task_id": task.ID, "approver_id": "pt-admin", "kind": "decision", "summary": "Human approval needed"}, 201, &human)
	call(managerToken, http.MethodPost, path+"/approvals/"+human.ID+"/resolve", map[string]any{"status": "approved"}, 403, nil)
	call(h.serverAccessToken, http.MethodPost, path+"/approvals/"+human.ID+"/resolve", map[string]any{"status": "approved", "approver_id": "pt-admin"}, 200, nil)
}
