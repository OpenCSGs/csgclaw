package api

import (
	"context"
	"encoding/json"
	"fmt"
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
)

func TestDiagnosticRuntimeBundleHTTPIncludesProcessFailuresAndSectionCoverage(t *testing.T) {
	service := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Role: "admin"}}, Rooms: []im.Room{{ID: "room", Members: []string{im.AdminUserID}}}})
	store := service.Diagnostics()
	store.Source("room", "source", time.Now())
	record := store.Begin("room", "source", "", "a", "turn")
	record.Running()
	record.Capture("execution.request", map[string]any{"input": []any{}})
	path := filepath.Join(t.TempDir(), "stderr.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	observer := diagnostics.NewRuntimeObserver(path, "runtime", os.Getpid(), 0, file)
	ctx := diagnostics.WithRecord(context.Background(), record)
	observer.Bind(ctx)
	fmt.Fprintf(file, "%s ERROR native MCP connection reset\n", time.Now().UTC().Format(time.RFC3339Nano))
	fmt.Fprintf(file, "%s turn_id=other-turn secret other execution\n", time.Now().UTC().Format(time.RFC3339Nano))
	observer.Observe("stdout_decode_failed", fmt.Errorf("malformed protocol response"), nil)
	if _, err := service.DeliverMessageContext(ctx, im.DeliverMessageRequest{RoomID: "room", SenderID: im.AdminUserID, MessageID: "turn-final", Content: "Runtime failed"}); err != nil {
		t.Fatal(err)
	}
	observer.End(ctx)
	record.Capture("execution.result", map[string]any{"status": "failed"})
	record.Finish("failed")
	observer.Exit(2, fmt.Errorf("exit status 2"), nil)
	logs := diagnostics.NewLogJournal(t.TempDir())
	restore := diagnostics.SetCurrentLogs(logs)
	defer restore()
	logs.Add(diagnostics.LogEntry{At: time.Now(), TurnID: "turn", Message: "turn failed"})
	server := httptest.NewServer((&Handler{im: service}).Routes())
	defer server.Close()
	response, err := http.Get(server.URL + "/api/v1/rooms/room/diagnostics/" + record.Snapshot().ID + "/bundle")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	var bundle struct {
		Collection struct {
			Complete bool                 `json:"complete"`
			Sections []diagnosticCoverage `json:"sections"`
		} `json:"collection"`
	}
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"native MCP connection reset", "runtime_process_window", "stdout_decode_failed", "process_exit", "im.lock_wait", "im.lock_hold"} {
		if !strings.Contains(string(raw), text) {
			t.Fatalf("missing %s", text)
		}
	}
	if strings.Contains(string(raw), "secret other execution") {
		t.Fatal("another turn's log was exported")
	}
	sections := map[string]diagnosticCoverage{}
	for _, section := range bundle.Collection.Sections {
		sections[section.Section] = section
	}
	for _, name := range []string{"execution_context", "performance", "runtime_logs", "runtime_process_events"} {
		if sections[name].Status != "complete" {
			t.Fatalf("section %s: %+v", name, sections[name])
		}
	}
	if bundle.Collection.Complete || sections["system_logs"].Status != "partial" {
		t.Fatal("collector startup gap was not disclosed")
	}
}
