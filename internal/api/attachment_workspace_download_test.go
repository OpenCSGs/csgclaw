package api

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/im"
)

func TestWorkspaceAttachmentDownloadReflectsEditsAndDeletion(t *testing.T) {
	h, room, agentID, root, _ := referenceTestHandler(t)
	h.serverNoAuth = false
	h.serverAccessToken = "test-server-credential"
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"></svg>`
	files, err := h.im.StoreSessionResponseAttachments(t.Context(), agentID, "private-session", "upload", root, []im.AttachmentSource{{
		Name: "graphic.svg", MediaType: "image/svg+xml", SizeBytes: int64(len(svg)), Reader: io.NopCloser(strings.NewReader(svg)),
	}})
	if err != nil {
		t.Fatal(err)
	}
	att := files[0]
	local := filepath.Join(root, filepath.FromSlash(att.WorkspacePath))
	server := httptest.NewServer(h.Routes())
	defer server.Close()
	request := func(method, url string, headers map[string]string, status int) (http.Header, []byte) {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+url, nil)
		if err != nil {
			t.Fatal(err)
		}
		for header, value := range headers {
			req.Header.Set(header, value)
		}
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil || resp.StatusCode != status {
			t.Fatalf("%s %s HTTP %d, want %d: %s %v", method, req.URL.Path, resp.StatusCode, status, body, err)
		}
		return resp.Header, body
	}
	link, err := url.Parse(att.DownloadURL)
	if err != nil || link.Query().Get("token") == "" {
		t.Fatal("missing attachment capability")
	}
	request("GET", link.Path, nil, 401)
	request("HEAD", link.Path+"?token=wrong", nil, 401)
	first, data := request("GET", att.PreviewURL, nil, 200)
	if string(data) != svg || first.Get("Content-Type") != "image/svg+xml" || !strings.HasPrefix(first.Get("Content-Disposition"), "inline") || first.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("initial preview: %v %q", first, data)
	}

	edited := []byte("%PDF-1.4\nupdated document\n")
	if err := os.WriteFile(local, edited, 0600); err != nil {
		t.Fatal(err)
	}
	date := time.Now().UTC().Format(http.TimeFormat)
	headers, data := request("GET", att.DownloadURL, map[string]string{"If-Modified-Since": date}, 200)
	if !bytes.Equal(data, edited) || headers.Get("Content-Type") != "application/pdf" || !strings.HasPrefix(headers.Get("Content-Disposition"), "attachment") || headers.Get("Cache-Control") != "private, no-store" || headers.Get("Content-Length") != strconv.Itoa(len(edited)) || headers.Get("ETag") != "" || headers.Get("X-CSGClaw-File-SHA256") != "" {
		t.Fatalf("edited download: %v %q", headers, data)
	}
	headers, data = request("HEAD", link.Path, map[string]string{"Authorization": "Bearer " + h.serverAccessToken}, 200)
	if len(data) != 0 || headers.Get("Content-Length") != strconv.Itoa(len(edited)) || headers.Get("Content-Type") != "application/pdf" {
		t.Fatalf("HEAD: %v %q", headers, data)
	}
	headers, data = request("GET", att.PreviewURL, map[string]string{"Range": "bytes=0-3"}, 206)
	if string(data) != "%PDF" || headers.Get("Content-Range") != "bytes 0-3/"+strconv.Itoa(len(edited)) {
		t.Fatalf("range: %v %q", headers, data)
	}
	const html = `<!doctype html><html><body><svg width="1"></svg></body></html>`
	if err := os.WriteFile(local, []byte(html), 0600); err != nil {
		t.Fatal(err)
	}
	headers, data = request("GET", att.PreviewURL, nil, 200)
	if string(data) != html || !strings.HasPrefix(headers.Get("Content-Type"), "text/html") || !strings.HasPrefix(headers.Get("Content-Disposition"), "attachment") {
		t.Fatalf("HTML containing SVG was offered as an image: %v", headers)
	}
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	request("GET", link.Path, nil, 401)
	request("GET", att.DownloadURL, nil, 404)
	request("HEAD", att.DownloadURL, nil, 404)
	outside := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(outside, []byte("SECRET"), 0600); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(filepath.Dir(local), "replacement-link")
	if err := os.Symlink(outside, replacement); err == nil {
		if err := os.WriteFile(local, edited, 0600); err != nil {
			t.Fatal(err)
		}
		var swapErr error
		recorder := &workspaceDownloadSwap{ResponseRecorder: httptest.NewRecorder(), swap: func() {
			swapErr = os.Rename(replacement, local)
		}}
		h.Routes().ServeHTTP(recorder, httptest.NewRequest("GET", att.DownloadURL, nil))
		if recorder.Code != 200 || !bytes.Equal(recorder.Body.Bytes(), edited) {
			t.Fatal("replacing the workspace file during download changed the served content")
		}
		if swapErr != nil {
			if runtime.GOOS != "windows" {
				t.Fatal(swapErr)
			}
			// Windows can keep the name locked while a read handle is open.
			if err := os.Remove(local); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(replacement, local); err != nil {
				t.Fatal(err)
			}
		}
		_, data = request("GET", att.DownloadURL, nil, 404)
		if bytes.Contains(data, []byte("SECRET")) {
			t.Fatal("served external symlink contents")
		}
	} else if runtime.GOOS != "windows" {
		t.Fatal(err)
	}

	msg, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: room, SenderID: im.AdminUserID, Attachments: []im.MessageAttachmentUpload{{Name: "room.txt", Data: []byte("immutable room contents")}}})
	if err != nil {
		t.Fatal(err)
	}
	roomFile := msg.Attachments[0]
	headers, _ = request("GET", roomFile.DownloadURL, nil, 200)
	if headers.Get("Cache-Control") != "private, max-age=31536000, immutable" {
		t.Fatalf("room ID cache = %s", headers.Get("Cache-Control"))
	}
	headers, _ = request("GET", "/api/v1/rooms/"+room+"/attachments/"+roomFile.ID, map[string]string{"Authorization": "Bearer " + h.serverAccessToken}, 200)
	if headers.Get("Cache-Control") != "no-store" || headers.Get("X-CSGClaw-File-SHA256") != roomFile.SHA256 {
		t.Fatalf("room download: %v", headers)
	}
}

// ServeContent sets its status after determining the current representation
// and immediately before streaming bytes from the authorized handle.
type workspaceDownloadSwap struct {
	*httptest.ResponseRecorder
	swap func()
}

func (w *workspaceDownloadSwap) WriteHeader(status int) {
	w.swap()
	w.ResponseRecorder.WriteHeader(status)
}
