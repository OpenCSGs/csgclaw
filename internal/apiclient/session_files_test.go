package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestUploadSessionFilePreservesImageMediaType(t *testing.T) {
	const data = "verified image bytes"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, header, err := r.FormFile("files")
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		got, err := io.ReadAll(file)
		if err != nil || string(got) != data || header.Header.Get("Content-Type") != "image/png" || header.Filename != "截图.png" {
			t.Errorf("upload = %q, type %q, name %q, error %v", got, header.Header.Get("Content-Type"), header.Filename, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"attachments":[{"id":"image","media_type":"image/png"}]}`)
	}))
	defer server.Close()
	file, err := os.CreateTemp(t.TempDir(), "image")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(data); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	attachments, err := New(server.URL, "test-token", server.Client()).UploadSessionFile(context.Background(), "/attachments", "截图.png", "image/png", file)
	if err != nil || len(attachments) != 1 || attachments[0].MediaType != "image/png" {
		t.Fatalf("attachments = %+v, %v", attachments, err)
	}
}
