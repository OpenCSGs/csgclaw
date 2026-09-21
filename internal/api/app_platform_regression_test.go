package api

import (
	"context"
	"csgclaw/internal/apps"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	agent "csgclaw/internal/agentengine/agents"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type appRegressionRecords struct {
	*agent.Controller
	managerID string
	seen      chan struct{}
	once      sync.Once
}

func (r *appRegressionRecords) Agent(id string) (agent.Agent, bool) {
	a, ok := r.Controller.Agent(id)
	if ok && r.seen != nil {
		r.once.Do(func() { close(r.seen) })
	}
	if a.ID == r.managerID {
		a.Role = agent.RoleManager
	}
	return a, ok
}

func TestAppPlatformWorkerOperationsNeverExposeProfileSecrets(t *testing.T) {
	h, caller, worker, token, _ := newAppPlatformAuthFixture(t)
	controller := h.svc.(*agent.Controller)
	profile := worker.AgentProfile
	profile.Env = map[string]string{"GITLAB_TOKEN": "worker-env-private"}
	profile.Headers = map[string]string{"Authorization": "Bearer worker-header-private"}
	if _, err := controller.UpdateRecord(context.Background(), worker.ID, agent.UpdateRequest{AgentProfile: &profile}); err != nil {
		t.Fatal(err)
	}
	h.svc = &appRegressionRecords{Controller: controller, managerID: caller.ID}
	h.registerAppWorkerTools(caller.ID)
	client := taskMCPClient(t, h, caller.ID)
	for _, op := range []struct {
		name string
		args map[string]any
	}{{"agent_stop", map[string]any{"agent_id": worker.ID}}, {"agent_update", map[string]any{"agent_id": worker.ID, "description": "updated"}}} {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: op.name, Arguments: op.args})
		if err != nil || result.IsError {
			t.Fatalf("%s failed", op.name)
		}
		body, _ := json.Marshal(result)
		for _, secret := range []string{"worker-env-private", "worker-header-private", "agent_profile", "api_key_preview"} {
			if strings.Contains(string(body), secret) {
				t.Fatalf("%s exposed private profile fields", op.name)
			}
		}
	}
	response := appAuthRequest(t, h, http.MethodGet, "/api/v1/agents", "", token, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("discovery status=%d", response.Code)
	}
	for _, secret := range []string{"worker-env-private", "worker-header-private", "agent_profile"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("runtime discovery exposed private profile fields")
		}
	}
}

func TestAppCreationCannotOutliveAgentDeletion(t *testing.T) {
	h, target, _, _, _ := newAppPlatformAuthFixture(t)
	controller := h.svc.(*agent.Controller)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	controller.SetAgentResourceCleanup(func(ctx context.Context, id string) error {
		close(entered)
		<-release
		return h.apps.DeleteAgent(ctx, id)
	})
	deleted := make(chan error, 1)
	go func() { deleted <- controller.DeleteRecord(context.Background(), target.ID) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("delete did not acquire lifecycle lease")
	}
	observed := &appRegressionRecords{Controller: controller, seen: make(chan struct{})}
	h.svc = observed
	resource, resourceErr := h.apps.Create(context.Background(), "", apps.CreateRequest{AppID: "gitlab", Name: "late"})
	if resourceErr != nil {
		t.Fatal(resourceErr)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+target.ID+"/apps", strings.NewReader(`{"resource_id":"`+resource.InstallationID+`"}`))
		r.Header.Set("Authorization", "Bearer test-admin-secret")
		w := httptest.NewRecorder()
		h.Routes().ServeHTTP(w, r)
		result <- w
	}()
	select {
	case <-observed.seen:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not check existing Agent")
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-result:
		if response.Code != http.StatusNotFound {
			t.Fatalf("late create status=%d", response.Code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("late create hung")
	}
	items, err := h.apps.List(context.Background(), target.ID)
	if err != nil || len(items) != 0 {
		t.Fatal("deleted Agent retained App installations")
	}
}

func TestManagerBundledSkillRoutesKeepWorking(t *testing.T) {
	h, manager, worker, managerToken, workerToken := newAppPlatformAuthFixture(t)
	h.svc = &appRegressionRecords{Controller: h.svc.(*agent.Controller), managerID: manager.ID}
	routes := []struct{ method, path string }{
		{"GET", "/api/v1/hub/templates/builtin.codex-worker"},
		{"GET", "/api/v1/hub/templates/Agentic%2Fgitlab-assistant"},
		{"GET", "/api/v1/channels/csgclaw/participants"},
		{"POST", "/api/v1/channels/csgclaw/participants"},
		{"POST", "/api/v1/channels/feishu/participants"},
		{"PATCH", "/api/v1/channels/feishu/participants/worker"},
		{"POST", "/api/v1/agents/" + worker.ID + "/recreate"},
		{"POST", "/api/v1/agents/" + manager.ID + "/bindings:apply?channel=feishu"},
		{"POST", "/api/v1/channels/feishu/rooms"},
		{"POST", "/api/v1/channels/csgclaw/rooms/group/members"},
	}
	handler := h.authorizeAppPlatformRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if appRequestAgentID(r) != manager.ID {
			t.Error("missing trusted Manager identity")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, route := range routes {
		for _, token := range []string{managerToken, workerToken} {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			want := http.StatusNoContent
			if token == workerToken {
				want = http.StatusForbidden
			}
			if rec.Code != want {
				t.Errorf("%s %s: got %d, want %d", route.method, route.path, rec.Code, want)
			}
		}
	}
	for _, route := range []struct{ method, path string }{
		{"POST", "/api/v1/agents/" + worker.ID + "/apps"},
		{"POST", "/api/v1/agents/" + worker.ID + "/mcp"},
		{"GET", "/api/v1/config"},
	} {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+managerToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("unrelated access allowed: %s %s", route.method, route.path)
		}
	}
}
