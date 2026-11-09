package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/im"
)

const multipartSessionPath = "/api/v1/agents/agent-alpha/sessions/test-upload/responses"

type sessionUploadFixture struct {
	name, mediaType, data string
}

func sessionFormRequest(t *testing.T, input string, stream bool, files ...sessionUploadFixture) *http.Request {
	t.Helper()
	prompt, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return sessionMultipartForm(t, map[string][]string{"input": {string(prompt)}, "stream": {fmt.Sprint(stream)}}, map[string][]sessionUploadFixture{"file": files})
}

func sessionMultipartForm(t *testing.T, values map[string][]string, uploads map[string][]sessionUploadFixture) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, files := range uploads {
		for _, file := range files {
			header := make(textproto.MIMEHeader)
			header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, name, file.name))
			header.Set("Content-Type", file.mediaType)
			part, err := writer.CreatePart(header)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(part, file.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	for key, entries := range values {
		for _, value := range entries {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, multipartSessionPath, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestAgentSessionResponsesMultipartFiles(t *testing.T) {
	files := []sessionUploadFixture{
		{"社区工作.pdf", "application/pdf", "%PDF-1.7\x00\xff\r\n"},
		{"report.bin", "application/octet-stream", "\x00\xfe\x80"},
	}
	for _, tc := range []struct {
		name, prompt string
		stream       bool
	}{
		{"text and files", "Read the PDF", false},
		{"files only", "", true},
		{"files with blank prompt", "  ", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &fakeSessionEngine{}
			var paths []string
			h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
			workspace := sessionTestWorkspace(t, h)
			engine.run = func(ctx context.Context, agentID string, req agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
				prompt := sessionTurnText(req)
				if tc.stream && prompt != "" {
					t.Errorf("file-only prompt = %q", prompt)
				}
				if !tc.stream && prompt != tc.prompt {
					t.Errorf("prompt = %q", prompt)
				}
				paths = checkSessionWorkerFiles(t, workspace, req, files)
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "received files"}
			}
			rec := httptest.NewRecorder()
			req := sessionFormRequest(t, tc.prompt, tc.stream, files...)
			req.Header.Set("Authorization", "Bearer test-server-token")
			h.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || len(paths) != len(files) {
				t.Fatalf("status=%d, paths=%v, body=%s", rec.Code, paths, rec.Body)
			}
			if tc.stream {
				for _, want := range []string{"event: message_start", "received files", "event: message_stop"} {
					if !strings.Contains(rec.Body.String(), want) {
						t.Errorf("stream missing %q: %s", want, rec.Body)
					}
				}
			} else {
				var response agentSessionResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.Status != "completed" || len(response.Output) != 1 || len(response.Output[0].Content) != 1 || response.Output[0].Content[0].Text != "received files" {
					t.Errorf("response = %+v", response)
				}
			}
		})
	}
}

func sessionTestWorkspace(t *testing.T, h *Handler) string {
	t.Helper()
	root, err := h.workspace.WorkspaceRootByID("agent-alpha")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func checkSessionWorkerFiles(t *testing.T, workspace string, turn agentengine.TurnRequest, files []sessionUploadFixture) []string {
	t.Helper()
	var paths []string
	for _, part := range turn.Input {
		if part.Kind == agentengine.InputPartText {
			continue
		}
		if part.Kind != agentengine.InputPartWorkspaceFile || part.WorkspaceFile == nil || len(paths) >= len(files) {
			t.Errorf("unexpected file input: %+v", part)
			continue
		}
		got, want := part.WorkspaceFile, files[len(paths)]
		paths = append(paths, got.Path)
		if !filepath.IsLocal(filepath.FromSlash(got.Path)) {
			t.Errorf("invalid workspace path: %q", got.Path)
			continue
		}
		data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(got.Path)))
		if err != nil || string(data) != want.data || got.Name != want.name || got.MIMEType != want.mediaType {
			t.Errorf("file %q: metadata=%+v, content matches=%t, error=%v", want.name, got, string(data) == want.data, err)
		}
	}
	if len(paths) != len(files) {
		t.Errorf("worker received %d files, want %d", len(paths), len(files))
	}
	return paths
}

func TestAgentSessionResponsesMultipartInputFields(t *testing.T) {
	files := []sessionUploadFixture{
		{"会议纪要.pdf", "application/pdf", "%PDF-1.7\r\n\x00\xff"},
		{"notes.txt", "text/plain", "notes"},
	}
	for _, tc := range []struct {
		name, input string
		stream      bool
	}{
		{"string input", `"Read the PDF."`, false},
		{"message array", `[{"role":"user","content":[{"type":"input_text","text":"Read the PDF."}]}]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &fakeSessionEngine{}
			server, workspace := sessionOutputTestServer(t, engine, "")
			engine.run = func(ctx context.Context, agentID string, turn agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
				if prompt := sessionTurnText(turn); prompt != "Read the PDF." {
					t.Errorf("worker prompt = %q", prompt)
				}
				if turn.Interaction != agentengine.InteractionUserInputOnly {
					t.Errorf("multipart user_input selected policy %q", turn.Interaction)
				}
				checkSessionWorkerFiles(t, workspace, turn, files)
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Read."}
			}
			form := sessionMultipartForm(t, map[string][]string{"input": {tc.input}, "stream": {fmt.Sprint(tc.stream)}, "user_input": {"true"}}, map[string][]sessionUploadFixture{"file": files})
			status, body := sessionFileHTTP(t, server, http.MethodPost, multipartSessionPath, form.Header.Get("Content-Type"), form.Body, "Bearer test-server-token")
			if status != http.StatusOK || len(engine.client.Calls()) != 1 {
				t.Fatalf("HTTP %d, worker calls %d: %s", status, len(engine.client.Calls()), body)
			}
			if tc.stream && !bytes.HasPrefix(body, []byte("event: message_start\n")) {
				t.Fatalf("unexpected first SSE event: %s", body)
			}
			published := sessionResponsePublishedFiles(t, body, tc.stream)
			if len(published.Inputs) != len(files) {
				t.Fatalf("published input count = %d, want %d", len(published.Inputs), len(files))
			}
			for i, file := range files {
				checkSessionDownload(t, server, workspace, published.Inputs[i], file.name, file.mediaType, []byte(file.data))
			}
		})
	}
}

func TestAgentSessionResponsesRejectsInvalidMultipart(t *testing.T) {
	file := sessionUploadFixture{"note.txt", "text/plain", "note"}
	tooMany := make([]sessionUploadFixture, im.MaxAttachmentsPerMessage+1)
	for i := range tooMany {
		tooMany[i] = file
	}
	for _, tc := range []struct {
		name  string
		files []sessionUploadFixture
	}{
		{"missing file", nil},
		{"empty file", []sessionUploadFixture{{"file.bin", "application/octet-stream", ""}}},
		{"relative filename", []sessionUploadFixture{file, {"../bad.txt", "text/plain", "bad"}}},
		{"too many files", tooMany},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &fakeSessionEngine{}
			h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
			rec := httptest.NewRecorder()
			req := sessionFormRequest(t, "Read", true, tc.files...)
			req.Header.Set("Authorization", "Bearer test-server-token")
			h.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_request") || len(engine.client.Calls()) != 0 {
				t.Fatalf("status=%d, calls=%d, body=%s", rec.Code, len(engine.client.Calls()), rec.Body)
			}
		})
	}
}

func TestAgentSessionRejectsInvalidMultipartInputFields(t *testing.T) {
	engine := &fakeSessionEngine{}
	server, _ := sessionOutputTestServer(t, engine, "")
	file := sessionUploadFixture{"notes.txt", "text/plain", "notes"}
	const input = `"Read."`
	const payload = `{"input":"Read."}`
	for _, tc := range []struct {
		name   string
		fields map[string][]string
		files  []string
	}{
		{"missing input", nil, []string{"file"}},
		{"payload and files", map[string][]string{"payload": {payload}}, []string{"files"}},
		{"payload and file", map[string][]string{"payload": {payload}}, []string{"file"}},
		{"input and files", map[string][]string{"input": {input}}, []string{"files"}},
		{"inline and multipart files", map[string][]string{"input": {`[{"role":"user","content":[{"type":"input_file","filename":"b.txt","file_data":"data:text/plain;base64,YQ=="}]}]`}}, []string{"file"}},
		{"payload with input", map[string][]string{"payload": {payload}, "input": {input}}, []string{"files"}},
		{"payload with stream", map[string][]string{"payload": {payload}, "stream": {"true"}}, []string{"files"}},
		{"duplicate input", map[string][]string{"input": {input, input}}, []string{"file"}},
		{"duplicate stream", map[string][]string{"input": {input}, "stream": {"false", "true"}}, []string{"file"}},
		{"duplicate user input flag", map[string][]string{"input": {input}, "user_input": {"false", "true"}}, []string{"file"}},
		{"invalid user input flag", map[string][]string{"input": {input}, "user_input": {"unknown"}}, []string{"file"}},
		{"unknown text field", map[string][]string{"input": {input}, "unexpected": {"field"}}, []string{"file"}},
		{"invalid input JSON", map[string][]string{"input": {"Read."}}, []string{"file"}},
		{"null input", map[string][]string{"input": {"null"}}, []string{"file"}},
		{"invalid input object", map[string][]string{"input": {`{"content":"read"}`}}, []string{"file"}},
		{"invalid stream", map[string][]string{"input": {input}, "stream": {"yes"}}, []string{"file"}},
		{"null stream", map[string][]string{"input": {input}, "stream": {"null"}}, []string{"file"}},
		{"mixed file parts", map[string][]string{"input": {input}}, []string{"file", "files"}},
		{"unknown file part", map[string][]string{"input": {input}}, []string{"file", "other"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uploads := make(map[string][]sessionUploadFixture)
			for _, name := range tc.files {
				uploads[name] = []sessionUploadFixture{file}
			}
			req := sessionMultipartForm(t, tc.fields, uploads)
			result := postSessionUploadTest(t, server, multipartSessionPath, "Bearer test-server-token", req.Header.Get("Content-Type"), req.Body, http.StatusBadRequest)
			if !bytes.Contains(result, []byte(`"invalid_request"`)) {
				t.Fatalf("unexpected error: %s", result)
			}
		})
	}
	t.Run("file on standalone upload", func(t *testing.T) {
		req := sessionMultipartForm(t, nil, map[string][]sessionUploadFixture{"file": {file}})
		path := strings.TrimSuffix(multipartSessionPath, "/responses") + "/attachments"
		postSessionUploadTest(t, server, path, "Bearer test-server-token", req.Header.Get("Content-Type"), req.Body, http.StatusBadRequest)
	})
	if calls := len(engine.client.Calls()); calls != 0 {
		t.Errorf("invalid requests dispatched %d worker turns", calls)
	}
}

func TestAgentSessionResponsesCleansMultipartFilesOnTurnFailure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	payload := strings.Repeat("a", multipartAttachmentMemory+1)
	engine := &fakeSessionEngine{run: func(_ context.Context, _ string, req agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: "runtime failed"}}
	}}
	h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	req := sessionFormRequest(t, "Read", false, sessionUploadFixture{"notes.txt", "text/plain", payload})
	req.Header.Set("Authorization", "Bearer test-server-token")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, body=%s", rec.Code, rec.Body)
	}
	assertNoMultipartStaging(t, tmp)
}

func TestAgentSessionResponsesReportsOversizedMultipartRequest(t *testing.T) {
	engine := &fakeSessionEngine{}
	h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	req := sessionFormRequest(t, strings.Repeat("a", agentSessionResponseBodyLimit), true, sessionUploadFixture{"notes.txt", "text/plain", "notes"})
	req.Header.Set("Authorization", "Bearer test-server-token")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "invalid_request") || len(engine.client.Calls()) != 0 {
		t.Fatalf("status=%d, calls=%d, body=%s", rec.Code, len(engine.client.Calls()), rec.Body)
	}
}
