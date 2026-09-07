package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agenttask"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/scheduledtask"
	"csgclaw/internal/taskcore"
)

func TestScheduledTaskResumesAfterBlockedRunOverHTTP(t *testing.T) {
	for _, status := range []string{taskcore.StatusCompleted, taskcore.StatusFailed} {
		t.Run(status, func(t *testing.T) {
			now := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
			coreStore, err := taskcore.NewStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			core := taskcore.NewService(taskcore.WithStore(coreStore))
			imSvc := im.NewService()
			tasks := agenttask.NewService(core, imSvc, nil, nil)
			scheduleStore, err := scheduledtask.NewStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			schedules, err := scheduledtask.NewService(scheduleStore, tasks, scheduledtask.WithNowFunc(func() time.Time { return now }))
			if err != nil {
				t.Fatal(err)
			}
			h := &Handler{im: imSvc, agentTaskSvc: tasks, scheduledTaskSvc: schedules}
			server := httptest.NewServer(h.Routes())
			defer server.Close()
			request := func(method, path, body string, wantStatus int, result any) {
				t.Helper()
				req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				resp, err := server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				data, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				if resp.StatusCode != wantStatus {
					t.Fatalf("%s %s: status %d, want %d: %s", method, path, resp.StatusCode, wantStatus, data)
				}
				if result != nil {
					if err := json.Unmarshal(data, result); err != nil {
						t.Fatal(err)
					}
				}
			}
			var schedule apitypes.ScheduledTask
			request(http.MethodPost, "/api/v1/scheduled-tasks", `{"title":"Daily check","agent_id":"agent-dev","prompt":"Check work","recurrence":"daily","first_run_at":"2026-09-06T09:00:00Z"}`, http.StatusCreated, &schedule)
			var firstRun apitypes.ScheduledTaskRun
			runPath := "/api/v1/scheduled-tasks/" + schedule.ID + "/run-now"
			request(http.MethodPost, runPath, `{}`, http.StatusCreated, &firstRun)
			taskPath := "/api/v1/agent-tasks/" + firstRun.TaskID
			request(http.MethodPost, taskPath+"/claim", `{"participant_id":"pt-dev"}`, http.StatusOK, nil)
			request(http.MethodPatch, taskPath, `{"actor_id":"pt-dev","status":"blocked","reason":"Waiting for input"}`, http.StatusOK, nil)
			request(http.MethodPost, runPath, `{}`, http.StatusConflict, nil)
			request(http.MethodPost, taskPath+"/claim", `{"participant_id":"pt-other"}`, http.StatusBadRequest, nil)

			var resumed apitypes.TeamTask
			request(http.MethodPost, taskPath+"/claim", `{"participant_id":"pt-dev"}`, http.StatusOK, &resumed)
			if resumed.Status != taskcore.StatusInProgress || resumed.Error != "" || resumed.ClaimedBy != "pt-dev" {
				t.Fatalf("resumed task retained blocked state: %+v", resumed)
			}
			request(http.MethodPatch, taskPath, `{"actor_id":"pt-dev","status":"`+status+`","result":"done","error":"cannot continue"}`, http.StatusOK, nil)
			reloaded := taskcore.NewService(taskcore.WithStore(coreStore))
			persisted, ok := reloaded.Get(firstRun.TaskID)
			if !ok || persisted.Status != status {
				t.Fatalf("persisted task = %+v, found %v", persisted, ok)
			}

			now = now.AddDate(0, 0, 1)
			runs := schedules.TriggerDue(t.Context())
			if len(runs) != 1 || runs[0].Status != scheduledtask.StatusTriggered || runs[0].TaskID == firstRun.TaskID {
				t.Fatalf("next automatic run = %+v, want a new task", runs)
			}
			var history []apitypes.ScheduledTaskRun
			request(http.MethodGet, "/api/v1/scheduled-tasks/"+schedule.ID+"/runs", "", http.StatusOK, &history)
			if len(history) != 2 {
				t.Fatalf("run history = %+v, want both scheduled runs", history)
			}
		})
	}
}

func TestAgentTaskAPI(t *testing.T) {
	core := taskcore.NewService()
	imSvc := im.NewService()
	h := &Handler{
		im:           imSvc,
		agentTaskSvc: agenttask.NewService(core, imSvc, nil, nil),
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/agent-tasks", strings.NewReader(`{"agent_id":"agent-dev","title":"Fix flaky test","body":"Investigate it."}`))
	createRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	var created apitypes.TeamTask
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.AssignmentType != taskcore.AssignmentTypeAgent || created.AssignmentID != "agent-dev" || created.RoomID == "" {
		t.Fatalf("created task = %+v, want agent assignment with room", created)
	}

	claimReq := httptest.NewRequest(http.MethodPost, "/api/v1/agent-tasks/"+created.ID+"/claim", strings.NewReader(`{"participant_id":"pt-dev"}`))
	claimRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(claimRec, claimReq)
	if claimRec.Code != http.StatusOK {
		t.Fatalf("claim status = %d, want %d: %s", claimRec.Code, http.StatusOK, claimRec.Body.String())
	}

	completeReq := httptest.NewRequest(http.MethodPatch, "/api/v1/agent-tasks/"+created.ID, strings.NewReader(`{"actor_id":"pt-dev","status":"completed","result":"done"}`))
	completeRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("complete status = %d, want %d: %s", completeRec.Code, http.StatusOK, completeRec.Body.String())
	}
	var completed apitypes.TeamTask
	if err := json.NewDecoder(completeRec.Body).Decode(&completed); err != nil {
		t.Fatalf("decode complete response: %v", err)
	}
	if completed.Status != taskcore.StatusCompleted || completed.Result != "done" {
		t.Fatalf("completed task = %+v, want completed result", completed)
	}

	globalReq := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	globalRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(globalRec, globalReq)
	if globalRec.Code != http.StatusOK {
		t.Fatalf("global status = %d, want %d: %s", globalRec.Code, http.StatusOK, globalRec.Body.String())
	}
	var global []apitypes.GlobalTask
	if err := json.NewDecoder(globalRec.Body).Decode(&global); err != nil {
		t.Fatalf("decode global response: %v", err)
	}
	if len(global) != 1 || global[0].AssignmentType != taskcore.AssignmentTypeAgent {
		t.Fatalf("global tasks = %+v, want one agent task", global)
	}
}
