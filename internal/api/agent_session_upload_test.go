package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"mime"
	"net"
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

func sessionUploadForm(t *testing.T, files ...sessionUploadFixture) *http.Request {
	t.Helper()
	return sessionMultipartForm(t, nil, map[string][]sessionUploadFixture{"files": files})
}

func TestAgentSessionFilesRequireServerCredentials(t *testing.T) {
	engine := &fakeSessionEngine{}
	server, workspace := sessionOutputTestServer(t, engine, "")
	const name = "private.txt"
	inline, err := json.Marshal(sessionInlineFileRequest(false, sessionInlineFilePart(name, "text/plain", []byte("original"))))
	if err != nil {
		t.Fatal(err)
	}
	for _, bearer := range []string{"", "Bearer wrong-token"} {
		form := sessionFormRequest(t, "Read", false, sessionUploadFixture{name, "text/plain", "original"})
		postSessionUploadTest(t, server, multipartSessionPath, bearer, form.Header.Get("Content-Type"), form.Body, http.StatusUnauthorized)
		postSessionUploadTest(t, server, multipartSessionPath, bearer, "application/json", bytes.NewReader(inline), http.StatusUnauthorized)
		postSessionUploadTest(t, server, multipartSessionPath, bearer, "application/json", strings.NewReader(`{"attachment_ids":["unknown"]}`), http.StatusUnauthorized)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".csgclaw", "attachments")); !os.IsNotExist(err) || len(engine.client.Calls()) != 0 {
		t.Fatalf("unauthorized request wrote files or dispatched a turn: %v, calls=%d", err, len(engine.client.Calls()))
	}
	postSessionUploadTest(t, server, multipartSessionPath, "", "application/json", strings.NewReader(`{"input":"Hello"}`), http.StatusOK)
}

func TestAgentSessionRejectsOversizedAttachmentIDsBeforeRequestEnds(t *testing.T) {
	server, _ := sessionOutputTestServer(t, &fakeSessionEngine{}, "")

	// Send only ten IDs and the first byte of the eleventh. The client keeps
	// the chunked body open until it reads the rejection.
	var prefix strings.Builder
	prefix.WriteString(`{"attachment_ids":[`)
	for i := range im.MaxAttachmentsPerMessage {
		fmt.Fprintf(&prefix, `"att-%d",`, i)
	}
	prefix.WriteByte('"')
	for _, tc := range []struct {
		name, body, message string
	}{
		{"eleventh ID", prefix.String(), "too many"},
		{"oversized ID", `{"attachment_ids":["` + strings.Repeat("x", agentSessionResponseBodyLimit), "metadata limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, failure := postUnfinishedSessionJSON(t, server, "Bearer test-server-token", tc.body)
			if status != http.StatusBadRequest || failure.Error.Param == nil || *failure.Error.Param != "attachment_ids" || !strings.Contains(failure.Error.Message, tc.message) {
				t.Fatalf("unexpected response: HTTP %d, %+v", status, failure)
			}
		})
	}
}

func postUnfinishedSessionJSON(t *testing.T, server *httptest.Server, bearer, body string) (int, agentSessionErrorEnvelope) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", server.Listener.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(4 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(conn, "POST %s HTTP/1.1\r\nHost: %s\r\nAuthorization: %s\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n%s\r\n",
		multipartSessionPath, server.Listener.Addr(), bearer, len(body), body)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("server waited for more than %d JSON bytes: %v", len(body), err)
	}
	defer response.Body.Close()
	var failure agentSessionErrorEnvelope
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, failure
}

func TestAgentSessionAnonymousJSONBodyLimit(t *testing.T) {
	const limit = agentSessionResponseBodyLimit
	engine := &fakeSessionEngine{}
	h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	server := httptest.NewServer(h.Routes())
	defer server.Close()
	const text = `{"input":"hello"}`
	postSessionUploadTest(t, server, multipartSessionPath, "", "application/json",
		strings.NewReader(text+strings.Repeat(" ", limit-len(text))), http.StatusOK)

	body := `{"input":[{"role":"user","content":[{"type":"input_file","filename":"upload.bin","file_data":"data:application/octet-stream;base64,` + strings.Repeat("A", limit*2)
	status, _ := postUnfinishedSessionJSON(t, server, "Bearer wrong-token", body[:limit+1])
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("unfinished anonymous upload: HTTP %d", status)
	}
	for _, length := range []int64{-1, int64(len(body))} {
		unread := &io.LimitedReader{R: strings.NewReader(body), N: int64(len(body))}
		req := httptest.NewRequest(http.MethodPost, multipartSessionPath, unread)
		req.ContentLength = length
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, req)
		read := int64(len(body)) - unread.N
		if rec.Code != http.StatusRequestEntityTooLarge || read > limit+1 || (length >= 0 && read != 0) {
			t.Errorf("anonymous upload with Content-Length %d: HTTP %d after reading %d bytes", length, rec.Code, read)
		}
	}
	if len(engine.client.Calls()) != 1 {
		t.Fatalf("only the text turn should run, got %d turns", len(engine.client.Calls()))
	}
}

func TestAgentSessionUploadThenReference(t *testing.T) {
	engine := &fakeSessionEngine{}
	server, workspace := sessionOutputTestServer(t, engine, "")
	const bearer = "Bearer test-server-token"
	const sessionPath = "/api/v1/agents/agent-alpha/sessions/test-upload"
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	newUpload := func() *http.Request {
		return sessionUploadForm(t, sessionUploadFixture{"picture.png", "image/png", picture.String()})
	}
	upload := newUpload()
	postSessionUploadTest(t, server, sessionPath+"/attachments", "", upload.Header.Get("Content-Type"), upload.Body, http.StatusUnauthorized)
	upload = newUpload()
	data := postSessionUploadTest(t, server, sessionPath+"/attachments", bearer, upload.Header.Get("Content-Type"), upload.Body, http.StatusCreated)
	var saved struct {
		Attachments []im.MessageAttachment
	}
	if err := json.Unmarshal(data, &saved); err != nil || len(saved.Attachments) != 1 {
		t.Fatalf("upload: %s, %v", data, err)
	}
	if len(engine.client.Calls()) != 0 {
		t.Fatal("upload dispatched an agent turn")
	}
	att := saved.Attachments[0]
	checkSessionDownload(t, server, workspace, att, "picture.png", "image/png", picture.Bytes())
	preview, err := server.Client().Get(server.URL + att.PreviewURL)
	if err != nil {
		t.Fatal(err)
	}
	previewBytes, err := io.ReadAll(preview.Body)
	preview.Body.Close()
	if err != nil || preview.StatusCode != http.StatusOK || !bytes.Equal(previewBytes, picture.Bytes()) ||
		!strings.HasPrefix(preview.Header.Get("Content-Disposition"), "inline") || att.Width != 2 || att.Height != 3 {
		t.Fatalf("preview: HTTP %d, %q, image %dx%d, error %v", preview.StatusCode, preview.Header.Get("Content-Disposition"), att.Width, att.Height, err)
	}

	local := filepath.Join(workspace, filepath.FromSlash(att.WorkspacePath))
	var editedPicture bytes.Buffer
	if err := png.Encode(&editedPicture, image.NewRGBA(image.Rect(0, 0, 4, 5))); err != nil {
		t.Fatal(err)
	}
	edited := editedPicture.String()
	engine.run = func(ctx context.Context, agentID string, req agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		edit := sessionTurnText(req) == "Edit the picture."
		want := edited
		if edit {
			want = picture.String()
		}
		paths := checkSessionWorkerFiles(t, workspace, req, []sessionUploadFixture{{"picture.png", "image/png", want}})
		if len(paths) != 1 || paths[0] != att.WorkspacePath {
			t.Errorf("worker paths = %v, want %q", paths, att.WorkspacePath)
		}
		if edit {
			if err := os.WriteFile(local, []byte(edited), 0600); err != nil {
				t.Error(err)
			}
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Done."}
	}
	for _, input := range []string{"Edit the picture.", ""} {
		payload := map[string]any{"attachment_ids": []string{att.ID}}
		if input != "" {
			payload["input"] = input
		}
		data := postSessionReferenceTest(t, server, sessionPath, bearer, payload, http.StatusOK)
		files := sessionResponsePublishedFiles(t, data, false)
		if len(files.Inputs) != 1 || files.Inputs[0].ID != att.ID || files.Inputs[0].WorkspacePath != att.WorkspacePath || len(files.Outputs) != 0 {
			t.Fatalf("referenced input metadata: %s", data)
		}
		if input == "" && (files.Inputs[0].SizeBytes != int64(len(edited)) || files.Inputs[0].SHA256 != att.SHA256) {
			t.Fatalf("edited input size or published checksum changed unexpectedly: %+v", files.Inputs[0])
		}
	}
	postSessionReferenceTest(t, server, sessionPath, "", map[string]any{"attachment_ids": []string{att.ID}}, http.StatusUnauthorized)
	postSessionReferenceTest(t, server, strings.Replace(sessionPath, "test-upload", "another-session", 1), bearer, map[string]any{"attachment_ids": []string{att.ID}}, http.StatusNotFound)
	postSessionReferenceTest(t, server, sessionPath, bearer, map[string]any{"attachment_ids": []string{att.ID, att.ID}}, http.StatusBadRequest)
	postSessionReferenceTest(t, server, sessionPath, bearer, map[string]any{"attachment_ids": []string{att.ID, " " + att.ID}}, http.StatusBadRequest)
	var tooMany []string
	for i := range im.MaxAttachmentsPerMessage + 1 {
		tooMany = append(tooMany, fmt.Sprintf("att-file-%d", i))
	}
	postSessionReferenceTest(t, server, sessionPath, bearer, map[string]any{"attachment_ids": tooMany}, http.StatusBadRequest)
	if calls := engine.client.Calls(); len(calls) != 2 {
		t.Fatalf("expected only the two valid turns, got %d", len(calls))
	} else if sessionTurnText(calls[0].Request) != "Edit the picture." || sessionTurnText(calls[1].Request) != "" {
		t.Errorf("unexpected prompts: %q, %q", sessionTurnText(calls[0].Request), sessionTurnText(calls[1].Request))
	}
	if err := filepath.WalkDir(workspace, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && path != local {
			t.Errorf("reference created another workspace file: %s", path)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(local); err != nil || string(got) != edited {
		t.Fatalf("reference changed the workspace copy: %q, %v", got, err)
	}
	status, current := sessionFileHTTP(t, server, http.MethodGet, att.DownloadURL, "", nil)
	if status != http.StatusOK || string(current) != edited {
		t.Fatalf("download did not reflect the workspace edit: HTTP %d", status)
	}
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	postSessionReferenceTest(t, server, sessionPath, bearer, map[string]any{"attachment_ids": []string{att.ID}}, http.StatusNotFound)
	status, _ = sessionFileHTTP(t, server, http.MethodGet, att.DownloadURL, "", nil)
	if status != http.StatusNotFound || len(engine.client.Calls()) != 2 {
		t.Fatalf("removed file: download HTTP %d, turns %d", status, len(engine.client.Calls()))
	}
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Fatalf("referencing a removed workspace file recreated it: %v", err)
	}
}

func TestAgentSessionWorkspaceFilesAfterRestart(t *testing.T) {
	const bearer = "Bearer test-server-token"
	const agentID, sessionID = "agent-alpha", "work-files"
	const path = "/api/v1/agents/" + agentID + "/sessions/" + sessionID
	statePath := filepath.Join(t.TempDir(), "im", "state.json")
	workspace := t.TempDir()
	start := func(root string) (*httptest.Server, *fakeSessionEngine) {
		t.Helper()
		worker := sessionCodexAgent(agentID, "Alpha")
		worker.RuntimeOptions = map[string]any{"local_workspace_dir": root}
		engine := &fakeSessionEngine{}
		h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{worker}, engine, "")
		store, err := im.NewServiceFromPath(statePath)
		if err != nil {
			t.Fatal(err)
		}
		h.im = store
		server := httptest.NewServer(h.Routes())
		t.Cleanup(server.Close)
		return server, engine
	}

	server, engine := start(workspace)
	report := createSessionTestOutput(t, engine.Conversations(agentID).Files(), "result.txt", "text/plain", []byte("first draft"))
	engine.run = func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Published", Files: []agentengine.OutputFile{report}}
	}
	form := sessionFormRequest(t, "Review", false, sessionUploadFixture{"source.txt", "text/plain", "original input"})
	body := postSessionUploadTest(t, server, path+"/responses", bearer, form.Header.Get("Content-Type"), form.Body, http.StatusOK)
	published := sessionResponsePublishedFiles(t, body, false)
	if len(published.Inputs) != 1 || len(published.Outputs) != 1 {
		t.Fatalf("unexpected files: %+v", published)
	}
	server.Close()

	server, engine = start(workspace)
	files := []im.MessageAttachment{published.Inputs[0], published.Outputs[0]}
	var expected []sessionUploadFixture
	var ids []string
	for _, file := range files {
		edited := "updated " + file.Name
		if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(file.WorkspacePath)), []byte(edited), 0600); err != nil {
			t.Fatal(err)
		}
		expected = append(expected, sessionUploadFixture{file.Name, "text/plain", edited})
		ids = append(ids, file.ID)
		status, content := sessionFileHTTP(t, server, http.MethodGet, "/api/v1/attachments/"+file.ID, "", nil, bearer)
		if status != http.StatusOK || string(content) != edited {
			t.Fatalf("history download %q: HTTP %d, %q", file.Name, status, content)
		}
	}
	engine.run = func(_ context.Context, _ string, turn agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		checkSessionWorkerFiles(t, workspace, turn, expected)
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Read both edits"}
	}
	postSessionReferenceTest(t, server, path, bearer, map[string]any{"attachment_ids": ids}, http.StatusOK)
	target := filepath.Join(workspace, filepath.FromSlash(files[1].WorkspacePath))
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	postSessionReferenceTest(t, server, path, bearer, map[string]any{"attachment_ids": ids}, http.StatusNotFound)
	if status, _ := sessionFileHTTP(t, server, http.MethodGet, "/api/v1/attachments/"+files[1].ID, "", nil, bearer); status != http.StatusNotFound {
		t.Fatalf("deleted output download: HTTP %d", status)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) || len(engine.client.Calls()) != 1 {
		t.Fatalf("deleted output reference: stat=%v, turns=%d", err, len(engine.client.Calls()))
	}
	server.Close()
	replacementWorkspace := t.TempDir()
	shadow := filepath.Join(replacementWorkspace, filepath.FromSlash(files[0].WorkspacePath))
	if err := os.MkdirAll(filepath.Dir(shadow), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shadow, []byte("different workspace"), 0600); err != nil {
		t.Fatal(err)
	}
	server, engine = start(replacementWorkspace)
	postSessionReferenceTest(t, server, path, bearer, map[string]any{"attachment_ids": ids[:1]}, http.StatusNotFound)
	if len(engine.client.Calls()) != 0 {
		t.Fatal("dispatched a file from a different workspace")
	}
}

func TestAgentSessionCanceledRequestDoesNotPublishUploads(t *testing.T) {
	for _, endpoint := range []string{"responses", "attachments"} {
		t.Run(endpoint, func(t *testing.T) {
			engine := &fakeSessionEngine{}
			srv, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
			file := sessionUploadFixture{"report.txt", "text/plain", "data"}
			var form *http.Request
			if endpoint == "responses" {
				form = sessionFormRequest(t, "Read", false, file)
			} else {
				form = sessionUploadForm(t, file)
			}
			form.URL.Path = strings.TrimSuffix(multipartSessionPath, "responses") + endpoint
			form.Header.Set("Authorization", "Bearer test-server-token")
			ctx, cancel := context.WithCancel(form.Context())
			cancel()
			rec := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rec, form.WithContext(ctx))
			if len(engine.client.Calls()) != 0 || rec.Code < 400 {
				t.Fatalf("canceled upload: HTTP %d, turns %d", rec.Code, len(engine.client.Calls()))
			}
			if err := filepath.WalkDir(sessionTestWorkspace(t, srv), func(path string, entry fs.DirEntry, err error) error {
				if err == nil && !entry.IsDir() {
					return fmt.Errorf("canceled upload published %s", path)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAgentSessionUploadPDF(t *testing.T) {
	server, _ := sessionOutputTestServer(t, &fakeSessionEngine{}, "")
	const name = "会议纪要.pdf"
	const mediaType = "application/pdf"
	const pdf = "%PDF-1.4\n" +
		"1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\n" +
		"2 0 obj\n<</Type/Pages/Kids[3 0 R]/Count 1>>\nendobj\n" +
		"3 0 obj\n<</Type/Page/Parent 2 0 R/MediaBox[0 0 100 100]/Resources<<>>>>\nendobj\n" +
		"xref\n0 4\n0000000000 65535 f\r\n0000000009 00000 n\r\n0000000054 00000 n\r\n0000000105 00000 n\r\n" +
		"trailer\n<</Size 4/Root 1 0 R>>\nstartxref\n184\n%%EOF\n"
	upload := sessionUploadForm(t, sessionUploadFixture{name, mediaType, pdf})
	body := postSessionUploadTest(t, server, "/api/v1/agents/agent-alpha/sessions/test-pdf/attachments",
		"Bearer test-server-token", upload.Header.Get("Content-Type"), upload.Body, http.StatusCreated)
	var saved struct {
		Attachments []im.MessageAttachment
	}
	if err := json.Unmarshal(body, &saved); err != nil || len(saved.Attachments) != 1 {
		t.Fatalf("upload: %s, %v", body, err)
	}
	att := saved.Attachments[0]
	if att.Name != name || att.MediaType != mediaType || att.Kind != "file" || att.DownloadURL == "" || att.PreviewURL != "" {
		t.Fatalf("unexpected PDF metadata: %+v", att)
	}
	resp, err := server.Client().Get(server.URL + att.DownloadURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	disposition, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	if err != nil || disposition != "attachment" || params["filename"] != name {
		t.Fatalf("PDF disposition: %q, %v", resp.Header.Get("Content-Disposition"), err)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != mediaType {
		t.Fatalf("PDF download: HTTP %d, Content-Type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil || !bytes.Equal(data, []byte(pdf)) {
		t.Fatalf("PDF download differs from original: %v", err)
	}
}

func TestAgentSessionUploadRejectsInvalidForm(t *testing.T) {
	engine := &fakeSessionEngine{}
	server, _ := sessionOutputTestServer(t, engine, "")
	const path = "/api/v1/agents/agent-alpha/sessions/test-upload/attachments"
	const bearer = "Bearer test-server-token"
	postSessionUploadTest(t, server, path, bearer, "application/json", strings.NewReader(`{"files":[]}`), http.StatusUnsupportedMediaType)
	tooMany := make([]sessionUploadFixture, im.MaxAttachmentsPerMessage+1)
	for i := range tooMany {
		tooMany[i] = sessionUploadFixture{fmt.Sprintf("input-%d.txt", i), "text/plain", "contents"}
	}
	for _, form := range []*http.Request{
		sessionUploadForm(t),
		sessionMultipartForm(t, map[string][]string{"input": {`"run"`}}, map[string][]sessionUploadFixture{"files": {tooMany[0]}}),
		sessionUploadForm(t, sessionUploadFixture{"../outside.txt", "text/plain", "unsafe name"}),
		sessionUploadForm(t, tooMany...),
	} {
		postSessionUploadTest(t, server, path, bearer, form.Header.Get("Content-Type"), form.Body, http.StatusBadRequest)
	}
	if len(engine.client.Calls()) != 0 {
		t.Fatal("invalid upload dispatched an agent turn")
	}
}

func postSessionReferenceTest(t *testing.T, server *httptest.Server, sessionPath, bearer string, payload map[string]any, wantStatus int) []byte {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return postSessionUploadTest(t, server, sessionPath+"/responses", bearer, "application/json", bytes.NewReader(body), wantStatus)
}

func postSessionUploadTest(t *testing.T, server *httptest.Server, path, bearer, contentType string, body io.Reader, wantStatus int) []byte {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, server.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != wantStatus {
		t.Fatalf("POST %s: HTTP %d, want %d, %s, %v", path, resp.StatusCode, wantStatus, data, err)
	}
	return data
}
