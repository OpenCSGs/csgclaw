package api

import (
	"context"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	agentruntime "csgclaw/internal/runtime"
	skill "csgclaw/internal/skill/state"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type resourceEnablementRuntime struct{ snapshotMCPServersRuntime }

func (*resourceEnablementRuntime) ReconcileSkills(context.Context, agentruntime.Handle, map[string]skill.State) error {
	return nil
}

func TestResourceEnablementHTTP(t *testing.T) {
	for _, kind := range []string{agent.RuntimeKindCodex, agent.RuntimeKindDSH} {
		t.Run(kind, func(t *testing.T) {
			item := completeWorkerAgent("agent-resources", "Resources")
			item.RuntimeKind = kind
			item.RuntimeName = kind
			item.Status = "stopped"
			item.DesiredState = "stopped"
			item.MCPServers = map[string]any{"search": map[string]any{"url": "https://example.com/mcp"}}
			rt := &resourceEnablementRuntime{snapshotMCPServersRuntime: snapshotMCPServersRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: kind}}}
			controller := mustNewSeededServiceWithOptions(t, []agent.Agent{item}, agent.WithRuntime(rt))
			layout, err := controller.AgentLayout(item.ID)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(layout.SkillsRoot, "reviewer")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: reviewer\ndescription: review\n---\n"), 0600); err != nil {
				t.Fatal(err)
			}
			h := &Handler{svc: controller, agentEngine: agentengine.New(controller), workspace: controller.Workspace(), agentRuntime: controller}
			routes := h.Routes()
			send := func(method, path, body string) *httptest.ResponseRecorder {
				t.Helper()
				w := httptest.NewRecorder()
				routes.ServeHTTP(w, httptest.NewRequest(method, "/api/v1/agents/"+item.ID+path, strings.NewReader(body)))
				return w
			}
			for _, resource := range []struct{ list, path string }{{"/skill-summaries", "/skills/reviewer/enabled"}, {"/mcp-servers", "/mcp-servers/search/enabled"}} {
				list := send(http.MethodGet, resource.list, "")
				if list.Code != http.StatusOK {
					t.Fatalf("读取列表失败：%d %s", list.Code, list.Body.String())
				}
				version, err := strconv.Unquote(list.Header().Get("ETag"))
				if err != nil {
					t.Fatal(err)
				}
				body := fmt.Sprintf(`{"enabled":false,"resource_version":%q}`, version)
				for _, invalid := range []string{`{}`, `{"enabled":"false","resource_version":"v"}`, body + ` {}`} {
					if got := send(http.MethodPut, resource.path, invalid); got.Code != http.StatusBadRequest {
						t.Fatalf("无效请求：%d %s", got.Code, got.Body.String())
					}
				}
				updated := send(http.MethodPut, resource.path, body)
				if updated.Code != http.StatusOK {
					t.Fatalf("更新失败：%d %s", updated.Code, updated.Body.String())
				}
				var response agentResourceEnabledResponse
				if err := json.Unmarshal(updated.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.Enabled || response.RuntimeState != "stopped" || response.ResourceVersion == version {
					t.Fatalf("响应错误：%+v", response)
				}
				if got := send(http.MethodPut, resource.path, body); got.Code != http.StatusConflict {
					t.Fatalf("并发版本未拒绝：%d %s", got.Code, got.Body.String())
				}
			}
			summaries := send(http.MethodGet, "/skill-summaries", "")
			if !strings.Contains(summaries.Body.String(), `"enabled":false`) {
				t.Fatalf("列表遗漏状态：%s", summaries.Body.String())
			}
		})
	}
}
