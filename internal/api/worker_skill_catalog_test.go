package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/app/runtimewiring"
	"csgclaw/internal/auth"
	"csgclaw/internal/skill/catalog"
)

func TestWorkerCatalogAcceptance(t *testing.T) {
	item := completeWorkerAgent("agent-catalog", "catalog")
	item.RuntimeKind, item.RuntimeName = agent.RuntimeKindDSH, agent.RuntimeKindDSH
	item.Status, item.DesiredState = "stopped", "stopped"
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{item}, runtimewiring.WithDSHRuntime())
	t.Cleanup(func() { _ = controller.Close() })
	h := &Handler{svc: controller, workspace: controller.Workspace(), agentEngine: agentengine.New(controller), agentRuntime: controller}
	if err := h.EnableConnectors(filepath.Join(t.TempDir(), "apps.json")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.CloseConnectors() })
	h.registerAppPlatformTools(item.ID)
	client := taskMCPClient(t, h, item.ID)
	t.Setenv("CSGCLAW_DISABLE_PUBLIC_SKILLS", "true")
	search := func(query string) catalog.Result {
		t.Helper()
		var out catalog.Result
		callTaskTool(t, client, "skill_search", map[string]any{"query": query}, &out)
		return out
	}
	inspect := func(name string) agent.WorkerSkillStatus {
		t.Helper()
		var out agent.WorkerSkillStatus
		callTaskTool(t, client, "skill_status", map[string]any{"name": name}, &out)
		return out
	}
	const repo = "private/unique-catalog-probe"
	loggedOut := search("unique-catalog-probe")
	if loggedOut.Sources["site"] != "login_required" || loggedOut.Sources["skills_sh"] != "disabled" {
		t.Fatalf("signed out: %+v", loggedOut)
	}
	var calls atomic.Int32
	archive := mustZipBytes(t, map[string]string{"SKILL.md": "---\nname: unique-catalog-probe\ndescription: from private site\n---\n", "scripts/file.txt": "v1"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer fixture-site" {
			http.Error(w, "login required", 401)
			return
		}
		switch r.URL.Path {
		case "/intranet/api/v1/agent/skills":
			fmt.Fprintf(w, `{"data":[{"path":%q,"description":"private","default_branch":"dev"}]}`, repo)
		case "/intranet/api/v1/skills/" + repo:
			fmt.Fprint(w, `{"data":{"default_branch":"dev","latest_version":"2.0","versions":[{"version":"2.0","commit":"abc"}]}}`)
		case "/intranet/api/v1/skills/" + repo + "/download_archive/refs/abc":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base := server.URL + "/intranet"
	if err := auth.Default().Store.Save(auth.Record{Tokens: auth.Tokens{AccessToken: "fixture-site"}, Account: auth.Account{BaseURL: base, OpenCSGBaseURL: server.URL + "/portal"}}); err != nil {
		t.Fatal(err)
	}
	global := search("skill-installer")
	if len(global.Candidates) == 0 || global.Candidates[0].Key != "skill-installer" || len(global.Sources) != 1 || calls.Load() != 0 {
		t.Fatalf("global first: %+v", global)
	}
	globalArgs := map[string]any{"source": "global", "key": "skill-installer"}
	callTaskTool(t, client, "skill_install_from_catalog", globalArgs, nil)
	denyTaskTool(t, client, "skill_install_from_catalog", globalArgs)
	if got := inspect("skill-installer"); !got.Installed || got.DiskSync != "pending" || got.Source != "builtin" {
		t.Fatalf("copied builtin: %+v", got)
	}

	found := search("unique-catalog-probe")
	if len(found.Candidates) != 1 || found.Sources["site"] != "ok" || found.Sources["skills_sh"] != "disabled" {
		t.Fatalf("intranet only: %+v", found)
	}
	c := found.Candidates[0]
	args := map[string]any{"source": c.Source, "key": c.Key, "ref": c.Ref, "site": "https://different.example"}
	denyTaskTool(t, client, "skill_install_from_catalog", args)
	args["site"] = c.Site
	callTaskTool(t, client, "skill_install_from_catalog", args, nil)
	name := "unique-catalog-probe"
	if got := inspect(name); !got.Installed || got.DiskSync != "pending" || got.Version != "2.0" || got.URL != server.URL+"/portal/skills/"+repo {
		t.Fatalf("installed but not synced: %+v", got)
	}
	callTaskTool(t, client, "skill_ensure_enabled", map[string]any{"name": name}, nil)
	if got := inspect(name); got.DiskSync != "in_sync" {
		t.Fatalf("synced: %+v", got)
	}
	layout, _ := controller.AgentLayout(item.ID)
	changed := filepath.Join(layout.SkillsRoot, name, "scripts/file.txt")
	if err := os.WriteFile(changed, []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if got := inspect(name); got.DiskSync != "pending" || !got.Enabled {
			t.Fatalf("read-only inspection must not repair: %+v", got)
		}
	}
	if got := inspect("missing"); got.Installed || got.DiskSync != "not_applicable" {
		t.Fatalf("absent: %+v", got)
	}
}
