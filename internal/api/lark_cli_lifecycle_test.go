package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
)

type retryCleanupRuntime struct {
	fakeCompatRuntime
	fail bool
}

func (r *retryCleanupRuntime) PrepareExtensionDelete(ctx context.Context, id, name string) (agentruntime.PreparedExtension, error) {
	if r.fail {
		return nil, errors.New("injected cleanup failure")
	}
	return r.fakeCompatRuntime.PrepareExtensionDelete(ctx, id, name)
}
func TestFeishuSourceRevisionAndDisconnectCleanupThroughHTTP(t *testing.T) {
	previous := larkCLILookPath
	larkCLILookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { larkCLILookPath = previous })
	rt := &retryCleanupRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}}
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{{ID: "agent-dev", Name: "dev", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex, ProfileComplete: true}}, agent.WithRuntime(rt))
	engine := agentengine.New(controller)
	participants := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{ID: "previous", Channel: participant.ChannelFeishu, Type: participant.TypeAgent, Name: "dev", AgentID: "agent-dev", ChannelUserKind: participant.ChannelUserKindAppID, ChannelAppConfig: map[string]any{"app_id": "cli_old", "app_secret": "old-test-secret"}}}), participant.WithAgentEngine(engine))
	h := &Handler{svc: controller, agentEngine: engine, agentRuntime: controller, workspace: controller.Workspace(), agentModels: controller.Models(), participant: participants, channelBindings: &fakeCodexBridgeController{}, serverNoAuth: true}
	router := h.Routes()
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	infoPath := "/api/v1/agents/agent-dev/feishu/app-info"
	if response := request(http.MethodGet, infoPath, "", ""); response.Code != http.StatusUnauthorized {
		t.Fatal("credential endpoint allowed an unsigned request")
	}
	oldToken, err := h.larkCLISourceAccessToken("agent-dev")
	if err != nil {
		t.Fatal(err)
	}
	if response := request(http.MethodGet, infoPath, "", oldToken); response.Code != http.StatusOK {
		t.Fatal("initial source token rejected")
	}
	response := request(http.MethodPost, "/api/v1/channels/feishu/participants", `{"id":"pt-dev","type":"agent","name":"dev","channel_user":{"kind":"app_id"},"agent_binding":{"mode":"reuse","agent_id":"agent-dev"},"channel_app_config":{"app_id":"cli_new","app_secret":"new-test-secret"}}`, "")
	if response.Code != http.StatusCreated {
		t.Fatalf("Bot switch HTTP=%d %s", response.Code, response.Body)
	}
	item, err := engine.RuntimeExtensions("agent-dev").Get(context.Background(), "feishu-lark-cli")
	if err != nil || item.Status.State != agentengine.RuntimeExtensionUnavailable {
		t.Fatalf("optional missing CLI = %+v %v", item, err)
	}
	if response = request(http.MethodGet, infoPath, "", oldToken); response.Code != http.StatusUnauthorized {
		t.Fatal("old Bot source token remained valid")
	}
	newToken, err := h.larkCLISourceAccessToken("agent-dev")
	if err != nil {
		t.Fatal(err)
	}
	if response = request(http.MethodGet, infoPath, "", newToken); response.Code != http.StatusOK {
		t.Fatal("new Bot source token rejected")
	}
	// A stale cleanup action must never delete a freshly connected Bot.
	if response = request(http.MethodPost, "/api/v1/agents/agent-dev/lark-cli:cleanup", "{}", ""); response.Code != http.StatusConflict {
		t.Fatal("cleanup was allowed while connected")
	}
	rt.fail = true
	response = request(http.MethodDelete, "/api/v1/channels/feishu/participants/pt-dev", "", "")
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"status":"partial"`) {
		t.Fatalf("partial disconnect=%d %s", response.Code, response.Body)
	}
	if _, exists := participants.Get(participant.ChannelFeishu, "pt-dev"); exists {
		t.Fatal("disconnect retained credential facts")
	}
	if response = request(http.MethodGet, infoPath, "", newToken); response.Code != http.StatusUnauthorized {
		t.Fatal("disconnected source token remained valid")
	}
	target, _ := controller.Agent("agent-dev")
	if status := h.agentLarkCLIStatus(target); status == nil || !status.CleanupPending {
		t.Fatalf("cleanup status=%+v", status)
	}
	rt.fail = false
	response = request(http.MethodPost, "/api/v1/agents/agent-dev/lark-cli:cleanup", "{}", "")
	if response.Code != http.StatusNoContent {
		t.Fatalf("cleanup retry=%d %s", response.Code, response.Body)
	}
	if _, err := engine.RuntimeExtensions("agent-dev").Get(context.Background(), "feishu-lark-cli"); agentengine.ErrorCodeOf(err) != agentengine.ErrorRuntimeExtensionNotFound {
		t.Fatalf("cleanup resource remains: %v", err)
	}
}
