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
	"csgclaw/internal/skill/installstate"
	"csgclaw/internal/skill/local"
	skillstate "csgclaw/internal/skill/state"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestWorkerCatalogAcceptance(t *testing.T) {
	item := completeWorkerAgent("agent-catalog", "catalog")
	item.RuntimeKind, item.RuntimeName = agent.RuntimeKindDSH, agent.RuntimeKindDSH
	item.Status, item.DesiredState = "stopped", "stopped"
	item.SkillStates = map[string]skillstate.State{"unique-catalog-probe": {Enabled: false}}
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
	denyTaskTool(t, client, "skill_install_from_catalog", globalArgs)
	globalArgs["selection"] = global.Candidates[0].Selection
	callTaskTool(t, client, "skill_install_from_catalog", globalArgs, nil)
	callTaskTool(t, client, "skill_install_from_catalog", globalArgs, nil)
	if got := inspect("skill-installer"); !got.Installed || got.DiskSync != "pending" || got.Source != "builtin" {
		t.Fatalf("copied builtin: %+v", got)
	}

	// A same-named entry can change provenance without changing a single byte.
	// Also reject same-origin edits to a companion file, not just SKILL.md.
	root, err := local.SkillsRoot()
	if err != nil {
		t.Fatal(err)
	}
	const swap = "selection-probe"
	dir := filepath.Join(root, swap)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# probe"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script"), []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}
	origin := installstate.Record{Source: installstate.SourceLocal, SourceURL: "https://skills.sh/one/repo/" + swap}
	if err := installstate.Update(root, swap, &origin, nil); err != nil {
		t.Fatal(err)
	}
	selected := search(swap).Candidates[0]
	swapArgs := map[string]any{"source": "global", "key": swap, "selection": selected.Selection}
	origin.SourceURL = "https://skills.sh/two/repo/" + swap
	if err := installstate.Update(root, swap, &origin, nil); err != nil {
		t.Fatal(err)
	}
	denyTaskTool(t, client, "skill_install_from_catalog", swapArgs)
	swapArgs["selection"] = search(swap).Candidates[0].Selection
	if err := os.WriteFile(filepath.Join(dir, "script"), []byte("after"), 0644); err != nil {
		t.Fatal(err)
	}
	denyTaskTool(t, client, "skill_install_from_catalog", swapArgs)
	if got := inspect(swap); got.Installed {
		t.Fatal("stale selections installed files")
	}
	swapArgs["selection"] = search(swap).Candidates[0].Selection
	callTaskTool(t, client, "skill_install_from_catalog", swapArgs, nil)
	if got := inspect(swap); got.URL != origin.SourceURL {
		t.Fatalf("unexpected selected source: %+v", got)
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
	if got := inspect(name); !got.Installed || got.Enabled || got.Version != "2.0" || got.URL != server.URL+"/portal/skills/"+repo {
		t.Fatalf("installed but not synced: %+v", got)
	}
	args["replace"] = true
	callTaskTool(t, client, "skill_install_from_catalog", args, nil)
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

	// Even --replace for an explicit reinstall must not switch origins by
	// itself. Both choices are identified before anything is overwritten.
	sameName := filepath.Join(root, name)
	if err := os.MkdirAll(sameName, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sameName, "SKILL.md"), []byte("# alternative"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := installstate.Update(root, name, &origin, nil); err != nil {
		t.Fatal(err)
	}
	choice := search(name).Candidates[0]
	collision := map[string]any{"source": "global", "key": name, "selection": choice.Selection, "replace": true}
	response, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_install_from_catalog", Arguments: collision})
	if err != nil {
		t.Fatal(err)
	}
	details, ok := response.StructuredContent.(map[string]any)
	if !response.IsError || !ok || details["installed_source"] != server.URL+"/portal/skills/"+repo || details["requested_source"] != origin.SourceURL {
		t.Fatalf("unexpected conflict: %+v", response)
	}
	if got := inspect(name); got.URL != details["installed_source"] {
		t.Fatal("unconfirmed install changed origin")
	}
	collision["confirmation"] = details["confirmation"]
	callTaskTool(t, client, "skill_install_from_catalog", collision, nil)
	if got := inspect(name); got.URL != origin.SourceURL {
		t.Fatal("confirmed install did not replace skill")
	}
}
