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

func multipartSessionRequest(t *testing.T, payloads []string, files ...sessionUploadFixture) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, file := range files {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files"; filename=%q`, file.name))
		header.Set("Content-Type", file.mediaType)
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, file.data); err != nil {
			t.Fatal(err)
		}
	}
	for _, payload := range payloads {
		if err := writer.WriteField("payload", payload); err != nil {
			t.Fatal(err)
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
		name, payload string
		stream        bool
	}{
		{"text and files", `{"input":"Read the PDF","extra":"from the portal","extra_type":"plain_text","stream":false}`, false},
		{"files only", `{"stream":true}`, true},
		{"files with blank prompt", `{"input":"  ","stream":true}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &fakeSessionEngine{}
			var ids []string
			engine.run = func(ctx context.Context, agentID string, req agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
				prompt := sessionTurnText(req)
				if tc.stream && prompt != "" {
					t.Errorf("file-only prompt = %q", prompt)
				}
				if !tc.stream && (!strings.Contains(prompt, "Read the PDF") || !strings.Contains(prompt, "from the portal")) {
					t.Errorf("prompt = %q", prompt)
				}
				for _, part := range req.Input {
					if part.Kind != agentengine.InputPartFile {
						continue
					}
					if part.File == nil || len(ids) >= len(files) {
						t.Fatalf("unexpected file input: %+v", part)
					}
					want := files[len(ids)]
					ids = append(ids, part.File.ID)
					got, err := engine.Conversations(agentID).Files().Get(ctx, part.File.ID)
					if err != nil {
						t.Fatal(err)
					}
					defer got.Content.Close()
					data, err := io.ReadAll(got.Content)
					if err != nil || string(data) != want.data || got.Metadata.Name != want.name || got.Metadata.MediaType != want.mediaType {
						t.Errorf("file %q: metadata=%+v, bytes=%x, error=%v", want.name, got.Metadata, data, err)
					}
				}
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "received files"}
			}
			h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
			rec := httptest.NewRecorder()
			h.Routes().ServeHTTP(rec, multipartSessionRequest(t, []string{tc.payload}, files...))
			if rec.Code != http.StatusOK || len(ids) != len(files) {
				t.Fatalf("status=%d, file IDs=%v, body=%s", rec.Code, ids, rec.Body)
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
			assertSessionInputFilesDeleted(t, engine.Conversations("agent-alpha").Files(), ids)
		})
	}
}

func TestAgentSessionResponsesRejectsInvalidMultipart(t *testing.T) {
	file := sessionUploadFixture{"note.txt", "text/plain", "note"}
	payload := `{"input":"Read","stream":true}`
	tooMany := make([]sessionUploadFixture, im.MaxAttachmentsPerMessage+1)
	for i := range tooMany {
		tooMany[i] = file
	}
	for _, tc := range []struct {
		name     string
		payloads []string
		files    []sessionUploadFixture
	}{
		{"missing payload", nil, []sessionUploadFixture{file}},
		{"duplicate payload", []string{payload, payload}, []sessionUploadFixture{file}},
		{"missing files", []string{payload}, nil},
		{"invalid JSON", []string{"{"}, []sessionUploadFixture{file}},
		{"null payload", []string{"null"}, []sessionUploadFixture{file}},
		{"unknown payload field", []string{`{"input":"Hi","unknown":true}`}, []sessionUploadFixture{file}},
		{"inline and multipart files", []string{`{"input":[{"role":"user","content":[{"type":"input_file","filename":"b.txt","file_data":"data:text/plain;base64,YQ=="}]}]}`}, []sessionUploadFixture{file}},
		{"empty file", []string{payload}, []sessionUploadFixture{{"file.bin", "application/octet-stream", ""}}},
		{"relative filename", []string{payload}, []sessionUploadFixture{file, {"../bad.txt", "text/plain", "bad"}}},
		{"too many files", []string{payload}, tooMany},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &fakeSessionEngine{}
			h, _, bindings, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
			store := &recordingSessionInputFiles{FileInterface: engine.Conversations("agent-alpha").Files()}
			h.SetAgentEngine(sessionFileRecordingEngine{Interface: engine, files: store}, bindings)
			rec := httptest.NewRecorder()
			h.Routes().ServeHTTP(rec, multipartSessionRequest(t, tc.payloads, tc.files...))
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_request") || len(engine.client.Calls()) != 0 {
				t.Fatalf("status=%d, calls=%d, body=%s", rec.Code, len(engine.client.Calls()), rec.Body)
			}
			assertSessionInputFilesDeleted(t, store, store.created)
		})
	}
}

func TestAgentSessionResponsesCleansMultipartFilesOnTurnFailure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	payload := strings.Repeat("a", multipartAttachmentMemory+1)
	var ids []string
	engine := &fakeSessionEngine{run: func(_ context.Context, _ string, req agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		for _, part := range req.Input {
			if part.File != nil {
				ids = append(ids, part.File.ID)
			}
		}
		return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: "runtime failed"}}
	}}
	h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	req := streamedAttachmentRequest(t, multipartSessionPath, map[string]any{"stream": false}, strings.NewReader(payload), int64(len(payload)))
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || len(ids) != 1 {
		t.Fatalf("status=%d, file IDs=%v, body=%s", rec.Code, ids, rec.Body)
	}
	assertSessionInputFilesDeleted(t, engine.Conversations("agent-alpha").Files(), ids)
	assertNoMultipartStaging(t, tmp)
}

func TestAgentSessionResponsesReportsOversizedMultipartRequest(t *testing.T) {
	engine := &fakeSessionEngine{}
	h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	payload := `{"stream":true,"input":"` + strings.Repeat("a", agentSessionResponseBodyLimit) + `"}`
	req := multipartSessionRequest(t, []string{payload}, sessionUploadFixture{"notes.txt", "text/plain", "notes"})
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "invalid_request") || len(engine.client.Calls()) != 0 {
		t.Fatalf("status=%d, calls=%d, body=%s", rec.Code, len(engine.client.Calls()), rec.Body)
	}
}
