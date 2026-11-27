package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientUploadsToPortalAndSubmitsJSON(t *testing.T) {
	uploaded := false
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("user_token")
		if err != nil || cookie.Value != "test-token" {
			t.Error("missing storage proxy credential")
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Fatal(err)
		}
		defer r.MultipartForm.RemoveAll()
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if header.Filename == "feedback-1.zip" && string(data) == "archive" {
			if r.URL.RequestURI() != ArchiveUploadEndpoint {
				t.Errorf("unexpected archive upload URL: %s", r.URL)
			}
			io.WriteString(w, `{"url":"https://files.example/diagnostic.zip"}`)
			return
		}
		if r.URL.RequestURI() != UploadEndpoint {
			t.Errorf("unexpected image upload URL: %s", r.URL)
		}
		if header.Filename != "feedback-0.png" || string(data) != "image" {
			t.Error(string(data))
		}
		uploaded = true
		io.WriteString(w, `{"url":"https://images.example/test.png"}`)
	}))
	defer portal.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !uploaded || r.URL.Path != Endpoint || r.Header.Get("Content-Type") != "application/json" {
			t.Error("incorrect submission sequence or format")
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing authentication")
		}
		var payload struct {
			Description    string   `json:"problem_description"`
			ScreenshotURLs []string `json:"screenshot_urls"`
			UserName       string   `json:"user_name"`
			CSGClaw        struct {
				SiteURL string `json:"site_url"`
				Version string `json:"version"`
			} `json:"csgclaw"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(payload.Description, "- [feedback-1.zip](<https://files.example/diagnostic.zip>)\n\nTest\n```text\nlog") {
			t.Errorf("archive link must precede the unfinished code fence: %q", payload.Description)
		}
		if len(payload.ScreenshotURLs) != 1 || payload.ScreenshotURLs[0] != "https://images.example/test.png" || payload.CSGClaw.SiteURL != portal.URL || payload.CSGClaw.Version != "v0.8.0" || payload.UserName != "alice" {
			t.Errorf("payload=%+v", payload)
		}
		io.WriteString(w, `{"success":true,"issue_id":42}`)
	}))
	defer upstream.Close()
	client := Client{BaseURL: upstream.URL, Token: "test-token"}
	result, err := client.Submit(context.Background(), Submission{Description: "Test\n```text\nlog", SiteURL: portal.URL, Version: "v0.8.0", Channel: "release", UserName: "alice", Images: []Image{{Data: []byte("image"), Extension: "png"}}, Archives: []Archive{{Data: []byte("archive"), Extension: "zip"}}})
	if err != nil || result.IssueID != 42 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestClientUploadFailurePreventsIssueCreation(t *testing.T) {
	for _, body := range []string{`{"url":""}`, `{"url":"javascript:alert(1)"}`, `invalid JSON`} {
		t.Run(body, func(t *testing.T) {
			portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
			defer portal.Close()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("created issue after failed upload") }))
			defer upstream.Close()
			_, err := (&Client{BaseURL: upstream.URL}).Submit(context.Background(), Submission{SiteURL: portal.URL, Images: []Image{{Data: []byte("image"), Extension: "png"}}})
			var uploadErr *UploadError
			if !errors.As(err, &uploadErr) || uploadErr.Kind != "image" {
				t.Fatalf("expected image upload error, got %v", err)
			}
		})
	}
}

func TestClientRejectsRedirect(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer source.Close()
	for _, images := range [][]Image{nil, {{Data: []byte("image"), Extension: "png"}}} {
		_, err := (&Client{BaseURL: source.URL, Token: "test-token"}).Submit(context.Background(), Submission{SiteURL: source.URL, Images: images})
		if err == nil || redirected {
			t.Fatalf("err=%v redirected=%v", err, redirected)
		}
	}
}

func TestClientCapturesBoundedAndRedactedHTTPErrorResponse(t *testing.T) {
	const token = "test-private-feedback-token"
	for _, suffix := range []string{"", strings.Repeat("x", maxErrorResponseBytes)} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Request-ID", "apisix-request-123")
			w.Header().Set("X-Correlation-ID", "correlation-456")
			w.Header().Set("Set-Cookie", "session="+token)
			w.Header().Set("X-Unknown-Header", token)
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"detail":"GitLab rejected the issue: https://files.example/diagnostic.zip?X-Amz-Signature=private-signature&Expires=123","escaped_url":"https:\/\/files.example\/data?token=escaped-secret","credential":"`+token+`"}`+suffix)
		}))
		_, err := (&Client{BaseURL: upstream.URL, Token: token}).Submit(context.Background(), Submission{})
		upstream.Close()
		var upstreamErr *HTTPError
		if !errors.As(err, &upstreamErr) || upstreamErr.Status != http.StatusInternalServerError {
			t.Fatalf("expected upstream HTTP 500, got %v", err)
		}
		if !strings.Contains(upstreamErr.ResponseBody, "GitLab rejected the issue") || strings.Contains(upstreamErr.ResponseBody, token) {
			t.Fatalf("incorrect error response: %q", upstreamErr.ResponseBody)
		}
		if !strings.Contains(upstreamErr.ResponseBody, "https://files.example/diagnostic.zip?[redacted]") ||
			strings.Contains(upstreamErr.ResponseBody, "private-signature") || strings.Contains(upstreamErr.ResponseBody, "escaped-secret") {
			t.Errorf("signed URL was not redacted: %q", upstreamErr.ResponseBody)
		}
		for name, want := range map[string]string{
			"X-Request-ID": "apisix-request-123", "X-Correlation-ID": "correlation-456",
			"Set-Cookie": "[redacted]", "X-Unknown-Header": "[redacted]",
		} {
			if got := upstreamErr.ResponseHeaders.Get(name); got != want {
				t.Errorf("header %s = %q, want %q", name, got, want)
			}
		}
		if upstreamErr.ResponseBodyTruncated != (suffix != "") {
			t.Errorf("unexpected truncation: %v", upstreamErr.ResponseBodyTruncated)
		}
	}
}

func TestClientRedactsCredentialAcrossErrorBodyLimit(t *testing.T) {
	const token = "private-feedback-session-credential"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, strings.Repeat(" ", maxErrorResponseBytes-12)+token)
	}))
	defer upstream.Close()

	_, err := (&Client{BaseURL: upstream.URL, Token: token}).Submit(context.Background(), Submission{})
	var upstreamErr *HTTPError
	if !errors.As(err, &upstreamErr) {
		t.Fatalf("expected HTTP error, got %v", err)
	}
	if !upstreamErr.ResponseBodyTruncated || upstreamErr.ResponseBody != "[redacted]" {
		t.Fatal("truncated response must contain only the redaction marker")
	}
}
