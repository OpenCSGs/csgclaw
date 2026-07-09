package agenttask

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/taskcore"
)

func TestCreateAgentTaskBindsDirectRoomAndSendsInitialMessage(t *testing.T) {
	core := taskcore.NewService()
	imSvc := im.NewService()
	agentSvc := newTestAgentService(t, []agent.Agent{testWorkerAgent("agent-dev", "dev")})
	svc := NewService(core, imSvc, agentSvc, nil)

	task, err := svc.CreateAgentTask(context.Background(), CreateInput{
		AgentID:   "agent-dev",
		Title:     "Fix flaky test",
		Body:      "Investigate the failure.",
		CreatedBy: im.AdminUserID,
	})
	if err != nil {
		t.Fatalf("CreateAgentTask() error = %v", err)
	}
	if task.AssignmentType != taskcore.AssignmentTypeAgent || task.AssignmentID != "agent-dev" {
		t.Fatalf("task assignment = %s/%s, want agent/agent-dev", task.AssignmentType, task.AssignmentID)
	}
	if task.RoomID == "" {
		t.Fatal("task.RoomID = empty, want direct room")
	}
	room, ok := imSvc.Room(task.RoomID)
	if !ok {
		t.Fatalf("Room(%s) found = false", task.RoomID)
	}
	if !room.IsDirect {
		t.Fatalf("room.IsDirect = false, want true")
	}
	if len(room.Messages) < 2 {
		t.Fatalf("room messages len = %d, want bootstrap and task message", len(room.Messages))
	}
	last := room.Messages[len(room.Messages)-1]
	if last.Kind != im.MessageKindEvent || last.Event == nil || last.Event.Key != "task_assigned" {
		t.Fatalf("initial message event = kind %q payload %+v, want task_assigned event", last.Kind, last.Event)
	}
	if last.Event.Title != "task-1 [Fix flaky test]" || len(last.Event.TargetIDs) != 1 || last.Event.TargetIDs[0] != "user-dev" {
		t.Fatalf("initial message event = %+v, want full task title and user-dev target", last.Event)
	}
	if !strings.Contains(last.Content, task.ID) || !strings.Contains(last.Content, "Fix flaky test") {
		t.Fatalf("initial message content = %q, want task id and title", last.Content)
	}
	for _, want := range []string{
		"csgclaw-cli task claim --task " + task.ID,
		"csgclaw-cli task update --task " + task.ID,
	} {
		if !strings.Contains(last.Content, want) {
			t.Fatalf("initial message content = %q, want %q", last.Content, want)
		}
	}
	if strings.Contains(last.Content, "POST /api/v1/agent-tasks") || strings.Contains(last.Content, "PATCH /api/v1/agent-tasks") {
		t.Fatalf("initial message still contains raw HTTP guidance: %q", last.Content)
	}
	if len(last.Mentions) != 1 || last.Mentions[0].ID != "user-dev" {
		t.Fatalf("mentions = %+v, want user-dev", last.Mentions)
	}
}

func TestCreateAgentTaskRejectsMissingAgentWhenAgentServiceConfigured(t *testing.T) {
	core := taskcore.NewService()
	imSvc := im.NewService()
	agentSvc := newTestAgentService(t, nil)
	svc := NewService(core, imSvc, agentSvc, nil)

	_, err := svc.CreateAgentTask(context.Background(), CreateInput{
		AgentID:   "agent-missing",
		Title:     "Fix flaky test",
		CreatedBy: im.AdminUserID,
	})
	if err == nil {
		t.Fatal("CreateAgentTask() error = nil, want missing agent error")
	}
	if !strings.Contains(err.Error(), `agent "agent-missing" not found`) {
		t.Fatalf("CreateAgentTask() error = %v, want missing agent context", err)
	}
	if _, ok := imSvc.User("user-missing"); ok {
		t.Fatal("User(user-missing) exists, want no IM user for missing agent")
	}
	if rooms := imSvc.ListRooms(); len(rooms) != 0 {
		t.Fatalf("ListRooms() len = %d, want no IM rooms for missing agent", len(rooms))
	}
}

func TestClaimAndCompleteAgentTask(t *testing.T) {
	core := taskcore.NewService()
	imSvc := im.NewService()
	svc := NewService(core, imSvc, nil, nil)
	task, err := svc.CreateAgentTask(context.Background(), CreateInput{
		AgentID:   "agent-dev",
		Title:     "Fix flaky test",
		CreatedBy: im.AdminUserID,
	})
	if err != nil {
		t.Fatalf("CreateAgentTask() error = %v", err)
	}
	claimed, err := svc.Claim(ClaimInput{TaskID: task.ID, ParticipantID: "pt-dev"})
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if claimed.Status != taskcore.StatusInProgress || claimed.ClaimedBy != "pt-dev" {
		t.Fatalf("Claim() = %+v, want in_progress by pt-dev", claimed)
	}
	completed, err := svc.Update(UpdateInput{
		TaskID:  task.ID,
		ActorID: "pt-dev",
		Status:  taskcore.StatusCompleted,
		Result:  "done",
	})
	if err != nil {
		t.Fatalf("Update(completed) error = %v", err)
	}
	if completed.Status != taskcore.StatusCompleted || completed.Result != "done" {
		t.Fatalf("Update(completed) = %+v, want completed", completed)
	}
}

func newTestAgentService(t *testing.T, agents []agent.Agent) *agent.Service {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(map[string]any{"agents": agents})
	if err != nil {
		t.Fatalf("marshal seeded agents: %v", err)
	}
	if err := os.WriteFile(statePath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write seeded agents: %v", err)
	}
	svc, err := agent.NewService(config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		BaseURL:  "http://127.0.0.1:4000",
		APIKey:   "sk-test",
		ModelID:  "model-1",
	}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	return svc
}

func testWorkerAgent(id, name string) agent.Agent {
	return agent.Agent{
		ID:          id,
		Name:        name,
		Description: "Test worker",
		Role:        agent.RoleWorker,
		RuntimeKind: agent.RuntimeKindPicoClawSandbox,
		RuntimeID:   "rt-" + id,
		Image:       "agent-image:test",
		Status:      "running",
		AgentProfile: agent.AgentProfile{
			Provider:        agent.ProviderAPI,
			BaseURL:         "http://127.0.0.1:4000",
			APIKey:          "sk-test",
			ModelID:         "model-1",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}
}
