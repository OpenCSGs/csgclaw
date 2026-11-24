package api

import (
	"context"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/app/runtimewiring"
	"csgclaw/internal/config"
	"csgclaw/internal/knowledgebase"
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

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type resourceEnablementRuntime struct{ snapshotMCPServersRuntime }

func TestWorkerSkillMCPProjectsReinstallToNativeRuntime(t *testing.T) {
	for _, tc := range []struct {
		name, kind, config string
	}{
		{"codex", agent.RuntimeKindCodex, "[[skills.config]]\npath = %q\nenabled = false # external demo rule\n"},
		{"codex-inline", agent.RuntimeKindCodex, "[skills]\nconfig = [{path = %q, enabled = false}] # external demo rule\n"},
		{"dsh", agent.RuntimeKindDSH, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind := tc.kind
			item := completeWorkerAgent("agent-native-skill", "native")
			item.RuntimeKind, item.RuntimeName = kind, kind
			item.SkillStates = map[string]skill.State{"demo": {Enabled: false}, "other": {Enabled: false}}
			controller := mustNewSeededServiceWithOptions(t, []agent.Agent{item}, runtimewiring.WithDSHRuntime(), runtimewiring.WithCodexRuntime())
			t.Cleanup(func() { _ = controller.Close() })
			layout, err := controller.AgentLayout(item.ID)
			if err != nil {
				t.Fatal(err)
			}
			profile, err := controller.RuntimeHost().ResolveRuntimeProfile(agentruntime.Handle{RuntimeID: item.RuntimeID})
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(layout.SkillsRoot, "demo")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			const externalComment = "# user's own skill configuration"
			if kind == agent.RuntimeKindCodex {
				p := filepath.Join(filepath.Dir(layout.SkillsRoot), "config.toml")
				content := externalComment + "\n" + fmt.Sprintf(tc.config, filepath.Join(dir, "SKILL.md"))
				if err := os.WriteFile(p, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			h := &Handler{svc: controller, workspace: controller.Workspace(), agentEngine: agentengine.New(controller), agentRuntime: controller}
			if err := h.EnableConnectors(filepath.Join(t.TempDir(), "apps.json")); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = h.CloseConnectors() })
			routes := h.Routes()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+profile.Env["CSGCLAW_ACCESS_TOKEN"])
				routes.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			client := mcp.NewClient(&mcp.Implementation{Name: "worker-skills-test", Version: "1"}, nil)
			session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
				Endpoint: server.URL + "/api/v1/agents/" + item.ID + "/mcp", DisableStandaloneSSE: true,
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = session.Close() })
			denyTaskTool(t, session, "skill_ensure_enabled", map[string]any{"name": "demo", "agent_id": "other"})
			for _, invalid := range []string{"", "../demo", "not-installed"} {
				denyTaskTool(t, session, "skill_ensure_enabled", map[string]any{"name": invalid})
			}
			for _, contents := range []string{"first", "updated"} {
				if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(contents), 0644); err != nil {
					t.Fatal(err)
				}
				var result struct {
					Name       string
					Enabled    bool
					DiskSynced bool `json:"disk_synced"`
				}
				callTaskTool(t, session, "skill_ensure_enabled", map[string]any{"name": "demo"}, &result)
				if result.Name != "demo" || !result.Enabled || !result.DiskSynced {
					t.Fatalf("unexpected MCP result: %+v", result)
				}
				var status agent.WorkerSkillStatus
				callTaskTool(t, session, "skill_status", map[string]any{"name": "demo"}, &status)
				if !status.Installed || !status.Enabled || status.DiskSync != "in_sync" {
					t.Fatalf("unexpected post-install status: %+v", status)
				}
				home := filepath.Dir(layout.SkillsRoot)
				if kind == agent.RuntimeKindDSH {
					data, err := os.ReadFile(filepath.Join(filepath.Dir(home), "skill-view", "skills", "demo", "SKILL.md"))
					if err != nil || string(data) != contents {
						t.Fatalf("DSH projected %q: %v", data, err)
					}
				} else {
					data, err := os.ReadFile(filepath.Join(home, "config.toml"))
					if err != nil || !strings.Contains(string(data), "other") || !strings.Contains(string(data), externalComment) || !strings.Contains(string(data), "# external demo rule") {
						t.Fatalf("Codex enablement not applied: %v", err)
					}
				}
			}
		})
	}
}

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

func TestKnowledgeBaseEnablementPreservesListingAndMetadata(t *testing.T) {
	for _, kind := range []string{agent.RuntimeKindCodex, agent.RuntimeKindDSH} {
		t.Run(kind, func(t *testing.T) {
			item := completeWorkerAgent("agent-knowledge", "Knowledge")
			item.RuntimeKind, item.RuntimeName = kind, kind
			item.Status, item.DesiredState = "stopped", "stopped"
			item.MCPServers = map[string]any{"wiki": map[string]any{
				"url": "https://example.test/wiki/mcp", "enabled": true,
				"_meta": map[string]any{"com.opencsg/mcp": map[string]any{
					"type": "llm_wiki", "resource_id": "42", "content_id": "wiki-42", "auth_type": "csghub_access_token",
				}},
			}}
			rt := &resourceEnablementRuntime{snapshotMCPServersRuntime: snapshotMCPServersRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: kind}}}
			seeded, statePath := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{item}, agent.WithRuntime(rt))
			if err := seeded.Close(); err != nil {
				t.Fatal(err)
			}
			controller, err := agent.NewController(config.ModelConfig{}, config.ServerConfig{
				ListenAddr: "127.0.0.1:18080", AccessToken: "internal-token",
			}, "manager-image:test", statePath, agent.WithRuntime(rt))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = controller.Close() })
			h := &Handler{svc: controller, agentEngine: agentengine.New(controller), workspace: controller.Workspace(), agentRuntime: controller}
			routes := h.Routes()
			send := func(method, suffix, body string) *httptest.ResponseRecorder {
				t.Helper()
				w := httptest.NewRecorder()
				routes.ServeHTTP(w, httptest.NewRequest(method, "/api/v1/agents/"+item.ID+suffix, strings.NewReader(body)))
				if w.Code != http.StatusOK {
					t.Fatalf("%s %s: %d %s", method, suffix, w.Code, w.Body)
				}
				return w
			}
			listPath := "/mcp-servers?pagination=cursor&limit=20&kind=knowledge"
			for _, enabled := range []bool{false, true} {
				before := send(http.MethodGet, listPath, "")
				version, err := strconv.Unquote(before.Header().Get("ETag"))
				if err != nil {
					t.Fatal(err)
				}
				updated := send(http.MethodPut, "/mcp-servers/wiki/enabled", fmt.Sprintf(`{"enabled":%t,"resource_version":%q}`, enabled, version))
				if updated.Header().Get("ETag") == before.Header().Get("ETag") {
					t.Fatal("resource version did not change after update")
				}
				if err := controller.Close(); err != nil {
					t.Fatal(err)
				}
				controller, err = agent.NewController(config.ModelConfig{}, config.ServerConfig{
					ListenAddr: "127.0.0.1:18080", AccessToken: "internal-token",
				}, "manager-image:test", statePath, agent.WithRuntime(rt))
				if err != nil {
					t.Fatal(err)
				}
				h = &Handler{svc: controller, agentEngine: agentengine.New(controller), workspace: controller.Workspace(), agentRuntime: controller}
				routes = h.Routes()
				after := send(http.MethodGet, listPath, "")
				var page struct {
					Total int `json:"total"`
					Items []struct {
						Name   string         `json:"name"`
						Config map[string]any `json:"config"`
					} `json:"items"`
				}
				if err := json.Unmarshal(after.Body.Bytes(), &page); err != nil {
					t.Fatal(err)
				}
				if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Name != "wiki" {
					t.Fatalf("knowledge base missing from listing: %s", after.Body)
				}
				entry := page.Items[0].Config
				metadata, managed := knowledgebase.ManagedMetadataFromServer(entry)
				if entry["enabled"] != enabled || !managed || metadata.KnowledgeBaseID != 42 || metadata.ContentID != "wiki-42" {
					t.Fatalf("knowledge base enablement or metadata = %#v", entry)
				}
				if strings.Contains(after.Body.String(), "internal-token") {
					t.Fatal("knowledge base configuration contains runtime credentials")
				}
			}
		})
	}
}
