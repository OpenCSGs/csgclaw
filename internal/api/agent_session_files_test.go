package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
			names := []string{"report.pdf", "diagram.png"}
			mediaTypes := []string{"application/pdf", "image/png"}
			var fileIDs []string
			engine := &fakeSessionEngine{}
			engine.run = func(ctx context.Context, agentID string, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
				if !strings.Contains(sessionTurnText(request), "Read both files.") {
					t.Errorf("prompt = %q", sessionTurnText(request))
				}
				for _, part := range request.Input {
					if part.Kind != agentengine.InputPartFile {
						continue
					}
					if part.File == nil || len(fileIDs) >= len(contents) {
						t.Fatalf("unexpected file input: %+v", part)
					}
					index := len(fileIDs)
					fileIDs = append(fileIDs, part.File.ID)
					file, err := engine.client.Conversations(agentID).Files().Get(ctx, part.File.ID)
					if err != nil {
						t.Fatal(err)
					}
					data, readErr := io.ReadAll(file.Content)
					closeErr := file.Content.Close()
					if readErr != nil || closeErr != nil {
						t.Fatalf("read file: %v; close: %v", readErr, closeErr)
					}
					if !bytes.Equal(data, contents[index]) {
						t.Errorf("file content = %v, want %v", data, contents[index])
					}
					digest := sha256.Sum256(contents[index])
					if file.Metadata.Name != names[index] || file.Metadata.MediaType != mediaTypes[index] ||
						file.Metadata.SizeBytes != int64(len(contents[index])) || file.Metadata.SHA256 != hex.EncodeToString(digest[:]) {
						t.Errorf("file metadata = %+v", file.Metadata)
					}
				}
				if len(fileIDs) != len(contents) {
					t.Errorf("file count = %d, want %d", len(fileIDs), len(contents))
				}
				if sink != nil {
					if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "file answer"}); err != nil {
						t.Fatal(err)
					}
				}
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "file answer"}
			}
			handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
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
			assertSessionInputFilesDeleted(t, engine.client.Conversations("agent-alpha").Files(), fileIDs)
		})
	}
}

func TestAgentSessionResponsesAcceptsFileOnlyInput(t *testing.T) {
	var fileIDs []string
	engine := &fakeSessionEngine{run: func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		for _, part := range request.Input {
			if part.Kind == agentengine.InputPartFile && part.File != nil {
				fileIDs = append(fileIDs, part.File.ID)
			}
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "received file"}
	}}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "file-only", sessionInlineFileRequest(false,
		sessionInlineFilePart("note.txt", "text/plain", bytes.Repeat([]byte("a"), (1<<20)+1)),
	))
	if recorder.Code != http.StatusOK || len(fileIDs) != 1 {
		t.Fatalf("status = %d, file IDs = %v, body = %s", recorder.Code, fileIDs, recorder.Body.String())
	}
	assertSessionInputFilesDeleted(t, engine.client.Conversations("agent-alpha").Files(), fileIDs)
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

func TestAgentSessionResponsesCleansInlineFilesAfterFailure(t *testing.T) {
	var fileIDs []string
	engine := &fakeSessionEngine{run: func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		for _, part := range request.Input {
			if part.Kind == agentengine.InputPartFile && part.File != nil {
				fileIDs = append(fileIDs, part.File.ID)
			}
		}
		return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: "runtime failed"}}
	}}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "failed-files", sessionInlineFileRequest(false,
		sessionInlineFilePart("note.txt", "text/plain", []byte("a")),
	))
	if recorder.Code != http.StatusServiceUnavailable || len(fileIDs) != 1 {
		t.Fatalf("status = %d, file IDs = %v, body = %s", recorder.Code, fileIDs, recorder.Body.String())
	}
	assertSessionInputFilesDeleted(t, engine.client.Conversations("agent-alpha").Files(), fileIDs)
}

func TestAgentSessionInputFileSizeLimits(t *testing.T) {
	for _, test := range []struct {
		name  string
		sizes []int64
		large bool
	}{
		{name: "single file at limit", sizes: []int64{im.MaxAttachmentFileBytes}},
		{name: "single file above limit", sizes: []int64{im.MaxAttachmentFileBytes + 1}, large: true},
		{name: "total at limit", sizes: []int64{im.MaxAttachmentFileBytes, im.MaxAttachmentFileBytes}},
		{name: "total above limit", sizes: []int64{im.MaxAttachmentFileBytes, im.MaxAttachmentFileBytes, 1}, large: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := agentSessionInput{}
			for _, size := range test.sizes {
				input.Files = append(input.Files, agentSessionInputFile{SizeBytes: size})
			}
			err := input.validateFileLimits()
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

func TestAgentSessionResponsesCleansInlineFilesAfterCancellation(t *testing.T) {
	started := make(chan struct{})
	done := make(chan struct{})
	var fileIDs []string
	engine := &fakeSessionEngine{run: func(ctx context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		for _, part := range request.Input {
			if part.Kind == agentengine.InputPartFile && part.File != nil {
				fileIDs = append(fileIDs, part.File.ID)
			}
		}
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
	if len(fileIDs) != 1 {
		t.Fatalf("file IDs = %v", fileIDs)
	}
	assertSessionInputFilesDeleted(t, engine.client.Conversations("agent-alpha").Files(), fileIDs)
}

func TestAgentSessionResponsesCleansPartiallyCreatedInlineFiles(t *testing.T) {
	engine := &fakeSessionEngine{}
	handler, _, bindings, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	files := &recordingSessionInputFiles{FileInterface: engine.client.Conversations("agent-alpha").Files()}
	handler.SetAgentEngine(sessionFileRecordingEngine{Interface: engine, files: files}, bindings)
	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "partial-files", sessionInlineFileRequest(false,
		sessionInlineFilePart("valid.txt", "text/plain", []byte("a")),
		sessionInlineFilePart("../invalid.txt", "text/plain", []byte("b")),
	))
	if recorder.Code != http.StatusBadRequest || len(files.created) != 1 {
		t.Fatalf("status = %d, created = %v, body = %s", recorder.Code, files.created, recorder.Body.String())
	}
	if calls := engine.client.Calls(); len(calls) != 0 {
		t.Fatalf("partial upload dispatched %d turns", len(calls))
	}
	assertSessionInputFilesDeleted(t, files, files.created)
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

func assertSessionInputFilesDeleted(t *testing.T, files agentengine.FileInterface, ids []string) {
	t.Helper()
	for _, id := range ids {
		file, err := files.Get(context.Background(), id)
		if err == nil {
			_ = file.Content.Close()
			t.Errorf("file %q remains after request completion", id)
		} else if agentengine.ErrorCodeOf(err) != agentengine.ErrorFileNotFound {
			t.Errorf("Get(%q) error = %v, want file_not_found", id, err)
		}
	}
}

type recordingSessionInputFiles struct {
	agentengine.FileInterface
	created []string
}

type sessionOversizedRequestBody struct{}

func (sessionOversizedRequestBody) Read([]byte) (int, error) {
	return 0, &http.MaxBytesError{Limit: agentSessionFileBodyLimit}
}

func (sessionOversizedRequestBody) Close() error { return nil }

func (f *recordingSessionInputFiles) Create(ctx context.Context, request agentengine.FileCreateRequest, content io.Reader) (agentengine.OutputFile, error) {
	file, err := f.FileInterface.Create(ctx, request, content)
	if err == nil {
		f.created = append(f.created, file.ID)
	}
	return file, err
}

type sessionFileRecordingEngine struct {
	agentengine.Interface
	files *recordingSessionInputFiles
}

func (e sessionFileRecordingEngine) Conversations(agentID string) agentengine.ConversationInterface {
	return sessionFileRecordingConversations{ConversationInterface: e.Interface.Conversations(agentID), files: e.files}
}

type sessionFileRecordingConversations struct {
	agentengine.ConversationInterface
	files *recordingSessionInputFiles
}

func (c sessionFileRecordingConversations) Files() agentengine.FileInterface {
	return c.files
}
