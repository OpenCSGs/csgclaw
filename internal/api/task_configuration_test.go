package api

import (
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	"csgclaw/internal/taskconfig"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type taskRecordsFixture struct{ AgentRecords }

func (taskRecordsFixture) Agent(id string) (agent.Agent, bool) {
	return agent.Agent{ID: "agent-manager", Name: "manager", Role: agent.RoleManager}, agent.CanonicalID(id) == "agent-manager"
}

type taskModelsFixture struct{ AgentModels }

func (taskModelsFixture) ResolvedTaskProfile(_ string, cfg taskconfig.Config) (agent.AgentProfile, error) {
	return agent.AgentProfile{ModelID: cfg.ModelID}, nil
}
func (taskModelsFixture) ValidateTaskConfiguration(_ string, cfg taskconfig.Config) (taskconfig.Config, error) {
	return cfg, nil
}

func TestTaskLLMReadsRoomProfileAndRejectsOtherAgents(t *testing.T) {
	svc := im.NewServiceFromBootstrap(im.Bootstrap{Rooms: []im.Room{{ID: "own", IsDirect: true, Members: []string{"user-admin", "manager"}, Configuration: &taskconfig.Config{ModelID: "session-model"}}, {ID: "other", IsDirect: true, Members: []string{"user-admin", "unrelated"}, Configuration: &taskconfig.Config{ModelID: "other-model"}}}})
	h := &Handler{svc: taskRecordsFixture{}, im: svc, agentModels: taskModelsFixture{}, llm: llm.NewService(config.ModelConfig{}, nil), serverNoAuth: true}
	for _, test := range []struct {
		room   string
		status int
	}{{"own", 200}, {"other", 403}, {"missing", 404}} {
		rec := httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-manager/tasks/"+test.room+"/llm/v1/models", nil))
		if rec.Code != test.status {
			t.Fatalf("%s: %d %s", test.room, rec.Code, rec.Body.String())
		}
		if test.status == 200 && !strings.Contains(rec.Body.String(), "session-model") {
			t.Fatal("did not use task profile")
		}
	}
	if !h.authorizeAgentPlatformRoute(httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-manager/tasks/own/llm/responses", nil), "agent-manager") {
		t.Fatal("own task route is blocked")
	}
	if h.authorizeAgentPlatformRoute(httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-manager/tasks/other/llm/responses", nil), "agent-manager") {
		t.Fatal("another task route is permitted")
	}
}

func TestTaskConfigurationPatchRejectsRunningWork(t *testing.T) {
	svc := im.NewService()
	room, err := svc.CreateRoom(im.CreateRoomRequest{Title: "Task", CreatorID: "user-admin", MemberIDs: []string{"manager"}, IsDirect: true, TaskSession: true})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: svc, participantWork: taskEntryWorkReporter{}}
	rec := httptest.NewRecorder()
	h.handleLocalRoomByID(rec, httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"configuration":{"model_provider_id":"codex","model_id":"new-model"}}`)), room.ID)
	if rec.Code != http.StatusConflict {
		t.Fatalf("%d: %s", rec.Code, rec.Body.String())
	}
	current, _ := svc.Room(room.ID)
	data, _ := json.Marshal(current)
	var result apitypes.Room
	_ = json.Unmarshal(data, &result)
	if result.Configuration != nil {
		t.Fatal("mutated running conversation")
	}
}
