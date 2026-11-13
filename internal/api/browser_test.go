package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/browser"
	"csgclaw/internal/im"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBrowserMCPWorkflow(t *testing.T) {
	t.Setenv("CSGCLAW_BROWSER_ALLOW_PRIVATE_NETWORK", "true")
	if _, ok := launcher.LookPath(); !ok && os.Getenv("CSGCLAW_BROWSER_BIN") == "" {
		t.Skip("Chromium is not installed")
	}
	h := newAppTaskTestHandler(t)
	t.Cleanup(h.CloseBrowser)
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Browser", CreatorID: "admin", MemberIDs: []string{"dev"}})
	if err != nil {
		t.Fatal(err)
	}
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<title>MCP browser</title><label>Email<input type="email"></label><button onclick="document.querySelector('h1').textContent=document.querySelector('input').value">Apply</button><h1>Ready</h1>`))
	}))
	t.Cleanup(page.Close)
	h.registerBrowserTools("agent-dev")
	h.registerBrowserTools("agent-outsider")
	client := taskMCPClient(t, h, "agent-dev")
	outsider := taskMCPClient(t, h, "agent-outsider")
	denyTaskTool(t, outsider, "browser_open", map[string]any{"room_id": room.ID, "url": page.URL})
	var state browser.State
	callTaskTool(t, client, "browser_open", map[string]any{"room_id": room.ID, "url": page.URL}, &state)
	args := map[string]any{"room_id": room.ID, "browser_id": state.ID}
	var snapshot browser.Snapshot
	callTaskTool(t, client, "browser_snapshot", args, &snapshot)
	textbox := regexp.MustCompile(`\[(e\d+)\] textbox "Email"`).FindStringSubmatch(snapshot.Content)
	button := regexp.MustCompile(`\[(e\d+)\] button "Apply"`).FindStringSubmatch(snapshot.Content)
	if len(textbox) != 2 || len(button) != 2 {
		t.Fatalf("snapshot: %s", snapshot.Content)
	}
	callTaskTool(t, client, "browser_act", map[string]any{"room_id": room.ID, "browser_id": state.ID, "action": "fill", "ref": textbox[1], "text": "browser@example.test"}, nil)
	callTaskTool(t, client, "browser_act", map[string]any{"room_id": room.ID, "browser_id": state.ID, "action": "click", "ref": button[1]}, nil)
	callTaskTool(t, client, "browser_snapshot", args, &snapshot)
	if !strings.Contains(snapshot.Content, `heading "browser@example.test"`) {
		t.Fatalf("form was not applied: %s", snapshot.Content)
	}
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "browser_screenshot", Arguments: map[string]any{"room_id": room.ID, "browser_id": state.ID, "send_to_chat": false}})
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("screenshot=%+v, %v", result, err)
	}
	image, ok := result.Content[0].(*mcp.ImageContent)
	if !ok || image.MIMEType != "image/png" || len(image.Data) < 100 {
		t.Fatal("screenshot is not an MCP image")
	}
	denyTaskTool(t, outsider, "browser_snapshot", args)
	// The HTTP view uses the same session, not a separately navigated iframe.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+room.ID+"/browsers", nil)
	response := httptest.NewRecorder()
	h.Routes().ServeHTTP(response, req)
	var listed []browser.State
	if err = json.Unmarshal(response.Body.Bytes(), &listed); err != nil || len(listed) != 1 || listed[0].ID != state.ID {
		t.Fatalf("shared UI list=%s, %v", response.Body.String(), err)
	}
	// Native turn metadata binds all browser tools to the originating room.
	bound := map[string]any{"room_id": "other"}
	if err = h.scopeAppPlatformRoomConversation("agent-dev", "browser_list", "csgclaw-im:csgclaw:room:"+room.ID, bound); err != nil || bound["room_id"] != room.ID {
		t.Fatalf("scope=%v %v", bound, err)
	}
	callTaskTool(t, client, "browser_close", args, nil)
	denyTaskTool(t, client, "browser_snapshot", args)
}

func TestBrowserAPIScopeAndValidation(t *testing.T) {
	h := newAppTaskTestHandler(t)
	t.Cleanup(h.CloseBrowser)
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Browser", CreatorID: "admin", MemberIDs: []string{"dev"}})
	if err != nil {
		t.Fatal(err)
	}
	router := h.Routes()
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/v1/rooms/missing/browsers", "", 404},
		{"GET", "/api/v1/rooms/" + room.ID + "/browsers/missing", "", 404},
		{"POST", "/api/v1/rooms/" + room.ID + "/browsers", `{"url":"file:///etc/passwd"}`, 422},
		{"POST", "/api/v1/rooms/" + room.ID + "/browsers/missing", `{"action":"fill","unknown":true}`, 400},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s %s = %d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err = h.browserService().Run(ctx, "other", "missing", browser.Command{Action: "reload"}); err == nil {
		t.Fatal("cross-room API access succeeded")
	}
}

func TestBrowserToolsRespectReadOnly(t *testing.T) {
	t.Setenv("CSGCLAW_EXECUTION_MODE", "read_only")
	h := newAppTaskTestHandler(t)
	t.Cleanup(h.CloseBrowser)
	records := h.svc.(appTaskTestRecords)
	worker := records.records["agent-dev"]
	worker.RuntimeKind = "codex"
	records.records[worker.ID] = worker
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Read-only browser", CreatorID: "admin", MemberIDs: []string{"dev"}})
	if err != nil {
		t.Fatal(err)
	}
	h.registerBrowserTools(worker.ID)
	client := taskMCPClient(t, h, worker.ID)
	denyTaskTool(t, client, "browser_open", map[string]any{"room_id": room.ID, "url": "https://example.test"})
	callTaskTool(t, client, "browser_list", map[string]any{"room_id": room.ID}, nil)
	if len(h.browserService().List(room.ID)) != 0 {
		t.Fatal("read-only Agent created a browser")
	}
}

func TestBrowserScreenshotDeliveredAndSurvivesBrowserClose(t *testing.T) {
	t.Setenv("CSGCLAW_BROWSER_ALLOW_PRIVATE_NETWORK", "true")
	if _, ok := launcher.LookPath(); !ok && os.Getenv("CSGCLAW_BROWSER_BIN") == "" {
		t.Skip("Chromium is not installed")
	}
	h := newAppTaskTestHandler(t)
	t.Cleanup(h.CloseBrowser)
	statePath := filepath.Join(t.TempDir(), "im", "state.json")
	if err := im.SaveBootstrap(statePath, h.im.Bootstrap()); err != nil {
		t.Fatal(err)
	}
	bus := im.NewBus()
	var err error
	h.im, err = im.NewServiceFromPathWithBus(statePath, bus)
	if err != nil {
		t.Fatal(err)
	}
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Screenshot delivery", CreatorID: "admin", MemberIDs: []string{"dev"}})
	if err != nil {
		t.Fatal(err)
	}
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<h1>Screenshot delivery</h1>")
	}))
	defer page.Close()
	h.registerBrowserTools("agent-dev")
	client := taskMCPClient(t, h, "agent-dev")
	var state browser.State
	callTaskTool(t, client, "browser_open", map[string]any{"room_id": room.ID, "url": page.URL}, &state)
	inspection, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "browser_screenshot", Arguments: map[string]any{"room_id": room.ID, "browser_id": state.ID, "send_to_chat": false}})
	if err != nil || inspection.IsError || len(inspection.Content) != 1 {
		t.Fatalf("inspection screenshot: %v %+v", err, inspection)
	}
	before, _ := h.im.Room(room.ID)
	for _, message := range before.Messages {
		if len(message.Attachments) > 0 {
			t.Fatal("inspection-only screenshot was sent to the user")
		}
	}
	events, unsubscribe := bus.Subscribe()
	defer unsubscribe()
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "browser_screenshot", Arguments: map[string]any{"room_id": room.ID, "browser_id": state.ID}})
	if err != nil || result.IsError {
		t.Fatalf("screenshot failed: %v %+v", err, result)
	}
	saved, _ := h.im.Room(room.ID)
	var attachment im.MessageAttachment
	for _, message := range saved.Messages {
		for _, file := range message.Attachments {
			if file.MediaType == "image/png" {
				attachment = file
				if message.SenderID != h.im.ResolveUserID("agent-dev") {
					t.Fatalf("wrong screenshot sender %s", message.SenderID)
				}
			}
		}
	}
	if attachment.ID == "" {
		t.Fatal("browser screenshot returned an image to the model but delivered no PNG attachment to the chat")
	}
	confirmation, ok := result.StructuredContent.(map[string]any)
	if !ok || confirmation["delivered"] != true || confirmation["message_id"] == "" {
		t.Fatalf("missing delivery confirmation: %#v", result.StructuredContent)
	}
	select {
	case event := <-events:
		if event.Type != im.EventTypeMessageCreated || event.RoomID != room.ID || event.Message == nil || len(event.Message.Attachments) != 1 {
			t.Fatalf("missing screenshot event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("screenshot was not announced to the UI")
	}
	root, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "Thread screenshot"})
	if err != nil {
		t.Fatal(err)
	}
	callTaskTool(t, client, "browser_screenshot", map[string]any{"room_id": room.ID, "browser_id": state.ID, "thread_root_id": root.ID}, nil)
	thread, err := h.im.GetThread(room.ID, root.ID)
	if err != nil || len(thread.Replies) != 1 || len(thread.Replies[0].Attachments) != 1 {
		t.Fatalf("screenshot was not delivered to its thread: %+v %v", thread, err)
	}
	callTaskTool(t, client, "browser_close", map[string]any{"room_id": room.ID, "browser_id": state.ID}, nil)
	h.im, err = im.NewServiceFromPath(statePath)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	h.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, attachment.DownloadURL, nil))
	if response.Code != http.StatusOK || !bytes.HasPrefix(response.Body.Bytes(), []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("screenshot download after close/reload: %d", response.Code)
	}
	image := result.Content[0].(*mcp.ImageContent)
	if !bytes.Equal(image.Data, response.Body.Bytes()) {
		t.Fatal("downloaded attachment differs from the captured screenshot")
	}
}

func TestBrowserScreenshotNativeThreadScope(t *testing.T) {
	h := newAppTaskTestHandler(t)
	args := map[string]any{"room_id": "forged", "thread_root_id": "forged"}
	if err := h.scopeAppPlatformRoomConversation("agent-dev", "browser_screenshot", "csgclaw-im:binding:room:room%2Fone:thread:root%2Fone", args); err != nil {
		t.Fatal(err)
	}
	if args["room_id"] != "room/one" || args["thread_root_id"] != "root/one" {
		t.Fatalf("native scope was not bound: %v", args)
	}
	if err := h.scopeAppPlatformRoomConversation("agent-dev", "browser_screenshot", "csgclaw-im:binding:room:room%2Ftwo", args); err != nil {
		t.Fatal(err)
	}
	if args["room_id"] != "room/two" || args["thread_root_id"] != "" {
		t.Fatalf("room turn retained another thread: %v", args)
	}
}
