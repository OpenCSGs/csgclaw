package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agenttask"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/apps"
	"csgclaw/internal/im"
	"csgclaw/internal/taskcore"
	"csgclaw/internal/team"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type appTaskTestRecords struct {
	AgentRecords
	records map[string]agent.Agent
}

func (r appTaskTestRecords) Agent(id string) (agent.Agent, bool) {
	a, ok := r.records[agent.CanonicalID(id)]
	return a, ok
}
func (r appTaskTestRecords) AgentDisplayName(id string) (string, bool) {
	a, ok := r.Agent(id)
	return a.Name, ok
}

func newAppTaskTestHandler(t *testing.T) *Handler {
	t.Helper()
	messages := im.NewService()
	records := map[string]agent.Agent{}
	for _, name := range []string{"manager", "dev", "qa", "outsider"} {
		role := agent.RoleWorker
		if name == "manager" {
			role = agent.RoleManager
		}
		id := agent.CanonicalID(name)
		records[id] = agent.Agent{ID: id, Name: name, Role: role}
		if _, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: id, Name: name, Role: role}); err != nil {
			t.Fatal(err)
		}
	}
	appService, err := apps.NewService(filepath.Join(t.TempDir(), "state.json"), apps.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = appService.Close() })
	h := &Handler{svc: appTaskTestRecords{records: records}, im: messages, apps: appService, participantBridge: im.NewParticipantBridge("")}
	core := taskcore.NewService()
	h.SetRoomTaskCore(core)
	h.agentTaskSvc = agenttask.NewService(core, messages, nil, nil)
	adapter := team.NewCSGClawAdapter(messages)
	h.teamSvc = team.NewService(team.WithProjector(newTestTeamProjector(adapter)))
	h.teamAdapters = newTestTeamAdapterRegistry(adapter)
	return h
}

func taskMCPClient(t *testing.T, h *Handler, agentID string) *mcp.ClientSession {
	t.Helper()
	h.registerAppPlatformTaskTools(agentID)
	server := httptest.NewServer(h.apps.Handler(agentID))
	t.Cleanup(server.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "task-fixture", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: server.URL, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callTaskTool(t *testing.T, client *mcp.ClientSession, name string, args map[string]any, target any) {
	t.Helper()
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s: %+v", name, result.Content)
	}
	if target == nil {
		return
	}
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			if err := json.Unmarshal([]byte(text.Text), target); err == nil {
				return
			}
		}
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode %s: %s: %v", name, data, err)
	}
}

func denyTaskTool(t *testing.T, client *mcp.ClientSession, name string, args map[string]any) {
	t.Helper()
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err == nil && !result.IsError {
		t.Fatalf("%s unexpectedly allowed: %+v", name, result)
	}
}

func TestAppMCPRoomTaskWorkflowAndIdentity(t *testing.T) {
	h := newAppTaskTestHandler(t)
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Build", CreatorID: "admin", MemberIDs: []string{"dev", "qa"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	source, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "Build a feature"})
	if err != nil {
		t.Fatal(err)
	}
	manager := taskMCPClient(t, h, agent.ManagerUserID)
	worker := taskMCPClient(t, h, "agent-dev")
	qa := taskMCPClient(t, h, "agent-qa")
	outsider := taskMCPClient(t, h, "agent-outsider")
	var parent apitypes.RoomTask
	callTaskTool(t, manager, "room_task_create", map[string]any{"room_id": room.ID, "source_message_id": source.ID, "title": "Build", "body": "Implement feature"}, &parent)
	if parent.CreatedBy != "pt-admin" {
		t.Fatalf("source requester changed: %s", parent.CreatedBy)
	}
	denyTaskTool(t, worker, "room_task_create", map[string]any{"room_id": room.ID, "source_message_id": "forged", "title": "Spoofed"})
	denyTaskTool(t, outsider, "room_tasks_list", map[string]any{"room_id": room.ID})
	var plan apitypes.PlanRoomTaskResponse
	callTaskTool(t, manager, "room_task_plan", map[string]any{"room_id": room.ID, "task_id": parent.ID, "summary": "Build implementation", "auto_start": true, "tasks": []any{map[string]any{"id_ref": "dev", "title": "Implement", "assigned_to": "pt-dev"}}}, &plan)
	if len(plan.CreatedTasks) != 1 || !plan.Started {
		t.Fatalf("plan did not dispatch: %+v", plan)
	}
	child := plan.CreatedTasks[0]
	callTaskTool(t, manager, "room_task_dispatch", map[string]any{"room_id": room.ID, "task_id": child.ID}, &child)
	if child.Attempt < 1 {
		t.Fatal("missing dispatched attempt")
	}
	denyTaskTool(t, qa, "room_task_claim", map[string]any{"room_id": room.ID, "task_id": child.ID, "attempt": child.Attempt, "actor_id": "pt-dev"})
	denyTaskTool(t, qa, "room_task_claim", map[string]any{"room_id": room.ID, "task_id": child.ID, "attempt": child.Attempt})
	callTaskTool(t, worker, "room_task_claim", map[string]any{"room_id": room.ID, "task_id": child.ID, "attempt": child.Attempt}, nil)
	var message im.Message
	callTaskTool(t, worker, "room_task_message", map[string]any{"room_id": room.ID, "task_id": child.ID, "message_id": "clarify-1", "content": "Working on the assigned feature"}, &message)
	if !h.participantBridgeTargetForRoomMember("pt-dev").matches(message.SenderID) || message.Metadata["task_id"] != child.ID {
		t.Fatal("task message lost identity or task scope")
	}
	callTaskTool(t, worker, "room_task_update", map[string]any{"room_id": room.ID, "task_id": child.ID, "attempt": child.Attempt, "status": "completed", "result": "Implemented and verified"}, nil)
	denyTaskTool(t, worker, "room_task_review", map[string]any{"room_id": room.ID, "task_id": child.ID, "attempt": child.Attempt, "accept": true, "summary": "Self approved"})
	callTaskTool(t, manager, "room_task_review", map[string]any{"room_id": room.ID, "task_id": child.ID, "attempt": child.Attempt, "accept": true, "summary": "Verified"}, nil)
	callTaskTool(t, manager, "room_task_report", map[string]any{"room_id": room.ID, "task_id": parent.ID, "outcome": "succeeded", "summary": "Feature complete"}, nil)
	current, found := h.roomTaskSvc.Get(room.ID, parent.ID)
	if !found || current.ReportStatus != "delivered" {
		t.Fatalf("report was not delivered: %+v", current)
	}
}

func TestAppMCPDirectTaskOwnership(t *testing.T) {
	h := newAppTaskTestHandler(t)
	manager := taskMCPClient(t, h, agent.ManagerUserID)
	worker := taskMCPClient(t, h, "agent-dev")
	other := taskMCPClient(t, h, "agent-qa")
	var task apitypes.TeamTask
	callTaskTool(t, manager, "agent_task_create", map[string]any{"agent_id": "agent-dev", "title": "Write a summary", "body": "Use the assigned sources"}, &task)
	if task.CreatedBy != agent.ManagerParticipantID || task.AssignedTo != "pt-dev" {
		t.Fatalf("wrong direct task identities: %+v", task)
	}
	denyTaskTool(t, other, "agent_task_get", map[string]any{"task_id": task.ID})
	denyTaskTool(t, other, "agent_task_claim", map[string]any{"task_id": task.ID, "participant_id": "pt-dev"})
	callTaskTool(t, worker, "agent_task_claim", map[string]any{"task_id": task.ID}, nil)
	denyTaskTool(t, other, "agent_task_update", map[string]any{"task_id": task.ID, "status": "completed", "actor_id": "pt-dev"})
	callTaskTool(t, worker, "agent_task_update", map[string]any{"task_id": task.ID, "status": "completed", "result": "Summary complete"}, nil)
}

func TestAppMCPTeamMembershipAndApprovalOwnership(t *testing.T) {
	h := newAppTaskTestHandler(t)
	meta, err := h.teamSvc.CreateTeam(team.CreateTeamInput{Title: "Build team", LeadAgentID: agent.ManagerUserID, MemberAgentIDs: []string{"agent-dev", "agent-qa"}})
	if err != nil {
		t.Fatal(err)
	}
	manager := taskMCPClient(t, h, agent.ManagerUserID)
	worker := taskMCPClient(t, h, "agent-dev")
	other := taskMCPClient(t, h, "agent-qa")
	outsider := taskMCPClient(t, h, "agent-outsider")
	denyTaskTool(t, outsider, "team_tasks_list", map[string]any{"team_id": meta.ID})
	denyTaskTool(t, worker, "team_tasks_create", map[string]any{"team_id": meta.ID, "tasks": []any{map[string]any{"title": "Forged"}}})
	var batch apitypes.CreateTeamTasksBatchResponse
	callTaskTool(t, manager, "team_tasks_create", map[string]any{"team_id": meta.ID, "tasks": []any{map[string]any{"title": "Implement", "assign_to": "pt-dev"}}}, &batch)
	if len(batch.Tasks) != 1 || batch.Tasks[0].RoomID == "" {
		t.Fatalf("task workflow did not create execution room: %+v", batch)
	}
	task := batch.Tasks[0]
	callTaskTool(t, worker, "team_task_claim", map[string]any{"team_id": meta.ID, "task_id": task.ID}, nil)
	denyTaskTool(t, other, "team_task_update", map[string]any{"team_id": meta.ID, "task_id": task.ID, "status": "completed", "actor_id": "pt-dev"})
	denyTaskTool(t, other, "team_task_update", map[string]any{"team_id": meta.ID, "task_id": task.ID, "status": "completed"})
	var approval apitypes.TeamApproval
	callTaskTool(t, worker, "team_approval_request", map[string]any{"team_id": meta.ID, "task_id": task.ID, "approver_id": agent.ManagerParticipantID, "kind": "decision", "summary": "Approve the proposed result"}, &approval)
	if approval.RequestedBy != "pt-dev" {
		t.Fatalf("approval requester was not injected: %+v", approval)
	}
	denyTaskTool(t, other, "team_approval_resolve", map[string]any{"team_id": meta.ID, "approval_id": approval.ID, "status": "approved", "approver_id": agent.ManagerParticipantID})
	denyTaskTool(t, other, "team_approval_resolve", map[string]any{"team_id": meta.ID, "approval_id": approval.ID, "status": "approved"})
	callTaskTool(t, manager, "team_approval_resolve", map[string]any{"team_id": meta.ID, "approval_id": approval.ID, "status": "approved", "reason": "Approved"}, nil)
}

func TestAppMCPApprovalPolicyRejectsSelfApprovalAndDelegatesToHuman(t *testing.T) {
	h := newAppTaskTestHandler(t)
	meta, err := h.teamSvc.CreateTeam(team.CreateTeamInput{Title: "Approval policy", LeadAgentID: agent.ManagerUserID, MemberAgentIDs: []string{"agent-dev", "agent-qa"}})
	if err != nil {
		t.Fatal(err)
	}
	task, err := h.teamSvc.CreateTask(team.CreateTaskInput{TeamID: meta.ID, CreatedBy: agent.ManagerParticipantID, Title: "Worker task", AssignTo: "pt-dev"})
	if err != nil {
		t.Fatal(err)
	}
	worker := taskMCPClient(t, h, "agent-dev")
	manager := taskMCPClient(t, h, agent.ManagerUserID)
	request := func(approver string) map[string]any {
		return map[string]any{"team_id": meta.ID, "task_id": task.ID, "approver_id": approver, "kind": "decision", "summary": "Approval needed"}
	}
	denyTaskTool(t, worker, "team_approval_request", request("pt-dev"))
	denyTaskTool(t, worker, "team_approval_resolve", map[string]any{"team_id": meta.ID, "approval_id": "approval-1", "status": "approved"})
	if len(h.teamSvc.ListApprovals(meta.ID)) != 0 {
		t.Fatal("self-approval attempt created a usable approval")
	}
	denyTaskTool(t, worker, "team_approval_request", request("pt-qa"))
	var valid apitypes.TeamApproval
	callTaskTool(t, worker, "team_approval_request", request(""), &valid)
	if valid.ApproverID != agent.ManagerParticipantID {
		t.Fatal("Worker request did not select its team Manager")
	}
	denyTaskTool(t, worker, "team_approval_resolve", map[string]any{"team_id": meta.ID, "approval_id": valid.ID, "status": "approved"})
	callTaskTool(t, manager, "team_approval_resolve", map[string]any{"team_id": meta.ID, "approval_id": valid.ID, "status": "approved"}, nil)
	for _, actor := range []string{"pt-dev", agent.ManagerParticipantID} {
		bad, err := h.teamSvc.RequestApproval(team.RequestApprovalInput{TeamID: meta.ID, TaskID: task.ID, RequestedBy: actor, ApproverID: actor, Kind: "decision", Summary: "Previously stored self-approval"})
		if err != nil {
			t.Fatal(err)
		}
		client := worker
		if actor == agent.ManagerParticipantID {
			client = manager
		}
		denyTaskTool(t, client, "team_approval_resolve", map[string]any{"team_id": meta.ID, "approval_id": bad.ID, "status": "approved"})
		for _, approval := range h.teamSvc.ListApprovals(meta.ID) {
			if approval.ID == bad.ID && approval.Status != team.ApprovalStatusPending {
				t.Fatal("invalid existing approval was resolved")
			}
		}
	}
	for _, recipient := range []string{"", agent.ManagerParticipantID, "pt-qa", "pt-unknown-human"} {
		denyTaskTool(t, manager, "team_approval_request", request(recipient))
	}
	var human apitypes.TeamApproval
	callTaskTool(t, manager, "team_approval_request", request("pt-admin"), &human)
	if human.ApproverID != "pt-admin" || human.RequestedBy != agent.ManagerParticipantID {
		t.Fatal("Manager human review did not preserve identities")
	}
	denyTaskTool(t, manager, "team_approval_resolve", map[string]any{"team_id": meta.ID, "approval_id": human.ID, "status": "approved"})
}
