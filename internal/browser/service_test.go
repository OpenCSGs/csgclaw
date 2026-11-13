package browser

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
)

func TestBrowserURLValidation(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "javascript:alert(1)", "https://u:p@example.com", "data:text/html,test", "https:///path", ""} {
		if _, err := validURL(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestBrowserEndToEnd(t *testing.T) {
	t.Setenv("CSGCLAW_BROWSER_ALLOW_PRIVATE_NETWORK", "true")
	if _, ok := launcher.LookPath(); !ok && os.Getenv("CSGCLAW_BROWSER_BIN") == "" {
		t.Skip("Chromium is not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/set-cookie" {
			http.SetCookie(w, &http.Cookie{Name: "browser-test", Value: "private", Path: "/"})
			_, _ = w.Write([]byte("<h1>Cookie set</h1>"))
			return
		}
		if r.URL.Path == "/read-cookie" {
			value := "empty"
			if cookie, err := r.Cookie("browser-test"); err == nil {
				value = cookie.Value
			}
			_, _ = w.Write([]byte("<h1>Cookie: " + value + "</h1>"))
			return
		}
		if r.URL.Path == "/done" {
			_, _ = w.Write([]byte(`<title>Submitted</title><h1>Saved ` + r.URL.Query().Get("name") + `</h1>`))
			return
		}
		_, _ = w.Write([]byte(`<title>Browser test</title><form action="/done"><label>Name <input name="name" value="old"></label><label>Color <select name="color"><option>Red</option><option>Blue</option></select></label><button>Save</button></form><div style="height:1000px"></div><p>Page bottom</p>`))
	}))
	defer server.Close()
	s := New()
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	openCtx, openCancel := context.WithCancel(ctx)
	state, err := s.Open(openCtx, "room-a", server.URL)
	openCancel()
	if err != nil {
		t.Fatal(err)
	}
	if state.Title != "Browser test" || len(s.List("room-a")) != 1 || len(s.List("room-b")) != 0 {
		t.Fatalf("state=%+v", state)
	}
	if _, err = s.Snapshot(ctx, "room-b", state.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-room read: %v", err)
	}
	snapshot, err := s.Snapshot(ctx, "room-a", state.ID)
	if err != nil {
		t.Fatal(err)
	}
	ref := func(role, name string) string {
		t.Helper()
		match := regexp.MustCompile(`\[(e\d+)\] ` + role + ` "` + name + `"`).FindStringSubmatch(snapshot.Content)
		if len(match) != 2 {
			t.Fatalf("missing %s %s: %s", role, name, snapshot.Content)
		}
		return match[1]
	}
	nameRef, selectRef, buttonRef := ref("textbox", "Name"), ref("combobox", "Color"), ref("button", "Save")
	for _, cmd := range []Command{{Action: "fill", Ref: nameRef, Text: "Alice 用户"}, {Action: "select", Ref: selectRef, Text: "Blue"}} {
		if _, err = s.Run(ctx, "room-a", state.ID, cmd); err != nil {
			t.Fatal(err)
		}
	}
	// A canceled operation must not terminate the shared Chromium session.
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err = s.Screenshot(canceled, "room-a", state.ID, false); err == nil {
		t.Fatal("canceled capture succeeded")
	}
	data, err := s.Screenshot(ctx, "room-a", state.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != Width || img.Bounds().Dy() != Height {
		t.Fatalf("viewport screenshot: %v", err)
	}
	data, err = s.Screenshot(ctx, "room-a", state.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	img, err = png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dy() <= Height {
		t.Fatalf("full screenshot: %v", err)
	}
	if _, err = s.Run(ctx, "room-a", state.ID, Command{Action: "click", Ref: buttonRef}); err != nil {
		t.Fatal(err)
	}
	for {
		snapshot, err = s.Snapshot(ctx, "room-a", state.ID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(snapshot.Content, "Saved Alice 用户") {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("submission not observed: %s", snapshot.Content)
		case <-time.After(30 * time.Millisecond):
		}
	}
	if !snapshot.CanGoBack {
		t.Fatal("missing navigation history")
	}
	if _, err = s.Run(ctx, "room-a", state.ID, Command{Action: "back"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/set-cookie", "/read-cookie"} {
		if _, err = s.Run(ctx, "room-a", state.ID, Command{Action: "navigate", URL: server.URL + path}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err = s.Snapshot(ctx, "room-a", state.ID)
	if err != nil || !strings.Contains(snapshot.Content, "Cookie: private") {
		t.Fatalf("missing session cookie: %v %s", err, snapshot.Content)
	}
	isolated, err := s.Open(ctx, "room-b", server.URL+"/read-cookie")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.Snapshot(ctx, "room-b", isolated.ID)
	if err != nil || !strings.Contains(snapshot.Content, "Cookie: empty") {
		t.Fatalf("cookies crossed sessions: %v %s", err, snapshot.Content)
	}
	errorsFromCaptures := make(chan error, 4)
	for range 4 {
		go func() { _, err := s.Screenshot(ctx, "room-b", isolated.ID, false); errorsFromCaptures <- err }()
	}
	for range 4 {
		if err = <-errorsFromCaptures; err != nil {
			t.Fatal(err)
		}
	}
	if err = s.CloseSession(ctx, "room-b", isolated.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.CloseSession(ctx, "room-b", state.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-room close: %v", err)
	}
	if err = s.CloseSession(ctx, "room-a", state.ID); err != nil {
		t.Fatal(err)
	}
	if len(s.List("room-a")) != 0 {
		t.Fatal("session not removed")
	}
	if _, err = s.Snapshot(ctx, "room-a", state.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("closed session: %v", err)
	}
	// Losing Chromium must not leave the server permanently unable to open
	// another page. The new session also exercises idle and room cleanup.
	s.launcher.Kill()
	s.launcher.Cleanup()
	recovered, err := s.Open(ctx, "room-a", server.URL)
	if err != nil {
		t.Fatalf("recover browser process: %v", err)
	}
	if err = s.closeSession(ctx, "room-a", recovered.ID, time.Now().Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(s.List("room-a")) != 1 {
		t.Fatal("idle cleanup closed an active session")
	}
	s.CloseRoom(ctx, "room-a")
	if len(s.List("room-a")) != 0 {
		t.Fatal("room deletion retained a browser session")
	}
	if _, err = s.Open(ctx, "room-a", server.URL); err == nil {
		t.Fatal("opened a browser after its room was deleted")
	}
}
