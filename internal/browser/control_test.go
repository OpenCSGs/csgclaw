package browser

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
)

func TestBrowserUserTakeoverExcludesAgentAndStaleFrames(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok && os.Getenv("CSGCLAW_BROWSER_BIN") == "" {
		t.Skip("Chromium is not installed")
	}
	t.Setenv(PrivateNetworkEnv, "true")
	var hits atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/submit" {
			hits.Add(1)
			w.WriteHeader(204)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<button style="position:absolute;left:20px;top:20px;width:200px;height:80px" onclick="fetch('/submit')">Submit</button>`))
	}))
	defer site.Close()
	s := New()
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	state, err := s.Open(ctx, "room", site.URL)
	if err != nil {
		t.Fatal(err)
	}
	control, err := s.TakeControl(ctx, "room", state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !control.Session.UserControlled || control.Token == "" {
		t.Fatal("missing exclusive control")
	}
	pointer := Command{Action: "pointer", X: 100, Y: 60}
	if _, err = s.RunUser(ctx, "room", state.ID, pointer, control.Token); !errors.Is(err, ErrControlRequired) {
		t.Fatalf("input before fresh frame: %v", err)
	}
	frame, err := s.UserScreenshot(ctx, "room", state.ID, false, control.Token)
	if err != nil {
		t.Fatal(err)
	}
	pointer.FrameRevision = frame.Revision
	for _, command := range []Command{{Action: "navigate", URL: site.URL + "/other"}, {Action: "scroll", Y: 500}, {Action: "resize", Width: 800, Height: 600}, {Action: "type", Text: "unexpected"}} {
		if _, err = s.Run(ctx, "room", state.ID, command); !errors.Is(err, ErrUserControl) {
			t.Fatalf("Agent mutated a user-controlled browser: %s %v", command.Action, err)
		}
	}
	if err = s.CloseAgentSession(ctx, "room", state.ID); !errors.Is(err, ErrUserControl) {
		t.Fatalf("Agent closed user browser: %v", err)
	}
	if _, err = s.RunUser(ctx, "room", state.ID, Command{Action: "resize", Width: 800, Height: 600}, ""); !errors.Is(err, ErrUserControl) {
		t.Fatalf("other viewer resized user browser: %v", err)
	}
	if _, err = s.RunUser(ctx, "room", state.ID, Command{Action: "resize", Width: 800, Height: 600}, control.Token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RunUser(ctx, "room", state.ID, pointer, control.Token); !errors.Is(err, ErrControlRequired) {
		t.Fatalf("old frame accepted after resize: %v", err)
	}
	frame, err = s.UserScreenshot(ctx, "room", state.ID, false, control.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RunUser(ctx, "room", state.ID, pointer, control.Token); !errors.Is(err, ErrControlRequired) {
		t.Fatalf("displayed old frame accepted after a new capture: %v", err)
	}
	pointer.FrameRevision = frame.Revision
	if _, err = s.RunUser(ctx, "room", state.ID, pointer, control.Token); err != nil {
		t.Fatal(err)
	}
	for hits.Load() == 0 {
		select {
		case <-ctx.Done():
			t.Fatal("valid user input did not activate the control")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if hits.Load() != 1 {
		t.Fatal("user input was duplicated")
	}
	if _, err = s.ReleaseControl(ctx, "room", state.ID, "wrong"); !errors.Is(err, ErrControlRequired) {
		t.Fatalf("wrong owner released control: %v", err)
	}
	if _, err = s.ReleaseControl(ctx, "room", state.ID, control.Token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RunUser(ctx, "room", state.ID, pointer, control.Token); !errors.Is(err, ErrControlRequired) {
		t.Fatalf("released control accepted input: %v", err)
	}
	if _, err = s.Run(ctx, "room", state.ID, Command{Action: "navigate", URL: site.URL + "/resumed"}); err != nil {
		t.Fatal(err)
	}
	control, err = s.TakeControl(ctx, "room", state.ID)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.sessions[state.ID].controlUntil = time.Now().Add(-time.Second)
	s.mu.Unlock()
	if _, err = s.Run(ctx, "room", state.ID, Command{Action: "reload"}); err != nil {
		t.Fatalf("expired control blocked Agent recovery: %v", err)
	}
	if _, err = s.UserScreenshot(ctx, "room", state.ID, false, control.Token); !errors.Is(err, ErrControlRequired) {
		t.Fatalf("expired token renewed control: %v", err)
	}
}

func TestBrowserReloadInvalidatesDisplayedFrame(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok && os.Getenv("CSGCLAW_BROWSER_BIN") == "" {
		t.Skip("Chromium is not installed")
	}
	t.Setenv(PrivateNetworkEnv, "true")
	var loads, submissions atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/submit" {
			submissions.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		label := "Preview"
		if loads.Add(1) > 1 {
			label = "Submit"
		}
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintf(w, `<title>Same URL</title><button style="position:absolute;left:20px;top:20px;width:200px;height:80px" onclick="fetch('/submit')">%s</button>`, label)
	}))
	defer site.Close()
	s := New()
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	state, err := s.Open(ctx, "room", site.URL)
	if err != nil {
		t.Fatal(err)
	}
	control, err := s.TakeControl(ctx, "room", state.ID)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := s.UserScreenshot(ctx, "room", state.ID, false, control.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RunUser(ctx, "room", state.ID, Command{Action: "reload", FrameRevision: frame.Revision}, control.Token); err != nil {
		t.Fatal(err)
	}
	if loads.Load() != 2 {
		t.Fatalf("main document was not reloaded: %d", loads.Load())
	}
	pointer := Command{Action: "pointer", X: 100, Y: 60, FrameRevision: frame.Revision}
	if _, err = s.RunUser(ctx, "room", state.ID, pointer, control.Token); !errors.Is(err, ErrControlRequired) {
		t.Fatalf("old frame accepted after reload: %v", err)
	}
	fresh, err := s.UserScreenshot(ctx, "room", state.ID, false, control.Token)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Revision == frame.Revision {
		t.Fatal("reloaded document retained its frame revision")
	}
	if _, err = s.RunUser(ctx, "room", state.ID, pointer, control.Token); !errors.Is(err, ErrControlRequired) {
		t.Fatalf("old displayed frame accepted after a fresh capture: %v", err)
	}
	if submissions.Load() != 0 {
		t.Fatal("obsolete input submitted the new document")
	}
	pointer.FrameRevision = fresh.Revision
	if _, err = s.RunUser(ctx, "room", state.ID, pointer, control.Token); err != nil {
		t.Fatal(err)
	}
	for submissions.Load() != 1 {
		select {
		case <-ctx.Done():
			t.Fatal("fresh frame input did not submit")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
