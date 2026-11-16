package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/auth"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	"csgclaw/internal/skill/discovery"
	"csgclaw/internal/skill/installstate"
	"csgclaw/internal/skill/local"
	skillstate "csgclaw/internal/skill/state"
	"csgclaw/internal/skill/updates"
	webui "csgclaw/web"
)

type skillUpdateFixture struct {
	h               *Handler
	controller      *agent.Controller
	root, agentRoot string
	mu              sync.Mutex
	latest          string
	archiveRequests []string
	broken          bool
	unversioned     bool
}

func newSkillUpdateFixture(t *testing.T) *skillUpdateFixture {
	t.Helper()
	item := completeWorkerAgent("update-agent", "Update Agent")
	item.RuntimeKind = agent.RuntimeKindCodex
	item.Status, item.DesiredState = "stopped", "stopped"
	item.SkillStates = map[string]skillstate.State{"writer": {Enabled: false}}
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{item}, agent.WithRuntime(&resourceEnablementRuntime{snapshotMCPServersRuntime: snapshotMCPServersRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}}}))
	t.Cleanup(func() { _ = controller.Close() })
	root, err := local.SkillsRoot()
	if err != nil {
		t.Fatal(err)
	}
	layout, err := controller.Workspace().AgentLayout(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	f := &skillUpdateFixture{controller: controller, root: root, agentRoot: layout.SkillsRoot, latest: "1.1.0"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, name := range []string{"writer", "research"} {
			prefix := "/api/v1/skills/team/" + name
			if r.URL.Path == prefix {
				versions := []map[string]any{}
				for _, v := range []string{"1.0.0", "1.1.0", "1.2.0"} {
					versions = append(versions, map[string]any{"version": v, "commit": "commit-" + v, "changelog": "Changes for " + name + " " + v, "created_at": "2026-11-13T00:00:00Z"})
				}
				writeJSON(w, 200, map[string]any{"data": map[string]any{"latest_version": f.latest, "versions": versions}})
				return
			}
			if strings.HasPrefix(r.URL.Path, prefix+"/download_archive/refs/") {
				f.archiveRequests = append(f.archiveRequests, r.URL.Path)
				if f.broken {
					http.Error(w, "archive unavailable", 503)
					return
				}
				commit := strings.TrimPrefix(r.URL.Path, prefix+"/download_archive/refs/")
				w.Header().Set("Content-Type", "application/zip")
				_, _ = w.Write(publishedSkillArchive(t, name, commit, f.unversioned))
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	t.Setenv("CSGHUB_API_BASE_URL", server.URL)
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) {
		return auth.Status{Authenticated: true, OpenCSGBaseURL: server.URL, BaseURL: server.URL}, nil
	}))
	previousToken := remoteSkillsHubAccessToken
	remoteSkillsHubAccessToken = func() (string, error) { return "fixture-token", nil }
	t.Cleanup(func() { remoteSkillsHubAccessToken = previousToken })
	for _, name := range []string{"writer", "research"} {
		for _, dir := range []string{root, layout.SkillsRoot} {
			archive := mustZipBytes(t, map[string]string{"SKILL.md": fmt.Sprintf("---\nname: %s\ndescription: A useful skill\nversion: 1.0.0\n---\n# Local notes\n", name), "old.txt": "old reference"})
			_, err := local.InstallArchiveWithOptions(dir, name+".zip", archive, local.InstallArchiveOptions{Origin: installstate.Record{Source: installstate.SourceMarket, SourceURL: server.URL + "/skills/team/" + name, RemotePath: "team/" + name, Version: "1.0.0", Commit: "commit-1.0.0"}})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	engine := agentengine.New(controller)
	messages := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: "admin", Users: []im.User{{ID: "admin", Name: "Admin", Role: "admin"}, {ID: "update-agent", Name: "Update Agent", Role: "worker"}}, Rooms: []im.Room{}})
	f.h = NewHandlerWithAuth(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, engine, messages, nil, nil, nil, nil, "", true)
	f.h.SetParticipantService(participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(engine)))
	return f
}

func publishedSkillArchive(t *testing.T, name, commit string, unversioned bool) []byte {
	t.Helper()
	version := "version: " + strings.TrimPrefix(commit, "commit-") + "\n"
	if unversioned {
		version = ""
	}
	return mustZipBytes(t, map[string]string{"SKILL.md": fmt.Sprintf("---\nname: %s\ndescription: A useful skill\n%s---\n# %s\n", name, version, commit), "new.txt": "new reference"})
}

func TestSkillUpdatesWithoutDeclaredOrRecordedVersions(t *testing.T) {
	for _, test := range []struct {
		name, installed, commit, ref, wantVersion string
		edit, available                           bool
	}{
		{name: "published commit identifies older installation", installed: "1.0.0", commit: "commit-1.0.0", available: true, wantVersion: "1.0.0"},
		{name: "immutable ref identifies older installation", installed: "1.0.0", ref: "commit-1.0.0", available: true, wantVersion: "1.0.0"},
		{name: "published commit already current", installed: "1.1.0", commit: "commit-1.1.0", wantVersion: "1.1.0"},
		{name: "legacy digest differs from publication", installed: "1.0.0", ref: "main", available: true},
		{name: "legacy digest matches publication", installed: "1.1.0", ref: "main", wantVersion: "1.1.0"},
		{name: "local edits do not invent an upstream update", installed: "1.1.0", ref: "main", edit: true, wantVersion: "1.1.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newSkillUpdateFixture(t)
			f.mu.Lock()
			f.unversioned = true
			f.mu.Unlock()
			for _, root := range []string{f.root, f.agentRoot} {
				records, err := installstate.Read(root)
				if err != nil {
					t.Fatal(err)
				}
				record := records["writer"]
				record.Version, record.Commit, record.Ref = "", test.commit, test.ref
				_, err = local.InstallArchiveWithOptions(root, "writer.zip", publishedSkillArchive(t, "writer", "commit-"+test.installed, true), local.InstallArchiveOptions{Replace: true, Origin: record})
				if err != nil {
					t.Fatal(err)
				}
				if test.edit {
					if err := os.WriteFile(filepath.Join(root, "writer", "notes.txt"), []byte("my local edits"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			before, err := os.ReadFile(filepath.Join(f.root, "writer", "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			w := f.request(t, "GET", "/api/v1/skills/writer/update-check", nil)
			if w.Code != 200 {
				t.Fatalf("check: %d %s", w.Code, w.Body.String())
			}
			var info updates.Info
			if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
				t.Fatal(err)
			}
			if info.Available != test.available || info.LocalVersion != test.wantVersion {
				t.Fatalf("unversioned update: %+v", info)
			}
			after, _ := os.ReadFile(filepath.Join(f.root, "writer", "SKILL.md"))
			if !bytes.Equal(before, after) {
				t.Fatal("checking updates changed the local skill")
			}
			if test.ref == "main" {
				f.mu.Lock()
				count := len(f.archiveRequests)
				f.mu.Unlock()
				if count != 1 {
					t.Fatalf("same release downloaded %d times for two copies", count)
				}
			}
			if test.available {
				preview := f.preview(t, "writer")
				for _, result := range f.apply(t, decision(preview, "update")) {
					if result.Status != "updated" {
						t.Fatalf("confirmed unversioned update: %+v", result)
					}
				}
				for _, root := range []string{f.root, f.agentRoot} {
					records, err := installstate.Read(root)
					if err != nil || records["writer"].Version != "1.1.0" || records["writer"].Commit != "commit-1.1.0" {
						t.Fatalf("publication metadata was not recorded: %+v %v", records, err)
					}
				}
			}
		})
	}
}

func (f *skillUpdateFixture) request(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	f.h.Routes().ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewReader(data)))
	return w
}
func (f *skillUpdateFixture) preview(t *testing.T, name string) updates.Info {
	t.Helper()
	w := f.request(t, "GET", "/api/v1/skills/"+name+"/update-check?preview=true", nil)
	if w.Code != 200 {
		r := httptest.NewRequest("GET", "/", nil)
		conn, _ := f.h.skillUpdateConnection(r)
		_, err := f.h.checkSkillUpdate(r, conn, name, true)
		t.Fatalf("preview %d: %s (%v)", w.Code, w.Body.String(), err)
	}
	var info updates.Info
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	return info
}
func decision(info updates.Info, strategy string) updates.Decision {
	return updates.Decision{Name: info.Name, State: info.State, Version: info.LatestVersion, Commit: info.Commit, Strategy: strategy}
}
func (f *skillUpdateFixture) apply(t *testing.T, items ...updates.Decision) []updates.Result {
	t.Helper()
	w := f.request(t, "POST", "/api/v1/skills:updates", map[string]any{"items": items})
	if w.Code != 200 {
		t.Fatalf("apply %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		Results []updates.Result `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response.Results
}

func TestSkillUpdatesConfirmBatchPinsVersionsAndUpdatesAgentCopies(t *testing.T) {
	f := newSkillUpdateFixture(t)
	a, b := f.preview(t, "writer"), f.preview(t, "research")
	if !a.Available || a.LocalVersion != "1.0.0" || a.LatestVersion != "1.1.0" || len(a.Targets) != 2 || len(a.Targets[0].Changes) != 3 {
		t.Fatalf("plan: %+v", a)
	}
	f.mu.Lock()
	f.latest = "1.2.0"
	f.mu.Unlock()
	results := f.apply(t, decision(a, "update"), decision(b, "update"))
	if len(results) != 4 {
		t.Fatalf("results: %+v", results)
	}
	for _, result := range results {
		if result.Status != "updated" {
			t.Fatalf("result: %+v", result)
		}
	}
	for _, root := range []string{f.root, f.agentRoot} {
		for _, name := range []string{"writer", "research"} {
			data, err := os.ReadFile(filepath.Join(root, name, "SKILL.md"))
			if err != nil || !strings.Contains(string(data), "commit-1.1.0") {
				t.Fatalf("wrong pinned content: %s %v", data, err)
			}
			if _, err := os.Stat(filepath.Join(root, name, "old.txt")); !os.IsNotExist(err) {
				t.Fatal("deleted upstream file remained")
			}
			records, err := installstate.Read(root)
			if err != nil || records[name].Version != "1.1.0" || records[name].Commit != "commit-1.1.0" {
				t.Fatalf("record: %+v %v", records, err)
			}
		}
	}
	current, _ := f.controller.Agent("update-agent")
	if skillstate.Enabled(current.SkillStates, "writer") {
		t.Fatal("update enabled a disabled skill")
	}
}

func TestSkillUpdatesKeepIsDurableAndNewerReleasesStillNotify(t *testing.T) {
	f := newSkillUpdateFixture(t)
	info := f.preview(t, "writer")
	results := f.apply(t, decision(info, "keep-local"))
	if len(results) != 1 || results[0].Status != "kept" {
		t.Fatal(results)
	}
	info = f.preview(t, "writer")
	if info.Available || !info.Skipped {
		t.Fatal("ignored release was suggested")
	}
	for _, root := range []string{f.root, f.agentRoot} {
		data, _ := os.ReadFile(filepath.Join(root, "writer", "SKILL.md"))
		if !strings.Contains(string(data), "Local notes") {
			t.Fatal("ignore changed files")
		}
	}
	f.mu.Lock()
	f.latest = "1.2.0"
	f.mu.Unlock()
	if !f.preview(t, "writer").Available {
		t.Fatal("new release was suppressed")
	}
}

func TestSkillUpdatesRejectStaleConfirmationsAndPreserveFilesOnFailure(t *testing.T) {
	f := newSkillUpdateFixture(t)
	info := f.preview(t, "writer")
	if err := os.WriteFile(filepath.Join(f.agentRoot, "writer", "notes.txt"), []byte("precious local edit"), 0600); err != nil {
		t.Fatal(err)
	}
	results := f.apply(t, decision(info, "update"))
	if len(results) != 1 || results[0].Error != "skill_installation_changed" {
		t.Fatal(results)
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "writer", "SKILL.md"))
	if !strings.Contains(string(data), "Local notes") {
		t.Fatal("stale confirmation modified library")
	}
	info = f.preview(t, "writer")
	f.mu.Lock()
	f.broken = true
	f.mu.Unlock()
	results = f.apply(t, decision(info, "update"))
	if len(results) != 1 || results[0].Status != "failed" {
		t.Fatal(results)
	}
	data, _ = os.ReadFile(filepath.Join(f.agentRoot, "writer", "notes.txt"))
	if string(data) != "precious local edit" {
		t.Fatal("failed download lost local edits")
	}
}

func TestSkillUpdatesInitialInstallRecordsPublishedCommit(t *testing.T) {
	f := newSkillUpdateFixture(t)
	if err := local.Delete(f.root, "writer"); err != nil {
		t.Fatal(err)
	}
	w := f.request(t, "POST", "/api/v1/skills:install", map[string]any{"remote_path": "team/writer", "ref": "main"})
	if w.Code != 201 {
		t.Fatalf("install %d: %s", w.Code, w.Body.String())
	}
	records, err := installstate.Read(f.root)
	if err != nil || records["writer"].Version != "1.1.0" || records["writer"].Commit != "commit-1.1.0" || records["writer"].RemotePath != "team/writer" {
		t.Fatalf("installation: %+v %v", records, err)
	}
}

func TestSkillUpdatesNewVersionCanReuseACommit(t *testing.T) {
	f := newSkillUpdateFixture(t)
	for _, root := range []string{f.root, f.agentRoot} {
		records, err := installstate.Read(root)
		if err != nil {
			t.Fatal(err)
		}
		record := records["writer"]
		record.Commit = "commit-1.1.0"
		if err := installstate.Update(root, "writer", &record, nil); err != nil {
			t.Fatal(err)
		}
	}
	info := f.preview(t, "writer")
	if !info.Available || info.LocalVersion != "1.0.0" || info.LatestVersion != "1.1.0" {
		t.Fatalf("new release sharing a commit was hidden: %+v", info)
	}
}

func TestSkillUpdatesDoNotReplaceSameNameFromAnotherSource(t *testing.T) {
	f := newSkillUpdateFixture(t)
	records, err := installstate.Read(f.agentRoot)
	if err != nil {
		t.Fatal(err)
	}
	record := records["writer"]
	record.SourceURL = "https://elsewhere.example/skills/team/writer"
	if err := installstate.Update(f.agentRoot, "writer", &record, nil); err != nil {
		t.Fatal(err)
	}
	info := f.preview(t, "writer")
	if len(info.Targets) != 1 {
		t.Fatal("unrelated agent copy was included")
	}
	results := f.apply(t, decision(info, "update"))
	if len(results) != 1 || results[0].Status != "updated" {
		t.Fatal(results)
	}
	data, _ := os.ReadFile(filepath.Join(f.agentRoot, "writer", "SKILL.md"))
	if !strings.Contains(string(data), "Local notes") {
		t.Fatal("same-name copy from another source was overwritten")
	}
}

func TestSkillUpdatesFollowAnActiveExternalSource(t *testing.T) {
	f := newSkillUpdateFixture(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(home, ".claude", "skills", "writer")
	if err := os.MkdirAll(external, 0700); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: writer\ndescription: External tool managed skill\nversion: 9.0.0\n---\nExternal instructions\n"
	if err := os.WriteFile(filepath.Join(external, "SKILL.md"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := discovery.Rescan(f.root); err != nil {
		t.Fatal(err)
	}
	w := f.request(t, "GET", "/api/v1/skills/updates", nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var response struct {
		Items []updates.Info `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	for _, item := range response.Items {
		if item.Name == "writer" {
			t.Fatal("market update was offered for an active externally managed skill")
		}
	}
	w = f.request(t, "GET", "/api/v1/skills/writer/update-check?preview=true", nil)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "skill_update_source_unavailable") {
		t.Fatalf("external update: %d %s", w.Code, w.Body.String())
	}
	var responseError struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &responseError); err != nil || responseError.Error.Code != "skill_update_source_unavailable" {
		t.Fatalf("update error must use the shared API envelope: %s (%v)", w.Body.String(), err)
	}
	data, _ := os.ReadFile(filepath.Join(external, "SKILL.md"))
	if string(data) != content {
		t.Fatal("external source was modified")
	}
}

func TestSkillUpdatesBrowserFixture(t *testing.T) {
	ready := os.Getenv("CSGCLAW_SKILL_UPDATES_READY")
	if ready == "" {
		t.Skip("set CSGCLAW_SKILL_UPDATES_READY for headless acceptance")
	}
	f := newSkillUpdateFixture(t)
	router := f.h.Routes()
	done := make(chan struct{})
	var once sync.Once
	router.Post("/__e2e/finish", func(w http.ResponseWriter, _ *http.Request) { once.Do(func() { close(done) }); w.WriteHeader(204) })
	router.Post("/__e2e/edit", func(w http.ResponseWriter, _ *http.Request) {
		_ = os.WriteFile(filepath.Join(f.agentRoot, "writer", "local-edit.txt"), []byte("KEEP THIS EDIT"), 0600)
		w.WriteHeader(204)
	})
	router.Handle("/*", webui.Handler())
	server := httptest.NewServer(router)
	defer server.Close()
	data, _ := json.Marshal(map[string]string{"url": server.URL, "root": f.root, "agentRoot": f.agentRoot})
	if err := os.WriteFile(ready, data, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Minute):
		t.Fatal("skill update browser acceptance timed out")
	}
}

func TestSkillUpdatesExcludeNewerCopiesFromPreviewAndPinnedApply(t *testing.T) {
	for _, test := range []struct {
		name, library, agent, target string
	}{
		{"newer Agent copy", "1.0.0", "1.2.0", "library"},
		{"newer library copy", "1.2.0", "1.0.0", "agent-update-agent"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newSkillUpdateFixture(t)
			protectedRoot := f.agentRoot
			for _, copy := range []struct{ root, version string }{{f.root, test.library}, {f.agentRoot, test.agent}} {
				records, err := installstate.Read(copy.root)
				if err != nil {
					t.Fatal(err)
				}
				record := records["writer"]
				record.Version, record.Commit = copy.version, "commit-"+copy.version
				_, err = local.InstallArchiveWithOptions(copy.root, "writer.zip", publishedSkillArchive(t, "writer", record.Commit, false), local.InstallArchiveOptions{Replace: true, Origin: record})
				if err != nil {
					t.Fatal(err)
				}
				if copy.version == "1.2.0" {
					protectedRoot = copy.root
				}
			}
			_, before, err := installstate.Snapshot(protectedRoot, "writer")
			if err != nil {
				t.Fatal(err)
			}
			plan := f.preview(t, "writer")
			if !plan.Available || len(plan.Targets) != 1 || plan.Targets[0].ID != test.target {
				t.Errorf("preview should include only the older copy: %+v", plan)
			}
			// A new Hub release must not expand the scope of a pinned confirmation.
			f.mu.Lock()
			f.latest = "1.2.0"
			f.mu.Unlock()
			results := f.apply(t, decision(plan, "update"))
			if len(results) != 1 || results[0].TargetID != test.target || results[0].Status != "updated" {
				t.Errorf("pinned update should replace only the older copy: %+v", results)
			}
			_, after, err := installstate.Snapshot(protectedRoot, "writer")
			if err != nil || before != after {
				t.Errorf("newer copy was modified: %v", err)
			}
		})
	}
}

func TestSkillUpdatesResolveRecordedReleaseBeforeFileVersion(t *testing.T) {
	for _, test := range []struct{ name, commit, ref string }{
		{"commit", "commit-1.0.0", "main"},
		{"immutable ref", "", "commit-1.0.0"},
		{"version ref", "", "1.0.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newSkillUpdateFixture(t)
			for _, root := range []string{f.root, f.agentRoot} {
				records, err := installstate.Read(root)
				if err != nil {
					t.Fatal(err)
				}
				record := records["writer"]
				record.Version, record.Commit, record.Ref = "", test.commit, test.ref
				_, err = local.InstallArchiveWithOptions(root, "writer.zip", publishedSkillArchive(t, "writer", "commit-1.1.0", false), local.InstallArchiveOptions{Replace: true, Origin: record})
				if err != nil {
					t.Fatal(err)
				}
			}
			plan := f.preview(t, "writer")
			if !plan.Available || plan.LocalVersion != "1.0.0" {
				t.Fatalf("file metadata hid the recorded older release: %+v", plan)
			}
			for _, target := range plan.Targets {
				if target.Version != "1.0.0" {
					t.Fatalf("wrong copy version: %+v", target)
				}
			}
			for _, result := range f.apply(t, decision(plan, "update")) {
				if result.Status != "updated" {
					t.Fatal(result)
				}
			}
		})
	}
}

func TestSkillUpdatesKeepRecordsIgnoreOnLibraryWhenLibraryIsNewer(t *testing.T) {
	f := newSkillUpdateFixture(t)
	records, err := installstate.Read(f.root)
	if err != nil {
		t.Fatal(err)
	}
	record := records["writer"]
	record.Version = "1.2.0"
	if err := installstate.Update(f.root, "writer", &record, nil); err != nil {
		t.Fatal(err)
	}
	plan := f.preview(t, "writer")
	if len(plan.Targets) != 1 || plan.Targets[0].ID == "library" {
		t.Fatal("newer library must not be a replacement target")
	}
	results := f.apply(t, decision(plan, "keep-local"))
	if len(results) != 1 || results[0].TargetID != "library" || results[0].Status != "kept" {
		t.Fatal(results)
	}
	ignored := f.preview(t, "writer")
	if ignored.Available || !ignored.Skipped {
		t.Fatal("ignoring the release was not persisted on the library")
	}
}
