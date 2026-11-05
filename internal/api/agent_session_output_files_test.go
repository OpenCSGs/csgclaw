package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/im"
)

func TestAgentSessionResponseOutputDownloads(t *testing.T) {
	for _, tc := range []struct {
		name           string
		stream, inline bool
	}{
		{"multipart to JSON", false, false},
		{"multipart to SSE in configured workspace", true, false},
		{"inline JSON to JSON", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &fakeSessionEngine{}
			customWorkspace := ""
			if tc.stream {
				customWorkspace = t.TempDir()
			}
			server, workspace := sessionOutputTestServer(t, engine, customWorkspace)
			files := engine.Conversations("agent-alpha").Files()
			original := []byte("%PDF-1.4\noriginal\x00\xff")
			revised := []byte("%PDF-1.7\nrevised\x00\xfe")
			notes := []byte("notes")
			report := createSessionTestOutput(t, files, "分析.pdf", "application/pdf", revised)
			summary := createSessionTestOutput(t, files, "notes.txt", "text/plain", notes)
			engine.run = func(_ context.Context, _ string, req agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
				result := agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Done."}
				if sessionTurnText(req) == "Generate reports." {
					result.Files = []agentengine.OutputFile{report, summary}
				}
				return result
			}

			var body io.Reader
			var typ string
			if tc.inline {
				data, err := json.Marshal(sessionInlineFileRequest(tc.stream,
					map[string]any{"type": "input_text", "text": "Generate reports."},
					sessionInlineFilePart("分析.pdf", "application/pdf", original)))
				if err != nil {
					t.Fatal(err)
				}
				body, typ = bytes.NewReader(data), "application/json"
			} else {
				body, typ = multipartMessageBodyForTest(t, map[string]any{"input": "Generate reports.", "stream": tc.stream}, "files", "分析.pdf", "application/pdf", original)
			}
			status, response := sessionFileHTTP(t, server, http.MethodPost, multipartSessionPath, typ, body)
			if status != http.StatusOK {
				t.Fatalf("publish: HTTP %d: %s", status, response)
			}
			published := sessionResponsePublishedFiles(t, response, tc.stream)
			if len(published.Inputs) != 1 || len(published.Outputs) != 2 {
				t.Fatalf("expected one input and two output attachments, got %s", response)
			}

			// Exercise the published URLs after the engine releases its files and a new turn completes.
			for _, file := range []agentengine.OutputFile{report, summary} {
				if err := files.Delete(context.Background(), file.ID); err != nil {
					t.Fatal(err)
				}
			}
			status, next := sessionFileHTTP(t, server, http.MethodPost, multipartSessionPath, "application/json",
				strings.NewReader(fmt.Sprintf(`{"input":"Continue.","stream":%t}`, tc.stream)))
			if status != http.StatusOK || !bytes.Contains(next, []byte("Done.")) || bytes.Contains(next, []byte(`"attachments"`)) || bytes.Contains(next, []byte(`"input_attachments"`)) {
				t.Fatalf("next text turn: HTTP %d: %s", status, next)
			}
			checkSessionDownload(t, server, workspace, published.Inputs[0], "分析.pdf", "application/pdf", original)
			checkSessionDownload(t, server, workspace, published.Outputs[0], "分析.pdf", "application/pdf", revised)
			checkSessionDownload(t, server, workspace, published.Outputs[1], "notes.txt", "text/plain", notes)
		})
	}
}

func checkSessionDownload(t *testing.T, server *httptest.Server, workspace string, att im.MessageAttachment, name, mediaType string, want []byte) {
	t.Helper()
	kind := "file"
	if strings.HasPrefix(mediaType, "image/") {
		kind = "image"
		if att.PreviewURL != att.DownloadURL {
			t.Fatalf("unexpected preview URL for %q", name)
		}
	}
	if att.ID == "" || att.Name != name || att.MediaType != mediaType || att.Kind != kind ||
		att.CreatedAt.IsZero() || att.SizeBytes != int64(len(want)) || att.SHA256 != fmt.Sprintf("%x", sha256.Sum256(want)) {
		t.Fatalf("incorrect attachment metadata: %+v", att)
	}
	relative := filepath.FromSlash(att.WorkspacePath)
	if !filepath.IsLocal(relative) {
		t.Fatalf("expected a workspace relative path for %q, got %q", name, att.WorkspacePath)
	}
	stored, err := os.ReadFile(filepath.Join(workspace, relative))
	if err != nil || !bytes.Equal(stored, want) {
		t.Fatalf("workspace file %q: bytes %x, error %v", att.WorkspacePath, stored, err)
	}
	link, err := url.Parse(att.DownloadURL)
	if err != nil || link.Path != "/api/v1/attachments/"+att.ID || link.Query().Get("token") == "" {
		t.Fatalf("invalid attachment download URL: %q", att.DownloadURL)
	}
	status, data := sessionFileHTTP(t, server, http.MethodGet, att.DownloadURL, "", nil)
	if status != http.StatusOK || !bytes.Equal(data, want) {
		t.Errorf("download %q: HTTP %d, bytes %x", att.Name, status, data)
	}
	status, _ = sessionFileHTTP(t, server, http.MethodGet, link.Path, "", nil)
	if status != http.StatusUnauthorized {
		t.Errorf("download without a token: HTTP %d", status)
	}
}

func TestAgentSessionResponseWithUnavailableOutput(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			engine := &fakeSessionEngine{}
			server, workspace := sessionOutputTestServer(t, engine, "")
			file := createSessionTestOutput(t, engine.Conversations("agent-alpha").Files(), "first.txt", "text/plain", []byte("private contents"))
			missing := agentengine.OutputFile{OutputFileMetadata: agentengine.OutputFileMetadata{ID: "missing-output", Name: "unavailable.txt"}}
			engine.run = func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Ready.", Files: []agentengine.OutputFile{file, missing}}
			}
			upload, typ := multipartMessageBodyForTest(t, map[string]any{"stream": stream}, "files", "source.txt", "text/plain", []byte("private input"))
			status, body := sessionFileHTTP(t, server, http.MethodPost, multipartSessionPath, typ, upload)
			if stream {
				if status != http.StatusOK || !bytes.Contains(body, []byte("event: error\n")) || !bytes.Contains(body, []byte(`"stop_reason":"error"`)) {
					t.Fatalf("stream did not report file failure: HTTP %d: %s", status, body)
				}
			} else if status < 400 || !bytes.Contains(body, []byte(`"error"`)) {
				t.Fatalf("expected a JSON error: HTTP %d: %s", status, body)
			}
			var inputs []im.MessageAttachment
			if stream {
				inputs = sessionResponsePublishedFiles(t, body, true).Inputs
			} else {
				var failure agentSessionErrorEnvelope
				if err := json.Unmarshal(body, &failure); err != nil {
					t.Fatal(err)
				}
				inputs = failure.InputAttachments
			}
			if len(inputs) != 1 {
				t.Fatalf("expected the saved original, got %s", body)
			}
			checkSessionDownload(t, server, workspace, inputs[0], "source.txt", "text/plain", []byte("private input"))
			for _, unpublished := range []string{"private contents", "private input", "first.txt", "unavailable.txt"} {
				if bytes.Contains(body, []byte(unpublished)) {
					t.Errorf("failure exposed %q: %s", unpublished, body)
				}
			}
		})
	}
}

func sessionOutputTestServer(t *testing.T, engine *fakeSessionEngine, customWorkspace string) (*httptest.Server, string) {
	t.Helper()
	item := sessionCodexAgent("agent-alpha", "Alpha")
	if customWorkspace != "" {
		item.RuntimeOptions = map[string]any{"local_workspace_dir": customWorkspace}
	}
	h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{item}, engine, "")
	h.serverAccessToken = "test-server-token"
	workspace, err := h.workspace.WorkspaceRootByID("agent-alpha")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h.Routes())
	t.Cleanup(server.Close)
	if customWorkspace != "" {
		return server, customWorkspace
	}
	return server, workspace
}

func createSessionTestOutput(t *testing.T, files agentengine.FileInterface, name, mediaType string, data []byte) agentengine.OutputFile {
	t.Helper()
	file, err := files.Create(context.Background(), agentengine.FileCreateRequest{Name: name, MIMEType: mediaType, SizeBytes: int64(len(data))}, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func sessionFileHTTP(t *testing.T, server *httptest.Server, method, path, contentType string, body io.Reader) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data
}

type publishedSessionFiles struct {
	Inputs  []im.MessageAttachment `json:"input_attachments"`
	Outputs []im.MessageAttachment `json:"attachments"`
}

func sessionResponsePublishedFiles(t *testing.T, body []byte, stream bool) publishedSessionFiles {
	t.Helper()
	if !stream {
		var response struct {
			Inputs []im.MessageAttachment `json:"input_attachments"`
			Output []struct {
				Role        string
				Attachments []im.MessageAttachment
			}
		}
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatal(err)
		}
		if len(response.Output) != 1 || response.Output[0].Role != "assistant" {
			t.Fatalf("missing assistant output: %s", body)
		}
		return publishedSessionFiles{Inputs: response.Inputs, Outputs: response.Output[0].Attachments}
	}
	var files publishedSessionFiles
	for _, block := range strings.Split(string(body), "\n\n") {
		var event struct {
			Type string
			publishedSessionFiles
		}
		for _, line := range strings.Split(block, "\n") {
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					t.Fatal(err)
				}
			}
		}
		switch event.Type {
		case "message_start":
			if len(event.Outputs) != 0 {
				t.Fatalf("start event contains output files: %s", block)
			}
			files.Inputs = event.Inputs
		case "message_delta":
			if !bytes.Contains([]byte(block), []byte(`"stop_reason":"error"`)) {
				if !reflect.DeepEqual(event.Inputs, files.Inputs) {
					t.Fatalf("initial and final input attachments differ: %s", block)
				}
				files.Outputs = event.Outputs
			}
		default:
			if event.Inputs != nil || event.Outputs != nil {
				t.Fatalf("unexpected attachment event: %s", block)
			}
		}
		if event.Type == "message_stop" {
			return files
		}
	}
	t.Fatal("stream ended without message_stop")
	return publishedSessionFiles{}
}

func TestAgentSessionOriginalsAvailableDuringTurnAndAfterCancellation(t *testing.T) {
	started := make(chan struct{})
	engine := &fakeSessionEngine{run: func(ctx context.Context, _ string, _ agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		close(started)
		<-ctx.Done()
		return agentengine.TurnResult{Status: agentengine.TurnCanceled, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: "canceled"}}
	}}
	server, workspace := sessionOutputTestServer(t, engine, "")
	original := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff" +
		"\x2c\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02\x44\x01\x00\x3b")
	upload, typ := multipartMessageBodyForTest(t, map[string]any{"stream": true}, "files", "original.gif", "image/gif", original)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+multipartSessionPath, upload)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", typ)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	event, err := reader.ReadString('\n')
	if err != nil || event != "event: message_start\n" {
		t.Fatalf("first event: %q, %v", event, err)
	}
	data, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var start publishedSessionFiles
	if err := json.Unmarshal([]byte(strings.TrimPrefix(data, "data: ")), &start); err != nil || len(start.Inputs) != 1 {
		t.Fatalf("start lacks input attachment: %s, %v", data, err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("turn did not start")
	}
	checkSessionDownload(t, server, workspace, start.Inputs[0], "original.gif", "image/gif", original)

	preview, err := server.Client().Get(server.URL + start.Inputs[0].PreviewURL)
	if err != nil {
		t.Fatal(err)
	}
	preview.Body.Close()
	if preview.StatusCode != http.StatusOK || !strings.HasPrefix(preview.Header.Get("Content-Disposition"), "inline") {
		t.Fatalf("inline preview: HTTP %d, disposition %q", preview.StatusCode, preview.Header.Get("Content-Disposition"))
	}

	busyOriginal := []byte("second upload")
	busyBody, busyType := multipartMessageBodyForTest(t, map[string]any{}, "files", "busy.txt", "text/plain", busyOriginal)
	status, busyResponse := sessionFileHTTP(t, server, http.MethodPost, multipartSessionPath, busyType, busyBody)
	var busy agentSessionErrorEnvelope
	if err := json.Unmarshal(busyResponse, &busy); err != nil || status != http.StatusConflict || busy.Error.Code != "session_busy" || len(busy.InputAttachments) != 1 {
		t.Fatalf("busy response: HTTP %d, %s, %v", status, busyResponse, err)
	}
	checkSessionDownload(t, server, workspace, busy.InputAttachments[0], "busy.txt", "text/plain", busyOriginal)

	status, _ = sessionFileHTTP(t, server, http.MethodPost, multipartSessionPath+"/cancel", "", nil)
	if status != http.StatusNoContent {
		t.Fatalf("cancel: HTTP %d", status)
	}
	rest, err := io.ReadAll(reader)
	if err != nil || !bytes.Contains(rest, []byte("event: error\n")) {
		t.Fatalf("canceled stream: %s, %v", rest, err)
	}
	checkSessionDownload(t, server, workspace, start.Inputs[0], "original.gif", "image/gif", original)
}
