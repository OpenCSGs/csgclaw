package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/im"
)

func TestAgentSessionResponsesAcceptsInlineFiles(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			contents := [][]byte{[]byte("report!"), {0, 1, 2, 3, 4}}
			names := []string{"report.pdf", "data.bin"}
			mediaTypes := []string{"application/pdf", "application/octet-stream"}
			engine := &fakeSessionEngine{}
			handler, _, bindings, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
			handler.SetAgentEngine(sessionInputOnlyEngine{Interface: engine, t: t}, bindings)
			workspace := sessionTestWorkspace(t, handler)
			engine.run = func(ctx context.Context, agentID string, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
				if !strings.Contains(sessionTurnText(request), "Read both files.") {
					t.Errorf("prompt = %q", sessionTurnText(request))
				}
				checkSessionWorkerFiles(t, workspace, request, []sessionUploadFixture{
					{names[0], mediaTypes[0], string(contents[0])},
					{names[1], mediaTypes[1], string(contents[1])},
				})
				if sink != nil {
					if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "file answer"}); err != nil {
						t.Fatal(err)
					}
				}
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "file answer"}
			}
			recorder := performAgentSessionRequest(t, handler, "agent-alpha", "inline-files", sessionInlineFileRequest(stream,
				map[string]any{"type": "input_text", "text": "Read both files."},
				sessionInlineFilePart(names[0], mediaTypes[0], contents[0]),
				sessionInlineFilePart(names[1], mediaTypes[1], contents[1]),
			))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if stream {
				if !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/event-stream") {
					t.Fatalf("content type = %q", recorder.Header().Get("Content-Type"))
				}
				for _, want := range []string{"event: message_start", "file answer", "event: message_stop"} {
					if !strings.Contains(recorder.Body.String(), want) {
						t.Errorf("SSE response missing %q: %s", want, recorder.Body.String())
					}
				}
			} else {
				var response agentSessionResponse
				if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.Status != "completed" || len(response.Output) != 1 || len(response.Output[0].Content) != 1 ||
					response.Output[0].Content[0].Text != "file answer" {
					t.Fatalf("response = %+v", response)
				}
			}
		})
	}
}

func TestAgentSessionResponsesAcceptsFileOnlyInput(t *testing.T) {
	const name = "note.txt"
	content := bytes.Repeat([]byte("a"), (1<<20)+1)
	engine := &fakeSessionEngine{}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	workspace := sessionTestWorkspace(t, handler)
	engine.run = func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		checkSessionWorkerFiles(t, workspace, request, []sessionUploadFixture{{name, "text/plain", string(content)}})
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "received file"}
	}
	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "file-only", sessionInlineFileRequest(false,
		sessionInlineFilePart(name, "text/plain", content),
	))
	if recorder.Code != http.StatusOK || len(engine.client.Calls()) != 1 {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentSessionResponsesRejectsInvalidInlineFiles(t *testing.T) {
	for _, test := range []struct {
		name string
		part map[string]any
	}{
		{name: "invalid base64", part: map[string]any{"type": "input_file", "filename": "note.txt", "file_data": "data:text/plain;base64,%%%"}},
		{name: "empty file", part: sessionInlineFilePart("note.txt", "text/plain", nil)},
		{name: "missing media type", part: map[string]any{"type": "input_file", "filename": "note.txt", "file_data": "data:;base64,YQ=="}},
		{name: "missing base64 marker", part: map[string]any{"type": "input_file", "filename": "note.txt", "file_data": "data:text/plain,YQ=="}},
		{name: "raw base64", part: map[string]any{"type": "input_file", "filename": "note.txt", "file_data": "YQ=="}},
		{name: "remote URL", part: map[string]any{"type": "input_file", "filename": "note.txt", "file_data": "https://example.com/note.txt"}},
		{name: "missing filename", part: map[string]any{"type": "input_file", "file_data": "data:text/plain;base64,YQ=="}},
		{name: "filename path", part: sessionInlineFilePart("../note.txt", "text/plain", []byte("a"))},
		{name: "client workspace path", part: map[string]any{"type": "workspace_file", "path": "private.txt"}},
		{name: "file with text", part: map[string]any{"type": "input_file", "filename": "note.txt", "file_data": "data:text/plain;base64,YQ==", "text": "unexpected"}},
		{name: "text with file", part: map[string]any{"type": "input_text", "text": "hello", "filename": "note.txt", "file_data": "data:text/plain;base64,YQ=="}},
		{name: "unknown field", part: map[string]any{"type": "input_file", "filename": "note.txt", "file_data": "data:text/plain;base64,YQ==", "future": true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := &fakeSessionEngine{}
			handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
			recorder := performAgentSessionRequest(t, handler, "agent-alpha", "invalid-file", sessionInlineFileRequest(true, test.part))
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid_request") {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/event-stream") {
				t.Fatal("invalid file started an SSE response")
			}
			if calls := engine.client.Calls(); len(calls) != 0 {
				t.Fatalf("invalid file dispatched %d turns", len(calls))
			}
		})
	}
}

func TestAgentSessionResponsesLimitsInlineFileCountAndText(t *testing.T) {
	files := make([]map[string]any, 11)
	for index := range files {
		files[index] = sessionInlineFilePart("note.txt", "text/plain", []byte("a"))
	}
	for _, test := range []struct {
		name  string
		parts []map[string]any
	}{
		{name: "too many files", parts: files},
		{name: "text limit with file", parts: []map[string]any{
			{"type": "input_text", "text": strings.Repeat("x", (1<<20)+1)},
			sessionInlineFilePart("note.txt", "text/plain", []byte("a")),
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := &fakeSessionEngine{}
			handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
			recorder := performAgentSessionRequest(t, handler, "agent-alpha", "limits", sessionInlineFileRequest(false, test.parts...))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if calls := engine.client.Calls(); len(calls) != 0 {
				t.Fatalf("invalid request dispatched %d turns", len(calls))
			}
		})
	}
}

func TestAgentSessionResponsesReportsOversizedBodyAsInvalidRequest(t *testing.T) {
	engine := &fakeSessionEngine{}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-alpha/sessions/oversized-body/responses", nil)
	request.Body = sessionOversizedRequestBody{}
	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response agentSessionErrorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != "invalid_request" || response.Error.Type != "invalid_request_error" {
		t.Fatalf("error = %+v", response.Error)
	}
	if calls := engine.client.Calls(); len(calls) != 0 {
		t.Fatalf("oversized request dispatched %d turns", len(calls))
	}
}

func TestAgentSessionResponsesRetainsInlineFilesAfterFailure(t *testing.T) {
	engine := &fakeSessionEngine{run: func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: "runtime failed"}}
	}}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "failed-files", sessionInlineFileRequest(false,
		sessionInlineFilePart("note.txt", "text/plain", []byte("a")),
	))
	if recorder.Code != http.StatusServiceUnavailable || len(engine.client.Calls()) != 1 {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	checkSessionWorkerFiles(t, sessionTestWorkspace(t, handler), engine.client.Calls()[0].Request, []sessionUploadFixture{{"note.txt", "text/plain", "a"}})
}

func TestAgentSessionInputFileSizeLimits(t *testing.T) {
	for _, test := range []struct {
		name     string
		sizes    []int64
		existing []int64
		large    bool
	}{
		{name: "single file at limit", sizes: []int64{im.MaxAttachmentFileBytes}},
		{name: "single file above limit", sizes: []int64{im.MaxAttachmentFileBytes + 1}, large: true},
		{name: "total at limit", sizes: []int64{im.MaxAttachmentFileBytes, im.MaxAttachmentFileBytes}},
		{name: "total above limit", sizes: []int64{im.MaxAttachmentFileBytes, im.MaxAttachmentFileBytes, 1}, large: true},
		{name: "referenced file above limit", existing: []int64{im.MaxAttachmentFileBytes + 1}, large: true},
		{name: "uploads and references above total limit", existing: []int64{im.MaxAttachmentFileBytes}, sizes: []int64{im.MaxAttachmentFileBytes, 1}, large: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := agentSessionInput{}
			for _, size := range test.sizes {
				input.Files = append(input.Files, agentSessionInputFile{SizeBytes: size})
			}
			var references []im.AttachmentFile
			for _, size := range test.existing {
				references = append(references, im.AttachmentFile{Attachment: im.MessageAttachment{SizeBytes: size}})
			}
			err := input.validateFileLimits(references)
			if test.large {
				if !errors.Is(err, errAttachmentPayloadTooLarge) {
					t.Fatalf("error = %v, want attachment payload too large", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAgentSessionResponsesRetainsInlineFilesAfterCancellation(t *testing.T) {
	started := make(chan struct{})
	done := make(chan struct{})
	engine := &fakeSessionEngine{run: func(ctx context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		close(started)
		<-ctx.Done()
		return agentengine.TurnResult{Status: agentengine.TurnCanceled, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: ctx.Err().Error()}}
	}}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	body, err := json.Marshal(sessionInlineFileRequest(false, sessionInlineFilePart("note.txt", "text/plain", []byte("a"))))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-alpha/sessions/canceled-files/responses", bytes.NewReader(body)).WithContext(ctx)
	recorder := httptest.NewRecorder()
	go func() {
		handler.Routes().ServeHTTP(recorder, request)
		close(done)
	}()
	select {
	case <-started:
	case <-done:
		t.Fatalf("request returned before Run: %d %s", recorder.Code, recorder.Body.String())
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled request did not finish")
	}
	calls := engine.client.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected one canceled turn, got %d", len(calls))
	}
	checkSessionWorkerFiles(t, sessionTestWorkspace(t, handler), calls[0].Request, []sessionUploadFixture{{"note.txt", "text/plain", "a"}})
}

func TestAgentSessionResponsesRejectsInlineBatchBeforeSaving(t *testing.T) {
	engine := &fakeSessionEngine{}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "partial-files", sessionInlineFileRequest(false,
		sessionInlineFilePart("valid.txt", "text/plain", []byte("a")),
		sessionInlineFilePart("../invalid.txt", "text/plain", []byte("b")),
	))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if calls := engine.client.Calls(); len(calls) != 0 {
		t.Fatalf("partial upload dispatched %d turns", len(calls))
	}
	_, err := os.Stat(filepath.Join(sessionTestWorkspace(t, handler), ".csgclaw", "attachments"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid batch created a workspace attachment directory: %v", err)
	}
}

func sessionInlineFilePart(name, mediaType string, content []byte) map[string]any {
	return map[string]any{
		"type": "input_file", "filename": name,
		"file_data": "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(content),
	}
}

func sessionInlineFileRequest(stream bool, parts ...map[string]any) map[string]any {
	return map[string]any{
		"input":  []map[string]any{{"role": "user", "content": parts}},
		"stream": stream,
	}
}

type sessionOversizedRequestBody struct{}

func (sessionOversizedRequestBody) Read([]byte) (int, error) {
	return 0, &http.MaxBytesError{Limit: agentSessionFileBodyLimit}
}

func (sessionOversizedRequestBody) Close() error { return nil }

type sessionInputOnlyEngine struct {
	agentengine.Interface
	t *testing.T
}

func (e sessionInputOnlyEngine) Conversations(agentID string) agentengine.ConversationInterface {
	return sessionInputOnlyConversation{ConversationInterface: e.Interface.Conversations(agentID), t: e.t}
}

type sessionInputOnlyConversation struct {
	agentengine.ConversationInterface
	t *testing.T
}

func (c sessionInputOnlyConversation) Files() agentengine.FileInterface {
	c.t.Error("an input-only turn accessed temporary Engine file storage")
	return nil
}
