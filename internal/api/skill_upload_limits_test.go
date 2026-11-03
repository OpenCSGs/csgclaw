package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	skilllocal "csgclaw/internal/skill/local"
)

func TestSkillUploadArchiveLimit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, size := range []int64{skilllocal.MaxArchiveBytes + 1, skilllocal.MaxArchiveBytes + (1 << 20) + 1} {
		request := skillArchiveUploadRequest(t, io.LimitReader(zeroReader{}, size))
		recorder := httptest.NewRecorder()
		(&Handler{}).Routes().ServeHTTP(recorder, request)
		var result struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if recorder.Code != http.StatusRequestEntityTooLarge || json.Unmarshal(recorder.Body.Bytes(), &result) != nil || result.Error.Code != "payload_too_large" {
			t.Fatalf("size=%d: status=%d body=%s", size, recorder.Code, recorder.Body)
		}
	}
}

func TestSkillUploadRejectsMalformedZIP(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	recorder := httptest.NewRecorder()
	(&Handler{}).Routes().ServeHTTP(recorder, skillArchiveUploadRequest(t, bytes.NewBufferString("invalid zip")))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
}

func TestSkillUploadAcceptsArchiveAtLimit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeArchive := func(target io.Writer, padding int64) {
		t.Helper()
		archive := zip.NewWriter(target)
		for _, entry := range []struct {
			name string
			body io.Reader
		}{
			{"SKILL.md", bytes.NewBufferString("# Upload boundary\n")},
			{"payload.bin", io.LimitReader(zeroReader{}, padding)},
		} {
			file, err := archive.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Store})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.Copy(file, entry.body); err != nil {
				t.Fatal(err)
			}
		}
		if err := archive.Close(); err != nil {
			t.Fatal(err)
		}
	}
	var empty bytes.Buffer
	writeArchive(&empty, 0)
	file, err := os.CreateTemp(t.TempDir(), "archive-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writeArchive(file, skilllocal.MaxArchiveBytes-int64(empty.Len()))
	if info, err := file.Stat(); err != nil || info.Size() != skilllocal.MaxArchiveBytes {
		t.Fatalf("archive size: info=%v error=%v", info, err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	(&Handler{}).Routes().ServeHTTP(recorder, skillArchiveUploadRequest(t, file))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
}

func skillArchiveUploadRequest(t *testing.T, archive io.Reader) *http.Request {
	t.Helper()
	var envelope bytes.Buffer
	writer := multipart.NewWriter(&envelope)
	if _, err := writer.CreateFormFile("file", "boundary.zip"); err != nil {
		t.Fatal(err)
	}
	header := bytes.Clone(envelope.Bytes())
	envelope.Reset()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/skills:upload", io.MultiReader(bytes.NewReader(header), archive, bytes.NewReader(envelope.Bytes())))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}
