package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/im"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/taskconfig"
)

func TestSessionWorkspaceAccessUsesConfiguredRootAndCredentials(t *testing.T) {
	h, _, agentID, defaultRoot, _ := referenceTestHandler(t)
	custom := t.TempDir()
	svc := h.svc.(*agent.Controller)
	options := map[string]any{"local_workspace_dir": custom}
	if _, err := svc.UpdateRecord(t.Context(), agentID, agent.UpdateRequest{RuntimeOptions: &options}); err != nil {
		t.Fatal(err)
	}
	selected, err := agentengine.New(svc).Agents().Get(t.Context(), agentID, agentengine.AgentGetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	const contents = "private session contents"
	source := im.AttachmentSource{Name: "private.txt", MediaType: "text/plain", SizeBytes: int64(len(contents)), Reader: io.NopCloser(strings.NewReader(contents))}
	files, err := h.saveSessionAttachmentSources(t.Context(), selected, "session-x", "batch-x", []im.AttachmentSource{source})
	if err != nil {
		t.Fatal(err)
	}
	att := files[0]
	local := filepath.Join(custom, filepath.FromSlash(att.WorkspacePath))
	if _, err := os.Stat(local); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(defaultRoot, filepath.FromSlash(att.WorkspacePath))); !os.IsNotExist(err) {
		t.Fatalf("unexpected default workspace attachment: %v", err)
	}
	h.serverNoAuth = false
	h.serverAccessToken = "workspace-test-server"
	h.SetDesktopSessionToken("workspace-test-desktop")
	if err := h.EnableConnectors(filepath.Join(t.TempDir(), "apps.json")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.CloseConnectors() })
	routes := h.Routes()
	base := "/api/v1/agents/" + agentID + "/workspace"
	fileQuery := "?path=" + url.QueryEscape(att.WorkspacePath)
	parentQuery := "?path=" + url.QueryEscape(filepath.ToSlash(filepath.Dir(filepath.FromSlash(att.WorkspacePath))))
	skills := "/api/v1/agents/" + agentID + "/skills"
	for _, target := range []string{
		base, base + "?depth=1", base + parentQuery,
		base + "/file" + fileQuery, base + "/content" + fileQuery, base + "/location" + fileQuery,
		skills, skills + "/file?path=secret.md",
	} {
		req := httptest.NewRequest("GET", target, nil)
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "private.txt") {
			t.Fatalf("anonymous %s: %d %s", target, rec.Code, rec.Body.String())
		}
	}
	for _, token := range []string{h.serverAccessToken, h.desktopSessionToken} {
		req := httptest.NewRequest("GET", base+"/content"+fileQuery, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != contents {
			t.Fatalf("authorized preview: %d %s", rec.Code, rec.Body.String())
		}
	}
	capability, err := url.Parse(att.DownloadURL)
	if err != nil || capability.Query().Get("token") == "" {
		t.Fatalf("missing download token: %v", err)
	}
	capURL := base + "/content" + fileQuery + "&token=" + url.QueryEscape(capability.Query().Get("token"))
	req := httptest.NewRequest("GET", capURL, nil)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("attachment capability authorized workspace: %d", rec.Code)
	}

	// The alternate room root is outside the Agent's configured workspace.
	// A hard link gives the saved file a second name under that root.
	aliasRoot := t.TempDir()
	aliasDir := filepath.Join(aliasRoot, "shared")
	if err := os.Mkdir(aliasDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(local, filepath.Join(aliasDir, "ordinary.txt")); err != nil {
		t.Fatal(err)
	}
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Explicit workspace", CreatorID: im.AdminUserID, MemberIDs: []string{agentID}, Configuration: &taskconfig.Config{WorkspaceDir: aliasRoot}})
	if err != nil {
		t.Fatal(err)
	}
	roomQuery := url.Values{"room_id": {room.ID}, "path": {"shared/ordinary.txt"}}
	rec = httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest("GET", base+"/content?"+roomQuery.Encode(), nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("hard-link preview: %d", rec.Code)
	}
	for _, relative := range []string{"shared/ordinary.txt", "shared"} {
		body, _ := json.Marshal(map[string]any{"room_id": room.ID, "sender_id": im.AdminUserID, "content": "share", "file_references": []FileReference{{Source: "workspace", AgentID: agentID, Path: relative}}})
		for _, token := range []string{"", h.serverAccessToken} {
			req := httptest.NewRequest("POST", "/api/v1/messages", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			rec := httptest.NewRecorder()
			routes.ServeHTTP(rec, req)
			want := http.StatusUnauthorized
			if token != "" {
				want = http.StatusCreated
			}
			if rec.Code != want {
				t.Fatalf("share %s: %d, want %d: %s", relative, rec.Code, want, rec.Body.String())
			}
		}
	}
	actor, _ := svc.Agent(agentID)
	profile, err := svc.RuntimeHost().ResolveRuntimeProfile(agentruntime.Handle{RuntimeID: actor.RuntimeID})
	if err != nil {
		t.Fatal(err)
	}
	post := map[string]any{"room_id": room.ID, "content": "share", "file_references": []FileReference{{Source: "workspace", AgentID: agentID, Path: "shared/ordinary.txt"}}}
	encoded, err := json.Marshal(post)
	if err != nil {
		t.Fatal(err)
	}
	app := appAuthRequest(t, h, "POST", "/api/v1/messages", string(encoded), profile.APIKey, nil)
	if app.Code != http.StatusCreated {
		t.Fatalf("owning room Agent reference = %d %s", app.Code, app.Body.String())
	}
	app = appAuthRequest(t, h, "GET", base+"/content"+fileQuery, "", profile.APIKey, nil)
	if app.Code != http.StatusForbidden {
		t.Fatalf("room credential authorized general workspace browsing: %d", app.Code)
	}
}
