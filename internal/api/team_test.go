package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/team"
)

func TestTeamRoutesCreateAndTaskFlow(t *testing.T) {
	imSvc := im.NewService()
	adapter := team.NewCSGClawAdapter(imSvc)
	teamSvc := team.NewService(team.WithProjector(team.NewProjector(adapter, nil)))
	h := &Handler{teamSvc: teamSvc, teamAdapter: adapter}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"release","lead_bot_id":"bot-manager","member_bot_ids":["bot-worker"]}`))
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

	batchReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/batch", strings.NewReader(`{"created_by":"bot-manager","tasks":[{"id_ref":"draft","title":"Draft release note","assign_to":"bot-worker","priority":9}]}`))
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

	claimReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/claim-next", strings.NewReader(`{"bot_id":"bot-worker"}`))
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

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.ID+"/tasks/"+claimed.ID, strings.NewReader(`{"actor_id":"bot-worker","status":"completed","result":"done"}`))
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

func TestListGlobalTasks(t *testing.T) {
	imSvc := im.NewService()
	adapter := team.NewCSGClawAdapter(imSvc)
	teamSvc := team.NewService(team.WithProjector(team.NewProjector(adapter, nil)))
	h := &Handler{im: imSvc, teamSvc: teamSvc, teamAdapter: adapter}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"release","lead_bot_id":"bot-manager"}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var created apitypes.Team
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create team response: %v", err)
	}

	batchReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/batch", strings.NewReader(`{"created_by":"bot-manager","tasks":[{"title":"Draft release note","assign_to":"bot-worker","priority":9}]}`))
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

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"release","lead_bot_id":"bot-manager"}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var created apitypes.Team
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create team response: %v", err)
	}

	batchReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/batch", strings.NewReader(`{"created_by":"bot-manager","tasks":[{"id_ref":"story","title":"Release v1"},{"title":"Draft release note","parent_ref":"story","assign_to":"bot-worker"}]}`))
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

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"ops","lead_bot_id":"bot-manager"}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var created apitypes.Team
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	approvalReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/approvals", strings.NewReader(`{"requested_by":"bot-worker","approver_id":"bot-manager","kind":"command","summary":"run release"}`))
	approvalRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(approvalRec, approvalReq)
	if approvalRec.Code != http.StatusCreated {
		t.Fatalf("create approval status = %d, want %d: %s", approvalRec.Code, http.StatusCreated, approvalRec.Body.String())
	}

	var createdApproval apitypes.TeamApproval
	if err := json.NewDecoder(approvalRec.Body).Decode(&createdApproval); err != nil {
		t.Fatalf("decode approval response: %v", err)
	}

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/approvals/"+createdApproval.ID+"/resolve", strings.NewReader(`{"approver_id":"bot-manager","status":"approved","reason":"ok"}`))
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

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"ops","lead_bot_id":"bot-manager","member_bot_ids":["bot-worker"]}`))
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
		InviterID: "bot-manager",
		UserIDs:   []string{"u-admin"},
		Locale:    "zh",
	}); err != nil {
		t.Fatalf("AddRoomMembers() error = %v", err)
	}

	task, err := teamSvc.CreateTask(team.CreateTaskInput{
		TeamID:    created.ID,
		Title:     "Run tests",
		CreatedBy: "bot-manager",
		AssignTo:  "bot-worker",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := teamSvc.ClaimTask(team.ClaimTaskInput{TeamID: created.ID, TaskID: task.ID, BotID: "bot-worker"}); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	if _, err := teamSvc.UpdateTaskStatus(team.UpdateTaskStatusInput{
		TeamID:  created.ID,
		TaskID:  task.ID,
		ActorID: "bot-worker",
		Status:  team.TaskStatusBlocked,
		Reason:  "need approval",
	}); err != nil {
		t.Fatalf("UpdateTaskStatus(blocked) error = %v", err)
	}
	if _, err := teamSvc.RequestApproval(team.RequestApprovalInput{
		TeamID:      created.ID,
		TaskID:      task.ID,
		RequestedBy: "bot-worker",
		ApproverID:  "bot-manager",
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

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"ops","lead_bot_id":"bot-manager"}`))
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
		InviterID: "bot-manager",
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

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","title":"ops","lead_bot_id":"bot-manager","member_bot_ids":["bot-worker-a","bot-worker-b"]}`))
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
		InviterID: "bot-manager",
		UserIDs:   []string{"u-admin"},
		Locale:    "zh",
	}); err != nil {
		t.Fatalf("AddRoomMembers() error = %v", err)
	}

	task, err := teamSvc.CreateTask(team.CreateTaskInput{
		TeamID:    created.ID,
		Title:     "Investigate",
		CreatedBy: "bot-manager",
		AssignTo:  "bot-worker-a",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	messageReq := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"`+created.RoomID+`","sender_id":"u-admin","content":"reassign `+task.ID+` bot-worker-b"}`))
	messageRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(messageRec, messageReq)
	if messageRec.Code != http.StatusCreated {
		t.Fatalf("create message status = %d, want %d: %s", messageRec.Code, http.StatusCreated, messageRec.Body.String())
	}

	updated, ok := teamSvc.GetTask(created.ID, task.ID)
	if !ok {
		t.Fatalf("GetTask(%s) ok = false, want true", task.ID)
	}
	if updated.AssignedTo != "bot-worker-b" || updated.Status != team.TaskStatusAssigned {
		t.Fatalf("updated task = %+v, want assigned to bot-worker-b", updated)
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
		{id: "bot-manager", name: "bot manager", handle: "bot-manager", role: "manager"},
		{id: "bot-worker-a", name: "bot worker a", handle: "bot-worker-a", role: "worker"},
		{id: "bot-worker-b", name: "bot worker b", handle: "bot-worker-b", role: "worker"},
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
		MemberIDs: []string{"bot-manager"},
		Locale:    "en",
	})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	enableReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"channel":"csgclaw","room_id":"`+room.ID+`","title":"Launch","lead_bot_id":"bot-manager","member_bot_ids":["bot-worker-a","bot-worker-b"]}`))
	enableRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusCreated {
		t.Fatalf("enable team status = %d, want %d: %s", enableRec.Code, http.StatusCreated, enableRec.Body.String())
	}

	var created apitypes.Team
	if err := json.NewDecoder(enableRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode enable response: %v", err)
	}

	batchReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/batch", strings.NewReader(`{"created_by":"bot-manager","tasks":[{"id_ref":"A","title":"Draft rollout note","assign_to":"bot-worker-a","priority":8},{"id_ref":"B","title":"Prepare smoke checklist","assign_to":"bot-worker-b","priority":8},{"id_ref":"C","title":"Publish summary","depends_on_refs":["A","B"],"priority":5}]}`))
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
	h.Routes().ServeHTTP(claimARec, httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/claim-next", strings.NewReader(`{"bot_id":"bot-worker-a"}`)))
	if claimARec.Code != http.StatusOK {
		t.Fatalf("claim A status = %d, want %d: %s", claimARec.Code, http.StatusOK, claimARec.Body.String())
	}
	claimBRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(claimBRec, httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/tasks/claim-next", strings.NewReader(`{"bot_id":"bot-worker-b"}`)))
	if claimBRec.Code != http.StatusOK {
		t.Fatalf("claim B status = %d, want %d: %s", claimBRec.Code, http.StatusOK, claimBRec.Body.String())
	}

	blockReq := httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.ID+"/tasks/"+taskIDByRef["A"], strings.NewReader(`{"actor_id":"bot-worker-a","status":"blocked","reason":"need approval"}`))
	blockRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(blockRec, blockReq)
	if blockRec.Code != http.StatusOK {
		t.Fatalf("block task A status = %d, want %d: %s", blockRec.Code, http.StatusOK, blockRec.Body.String())
	}

	approvalReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+created.ID+"/approvals", strings.NewReader(`{"task_id":"`+taskIDByRef["A"]+`","requested_by":"bot-worker-a","approver_id":"bot-manager","kind":"command","summary":"Run publish step"}`))
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
	h.Routes().ServeHTTP(completeBRec, httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.ID+"/tasks/"+taskIDByRef["B"], strings.NewReader(`{"actor_id":"bot-worker-b","status":"completed","result":"checklist ready"}`)))
	if completeBRec.Code != http.StatusOK {
		t.Fatalf("complete task B status = %d, want %d: %s", completeBRec.Code, http.StatusOK, completeBRec.Body.String())
	}

	prematureClaimRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(prematureClaimRec, httptest.NewRequest(http.MethodPost, "/api/v1/teams/tasks/claim-next", strings.NewReader(`{"bot_id":"bot-worker-b"}`)))
	if prematureClaimRec.Code != http.StatusConflict {
		t.Fatalf("premature cross-team claim status = %d, want %d: %s", prematureClaimRec.Code, http.StatusConflict, prematureClaimRec.Body.String())
	}

	completeARec := httptest.NewRecorder()
	h.Routes().ServeHTTP(completeARec, httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.ID+"/tasks/"+taskIDByRef["A"], strings.NewReader(`{"actor_id":"bot-worker-a","status":"completed","result":"draft ready"}`)))
	if completeARec.Code != http.StatusOK {
		t.Fatalf("complete task A status = %d, want %d: %s", completeARec.Code, http.StatusOK, completeARec.Body.String())
	}

	claimCRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(claimCRec, httptest.NewRequest(http.MethodPost, "/api/v1/teams/tasks/claim-next", strings.NewReader(`{"bot_id":"bot-worker-b"}`)))
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
	h.Routes().ServeHTTP(completeCRec, httptest.NewRequest(http.MethodPatch, "/api/v1/teams/"+created.ID+"/tasks/"+taskIDByRef["C"], strings.NewReader(`{"actor_id":"bot-worker-b","status":"completed","result":"summary posted"}`)))
	if completeCRec.Code != http.StatusOK {
		t.Fatalf("complete task C status = %d, want %d: %s", completeCRec.Code, http.StatusOK, completeCRec.Body.String())
	}

	summaryReq := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"`+created.RoomID+`","sender_id":"bot-manager","content":"Summary: A/B/C completed"}`))
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
