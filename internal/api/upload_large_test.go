package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/im"
)

// Keep the upload body streamed so the test also exercises multipart disk staging.
func streamedAttachmentRequest(t *testing.T, endpoint string, payload any, source io.Reader, size int64) *http.Request {
	t.Helper()
	var envelope bytes.Buffer
	writer := multipart.NewWriter(&envelope)
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("payload", string(data)); err != nil {
		t.Fatal(err)
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="files"; filename="video.mp4"`)
	header.Set("Content-Type", "video/mp4")
	if _, err := writer.CreatePart(header); err != nil {
		t.Fatal(err)
	}
	prefix := append([]byte(nil), envelope.Bytes()...)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	suffix := append([]byte(nil), envelope.Bytes()[len(prefix):]...)
	request, err := http.NewRequest(http.MethodPost, endpoint, io.MultiReader(bytes.NewReader(prefix), io.LimitReader(source, size), bytes.NewReader(suffix)))
	if err != nil {
		t.Fatal(err)
	}
	request.ContentLength = int64(len(prefix)) + size + int64(len(suffix))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func TestLargeChatUploadReachesWorkspace(t *testing.T) {
	for _, size := range []int64{291 * 1024 * 1024, 1024 * 1024 * 1024} {
		t.Run(fmt.Sprintf("%d_MiB", size/(1024*1024)), func(t *testing.T) { checkLargeChatUploadReachesWorkspace(t, size) })
	}
}

func checkLargeChatUploadReachesWorkspace(t *testing.T, size int64) {
	multipartDir := t.TempDir()
	t.Setenv("TMPDIR", multipartDir)
	root := t.TempDir()
	source, err := os.Create(filepath.Join(root, "source.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if err := source.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if _, err := source.WriteAt([]byte("TAIL"), size-4); err != nil {
		t.Fatal(err)
	}
	service, err := im.NewServiceFromPath(filepath.Join(root, "im", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	room, err := service.CreateRoom(im.CreateRoomRequest{Title: "Upload", CreatorID: "user-admin"})
	if err != nil {
		t.Fatal(err)
	}
	routes := (&Handler{im: service, participantBridge: im.NewParticipantBridge("")}).Routes()
	completed := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routes.ServeHTTP(w, r)
		completed <- struct{}{}
	}))
	defer server.Close()
	hash := sha256.New()
	request := streamedAttachmentRequest(t, server.URL+"/api/v1/channels/csgclaw/messages", map[string]string{"room_id": room.ID, "sender_id": "user-admin"}, io.TeeReader(source, hash), size)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("upload status=%d, body=%s", response.StatusCode, body)
	}
	var message im.Message
	if err := json.NewDecoder(response.Body).Decode(&message); err != nil {
		t.Fatal(err)
	}
	if len(message.Attachments) != 1 || message.Attachments[0].SizeBytes != size || message.Attachments[0].SHA256 != hex.EncodeToString(hash.Sum(nil)) {
		t.Fatalf("unexpected attachments: %+v", message.Attachments)
	}
	<-completed
	assertNoMultipartStaging(t, multipartDir)
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	attachment, err := service.MaterializeAttachment(message.Attachments[0].ID, workspace, "videos")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.Open(filepath.Join(workspace, filepath.FromSlash(attachment.WorkspacePath)))
	if err != nil {
		t.Fatal(err)
	}
	defer saved.Close()
	savedHash := sha256.New()
	written, err := io.Copy(savedHash, saved)
	if err != nil || written != size || !bytes.Equal(hash.Sum(nil), savedHash.Sum(nil)) {
		t.Fatalf("workspace file size=%d, error=%v", written, err)
	}
}

func TestMultipartUploadSizeLimits(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sizes    []int64
		rejected bool
	}{
		{"single file boundary", []int64{1 << 30}, false},
		{"single file oversized", []int64{(1 << 30) + 1}, true},
		{"message boundary", []int64{1 << 30, 1 << 30}, false},
		{"message oversized", []int64{1 << 30, 1 << 30, 1}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			form := &multipart.Form{File: map[string][]*multipart.FileHeader{}}
			for i, size := range tc.sizes {
				name := fmt.Sprintf("video-%d.mp4", i)
				header := textproto.MIMEHeader{}
				header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files"; filename="%s"`, name))
				form.File["files"] = append(form.File["files"], &multipart.FileHeader{Filename: name, Header: header, Size: size})
			}
			uploads, err := readMultipartAttachmentUploads(form)
			if tc.rejected {
				if !errors.Is(err, errAttachmentPayloadTooLarge) {
					t.Fatalf("error=%v, want payload too large", err)
				}
			} else {
				if err != nil || len(uploads) != len(tc.sizes) {
					t.Fatalf("uploads=%d, error=%v", len(uploads), err)
				}
				for i, upload := range uploads {
					if upload.Open == nil || upload.Data != nil || upload.SizeBytes != tc.sizes[i] {
						t.Fatalf("upload %d did not retain its streamed source", i)
					}
				}
			}
		})
	}
}

func TestRejectedChatUploadCleansMultipartStaging(t *testing.T) {
	multipartDir := t.TempDir()
	t.Setenv("TMPDIR", multipartDir)
	const size = int64(9 * 1024 * 1024)
	source, err := os.CreateTemp(t.TempDir(), "rejected-upload-*")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if err := source.Truncate(size); err != nil {
		t.Fatal(err)
	}
	service, err := im.NewServiceFromPath(filepath.Join(t.TempDir(), "im", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	request := streamedAttachmentRequest(t, "/api/v1/channels/csgclaw/messages", map[string]string{"sender_id": "user-admin"}, source, size)
	response := httptest.NewRecorder()
	(&Handler{im: service, participantBridge: im.NewParticipantBridge("")}).Routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want missing room rejection", response.Code)
	}
	assertNoMultipartStaging(t, multipartDir)
}

func assertNoMultipartStaging(t *testing.T, dir string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "multipart-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("multipart staging files=%v, error=%v", files, err)
	}
}
