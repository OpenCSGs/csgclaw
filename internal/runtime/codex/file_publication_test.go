package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/diagnostics"
	"encoding/json"
)

func TestFilePublicationFailureKeepsExactTurnAndOSCause(t *testing.T) {
	store := diagnostics.New(t.TempDir())
	record := store.Begin("r", "s", "", "a", "turn")
	record.Running()
	adapter := &ConversationAdapter{runtimeID: "runtime", runtime: inputLifecycleBackend{workspace: t.TempDir()}}
	_, failure := adapter.authorizeOutputFile(diagnostics.WithRecord(context.Background(), record), activity.RuntimeFile{Path: "missing.html"})
	if failure == nil {
		t.Fatal("missing file unexpectedly published")
	}
	var detail map[string]any
	if err := store.ReadContext(context.Background(), record.Snapshot().ID, func(entry diagnostics.ContextEntry) error {
		if entry.Kind == "execution.failure" {
			return json.Unmarshal(entry.Data, &detail)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if detail["stage"] != "file.publish.open_source" || detail["error_kind"] != "not_found" || detail["runtime_id"] != "runtime" {
		t.Fatalf("failure detail=%+v", detail)
	}
	if !strings.Contains(detail["path"].(string), "missing.html") {
		t.Fatal("failed path lost")
	}
	if record.Snapshot().Error != nil {
		t.Fatal("recoverable publication error changed the turn result")
	}
}

// Manager-only fixtures still use the real Adapter authorization and snapshot
// code, then dispose of the authorized file after acknowledging the tool.
type filePublicationTestSink struct {
	recordingSink
	adapter *ConversationAdapter
}

func newFilePublicationTestSink(t *testing.T, workspace, path string) *filePublicationTestSink {
	t.Helper()
	path = filepath.Join(workspace, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	content := "fixture file"
	if filepath.Ext(path) == ".pdf" {
		content = "%PDF-1.7\nfixture file"
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return &filePublicationTestSink{adapter: &ConversationAdapter{runtime: inputLifecycleBackend{workspace: workspace}}}
}

func (s *filePublicationTestSink) Publish(event SessionEvent) {
	if publication, ok := event.Payload.(*filePublication); ok {
		var output conversationText
		var files []*contract.OutputFile
		s.adapter.handleEvent(publication.ctx, contract.TurnRequest{}, nil, event, &output, &files)
		cleanupOutputFiles(files)
		event.Payload = publication.file
	}
	s.recordingSink.Publish(event)
}

func activateFilePublicationTestTurn(t *testing.T, live *liveSession, thread, turn string) context.CancelFunc {
	t.Helper()
	waiter, err := live.registerAppServerTurnWaiter(thread)
	if err != nil {
		t.Fatal(err)
	}
	waiter.setTurnID(turn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	live.setAppServerTurnContext(thread, waiter, ctx)
	return cancel
}

func TestFilePublicationCancellationReleasesToolAndRejectsLateDelivery(t *testing.T) {
	spec := testAppServerSessionSpec(t.TempDir())
	sink := &recordingSink{}
	manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
	live := &liveSession{spec: spec, filePublishingThreads: map[string]bool{"thread": true}}
	cancel := activateFilePublicationTestTurn(t, live, "thread", "turn")
	finished := make(chan any, 1)
	go func() {
		response, _ := manager.handleAppServerServerRequest(spec.RuntimeID, live, appServerServerRequest{
			Method: "item/tool/call",
			Params: mustJSONRaw(t, map[string]any{"threadId": "thread", "turnId": "turn", "callId": "file", "tool": appServerPublishFileToolName, "arguments": map[string]any{"path": "game.html"}}),
		})
		finished <- response
	}()
	waitForRuntime(t, func() bool { return len(sink.snapshot()) == 1 })
	select {
	case <-finished:
		t.Fatal("tool reported success before the Adapter snapshotted the file")
	default:
	}
	cancel()
	select {
	case response := <-finished:
		if response.(map[string]any)["success"] != false {
			t.Fatalf("canceled response = %#v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("tool did not stop after turn cancellation")
	}
	adapter := &ConversationAdapter{runtime: inputLifecycleBackend{workspace: spec.WorkspaceDir}}
	var output conversationText
	var files []*contract.OutputFile
	if result := adapter.handleEvent(context.Background(), contract.TurnRequest{}, nil, sink.snapshot()[0], &output, &files); result != nil || len(files) != 0 {
		t.Fatalf("late publication affected another turn: result=%+v files=%d", result, len(files))
	}
}

func TestFilePublicationRequiresActiveTurn(t *testing.T) {
	sink := &recordingSink{}
	manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
	live := &liveSession{filePublishingThreads: map[string]bool{"thread": true}}
	response, err := manager.handleAppServerServerRequest("runtime", live, appServerServerRequest{
		Method: "item/tool/call",
		Params: mustJSONRaw(t, map[string]any{"threadId": "thread", "turnId": "turn", "callId": "file", "tool": appServerPublishFileToolName, "arguments": map[string]any{"path": "game.html"}}),
	})
	if err != nil || response.(map[string]any)["success"] != false || len(sink.snapshot()) != 0 || !strings.Contains(response.(map[string]any)["contentItems"].([]map[string]any)[0]["text"].(string), "active turn") {
		t.Fatalf("response=%#v error=%v events=%d", response, err, len(sink.snapshot()))
	}
}

func TestFilePublicationFailurePreservesPreviouslyAcknowledgedFiles(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "game.html"), []byte("<!doctype html><title>Tank game</title>"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := &ConversationAdapter{runtime: inputLifecycleBackend{workspace: workspace}}
	var output conversationText
	var files []*contract.OutputFile
	defer func() { cleanupOutputFiles(files) }()
	publish := func(path string) error {
		publication := &filePublication{file: activity.RuntimeFile{Path: path}, ctx: context.Background(), done: make(chan error, 1)}
		if result := adapter.handleEvent(context.Background(), contract.TurnRequest{}, nil, SessionEvent{Kind: SessionEventFileOutput, Payload: publication}, &output, &files); result != nil {
			t.Fatalf("publication aborted the whole turn: %+v", result)
		}
		return <-publication.done
	}
	if err := publish("game.html"); err != nil || len(files) != 1 {
		t.Fatalf("valid publication: files=%d error=%v", len(files), err)
	}
	for _, path := range []string{"missing.html", "."} {
		if err := publish(path); err == nil || len(files) != 1 || strings.Contains(err.Error(), workspace) {
			t.Fatalf("invalid publication: path=%s files=%d error=%v", path, len(files), err)
		}
	}
	if err := os.Remove(filepath.Join(workspace, "game.html")); err != nil {
		t.Fatal(err)
	}
	reader, err := files[0].Open(context.Background())
	if err != nil {
		t.Fatal("previously acknowledged file was lost:", err)
	}
	reader.Close()
}
