package api

import (
	"csgclaw/internal/diagnostics"
	"csgclaw/internal/im"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticRoutesScopePaginationAndTiming(t *testing.T) {
	svc := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Role: "admin"}}, Rooms: []im.Room{{ID: "room", Members: []string{im.AdminUserID}}, {ID: "other", Members: []string{im.AdminUserID}}}})
	s := svc.Diagnostics()
	s.Source("room", "source", time.Now())
	a := s.Begin("room", "source", "", "agent-a", "turn-a")
	a.Running()
	a.RuntimeStart()
	a.RuntimeEndAt(time.Now())
	a.Finish("succeeded")
	b := s.Begin("room", "source", "thread", "agent-b", "turn-b")
	b.Running()
	b.Failure("runtime_failed", "runtime", "api_key=hidden")
	b.Finish("failed")
	routes := (&Handler{im: svc}).Routes()
	call := func(method, path, body, caller string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-CSGClaw-Caller-Agent", caller)
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, r)
		return w
	}
	w := call("GET", "/api/v1/rooms/room/diagnostics?limit=1", "", "")
	if w.Code != 200 {
		t.Fatalf("list=%d %s", w.Code, w.Body.String())
	}
	var page struct {
		Items []diagnostics.Snapshot `json:"items"`
		Next  string                 `json:"next_cursor"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	if len(page.Items) != 1 || page.Next == "" || len(page.Items[0].Spans) != 0 {
		t.Fatalf("page=%+v", page)
	}
	w = call("GET", "/api/v1/rooms/room/diagnostics?limit=1&cursor="+page.Next, "", "")
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	if len(page.Items) != 1 || page.Next != "" {
		t.Fatalf("next=%s", w.Body.String())
	}
	w = call("GET", "/api/v1/rooms/other/diagnostics/"+a.Snapshot().ID, "", "")
	if w.Code != 404 {
		t.Fatalf("cross-room=%d", w.Code)
	}
	w = call("GET", "/api/v1/rooms/room/diagnostics/"+b.Snapshot().ID, "", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "hidden") {
		t.Fatalf("detail=%s", w.Body.String())
	}
	w = call("GET", "/api/v1/rooms/room/diagnostics", "", "agent-a")
	if w.Code != http.StatusForbidden {
		t.Fatal("runtime caller got diagnostics")
	}
	w = call("POST", "/api/v1/rooms/room/diagnostics/timings", `{"source_id":"source","turn_id":"turn-a","first_ms":100,"complete_ms":200}`, "")
	if w.Code != 204 {
		t.Fatalf("timing=%d %s", w.Code, w.Body.String())
	}
	_ = call("POST", "/api/v1/rooms/room/diagnostics/timings", `{"source_id":"source","turn_id":"turn-a","first_ms":110}`, "")
	if a.Snapshot().Browser.CompleteMS == nil || *a.Snapshot().Browser.CompleteMS != 200 {
		t.Fatal("late first-frame report erased completion")
	}
	if a.Snapshot().Browser == nil || b.Snapshot().Browser != nil {
		t.Fatal("browser timing leaked to another agent")
	}

	w = call("POST", "/api/v1/rooms/room/diagnostics/timings", `{"source_id":"source","turn_id":"turn-a","first_text_ms":150,"first_text_at":"2026-09-29T04:00:00.150Z"}`, "")
	if w.Code != 204 {
		t.Fatal(w.Body.String())
	}
	_ = call("POST", "/api/v1/rooms/room/diagnostics/timings", `{"source_id":"source","turn_id":"turn-a","first_text_ms":180,"first_text_at":"2026-09-29T04:00:00.180Z"}`, "")
	measured := a.Snapshot().Browser
	if measured.FirstTextMS == nil || *measured.FirstTextMS != 150 || measured.FirstTextAt == nil || measured.FirstTextAt.Nanosecond() != 150000000 || measured.CompleteMS == nil {
		t.Fatalf("first text observation was overwritten: %+v", measured)
	}
	w = call("POST", "/api/v1/rooms/room/diagnostics/timings", `{"source_id":"source","turn_id":"turn-a","first_text_ms":200}`, "")
	if w.Code != 400 {
		t.Fatal("unpaired browser timestamp accepted")
	}
	w = call("POST", "/api/v1/rooms/room/diagnostics/timings", `{"source_id":"source","first_ms":200,"complete_ms":100}`, "")
	if w.Code != 400 {
		t.Fatal("invalid timing accepted")
	}
	if _, err := svc.ClearRoomMessages("room"); err != nil {
		t.Fatal(err)
	}
	if len(s.List("room", "", "", "", "")) != 0 {
		t.Fatal("clear messages retained diagnostics")
	}
}

func TestLLMDiagnosticRequiresExactNativeIdentity(t *testing.T) {
	service := im.NewService()
	store := service.Diagnostics()
	record := store.Begin("room", "source", "", "agent-a", "turn")
	record.Running()
	record.RuntimeStart()
	record.RuntimeRef("session", "native-turn", "")
	handler := &Handler{im: service}
	for _, tc := range []struct {
		agent, header string
		want          bool
	}{
		{"agent-a", `{"thread_id":"session","turn_id":"native-turn"}`, true},
		{"agent-b", `{"thread_id":"session","turn_id":"native-turn"}`, false},
		{"agent-a", `{"thread_id":"session","turn_id":"previous-turn"}`, false},
		{"agent-a", `{"thread_id":"session"}`, false},
		{"agent-a", "", false},
	} {
		req := httptest.NewRequest("POST", "/responses", nil)
		req.Header.Set("X-Codex-Turn-Metadata", tc.header)
		got := diagnostics.Resolve(handler.withLLMDiagnostic(req, tc.agent).Context())
		if (got != nil) != tc.want {
			t.Fatalf("identity %q %q = %v", tc.agent, tc.header, got != nil)
		}
	}
}

func TestDiagnosticRecoversHistoricalToolFieldsFromExactTranscript(t *testing.T) {
	service := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Role: "admin"}}, Rooms: []im.Room{{ID: "room", Members: []string{im.AdminUserID}, Messages: []im.Message{{ID: "turn-final", SenderID: im.AdminUserID, Content: "done", Metadata: map[string]any{"csgclaw": map[string]any{"turn_progress": map[string]any{"id": "turn", "items": []any{map[string]any{"id": "call", "kind": "tool", "tool": map[string]any{"name": "Read file", "command": "cat demo.txt", "cwd": "/workspace", "output": "private-output"}}}}}}}}}}})
	handler := &Handler{im: service}
	record := diagnostics.Snapshot{RoomID: "room", TurnID: "turn", Spans: []diagnostics.Span{{ID: "tool:call", Owner: "tool", Name: "tool.exec_command"}}}
	handler.enrichDiagnosticTools(&record)
	details := record.Spans[0].Details
	if details == nil || details.Command != "cat demo.txt" || details.Directory != "/workspace" {
		t.Fatalf("historical details=%+v", details)
	}
	raw, _ := json.Marshal(record)
	if strings.Contains(string(raw), "private-output") {
		t.Fatal("tool output was copied into diagnostics")
	}
}
