package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
