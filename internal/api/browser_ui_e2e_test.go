package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apps"
	"csgclaw/internal/auth"
	"csgclaw/internal/browser"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	webui "csgclaw/web"
)

// This fixture serves the real Web UI, API, MCP gateway and Chromium page.
// A headless client verifies the Agent-to-user takeover boundary without a model.
func TestBrowserUIFixture(t *testing.T) {
	ready := os.Getenv("CSGCLAW_BROWSER_UI_READY_FILE")
	if ready == "" {
		t.Skip("set CSGCLAW_BROWSER_UI_READY_FILE for browser UI acceptance")
	}
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))
	rt := &fakeCompatRuntime{kind: agent.RuntimeKindCodex}
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{{ID: "agent-browser", Name: "Browser Agent", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "rt-browser", Status: "running", ProfileComplete: true}}, agent.WithRuntime(rt))
	engine := agentengine.New(controller)
	bus := im.NewBus()
	messages := im.NewServiceFromBootstrapWithBus(im.Bootstrap{CurrentUserID: "admin", Users: []im.User{{ID: "admin", Name: "验收用户", Role: "admin"}, {ID: "browser", Name: "Browser Agent", Role: "worker"}}, Rooms: []im.Room{{ID: "room-browser", Title: "Browser Use 验收", Members: []string{"admin", "browser"}, Messages: []im.Message{{ID: "browser-intro", SenderID: "browser", Content: "已在内置浏览器中打开表单，你可以直接接手填写。", Kind: "message"}}}}}, bus)
	h := NewHandlerWithAuth(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, engine, messages, bus, im.NewParticipantBridge(""), nil, nil, "", true)
	h.SetParticipantService(participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(engine)))
	service, err := apps.NewService(filepath.Join(t.TempDir(), "apps.json"), apps.Options{})
	if err != nil {
		t.Fatal(err)
	}
	h.apps = service
	t.Cleanup(func() { _ = service.Close(); h.CloseBrowser() })
	h.registerBrowserTools("agent-browser")
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><title>Demo form</title><style>body{font:16px system-ui;margin:0;background:#f8fafc;color:#172033}main{max-width:520px;padding:28px}h1{font-size:24px}label{display:block;margin:20px 0 8px}input{box-sizing:border-box;width:100%;font:inherit;padding:12px;border:1px solid #cbd5e1;border-radius:8px}button{font:inherit;background:#2563eb;color:white;border:0;padding:12px 22px;border-radius:8px;margin-top:20px}p{line-height:1.6}#result{color:#166534}</style><main><h1>Project request</h1><p>This live page is shared by you and your Agent.</p><label for="name">Project name</label><input id="name" autocomplete="off"><button onclick="document.getElementById('result').textContent='Saved: '+document.getElementById('name').value">Save request</button><p id="result">Ready to fill</p></main>`))
	}))
	t.Cleanup(page.Close)
	client := taskMCPClient(t, h, "agent-browser")
	var state browser.State
	callTaskTool(t, client, "browser_open", map[string]any{"room_id": "room-browser", "url": page.URL}, &state)
	router := h.Routes()
	finished := make(chan struct{})
	var once sync.Once
	router.Post("/__e2e/finish", func(w http.ResponseWriter, r *http.Request) { once.Do(func() { close(finished) }); w.WriteHeader(204) })
	router.Get("/__e2e/snapshot", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := h.browserService().Snapshot(r.Context(), "room-browser", state.ID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, 200, snapshot)
	})
	router.Handle("/*", webui.Handler())
	server := httptest.NewServer(router)
	defer server.Close()
	data, _ := json.Marshal(map[string]string{"url": server.URL, "browser_id": state.ID, "page_url": page.URL})
	if err = os.WriteFile(ready, data, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(5 * time.Minute):
		t.Fatal("browser UI acceptance timed out")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = h.browserService().CloseSession(ctx, "room-browser", state.ID); err != nil && err != browser.ErrNotFound {
		t.Fatal(err)
	}
}
