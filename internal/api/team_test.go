package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	"csgclaw/internal/team"
)

func TestTeamRoutesCreateAndTaskFlow(t *testing.T) {
	imSvc := im.NewService()
	adapter := team.NewCSGClawAdapter(imSvc)
	teamSvc := team.NewService(team.WithProjector(team.NewProjector(adapter, nil)))
	h := &Handler{teamSvc: teamSvc, teamAdapter: adapter}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"release","lead_bot_id":"u-manager","member_bot_ids":["u-worker"]}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var created apitypes.Team
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create team response: %v", err)
	}
	if created.ID == "" || created.RoomID == "" {
		t.Fatalf("created team = %+v, want ids", created)
	}
	if created.ID == created.RoomID {
		t.Fatalf("created.ID = created.RoomID = %q, want separate team and room ids", created.ID)
	}

	batchReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/batch", strings.NewReader(`{"created_by":"u-manager","tasks":[{"id_ref":"draft","title":"Draft release note","assign_to":"u-worker","priority":9}]}`))
	batchRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(batchRec, batchReq)
	if batchRec.Code != http.StatusCreated {
		t.Fatalf("create batch status = %d, want %d: %s", batchRec.Code, http.StatusCreated, batchRec.Body.String())
	}

	var batchResp apitypes.CreateTeamTasksBatchResponse
	if err := json.NewDecoder(batchRec.Body).Decode(&batchResp); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	if len(batchResp.Tasks) != 1 {
		t.Fatalf("batch tasks len = %d, want 1", len(batchResp.Tasks))
	}

	claimReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/claim-next", strings.NewReader(`{"bot_id":"u-worker"}`))
	claimRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(claimRec, claimReq)
	if claimRec.Code != http.StatusOK {
		t.Fatalf("claim-next status = %d, want %d: %s", claimRec.Code, http.StatusOK, claimRec.Body.String())
	}

	var claimed apitypes.TeamTask
	if err := json.NewDecoder(claimRec.Body).Decode(&claimed); err != nil {
		t.Fatalf("decode claim-next response: %v", err)
	}
	if claimed.Status != team.TaskStatusInProgress {
		t.Fatalf("claimed task status = %q, want %q", claimed.Status, team.TaskStatusInProgress)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.ID+"/tasks/"+claimed.ID, strings.NewReader(`{"actor_id":"u-worker","status":"completed","result":"done"}`))
	updateRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update task status = %d, want %d: %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}

	var updated apitypes.TeamTask
	if err := json.NewDecoder(updateRec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.Status != team.TaskStatusCompleted {
		t.Fatalf("updated task status = %q, want %q", updated.Status, team.TaskStatusCompleted)
	}
}

func TestTeamPlanAutoStartCreatesExecutionRoomAndDispatches(t *testing.T) {
	imSvc := im.NewService()
	adapter := team.NewCSGClawAdapter(imSvc)
	teamSvc := team.NewService(team.WithProjector(team.NewProjector(adapter, nil)))
	h := &Handler{im: imSvc, teamSvc: teamSvc, teamAdapter: adapter}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"release","lead_bot_id":"u-manager","member_bot_ids":["u-worker"]}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	var created apitypes.Team
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create team response: %v", err)
	}

	batchReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/batch", strings.NewReader(`{"created_by":"web","tasks":[{"id_ref":"parent","title":"Ship release"},{"title":"Draft release note","parent_ref":"parent","assign_to":"u-worker"}]}`))
	batchRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(batchRec, batchReq)
	if batchRec.Code != http.StatusCreated {
		t.Fatalf("create batch status = %d, want %d: %s", batchRec.Code, http.StatusCreated, batchRec.Body.String())
	}
	var batchResp apitypes.CreateTeamTasksBatchResponse
	if err := json.NewDecoder(batchRec.Body).Decode(&batchResp); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	if len(batchResp.Tasks) != 2 {
		t.Fatalf("batch tasks len = %d, want 2", len(batchResp.Tasks))
	}
	parentID := batchResp.Tasks[0].ID
	childID := batchResp.Tasks[1].ID

	planReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/"+parentID+"/plan", strings.NewReader(`{"actor_id":"web","auto_start":true}`))
	planRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(planRec, planReq)
	if planRec.Code != http.StatusOK {
		t.Fatalf("plan auto-start status = %d, want %d: %s", planRec.Code, http.StatusOK, planRec.Body.String())
	}
	var planResp apitypes.PlanTeamTaskResponse
	if err := json.NewDecoder(planRec.Body).Decode(&planResp); err != nil {
		t.Fatalf("decode plan response: %v", err)
	}
	if !planResp.Started || planResp.ScheduledTasks != 1 {
		t.Fatalf("plan auto-start response = %+v, want started with one scheduled task", planResp)
	}
	if planResp.Task.RoomID == "" || planResp.Task.RoomID == created.RoomID {
		t.Fatalf("plan task room = %q, want dedicated execution room distinct from %q", planResp.Task.RoomID, created.RoomID)
	}
	taskRoom, ok := imSvc.Room(planResp.Task.RoomID)
	if !ok {
		t.Fatalf("Room(%s) ok = false, want true", planResp.Task.RoomID)
	}
	if !roomContainsMention(taskRoom, "Task "+childID+" is ready for you", "u-worker") {
		t.Fatalf("task room messages missing dispatch mention: %+v", taskRoom.Messages)
	}
	if roomContains(taskRoom, "started assigning tasks") {
		t.Fatalf("task room should not include dispatch preamble: %+v", taskRoom.Messages)
	}
}

func TestTeamRoutesPlanStartDispatchesWithManagerLLM(t *testing.T) {
	var sawPlannerRequest bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("planner path = %q, want /v1/chat/completions", r.URL.Path)
		}
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode planner request: %v", err)
		}
		if payload.Model != "gpt-planner" {
			t.Fatalf("planner model = %q, want gpt-planner", payload.Model)
		}
		if len(payload.Messages) < 2 || !strings.Contains(payload.Messages[1].Content, "u-writer") || !strings.Contains(payload.Messages[1].Content, "release writer") {
			t.Fatalf("planner context messages = %+v, want team member capabilities", payload.Messages)
		}
		sawPlannerRequest = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "chatcmpl-plan",
			"object": "chat.completion",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role": "assistant",
						"content": `{
							"plan_summary": "Split because writing and verification are separate roles.",
							"tasks": [
								{"id_ref":"draft","title":"Draft release note","assign_to":"u-writer","priority":"high","goal":"Draft the release note","assignee_reason":"u-writer is the release writer","deliverable":"release note draft"},
								{"id_ref":"verify","title":"Verify release checklist","assign_to":"u-tester","depends_on_refs":["draft"],"priority":8,"goal":"Verify the release checklist","assignee_reason":"u-tester owns verification","deliverable":"passed checklist"}
							]
						}`,
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
	defer upstream.Close()

	agentSvc := mustNewSeededService(t, []agent.Agent{
		{
			ID:          agent.ManagerUserID,
			Name:        "manager",
			Description: "team manager",
			Role:        agent.RoleManager,
			AgentProfile: agent.AgentProfile{
				Name:            "manager",
				Provider:        agent.ProviderAPI,
				BaseURL:         upstream.URL + "/v1",
				APIKey:          "sk-test",
				ModelID:         "gpt-planner",
				ProfileComplete: true,
			},
			ProfileComplete: true,
			CreatedAt:       time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:          "u-writer",
			Name:        "writer",
			Description: "release writer",
			Role:        agent.RoleWorker,
			CreatedAt:   time.Date(2026, 5, 30, 9, 0, 1, 0, time.UTC),
		},
		{
			ID:          "u-tester",
			Name:        "tester",
			Description: "release verifier",
			Role:        agent.RoleWorker,
			CreatedAt:   time.Date(2026, 5, 30, 9, 0, 2, 0, time.UTC),
		},
	})
	imSvc := im.NewService()
	adapter := team.NewCSGClawAdapter(imSvc)
	teamSvc := team.NewService(team.WithProjector(team.NewProjector(adapter, nil)))
	h := &Handler{
		svc:         agentSvc,
		im:          imSvc,
		llm:         llm.NewService(config.ModelConfig{}, agentSvc),
		teamSvc:     teamSvc,
		teamAdapter: adapter,
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"release","lead_bot_id":"u-manager","member_bot_ids":["u-writer","u-tester"]}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	var created apitypes.Team
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create team response: %v", err)
	}

	taskReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/batch", strings.NewReader(`{"created_by":"web","tasks":[{"title":"Ship beta","body":"Prepare beta release notes and checklist."}]}`))
	taskRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusCreated {
		t.Fatalf("create parent task status = %d, want %d: %s", taskRec.Code, http.StatusCreated, taskRec.Body.String())
	}
	var batchResp apitypes.CreateTeamTasksBatchResponse
	if err := json.NewDecoder(taskRec.Body).Decode(&batchResp); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	parent := batchResp.Tasks[0]

	planReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/"+parent.ID+"/plan", strings.NewReader(`{"actor_id":"web"}`))
	planRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(planRec, planReq)
	if planRec.Code != http.StatusOK {
		t.Fatalf("plan task status = %d, want %d: %s", planRec.Code, http.StatusOK, planRec.Body.String())
	}
	if !sawPlannerRequest {
		t.Fatal("planner upstream was not called")
	}
	var planResp apitypes.PlanTeamTaskResponse
	if err := json.NewDecoder(planRec.Body).Decode(&planResp); err != nil {
		t.Fatalf("decode plan response: %v", err)
	}
	if len(planResp.CreatedTasks) != 2 {
		t.Fatalf("created plan tasks len = %d, want 2", len(planResp.CreatedTasks))
	}
	if planResp.Task.PlanSummary == "" || !strings.Contains(planResp.CreatedTasks[0].Body, "Assignee reason") {
		t.Fatalf("plan response = %+v, want summary and detailed child body", planResp)
	}
	if planResp.CreatedTasks[0].Status != team.TaskStatusPending || planResp.CreatedTasks[0].DispatchedAt != nil {
		t.Fatalf("first child after plan = %+v, want pending and not dispatched", planResp.CreatedTasks[0])
	}
	if planResp.CreatedTasks[0].Priority != 9 {
		t.Fatalf("first child priority = %d, want high mapped to 9", planResp.CreatedTasks[0].Priority)
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/"+parent.ID+"/start", strings.NewReader(`{"actor_id":"web"}`))
	startRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start task status = %d, want %d: %s", startRec.Code, http.StatusOK, startRec.Body.String())
	}
	var startResp apitypes.StartTeamTaskResponse
	if err := json.NewDecoder(startRec.Body).Decode(&startResp); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if startResp.ScheduledTasks != 1 {
		t.Fatalf("scheduled tasks = %d, want 1", startResp.ScheduledTasks)
	}
	if startResp.Task.RoomID == "" || startResp.Task.RoomID == created.RoomID {
		t.Fatalf("start task room = %q, want dedicated execution room distinct from team room %q", startResp.Task.RoomID, created.RoomID)
	}

	taskRoom, ok := imSvc.Room(startResp.Task.RoomID)
	if !ok {
		t.Fatalf("Room(%s) ok = false, want true", startResp.Task.RoomID)
	}
	if !strings.Contains(taskRoom.Title, parent.ID) {
		t.Fatalf("task room title = %q, want to contain parent task id %q", taskRoom.Title, parent.ID)
	}
	teamRoom, _ := imSvc.Room(created.RoomID)
	if !roomContains(teamRoom, "Execution room created") || !roomContains(teamRoom, startResp.Task.RoomID) {
		t.Fatalf("team room missing execution room notice: %+v", teamRoom.Messages)
	}
	if !roomContainsMention(taskRoom, "Task "+planResp.CreatedTasks[0].ID+" is ready for you", "u-writer") || !roomContains(taskRoom, "claim --team "+created.ID+" --task "+planResp.CreatedTasks[0].ID+" --bot-id u-writer") {
		t.Fatalf("task room messages missing first dispatch: %+v", taskRoom.Messages)
	}
	if roomContains(taskRoom, "started assigning tasks") {
		t.Fatalf("task room should not include dispatch preamble: %+v", taskRoom.Messages)
	}

	claimReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/"+planResp.CreatedTasks[0].ID+"/claim", strings.NewReader(`{"bot_id":"u-writer"}`))
	claimRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(claimRec, claimReq)
	if claimRec.Code != http.StatusOK {
		t.Fatalf("claim writer status = %d, want %d: %s", claimRec.Code, http.StatusOK, claimRec.Body.String())
	}
	completeReq := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.ID+"/tasks/"+planResp.CreatedTasks[0].ID, strings.NewReader(`{"actor_id":"u-writer","status":"completed","result":"draft ready"}`))
	completeRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("complete writer status = %d, want %d: %s", completeRec.Code, http.StatusOK, completeRec.Body.String())
	}
	taskRoom, _ = imSvc.Room(startResp.Task.RoomID)
	if !roomContainsMention(taskRoom, "Task "+planResp.CreatedTasks[1].ID+" is ready for you", "u-tester") {
		t.Fatalf("task room messages missing successor dispatch: %+v", taskRoom.Messages)
	}

	claimTesterReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/"+planResp.CreatedTasks[1].ID+"/claim", strings.NewReader(`{"bot_id":"u-tester"}`))
	claimTesterRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(claimTesterRec, claimTesterReq)
	if claimTesterRec.Code != http.StatusOK {
		t.Fatalf("claim tester status = %d, want %d: %s", claimTesterRec.Code, http.StatusOK, claimTesterRec.Body.String())
	}
	completeTesterReq := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.ID+"/tasks/"+planResp.CreatedTasks[1].ID, strings.NewReader(`{"actor_id":"u-tester","status":"completed","result":"checklist passed"}`))
	completeTesterRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(completeTesterRec, completeTesterReq)
	if completeTesterRec.Code != http.StatusOK {
		t.Fatalf("complete tester status = %d, want %d: %s", completeTesterRec.Code, http.StatusOK, completeTesterRec.Body.String())
	}
	updatedParent, ok := teamSvc.GetTask(created.ID, parent.ID)
	if !ok {
		t.Fatalf("GetTask(%s) ok = false, want true", parent.ID)
	}
	if updatedParent.Status != team.TaskStatusCompleted || !strings.Contains(updatedParent.Result, "draft ready") || !strings.Contains(updatedParent.Result, "checklist passed") {
		t.Fatalf("updated parent = %+v, want completed with aggregated results", updatedParent)
	}
}

func TestNormalizeManagerPlanRejectsNonCanonicalWorkerID(t *testing.T) {
	h := &Handler{}
	_, err := h.normalizeManagerPlan(managerPlanContext{
		TeamID:              "team-1",
		LeadBotID:           "u-manager",
		AssignableMemberIDs: []string{"u-p-w-0604"},
		Task: managerPlanTaskContext{
			ID:    "task-1",
			Title: "Ship release",
		},
	}, managerPlanLLMResponse{
		PlanSummary: "plan",
		Tasks: []managerPlanLLMTask{{
			IDRef:    "draft",
			Title:    "Draft release note",
			AssignTo: "p-w-0604",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "assignable_member_ids") {
		t.Fatalf("normalizeManagerPlan() error = %v, want non-canonical assign_to rejection", err)
	}
}

func roomContains(room im.Room, text string) bool {
	for _, message := range room.Messages {
		if strings.Contains(message.Content, text) {
			return true
		}
	}
	return false
}

func roomContainsMention(room im.Room, text string, mentionID string) bool {
	for _, message := range room.Messages {
		if !strings.Contains(message.Content, text) {
			continue
		}
		for _, mention := range message.Mentions {
			if mention.ID == mentionID {
				return true
			}
		}
	}
	return false
}

func TestListGlobalTasks(t *testing.T) {
	imSvc := im.NewService()
	adapter := team.NewCSGClawAdapter(imSvc)
	teamSvc := team.NewService(team.WithProjector(team.NewProjector(adapter, nil)))
	h := &Handler{im: imSvc, teamSvc: teamSvc, teamAdapter: adapter}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"release","lead_bot_id":"u-manager"}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var created apitypes.Team
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create team response: %v", err)
	}

	batchReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/batch", strings.NewReader(`{"created_by":"u-manager","tasks":[{"title":"Draft release note","assign_to":"u-worker","priority":9}]}`))
	batchRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(batchRec, batchReq)
	if batchRec.Code != http.StatusCreated {
		t.Fatalf("create batch status = %d, want %d: %s", batchRec.Code, http.StatusCreated, batchRec.Body.String())
	}

	listRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list global tasks status = %d, want %d: %s", listRec.Code, http.StatusOK, listRec.Body.String())
	}

	var tasks []apitypes.GlobalTask
	if err := json.NewDecoder(listRec.Body).Decode(&tasks); err != nil {
		t.Fatalf("decode list global tasks response: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1", len(tasks))
	}
	if tasks[0].TeamID != created.ID {
		t.Fatalf("task.TeamID = %q, want %q", tasks[0].TeamID, created.ID)
	}
	if tasks[0].TeamTitle != "release" {
		t.Fatalf("task.TeamTitle = %q, want %q", tasks[0].TeamTitle, "release")
	}
	if tasks[0].RoomID != created.RoomID {
		t.Fatalf("task.RoomID = %q, want %q", tasks[0].RoomID, created.RoomID)
	}
}

func TestCreateBatchTasksWithParentRef(t *testing.T) {
	imSvc := im.NewService()
	adapter := team.NewCSGClawAdapter(imSvc)
	teamSvc := team.NewService(team.WithProjector(team.NewProjector(adapter, nil)))
	h := &Handler{im: imSvc, teamSvc: teamSvc, teamAdapter: adapter}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"release","lead_bot_id":"u-manager"}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var created apitypes.Team
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create team response: %v", err)
	}

	batchReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/batch", strings.NewReader(`{"created_by":"u-manager","tasks":[{"id_ref":"story","title":"Release v1"},{"title":"Draft release note","parent_ref":"story","assign_to":"u-worker"}]}`))
	batchRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(batchRec, batchReq)
	if batchRec.Code != http.StatusCreated {
		t.Fatalf("create batch status = %d, want %d: %s", batchRec.Code, http.StatusCreated, batchRec.Body.String())
	}

	var batchResp apitypes.CreateTeamTasksBatchResponse
	if err := json.NewDecoder(batchRec.Body).Decode(&batchResp); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	if len(batchResp.Tasks) != 2 {
		t.Fatalf("batch tasks len = %d, want 2", len(batchResp.Tasks))
	}
	if batchResp.Tasks[1].ParentID != batchResp.Tasks[0].ID {
		t.Fatalf("child.ParentID = %q, want %q", batchResp.Tasks[1].ParentID, batchResp.Tasks[0].ID)
	}
}

func TestTeamRoutesApprovalFlow(t *testing.T) {
	imSvc := im.NewService()
	adapter := team.NewCSGClawAdapter(imSvc)
	teamSvc := team.NewService(team.WithProjector(team.NewProjector(adapter, nil)))
	h := &Handler{teamSvc: teamSvc, teamAdapter: adapter}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"ops","lead_bot_id":"u-manager"}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var created apitypes.Team
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	approvalReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/approvals", strings.NewReader(`{"requested_by":"u-worker","approver_id":"u-manager","kind":"command","summary":"run release"}`))
	approvalRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(approvalRec, approvalReq)
	if approvalRec.Code != http.StatusCreated {
		t.Fatalf("create approval status = %d, want %d: %s", approvalRec.Code, http.StatusCreated, approvalRec.Body.String())
	}

	var createdApproval apitypes.TeamApproval
	if err := json.NewDecoder(approvalRec.Body).Decode(&createdApproval); err != nil {
		t.Fatalf("decode approval response: %v", err)
	}

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/approvals/"+createdApproval.ID+"/resolve", strings.NewReader(`{"approver_id":"u-manager","status":"approved","reason":"ok"}`))
	resolveRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("resolve approval status = %d, want %d: %s", resolveRec.Code, http.StatusOK, resolveRec.Body.String())
	}

	listRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+created.ID+"/approvals", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list approvals status = %d, want %d", listRec.Code, http.StatusOK)
	}
	if !strings.Contains(listRec.Body.String(), `"status":"approved"`) {
		t.Fatalf("list approvals body = %s, want approved status", listRec.Body.String())
	}
}

func TestTeamRoomCommandApproveViaMessage(t *testing.T) {
	imSvc := im.NewService()
	adapter := team.NewCSGClawAdapter(imSvc)
	teamSvc := team.NewService(team.WithProjector(team.NewProjector(adapter, nil)))
	h := &Handler{im: imSvc, teamSvc: teamSvc, teamAdapter: adapter}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"ops","lead_bot_id":"u-manager","member_bot_ids":["u-worker"]}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var created apitypes.Team
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if _, err := imSvc.AddRoomMembers(im.AddRoomMembersRequest{
		RoomID:    created.RoomID,
		InviterID: "u-manager",
		UserIDs:   []string{"u-admin"},
		Locale:    "zh",
	}); err != nil {
		t.Fatalf("AddRoomMembers() error = %v", err)
	}

	task, err := teamSvc.CreateTask(team.CreateTaskInput{
		TeamID:    created.ID,
		Title:     "Run tests",
		CreatedBy: "u-manager",
		AssignTo:  "u-worker",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := teamSvc.ClaimTask(team.ClaimTaskInput{TeamID: created.ID, TaskID: task.ID, BotID: "u-worker"}); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	if _, err := teamSvc.UpdateTaskStatus(team.UpdateTaskStatusInput{
		TeamID:  created.ID,
		TaskID:  task.ID,
		ActorID: "u-worker",
		Status:  team.TaskStatusBlocked,
		Reason:  "need approval",
	}); err != nil {
		t.Fatalf("UpdateTaskStatus(blocked) error = %v", err)
	}
	if _, err := teamSvc.RequestApproval(team.RequestApprovalInput{
		TeamID:      created.ID,
		TaskID:      task.ID,
		RequestedBy: "u-worker",
		ApproverID:  "u-manager",
		Kind:        "command",
		Summary:     "Run go test ./...",
	}); err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}

	messageReq := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"`+created.RoomID+`","sender_id":"u-admin","content":"approve `+task.ID+`"}`))
	messageRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(messageRec, messageReq)
	if messageRec.Code != http.StatusCreated {
		t.Fatalf("create message status = %d, want %d: %s", messageRec.Code, http.StatusCreated, messageRec.Body.String())
	}

	approvals := teamSvc.ListApprovals(created.ID)
	if len(approvals) != 1 || approvals[0].Status != team.ApprovalStatusApproved {
		t.Fatalf("approvals = %+v, want one approved approval", approvals)
	}
	updatedTask, ok := teamSvc.GetTask(created.ID, task.ID)
	if !ok {
		t.Fatalf("GetTask(%s) ok = false, want true", task.ID)
	}
	if updatedTask.Status != team.TaskStatusInProgress {
		t.Fatalf("task status = %q, want %q", updatedTask.Status, team.TaskStatusInProgress)
	}

	room, ok := imSvc.Room(created.RoomID)
	if !ok {
		t.Fatalf("Room(%s) ok = false, want true", created.RoomID)
	}
	var sawInstruction bool
	var sawResolution bool
	for _, message := range room.Messages {
		if strings.Contains(message.Content, "Reply in this room with: approve "+task.ID+" or reject "+task.ID+" <reason>") {
			sawInstruction = true
		}
		if strings.Contains(message.Content, "resolved approval") && strings.Contains(message.Content, "approved") {
			sawResolution = true
		}
	}
	if !sawInstruction {
		t.Fatalf("room messages missing approval instruction for task %s", task.ID)
	}
	if !sawResolution {
		t.Fatalf("room messages missing approval resolution projection")
	}
}

func TestTeamRoomCommandReportsUsageErrors(t *testing.T) {
	imSvc := im.NewService()
	adapter := team.NewCSGClawAdapter(imSvc)
	teamSvc := team.NewService(team.WithProjector(team.NewProjector(adapter, nil)))
	h := &Handler{im: imSvc, teamSvc: teamSvc, teamAdapter: adapter}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"ops","lead_bot_id":"u-manager"}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var created apitypes.Team
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if _, err := imSvc.AddRoomMembers(im.AddRoomMembersRequest{
		RoomID:    created.RoomID,
		InviterID: "u-manager",
		UserIDs:   []string{"u-admin"},
		Locale:    "zh",
	}); err != nil {
		t.Fatalf("AddRoomMembers() error = %v", err)
	}

	messageReq := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"`+created.RoomID+`","sender_id":"u-admin","content":"approve"}`))
	messageRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(messageRec, messageReq)
	if messageRec.Code != http.StatusCreated {
		t.Fatalf("create message status = %d, want %d: %s", messageRec.Code, http.StatusCreated, messageRec.Body.String())
	}

	room, ok := imSvc.Room(created.RoomID)
	if !ok {
		t.Fatalf("Room(%s) ok = false, want true", created.RoomID)
	}
	last := room.Messages[len(room.Messages)-1]
	if !strings.Contains(last.Content, "usage: approve <task_id>") {
		t.Fatalf("last message = %q, want usage feedback", last.Content)
	}
}

func TestTeamRoomCommandReassignsTask(t *testing.T) {
	imSvc := im.NewService()
	adapter := team.NewCSGClawAdapter(imSvc)
	teamSvc := team.NewService(team.WithProjector(team.NewProjector(adapter, nil)))
	h := &Handler{im: imSvc, teamSvc: teamSvc, teamAdapter: adapter}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"ops","lead_bot_id":"u-manager","member_bot_ids":["u-worker-a","u-worker-b"]}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var created apitypes.Team
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if _, err := imSvc.AddRoomMembers(im.AddRoomMembersRequest{
		RoomID:    created.RoomID,
		InviterID: "u-manager",
		UserIDs:   []string{"u-admin"},
		Locale:    "zh",
	}); err != nil {
		t.Fatalf("AddRoomMembers() error = %v", err)
	}

	task, err := teamSvc.CreateTask(team.CreateTaskInput{
		TeamID:    created.ID,
		Title:     "Investigate",
		CreatedBy: "u-manager",
		AssignTo:  "u-worker-a",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	messageReq := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"`+created.RoomID+`","sender_id":"u-admin","content":"reassign `+task.ID+` u-worker-b"}`))
	messageRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(messageRec, messageReq)
	if messageRec.Code != http.StatusCreated {
		t.Fatalf("create message status = %d, want %d: %s", messageRec.Code, http.StatusCreated, messageRec.Body.String())
	}

	updated, ok := teamSvc.GetTask(created.ID, task.ID)
	if !ok {
		t.Fatalf("GetTask(%s) ok = false, want true", task.ID)
	}
	if updated.AssignedTo != "u-worker-b" || updated.Status != team.TaskStatusAssigned {
		t.Fatalf("updated task = %+v, want assigned to u-worker-b", updated)
	}
}

func TestTeamRoutesPhase3bPOCScenario(t *testing.T) {
	root := t.TempDir()
	store, err := team.NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	imSvc := im.NewService()
	adapter := team.NewCSGClawAdapter(imSvc)
	teamSvc := team.NewService(team.WithStore(store), team.WithProjector(team.NewProjector(adapter, nil)))
	h := &Handler{im: imSvc, teamSvc: teamSvc, teamAdapter: adapter}
	for _, member := range []struct {
		id     string
		name   string
		handle string
		role   string
	}{
		{id: "u-manager", name: "bot manager", handle: "u-manager", role: "manager"},
		{id: "u-worker-a", name: "bot worker a", handle: "u-worker-a", role: "worker"},
		{id: "u-worker-b", name: "bot worker b", handle: "u-worker-b", role: "worker"},
	} {
		if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
			ID:     member.id,
			Name:   member.name,
			Handle: member.handle,
			Role:   member.role,
		}); err != nil {
			t.Fatalf("EnsureAgentUser(%s) error = %v", member.id, err)
		}
	}

	room, err := imSvc.CreateRoom(im.CreateRoomRequest{
		Title:     "Launch",
		CreatorID: "u-admin",
		MemberIDs: []string{"u-manager"},
		Locale:    "en",
	})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	enableReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","room_id":"`+room.ID+`","title":"Launch","lead_bot_id":"u-manager","member_bot_ids":["u-worker-a","u-worker-b"]}`))
	enableRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusCreated {
		t.Fatalf("enable team status = %d, want %d: %s", enableRec.Code, http.StatusCreated, enableRec.Body.String())
	}

	var created apitypes.Team
	if err := json.NewDecoder(enableRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode enable response: %v", err)
	}

	batchReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/batch", strings.NewReader(`{"created_by":"u-manager","tasks":[{"id_ref":"A","title":"Draft rollout note","assign_to":"u-worker-a","priority":8},{"id_ref":"B","title":"Prepare smoke checklist","assign_to":"u-worker-b","priority":8},{"id_ref":"C","title":"Publish summary","depends_on_refs":["A","B"],"priority":5}]}`))
	batchRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(batchRec, batchReq)
	if batchRec.Code != http.StatusCreated {
		t.Fatalf("create batch status = %d, want %d: %s", batchRec.Code, http.StatusCreated, batchRec.Body.String())
	}

	var batchResp apitypes.CreateTeamTasksBatchResponse
	if err := json.NewDecoder(batchRec.Body).Decode(&batchResp); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	if len(batchResp.Tasks) != 3 {
		t.Fatalf("batch tasks len = %d, want 3", len(batchResp.Tasks))
	}

	taskIDByRef := map[string]string{}
	for _, ref := range batchResp.IDRefs {
		taskIDByRef[ref.IDRef] = ref.TaskID
	}
	if taskIDByRef["A"] == "" || taskIDByRef["B"] == "" || taskIDByRef["C"] == "" {
		t.Fatalf("id_refs = %+v, want A/B/C mappings", batchResp.IDRefs)
	}

	claimARec := httptest.NewRecorder()
	h.Routes().ServeHTTP(claimARec, httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/claim-next", strings.NewReader(`{"bot_id":"u-worker-a"}`)))
	if claimARec.Code != http.StatusOK {
		t.Fatalf("claim A status = %d, want %d: %s", claimARec.Code, http.StatusOK, claimARec.Body.String())
	}
	claimBRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(claimBRec, httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/claim-next", strings.NewReader(`{"bot_id":"u-worker-b"}`)))
	if claimBRec.Code != http.StatusOK {
		t.Fatalf("claim B status = %d, want %d: %s", claimBRec.Code, http.StatusOK, claimBRec.Body.String())
	}

	blockReq := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.ID+"/tasks/"+taskIDByRef["A"], strings.NewReader(`{"actor_id":"u-worker-a","status":"blocked","reason":"need approval"}`))
	blockRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(blockRec, blockReq)
	if blockRec.Code != http.StatusOK {
		t.Fatalf("block task A status = %d, want %d: %s", blockRec.Code, http.StatusOK, blockRec.Body.String())
	}

	approvalReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/approvals", strings.NewReader(`{"task_id":"`+taskIDByRef["A"]+`","requested_by":"u-worker-a","approver_id":"u-manager","kind":"command","summary":"Run publish step"}`))
	approvalRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(approvalRec, approvalReq)
	if approvalRec.Code != http.StatusCreated {
		t.Fatalf("create approval status = %d, want %d: %s", approvalRec.Code, http.StatusCreated, approvalRec.Body.String())
	}

	approveMessageReq := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"`+created.RoomID+`","sender_id":"u-admin","content":"approve `+taskIDByRef["A"]+`"}`))
	approveMessageRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(approveMessageRec, approveMessageReq)
	if approveMessageRec.Code != http.StatusCreated {
		t.Fatalf("approve message status = %d, want %d: %s", approveMessageRec.Code, http.StatusCreated, approveMessageRec.Body.String())
	}

	completeBRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(completeBRec, httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.ID+"/tasks/"+taskIDByRef["B"], strings.NewReader(`{"actor_id":"u-worker-b","status":"completed","result":"checklist ready"}`)))
	if completeBRec.Code != http.StatusOK {
		t.Fatalf("complete task B status = %d, want %d: %s", completeBRec.Code, http.StatusOK, completeBRec.Body.String())
	}

	prematureClaimRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(prematureClaimRec, httptest.NewRequest(http.MethodPost, "/api/v1/teams/tasks/claim-next", strings.NewReader(`{"bot_id":"u-worker-b"}`)))
	if prematureClaimRec.Code != http.StatusConflict {
		t.Fatalf("premature cross-team claim status = %d, want %d: %s", prematureClaimRec.Code, http.StatusConflict, prematureClaimRec.Body.String())
	}

	completeARec := httptest.NewRecorder()
	h.Routes().ServeHTTP(completeARec, httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.ID+"/tasks/"+taskIDByRef["A"], strings.NewReader(`{"actor_id":"u-worker-a","status":"completed","result":"draft ready"}`)))
	if completeARec.Code != http.StatusOK {
		t.Fatalf("complete task A status = %d, want %d: %s", completeARec.Code, http.StatusOK, completeARec.Body.String())
	}

	claimCRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(claimCRec, httptest.NewRequest(http.MethodPost, "/api/v1/teams/tasks/claim-next", strings.NewReader(`{"bot_id":"u-worker-b"}`)))
	if claimCRec.Code != http.StatusOK {
		t.Fatalf("claim C status = %d, want %d: %s", claimCRec.Code, http.StatusOK, claimCRec.Body.String())
	}

	var claimedC apitypes.TeamTask
	if err := json.NewDecoder(claimCRec.Body).Decode(&claimedC); err != nil {
		t.Fatalf("decode claim C response: %v", err)
	}
	if claimedC.ID != taskIDByRef["C"] {
		t.Fatalf("claimed C = %s, want %s", claimedC.ID, taskIDByRef["C"])
	}

	completeCRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(completeCRec, httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.ID+"/tasks/"+taskIDByRef["C"], strings.NewReader(`{"actor_id":"u-worker-b","status":"completed","result":"summary posted"}`)))
	if completeCRec.Code != http.StatusOK {
		t.Fatalf("complete task C status = %d, want %d: %s", completeCRec.Code, http.StatusOK, completeCRec.Body.String())
	}

	summaryReq := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"`+created.RoomID+`","sender_id":"u-manager","content":"Summary: A/B/C completed"}`))
	summaryRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(summaryRec, summaryReq)
	if summaryRec.Code != http.StatusCreated {
		t.Fatalf("summary message status = %d, want %d: %s", summaryRec.Code, http.StatusCreated, summaryRec.Body.String())
	}

	listTasksRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(listTasksRec, httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+created.ID+"/tasks", nil))
	if listTasksRec.Code != http.StatusOK || strings.Count(listTasksRec.Body.String(), `"status":"completed"`) != 3 {
		t.Fatalf("list tasks response = %s, want three completed tasks", listTasksRec.Body.String())
	}
	listApprovalsRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(listApprovalsRec, httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+created.ID+"/approvals", nil))
	if listApprovalsRec.Code != http.StatusOK || !strings.Contains(listApprovalsRec.Body.String(), `"status":"approved"`) {
		t.Fatalf("list approvals response = %s, want approved approval", listApprovalsRec.Body.String())
	}

	reloaded := team.NewService(team.WithStore(store))
	if tasks := reloaded.ListTasks(created.ID); len(tasks) != 3 {
		t.Fatalf("reloaded tasks len = %d, want 3", len(tasks))
	}
	if approvals := reloaded.ListApprovals(created.ID); len(approvals) != 1 {
		t.Fatalf("reloaded approvals len = %d, want 1", len(approvals))
	}
	if events := reloaded.ListEvents(created.ID); len(events) < 8 {
		t.Fatalf("reloaded events len = %d, want room-visible audit history", len(events))
	}

	projectedRoom, ok := imSvc.Room(created.RoomID)
	if !ok {
		t.Fatalf("Room(%s) ok = false, want true", created.RoomID)
	}
	var sawBatchSummary, sawApproval, sawClaimC, sawSummary bool
	for _, message := range projectedRoom.Messages {
		switch {
		case strings.Contains(message.Content, "created 3 tasks"):
			sawBatchSummary = true
		case strings.Contains(message.Content, "requested approval for "+taskIDByRef["A"]):
			sawApproval = true
		case strings.Contains(message.Content, "claimed "+taskIDByRef["C"]):
			sawClaimC = true
		case strings.Contains(message.Content, "Summary: A/B/C completed"):
			sawSummary = true
		}
	}
	if !sawBatchSummary || !sawApproval || !sawClaimC || !sawSummary {
		t.Fatalf("room history missing key messages: batch=%v approval=%v claimC=%v summary=%v", sawBatchSummary, sawApproval, sawClaimC, sawSummary)
	}
}
