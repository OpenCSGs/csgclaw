package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"csgclaw/cli/command"
	schedulecmd "csgclaw/cli/scheduledtask"
	taskcmd "csgclaw/cli/task"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agenttask"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/scheduledtask"
	"csgclaw/internal/taskcore"
)

func TestManagerScheduledTaskCLIWithScopedCredential(t *testing.T) {
	t.Setenv("CSGCLAW_EXECUTION_MODE", "standard")
	h, worker, _, workerToken, _ := newAppPlatformAuthFixture(t)
	controller := h.svc.(*agent.Controller)
	manager, err := controller.EnsureManager(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := controller.RuntimeHost().ResolveRuntimeProfile(agentruntime.Handle{RuntimeID: manager.RuntimeID})
	if err != nil {
		t.Fatal(err)
	}
	managerToken := profile.APIKey
	if !h.ValidateAgentAccessToken(managerToken) || managerToken == h.serverAccessToken {
		t.Fatal("fixture must use a valid scoped Manager credential")
	}

	core := taskcore.NewService()
	h.im = im.NewService()
	h.participantBridge = im.NewParticipantBridge("")
	h.SetRoomTaskCore(core)
	h.agentTaskSvc = agenttask.NewService(core, h.im, nil, nil)
	store, err := scheduledtask.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 30, 7, 0, 0, 0, time.UTC)
	h.scheduledTaskSvc, err = scheduledtask.NewService(store, h.agentTaskSvc, scheduledtask.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h.Routes())
	t.Cleanup(server.Close)
	var output bytes.Buffer
	run := &command.Context{Program: "csgclaw-cli", Stdout: &output, Stderr: io.Discard, HTTPClient: server.Client()}
	globals := command.GlobalOptions{Endpoint: server.URL, Token: managerToken, Output: "json"}
	call := func(cmd command.Command, args []string, out any) {
		t.Helper()
		output.Reset()
		if err := cmd.Run(context.Background(), run, args, globals); err != nil {
			t.Fatalf("%s %v: %v", cmd.Name(), args, err)
		}
		if out != nil {
			if err := json.Unmarshal(output.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
	}
	var info struct {
		Agents []struct{ ID string } `json:"agents"`
	}
	call(schedulecmd.NewCmd(), []string{"context"}, &info)
	if len(info.Agents) != 3 {
		t.Fatalf("context agents = %+v", info.Agents)
	}
	var saved apitypes.ScheduledTask
	call(schedulecmd.NewCmd(), []string{"create", "--title", "周报", "--agent-id", manager.ID, "--prompt", "汇总上周工作并输出周报", "--recurrence", "weekly", "--first-run-at", "2026-11-02T10:00:00+08:00", "--timezone", "Asia/Shanghai"}, &saved)
	if saved.ID == "" || saved.AgentID != manager.ID || saved.Timezone != "Asia/Shanghai" {
		t.Fatalf("saved = %+v", saved)
	}
	var listed []apitypes.ScheduledTask
	call(schedulecmd.NewCmd(), []string{"list"}, &listed)
	if len(listed) != 1 || listed[0].ID != saved.ID {
		t.Fatalf("list = %+v", listed)
	}
	// The same Manager credential may assign an existing worker, then edit it back.
	call(schedulecmd.NewCmd(), []string{"update", "--id", saved.ID, "--agent-id", worker.ID}, &saved)
	if saved.AgentID != worker.ID {
		t.Fatalf("updated executor = %q", saved.AgentID)
	}
	call(schedulecmd.NewCmd(), []string{"update", "--id", saved.ID, "--agent-id", manager.ID}, &saved)
	if runs := h.scheduledTaskSvc.TriggerDue(context.Background()); len(runs) != 0 {
		t.Fatal("schedule triggered early")
	}
	now = time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC)
	runs := h.scheduledTaskSvc.TriggerDue(context.Background())
	if len(runs) != 1 || runs[0].Status != scheduledtask.StatusTriggered {
		t.Fatalf("runs = %+v", runs)
	}
	call(taskcmd.NewCmd(), []string{"claim", "--task", runs[0].TaskID, "--actor-id", agent.ManagerParticipantID}, nil)
	call(taskcmd.NewCmd(), []string{"update", "--task", runs[0].TaskID, "--actor-id", agent.ManagerParticipantID, "--status", "completed", "--result", "周报已生成"}, nil)
	completed, ok := core.Get(runs[0].TaskID)
	if !ok || completed.Status != taskcore.StatusCompleted || completed.AssignedTo != agent.ManagerParticipantID {
		t.Fatalf("completed task = %+v", completed)
	}

	for _, noAuth := range []bool{false, true} {
		h.serverNoAuth = noAuth
		call(schedulecmd.NewCmd(), []string{"list"}, nil)
		call(schedulecmd.NewCmd(), []string{"update", "--id", saved.ID, "--enabled=true"}, nil)
		for _, route := range []struct{ method, path string }{
			{http.MethodGet, "/api/v1/scheduled-tasks/context"},
			{http.MethodGet, "/api/v1/scheduled-tasks"},
			{http.MethodPost, "/api/v1/scheduled-tasks"},
			{http.MethodPatch, "/api/v1/scheduled-tasks/" + saved.ID},
		} {
			if rec := appAuthRequest(t, h, route.method, route.path, `{}`, workerToken, nil); rec.Code != http.StatusForbidden {
				t.Fatalf("worker noAuth=%v %s %s: %d", noAuth, route.method, route.path, rec.Code)
			}
		}
		// Scheduling does not grant arbitrary management or extra schedule actions.
		for _, route := range []struct{ method, path string }{
			{http.MethodGet, "/api/v1/config"},
			{http.MethodDelete, "/api/v1/scheduled-tasks/" + saved.ID},
			{http.MethodPost, "/api/v1/scheduled-tasks/" + saved.ID + "/run-now"},
			{http.MethodPost, "/api/v1/scheduled-tasks/context"},
			{http.MethodPatch, "/api/v1/scheduled-tasks/context"},
		} {
			if rec := appAuthRequest(t, h, route.method, route.path, `{}`, managerToken, nil); rec.Code != http.StatusForbidden {
				t.Fatalf("Manager noAuth=%v %s %s: %d", noAuth, route.method, route.path, rec.Code)
			}
		}
	}
	t.Setenv("CSGCLAW_EXECUTION_MODE", "read_only")
	call(schedulecmd.NewCmd(), []string{"context"}, nil)
	call(schedulecmd.NewCmd(), []string{"list"}, nil)
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/scheduled-tasks"},
		{http.MethodPatch, "/api/v1/scheduled-tasks/" + saved.ID},
	} {
		if rec := appAuthRequest(t, h, route.method, route.path, `{}`, managerToken, nil); rec.Code != http.StatusForbidden {
			t.Fatalf("read-only Manager %s %s: %d", route.method, route.path, rec.Code)
		}
	}
}
