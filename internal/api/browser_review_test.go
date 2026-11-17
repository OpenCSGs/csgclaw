package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/browser"
	"csgclaw/internal/im"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReadOnlyBrowserScreenshotCannotPublish(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok && os.Getenv("CSGCLAW_BROWSER_BIN") == "" {
		t.Skip("Chromium is not installed")
	}
	t.Setenv("CSGCLAW_BROWSER_ALLOW_PRIVATE_NETWORK", "true")
	t.Setenv("CSGCLAW_EXECUTION_MODE", "read_only")
	h := newAppTaskTestHandler(t)
	t.Cleanup(h.CloseBrowser)
	records := h.svc.(appTaskTestRecords)
	worker := records.records["agent-dev"]
	worker.RuntimeKind = "codex"
	records.records[worker.ID] = worker
	path := filepath.Join(t.TempDir(), "im", "state.json")
	if err := im.SaveBootstrap(path, h.im.Bootstrap()); err != nil {
		t.Fatal(err)
	}
	var err error
	h.im, err = im.NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Read-only screenshots", CreatorID: "admin", MemberIDs: []string{"dev"}})
	if err != nil {
		t.Fatal(err)
	}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<h1>Screenshot</h1>")) }))
	defer site.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	state, err := h.browserService().Open(ctx, room.ID, site.URL)
	if err != nil {
		t.Fatal(err)
	}
	h.registerBrowserTools(worker.ID)
	client := taskMCPClient(t, h, worker.ID)
	denyTaskTool(t, client, "browser_screenshot", map[string]any{"room_id": room.ID, "browser_id": state.ID})
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "browser_screenshot", Arguments: map[string]any{"room_id": room.ID, "browser_id": state.ID, "send_to_chat": false}})
	if err != nil || result.IsError {
		t.Fatalf("read-only capture failed: %v %+v", err, result)
	}
	saved, _ := h.im.Room(room.ID)
	for _, message := range saved.Messages {
		if len(message.Attachments) > 0 {
			t.Fatal("read-only screenshot created a persistent attachment")
		}
	}
}

func TestBrowserPointerRequiresUserTakeover(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok && os.Getenv("CSGCLAW_BROWSER_BIN") == "" {
		t.Skip("Chromium is not installed")
	}
	t.Setenv("CSGCLAW_BROWSER_ALLOW_PRIVATE_NETWORK", "true")
	h := newAppTaskTestHandler(t)
	t.Cleanup(h.CloseBrowser)
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Stale frame", CreatorID: "admin", MemberIDs: []string{"dev"}})
	if err != nil {
		t.Fatal(err)
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	state, err := h.browserService().Open(ctx, room.ID, site.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.browserService().Screenshot(ctx, room.ID, state.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = h.browserService().Run(ctx, room.ID, state.ID, browser.Command{Action: "navigate", URL: site.URL + "/changed"}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+room.ID+"/browsers/"+state.ID, strings.NewReader(`{"action":"pointer","x":100,"y":60}`))
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("pointer without takeover returned %d; expected rejection", w.Code)
	}
	if hits.Load() != 0 {
		t.Fatal("stale pointer activated a different page control")
	}
}
