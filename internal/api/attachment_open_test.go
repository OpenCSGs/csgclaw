package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"csgclaw/internal/im"
)

func TestGeneratedAttachmentLocalOpening(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("native display required")
	}
	svc, err := im.NewServiceFromPath(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	room, err := svc.CreateRoom(im.CreateRoomRequest{Title: "Generated", CreatorID: "user-admin"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := svc.DeliverMessage(im.DeliverMessageRequest{RoomID: room.ID, SenderID: "user-admin", AttachmentSources: []im.AttachmentSource{{Name: "生成的文件.json", MediaType: "application/json", SizeBytes: 11, Reader: io.NopCloser(strings.NewReader(`{"ok":true}`))}}})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: svc, serverNoAuth: true}
	var opened string
	launches := 0
	h.workspaceOpener = func(_ context.Context, path, target string, directory bool) error {
		opened = path
		launches++
		if directory || target != "system" {
			t.Fatal("invalid launch")
		}
		return nil
	}
	prefix := "/api/v1/rooms/" + room.ID + "/attachments/" + message.Attachments[0].ID
	request := func(method, path, body, peer, origin, auth string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = peer
		r.Host = "127.0.0.1:18080"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		h.Routes().ServeHTTP(w, r)
		return w
	}
	w := request("GET", prefix+"/location", "", "127.0.0.1:1", "", "")
	var location workspaceLocation
	if err := json.Unmarshal(w.Body.Bytes(), &location); err != nil || w.Code != 200 || filepath.Base(location.AbsolutePath) != "生成的文件.json" || len(location.OpenTargets) == 0 {
		t.Fatalf("location: %d %s %v", w.Code, w.Body.String(), err)
	}
	if data, err := os.ReadFile(location.AbsolutePath); err != nil || string(data) != `{"ok":true}` {
		t.Fatalf("copy: %s %v", data, err)
	}
	for _, tc := range []struct {
		name, method, path, peer, origin, auth, body string
		status                                       int
	}{
		{"open", "POST", prefix + "/open", "127.0.0.1:1", "http://127.0.0.1:18080", "", `{"target":"system"}`, 204},
		{"remote", "GET", prefix + "/location", "192.0.2.1:1", "", "", "", 403},
		{"invalid credential", "GET", prefix + "/location", "127.0.0.1:1", "", "Bearer invalid", "", 403},
		{"cross origin", "POST", prefix + "/open", "127.0.0.1:1", "https://evil.example", "", `{"target":"system"}`, 403},
		{"invented app", "POST", prefix + "/open", "127.0.0.1:1", "http://127.0.0.1:18080", "", `{"target":"app:/bin/sh"}`, 403},
		{"wrong room", "GET", "/api/v1/rooms/missing/attachments/" + message.Attachments[0].ID + "/location", "127.0.0.1:1", "", "", "", 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(tc.method, tc.path, tc.body, tc.peer, tc.origin, tc.auth)
			if w.Code != tc.status {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
		})
	}
	if opened != location.AbsolutePath {
		t.Fatalf("opened %q", opened)
	}

	// Browser attachment actions retain the live file capability and local Origin boundary.
	h.serverNoAuth = false
	h.serverAccessToken = "management-token"
	download, _ := url.Parse(message.Attachments[0].DownloadURL)
	capability := download.Query().Get("token")
	w = request("GET", prefix+"/location?"+url.Values{"token": {capability}}.Encode(), "", "127.0.0.1:1", "", "")
	if w.Code != 200 {
		t.Fatalf("capability location: %d %s", w.Code, w.Body.String())
	}
	w = request("POST", prefix+"/open?"+url.Values{"token": {capability}}.Encode(), `{"target":"system"}`, "127.0.0.1:1", "http://127.0.0.1:18080", "")
	if w.Code != 204 {
		t.Fatalf("local capability open: %d %s", w.Code, w.Body.String())
	}

	protected := prefix + "/open?" + url.Values{"token": {capability}}.Encode()
	other, err := svc.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "user-admin", Attachments: []im.MessageAttachmentUpload{{Name: "other.json", Data: []byte(`{"other":true}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, method, path, peer, origin, auth, body string
		status                                       int
	}{
		{"missing capability", "POST", prefix + "/open", "127.0.0.1:1", "http://127.0.0.1:18080", "", `{"target":"system"}`, 401},
		{"wrong capability", "POST", prefix + "/open?token=invalid", "127.0.0.1:1", "http://127.0.0.1:18080", "", `{"target":"system"}`, 401},
		{"invalid credential overrides capability", "POST", protected, "127.0.0.1:1", "http://127.0.0.1:18080", "Bearer invalid", `{"target":"system"}`, 401},
		{"scoped credential", "POST", protected, "127.0.0.1:1", "http://127.0.0.1:18080", "Bearer agent.invalid", `{"target":"system"}`, 401},
		{"remote capability", "POST", protected, "192.0.2.1:1", "http://127.0.0.1:18080", "", `{"target":"system"}`, 401},
		{"cross-site capability", "POST", protected, "127.0.0.1:1", "https://evil.example", "", `{"target":"system"}`, 401},
		{"capability without origin", "POST", protected, "127.0.0.1:1", "", "", `{"target":"system"}`, 401},
		{"sandboxed origin", "POST", protected, "127.0.0.1:1", "null", "", `{"target":"system"}`, 401},
		{"invented app with capability", "POST", protected, "127.0.0.1:1", "http://127.0.0.1:18080", "", `{"target":"app:/bin/sh"}`, 403},
		{"another attachment", "POST", "/api/v1/rooms/" + room.ID + "/attachments/" + other.Attachments[0].ID + "/open?" + url.Values{"token": {capability}}.Encode(), "127.0.0.1:1", "http://127.0.0.1:18080", "", `{"target":"system"}`, 401},
		{"listing remains protected", "GET", "/api/v1/rooms/" + room.ID + "/attachments?" + url.Values{"token": {capability}}.Encode(), "127.0.0.1:1", "", "", "", 401},
		{"deletion remains protected", "DELETE", prefix + "?" + url.Values{"token": {capability}}.Encode(), "127.0.0.1:1", "http://127.0.0.1:18080", "", "", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := launches
			w := request(tc.method, tc.path, tc.body, tc.peer, tc.origin, tc.auth)
			if w.Code != tc.status {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			if launches != before {
				t.Fatal("denied request launched an application")
			}
		})
	}
	if runtime.GOOS == "darwin" {
		var app string
		for _, target := range location.OpenTargets {
			if strings.HasPrefix(target.ID, "app:") {
				app = target.ID
				break
			}
		}
		if app == "" {
			t.Fatal("system returned no applications for JSON")
		}
		// Exercise the process handoff with real OS associations and a recording opener.
		bin := t.TempDir()
		output := filepath.Join(bin, "arguments")
		if err := os.WriteFile(filepath.Join(bin, "open"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CSGCLAW_TEST_OPEN_OUTPUT\"\n"), 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("CSGCLAW_TEST_OPEN_OUTPUT", output)
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		h.workspaceOpener = nil
		body, _ := json.Marshal(map[string]string{"target": app})
		w = request("POST", protected, string(body), "127.0.0.1:1", "http://127.0.0.1:18080", "")
		if w.Code != 204 {
			t.Fatalf("native handoff: %d %s", w.Code, w.Body.String())
		}
		arguments, err := os.ReadFile(output)
		if err != nil || string(arguments) != "-a\n"+strings.TrimPrefix(app, "app:")+"\n"+location.AbsolutePath+"\n" {
			t.Fatalf("native argv: %s %v", arguments, err)
		}
	}
	if err := svc.DeleteRoomAttachment(room.ID, message.Attachments[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(location.AbsolutePath); !os.IsNotExist(err) {
		t.Fatalf("local copy survived deletion: %v", err)
	}
	w = request("POST", protected, `{"target":"system"}`, "127.0.0.1:1", "http://127.0.0.1:18080", "")
	if w.Code != 401 {
		t.Fatalf("deleted open: %d", w.Code)
	}
}
