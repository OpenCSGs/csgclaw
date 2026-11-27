package api

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/participant"
	hub "csgclaw/internal/template"
)

func TestParticipantRefreshesRemoteTemplateOnEveryBinding(t *testing.T) {
	for _, kind := range []string{"codex", "dsh"} {
		t.Run(kind, func(t *testing.T) { refreshTemplateBinding(t, kind) })
	}
}

func refreshTemplateBinding(t *testing.T, kind string) {
	controller, _ := mustNewSeededServiceWithPath(t, nil)
	engine := agentengine.New(controller)
	var archive atomic.Value
	var downloads atomic.Int32
	const endpoint = "/api/v1/codes/team/reviewer"
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case endpoint:
			fmt.Fprint(w, `{"data":{"default_branch":"release"}}`)
		case endpoint + "/download_archive/refs/release":
			downloads.Add(1)
			w.Write(archive.Load().([]byte))
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	service, err := hub.NewService(config.HubConfig{DefaultRegistry: "remote", Registries: []config.HubRegistryConfig{{Name: "remote", Kind: hub.RegistryKindRemote, Enabled: true, URL: remote.URL}}}, func(cfg config.HubRegistryConfig) (hub.Store, error) {
		if cfg.Kind == hub.RegistryKindRemote {
			return hub.NewRemoteStore(remote.URL, ""), nil
		}
		return hub.DefaultStoreFactory(cfg)
	})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{svc: controller, agentEngine: engine, agentModels: controller.Models(), agentRuntime: controller, workspace: controller.Workspace(), hub: service,
		participant: participant.NewService(nil, participant.WithAgentEngine(engine))}
	routes := h.Routes()
	original := `{"id":"pt-review","type":"agent","name":"Reviewer","agent_binding":{"mode":"create","agent_id":"agent-historical","agent":{"from_template":"builtin.` + kind + `-worker","description":"original"}}}`
	post := func(body string) (int, string) {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants", strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	if code, body := post(original); code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, body)
	}
	layout, err := controller.Workspace().AgentLayout("agent-historical")
	if err != nil {
		t.Fatal(err)
	}
	opts := map[string]any{"memory_mode": "disabled"}
	if _, err := controller.UpdateRecord(t.Context(), "agent-historical", agent.UpdateRequest{RuntimeOptions: &opts}); err != nil {
		t.Fatal(err)
	}
	const replay = `{"id":"pt-review","type":"agent","name":"Reviewer","agent_binding":{"mode":"create","agent_id":"agent-other","agent":{"from_template":"team/reviewer"}}}`
	skill := filepath.Join(layout.SkillsRoot, "review")
	if err := os.MkdirAll(skill, 0755); err != nil {
		t.Fatal(err)
	}
	write := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(skill, name), []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
	check := func(path, value string) {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil || string(b) != value {
			t.Fatalf("%s: %q, %v", path, b, err)
		}
	}
	write("personal.md", "keep")
	setArchive := func(kind, fields, content string, extra bool) {
		files := map[string]string{"snapshot/agent.toml": fmt.Sprintf("name = 'Template'\nruntime_kind = '%s'\n%s", kind, fields),
			"snapshot/instructions/AGENTS.md": content, "snapshot/skills/review/SKILL.md": content, "snapshot/mcps/mcp.json": "{}"}
		if extra {
			files["snapshot/skills/review/previous.md"] = "previous"
			files["snapshot/mcps/mcp.json"] = `{"审查 docs":{"url":"https://example.test/review"}}`
		}
		if content == "second" {
			files["snapshot/mcps/mcp.json"] = `{"审查 docs":{"url":"https://replacement.test/review","headers":{"X-Region":"test"}}}`
		}
		archive.Store(mustZipBytes(t, files))
	}
	checkReplay := func() {
		t.Helper()
		code, body := post(replay)
		var p apitypes.Participant
		if err := json.Unmarshal([]byte(body), &p); err != nil || code != http.StatusOK || p.ID != "pt-review" || p.AgentID != "agent-historical" || p.Name != "Reviewer" {
			t.Fatalf("replay: %d %s", code, body)
		}
	}
	key, mode, otherKind := "execution_mode", "standard", "dsh"
	if kind == "dsh" {
		key, mode, otherKind = "permission_mode", "read-only", "codex"
	}
	fields := fmt.Sprintf("description = 'remote'\n[runtime_options]\n%s='%s'\n[model_config]\nreasoning_effort='high'\n[[image.env]]\nname='FROM_TEMPLATE'\ndefault='latest'", key, mode)
	setArchive(" "+strings.ToUpper(kind)+" ", fields, "first", true)
	checkReplay()
	first, _ := controller.Agent("agent-historical")
	var serverID string
	for id, raw := range first.MCPServers {
		if entry, ok := raw.(map[string]any); ok && entry["display_name"] == "审查 docs" {
			serverID = id
		}
	}
	if serverID == "" {
		t.Fatal("template MCP server was not imported")
	}
	servers := maps.Clone(first.MCPServers)
	local := maps.Clone(servers[serverID].(map[string]any))
	local["headers"] = map[string]any{"Authorization": "local-test-value", "X-Private": "local-test-value"}
	local["env_http_headers"] = map[string]any{"X-API-Key": "LOCAL_TEST_KEY"}
	local["bearer_token_env_var"] = "LOCAL_TEST_TOKEN"
	local["tool_timeout_sec"] = 37
	servers[serverID] = local
	if _, err := controller.UpdateRecord(t.Context(), first.ID, agent.UpdateRequest{MCPServers: &servers, MCPServersSet: true}); err != nil {
		t.Fatal(err)
	}
	write("SKILL.md", "locally modified")
	checkReplay()
	check(filepath.Join(skill, "SKILL.md"), "first")
	same, _ := controller.Agent(first.ID)
	sameServer := same.MCPServers[serverID].(map[string]any)
	for _, key := range []string{"headers", "env_http_headers", "bearer_token_env_var"} {
		if !reflect.DeepEqual(sameServer[key], local[key]) {
			t.Fatalf("same-origin refresh lost %s", key)
		}
	}
	setArchive(kind, "", "second", false)
	checkReplay()
	got, _ := controller.Agent("agent-historical")
	if got.Description != "remote" || got.RuntimeOptions["memory_mode"] != "disabled" || got.RuntimeOptions[key] != mode || got.AgentProfile.ReasoningEffort != "high" || got.AgentProfile.Env["FROM_TEMPLATE"] != "latest" {
		t.Fatalf("unexpected fields: %s, %v", got.Description, got.RuntimeOptions)
	}
	check(filepath.Join(layout.WorkspaceRoot, "AGENTS.md"), "second")
	check(filepath.Join(skill, "SKILL.md"), "second")
	check(filepath.Join(skill, "previous.md"), "previous")
	if _, ok := got.MCPServers[serverID]; !ok || len(got.MCPServers) != len(first.MCPServers) {
		t.Fatalf("replaying the template changed MCP IDs: %v -> %v", first.MCPServers, got.MCPServers)
	}
	replacement := got.MCPServers[serverID].(map[string]any)
	if replacement["url"] != "https://replacement.test/review" || !reflect.DeepEqual(replacement["headers"], map[string]any{"X-Region": "test"}) {
		t.Fatal("new MCP endpoint did not receive exactly the template headers")
	}
	for _, key := range []string{"env_http_headers", "bearer_token_env_var"} {
		if _, exists := replacement[key]; exists {
			t.Fatalf("cross-origin refresh retained %s", key)
		}
	}
	if fmt.Sprint(replacement["tool_timeout_sec"]) != "37" {
		t.Fatal("cross-origin refresh lost the local timeout")
	}
	check(filepath.Join(skill, "personal.md"), "keep")
	archive.Store(mustZipBytes(t, map[string]string{"snapshot/agent.toml": "name='Template'\nruntime_kind='" + kind + "'\ndescription=''"}))
	checkReplay()
	got, _ = controller.Agent("agent-historical")
	if got.Description != "" {
		t.Fatal("explicit empty description was not applied")
	}
	check(filepath.Join(layout.WorkspaceRoot, "AGENTS.md"), "second")
	check(filepath.Join(skill, "SKILL.md"), "second")
	setArchive(otherKind, "description='unexpected'", "third", false)
	if code, body := post(replay); code < 400 || !strings.Contains(body, "differs from worker runtime") {
		t.Fatalf("runtime mismatch: %d %s", code, body)
	}
	check(filepath.Join(skill, "SKILL.md"), "second")
	got, _ = controller.Agent("agent-historical")
	if got.RuntimeKind != kind || got.Description != "" || downloads.Load() != 5 {
		t.Fatalf("after mismatch: %s %s %d", got.RuntimeKind, got.Description, downloads.Load())
	}
}
