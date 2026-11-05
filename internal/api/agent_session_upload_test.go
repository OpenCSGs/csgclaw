package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/im"
)

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
		return multipartSessionRequest(t, nil, sessionUploadFixture{"picture.png", "image/png", picture.String()})
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

	// Workers receive the immutable upload even if someone edits its workspace copy.
	edited := []byte("edited in workspace")
	local := filepath.Join(workspace, filepath.FromSlash(att.WorkspacePath))
	if err := os.WriteFile(local, edited, 0600); err != nil {
		t.Fatal(err)
	}
	engine.run = func(ctx context.Context, agentID string, req agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		var received int
		for _, part := range req.Input {
			if part.File == nil {
				continue
			}
			received++
			download, err := engine.Conversations(agentID).Files().Get(ctx, part.File.ID)
			if err != nil {
				t.Error(err)
				continue
			}
			content, err := io.ReadAll(download.Content)
			download.Content.Close()
			if err != nil || !bytes.Equal(content, picture.Bytes()) {
				t.Errorf("worker did not receive the original picture: %v", err)
			}
		}
		if received != 1 {
			t.Errorf("worker received %d files, want 1", received)
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Done."}
	}
	for _, input := range []string{"Describe the picture.", ""} {
		payload := map[string]any{"attachment_ids": []string{att.ID}}
		if input != "" {
			payload["input"] = input
		}
		data := postSessionReferenceTest(t, server, sessionPath, bearer, payload, http.StatusOK)
		files := sessionResponsePublishedFiles(t, data, false)
		if len(files.Inputs) != 1 || files.Inputs[0].ID != att.ID || files.Inputs[0].WorkspacePath != att.WorkspacePath || len(files.Outputs) != 0 {
			t.Fatalf("referenced input metadata: %s", data)
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
	} else if sessionTurnText(calls[0].Request) != "Describe the picture." || sessionTurnText(calls[1].Request) != "" {
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
	if got, err := os.ReadFile(local); err != nil || !bytes.Equal(got, edited) {
		t.Fatalf("reference changed the workspace copy: %q, %v", got, err)
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
	upload := multipartSessionRequest(t, nil, sessionUploadFixture{name, mediaType, pdf})
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
		multipartSessionRequest(t, nil),
		multipartSessionRequest(t, []string{`{"input":"run"}`}, tooMany[0]),
		multipartSessionRequest(t, nil, sessionUploadFixture{"../outside.txt", "text/plain", "unsafe name"}),
		multipartSessionRequest(t, nil, tooMany...),
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
