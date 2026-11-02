package api

import (
	"context"
	"csgclaw/internal/agenttask"
	"csgclaw/internal/im"
	"csgclaw/internal/taskcore"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/enginetest"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/participant"
	"csgclaw/internal/scheduledtask"
)

func TestScheduledTaskPatchCanClearExpiresAt(t *testing.T) {
	store, err := scheduledtask.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	svc, err := scheduledtask.NewService(store, nil)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	h := &Handler{}
	h.SetScheduledTaskService(svc)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/scheduled-tasks", strings.NewReader(`{
		"title":"Daily check",
		"agent_id":"worker",
		"prompt":"Report status.",
		"recurrence":"daily",
		"first_run_at":"2026-07-07T10:40:00+08:00",
		"expires_at":"2026-07-31T23:59:00+08:00"
	}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	var created apitypes.ScheduledTask
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ExpiresAt == nil {
		t.Fatalf("created ExpiresAt = nil, want value")
	}

	preserveReq := httptest.NewRequest(http.MethodPatch, "/api/v1/scheduled-tasks/"+created.ID, strings.NewReader(`{"enabled":false}`))
	preserveRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(preserveRec, preserveReq)
	if preserveRec.Code != http.StatusOK {
		t.Fatalf("preserve patch status = %d, want %d: %s", preserveRec.Code, http.StatusOK, preserveRec.Body.String())
	}
	var preserved apitypes.ScheduledTask
	if err := json.NewDecoder(preserveRec.Body).Decode(&preserved); err != nil {
		t.Fatalf("decode preserve response: %v", err)
	}
	if preserved.ExpiresAt == nil {
		t.Fatalf("preserved ExpiresAt = nil, want original value")
	}

	clearReq := httptest.NewRequest(http.MethodPatch, "/api/v1/scheduled-tasks/"+created.ID, strings.NewReader(`{"expires_at":null}`))
	clearRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(clearRec, clearReq)
	if clearRec.Code != http.StatusOK {
		t.Fatalf("clear patch status = %d, want %d: %s", clearRec.Code, http.StatusOK, clearRec.Body.String())
	}
	var cleared apitypes.ScheduledTask
	if err := json.NewDecoder(clearRec.Body).Decode(&cleared); err != nil {
		t.Fatalf("decode clear response: %v", err)
	}
	if cleared.ExpiresAt != nil {
		t.Fatalf("cleared ExpiresAt = %v, want nil", cleared.ExpiresAt)
	}
}

func TestScheduledTasksReturnAgentName(t *testing.T) {
	store, err := scheduledtask.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	svc, err := scheduledtask.NewService(store, nil)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:      "pt-gitlab",
		Channel: participant.ChannelCSGClaw,
		Type:    participant.TypeAgent,
		Name:    "gitlab",
		AgentID: "agent-acwxvj",
	}}))
	h := &Handler{participant: participantSvc}
	h.SetScheduledTaskService(svc)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/scheduled-tasks", strings.NewReader(`{
		"title":"Daily check",
		"agent_id":"agent-acwxvj",
		"prompt":"Report status.",
		"recurrence":"daily",
		"first_run_at":"2026-07-07T10:40:00+08:00"
	}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	var created apitypes.ScheduledTask
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.AgentName != "gitlab" {
		t.Fatalf("created AgentName = %q, want gitlab", created.AgentName)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/scheduled-tasks", nil)
	listRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d: %s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var listed []apitypes.ScheduledTask
	if err := json.NewDecoder(listRec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed) != 1 || listed[0].AgentName != "gitlab" {
		t.Fatalf("listed tasks = %+v, want agent_name gitlab", listed)
	}
}

func TestManagerScheduleTriggersAtDueTimeAndCanComplete(t *testing.T) {
	now := time.Date(2026, 11, 2, 1, 59, 0, 0, time.UTC)
	store, err := scheduledtask.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core := taskcore.NewService()
	imSvc := im.NewService()
	tasks := agenttask.NewService(core, imSvc, nil, nil)
	svc, err := scheduledtask.NewService(store, tasks, scheduledtask.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: imSvc, agentTaskSvc: tasks, scheduledTaskSvc: svc}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/scheduled-tasks", strings.NewReader(`{"title":"Weekly review","agent_id":"u-manager","prompt":"Summarize last week","recurrence":"weekly","first_run_at":"2026-11-02T10:00:00+08:00"}`))
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, request)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	var created apitypes.ScheduledTask
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.AgentID != "agent-manager" {
		t.Fatalf("created=%+v", created)
	}
	if runs := svc.TriggerDue(context.Background()); len(runs) != 0 {
		t.Fatalf("triggered early: %+v", runs)
	}
	now = now.Add(time.Minute)
	runs := svc.TriggerDue(context.Background())
	if len(runs) != 1 || runs[0].Status != scheduledtask.StatusTriggered {
		t.Fatalf("runs=%+v", runs)
	}
	generated, ok := core.Get(runs[0].TaskID)
	if !ok || generated.AssignedTo != "pt-manager" || generated.Body != "Summarize last week" {
		t.Fatalf("task=%+v", generated)
	}
	if _, err := tasks.Claim(agenttask.ClaimInput{TaskID: generated.ID, ParticipantID: "pt-manager"}); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Update(agenttask.UpdateInput{TaskID: generated.ID, ActorID: "pt-manager", Status: taskcore.StatusCompleted, Result: "Done"}); err != nil {
		t.Fatal(err)
	}
	next := svc.List()[0]
	if !next.NextRunAt.Equal(now.AddDate(0, 0, 7)) || !next.Enabled {
		t.Fatalf("next=%+v", next)
	}
	if again := svc.TriggerDue(context.Background()); len(again) != 0 {
		t.Fatalf("duplicate trigger=%+v", again)
	}
}

func TestScheduledTaskContextListsRealExecutorsWithoutCredentials(t *testing.T) {
	store, err := scheduledtask.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := scheduledtask.NewService(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	engine := enginetest.NewMemoryClient(agentengine.Agent{ID: "agent-manager", Spec: agentengine.AgentSpec{Name: "Manager", Role: agentengine.AgentRoleManager, Model: agentengine.ModelSpec{APIKey: "test-secret-must-not-leak", BaseURL: "https://private-model.invalid"}}})
	h := &Handler{scheduledTaskSvc: svc, agentEngine: engine}
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/scheduled-tasks/context", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("context=%d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Now         time.Time `json:"now"`
		Recurrences []string  `json:"recurrences"`
		Agents      []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Now.IsZero() || len(got.Recurrences) != 4 || len(got.Agents) != 1 || got.Agents[0].ID != "agent-manager" || got.Agents[0].Role != "manager" {
		t.Fatalf("context=%+v", got)
	}
	if strings.Contains(rec.Body.String(), "test-secret") || strings.Contains(rec.Body.String(), "private-model") {
		t.Fatal("scheduling context leaked model configuration")
	}
	if len(svc.List()) != 0 {
		t.Fatal("context mutated schedules")
	}
}
