package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/diagnostics"
	"csgclaw/internal/im"
	"csgclaw/internal/taskcore"
)

func TestDiagnosticBundleHTTPCombinesFullContextAndExactLogs(t *testing.T) {
	service := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Role: "admin"}}, Rooms: []im.Room{{ID: "room", Members: []string{im.AdminUserID}, Messages: []im.Message{{ID: "source", SenderID: im.AdminUserID, Content: "original user request"}, {ID: "unrelated", SenderID: im.AdminUserID, Content: "unrelated private message"}}}, {ID: "other", Members: []string{im.AdminUserID}}}})
	store := service.Diagnostics()
	store.Source("room", "source", time.Now())
	a := store.Begin("room", "source", "", "a", "turn-a")
	b := store.Begin("room", "source", "", "b", "turn-b")
	unrelated := store.Begin("room", "unrelated", "", "a", "unrelated-turn")
	a.Running()
	b.Running()
	unrelated.Running()
	long := strings.Repeat("full model context ", 1000) + "END"
	a.Capture("model.runtime_request", map[string]any{"body": map[string]any{"input": long, "api_key": "private-model-key"}})
	runtimePath := filepath.Join(t.TempDir(), "stderr.log")
	_ = os.WriteFile(runtimePath, []byte("turn_id=turn-a keep first\nturn_id=turn-ab must exclude\nturn_id=turn-b other turn\n[turn_id=turn-a] keep last\n"), 0600)
	a.Capture("runtime.configuration", map[string]string{"stderr_path": runtimePath})
	for _, record := range []*diagnostics.Record{a, b, unrelated} {
		record.Finish("succeeded")
	}
	journal := diagnostics.NewLogJournal(t.TempDir())
	restore := diagnostics.SetCurrentLogs(journal)
	defer restore()
	now := time.Now()
	journal.Add(diagnostics.LogEntry{At: now, TurnID: "turn-a", Message: "turn-a-log"})
	journal.Add(diagnostics.LogEntry{At: now, TurnID: "turn-b", Message: "turn-b-log"})
	journal.Add(diagnostics.LogEntry{At: now, TurnID: "unrelated-turn", Message: "unrelated-private-log"})
	journal.Add(diagnostics.LogEntry{At: now, Message: "nearby-system-log"})
	journal.Add(diagnostics.LogEntry{At: now.Add(-5 * time.Minute), Message: "distant-system-log"})
	server := httptest.NewServer((&Handler{im: service}).Routes())
	defer server.Close()
	path := server.URL + "/api/v1/rooms/room/diagnostics/" + a.Snapshot().ID + "/bundle?download=1&timezone=Asia%2FShanghai"
	response, err := http.Get(path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 {
		t.Fatalf("status=%d %s", response.StatusCode, body)
	}
	if response.Header.Get("Cache-Control") != "no-store" || !strings.Contains(response.Header.Get("Content-Disposition"), "csgclaw-turn-turn-a-diagnostics.json") {
		t.Fatal("missing export headers")
	}
	var bundle struct {
		Schema int    `json:"schema_version"`
		Turn   string `json:"turn_id"`
		Turns  []struct {
			Diagnostic diagnostics.Snapshot       `json:"diagnostic"`
			Context    []diagnostics.ContextEntry `json:"context"`
			Logs       []diagnostics.LogEntry     `json:"server_logs"`
		} `json:"turns"`
		Collection struct {
			Complete bool          `json:"complete"`
			Issues   []bundleIssue `json:"issues"`
		} `json:"collection"`
	}
	if err := json.Unmarshal(body, &bundle); err != nil {
		t.Fatalf("invalid JSON: %v %s", err, body)
	}
	if bundle.Schema != 1 || bundle.Turn != "turn-a" || len(bundle.Turns) != 2 {
		t.Fatalf("incorrect scope %+v", bundle)
	}
	for _, expected := range []string{long, "original user request", "turn-a-log", "turn-b-log", "nearby-system-log", "keep first", "keep last"} {
		if !strings.Contains(string(body), expected) {
			t.Fatalf("missing %q", expected[:min(50, len(expected))])
		}
	}
	for _, forbidden := range []string{"private-model-key", "unrelated private message", "unrelated-private-log", "distant-system-log", "must exclude"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("unexpected %q", forbidden)
		}
	}
	if bundle.Collection.Complete || len(bundle.Collection.Issues) == 0 {
		t.Fatal("missing runtime evidence was not disclosed")
	}
	for _, test := range []struct {
		room, caller string
		status       int
	}{{"other", "", 404}, {"room", "agent-a", 403}} {
		req, _ := http.NewRequest("GET", server.URL+"/api/v1/rooms/"+test.room+"/diagnostics/"+a.Snapshot().ID+"/bundle", nil)
		req.Header.Set("X-CSGClaw-Caller-Agent", test.caller)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != test.status {
			t.Fatalf("scope=%d want %d", resp.StatusCode, test.status)
		}
	}
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDiagnosticRuntimeLogFilterDoesNotMatchTurnIDPrefixes(t *testing.T) {
	lines := []string{}
	writer := diagnosticRuntimeLogWriter{turn: diagnostics.Snapshot{TurnID: "turn-1", RuntimeTurnID: "native-1"}, emit: func(line string) error { lines = append(lines, line); return nil }}
	for _, text := range []string{"turn_id=turn-10 wrong\n", `{"turn_id":"native-10"}` + "\n", "[turn_id=turn-1] correct\n", `{"turn_id":"native-1"}` + "\n"} {
		_, _ = writer.Write([]byte(text))
	}
	writer.finish()
	if len(lines) != 2 {
		t.Fatalf("matched wrong identities: %+v", lines)
	}
}

func TestDiagnosticBundleFollowsExplicitTaskRelations(t *testing.T) {
	service := im.NewService()
	core := taskcore.NewService()
	root, err := core.CreateRoot(taskcore.CreateRootInput{AssignmentType: taskcore.AssignmentTypeRoom, AssignmentID: "room", RoomID: "room", Title: "Main task", CreatedBy: "manager", SourceMessageID: "user-source"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := core.CreateChild(taskcore.CreateChildInput{ParentID: root.ID, Title: "Worker task", CreatedBy: "manager", AssignedTo: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := core.CreateRoot(taskcore.CreateRootInput{AssignmentType: taskcore.AssignmentTypeRoom, AssignmentID: "room", RoomID: "room", Title: "Unrelated task", CreatedBy: "manager"})
	if err != nil {
		t.Fatal(err)
	}
	store := service.Diagnostics()
	manager := store.Begin("room", "user-source", "", "manager", "manager-turn")
	worker := store.Begin("room", "worker-notice", "", "worker", "worker-turn")
	worker.Task(child.ID, 1)
	unrelated := store.Begin("room", "other-notice", "", "worker", "other-turn")
	unrelated.Task(other.ID, 1)
	h := &Handler{im: service}
	h.SetRoomTaskCore(core)
	turns, tasks := h.bundleRelated(manager.Snapshot(), nil)
	if len(turns) != 2 || len(tasks) != 2 {
		t.Fatalf("turns=%+v tasks=%+v", turns, tasks)
	}
	for _, turn := range turns {
		if turn.TurnID == "other-turn" {
			t.Fatal("included unrelated task for the same worker")
		}
	}
	snapshot, ok := h.roomTaskSvc.DiagnosticSnapshot("room", child.ID)
	if !ok || snapshot.Root.ID != root.ID || len(snapshot.Children) != 1 || len(snapshot.Events) == 0 {
		t.Fatalf("missing task evidence: %+v", snapshot)
	}
	if _, ok := h.roomTaskSvc.DiagnosticSnapshot("other-room", child.ID); ok {
		t.Fatal("cross-room task aggregate exposed")
	}
}
