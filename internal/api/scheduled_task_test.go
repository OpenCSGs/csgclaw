package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
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
