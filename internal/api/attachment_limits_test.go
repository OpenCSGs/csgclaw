package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"csgclaw/internal/im"
)

func TestAttachmentAboveFormerLimitUploadsAndDownloadsOverHTTP(t *testing.T) {
	imSvc, err := im.NewServiceFromPath(filepath.Join(t.TempDir(), "im", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	worker, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "worker", Name: "worker", Role: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := imSvc.CreateRoom(im.CreateRoomRequest{
		Title: "Large attachment", CreatorID: "user-admin", MemberIDs: []string{worker.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{im: imSvc, participantBridge: im.NewParticipantBridge(""), serverAccessToken: "secret"}
	server := httptest.NewServer(handler.Routes())
	t.Cleanup(server.Close)
	client := server.Client()
	payload := bytes.Repeat([]byte("large-file-data\n"), (26*1024*1024)/16)
	wantHash := sha256.Sum256(payload)
	body, contentType := multipartMessageBodyForTest(t, map[string]any{
		"room_id": room.ID, "sender_id": "user-admin", "content": "",
	}, "files", "large.txt", "text/plain", payload)
	response, err := client.Post(server.URL+"/api/v1/channels/csgclaw/messages", contentType, body)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		t.Fatalf("upload status = %d, want 201; body=%s", response.StatusCode, detail)
	}
	var message im.Message
	if err := json.NewDecoder(response.Body).Decode(&message); err != nil {
		t.Fatal(err)
	}
	if len(message.Attachments) != 1 {
		t.Fatalf("attachment count = %d, want 1", len(message.Attachments))
	}
	attachment := message.Attachments[0]
	if attachment.SizeBytes != int64(len(payload)) || attachment.SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("attachment metadata = %+v, want original size and hash", attachment)
	}

	download, err := client.Get(server.URL + attachment.DownloadURL)
	if err != nil {
		t.Fatal(err)
	}
	defer download.Body.Close()
	gotHash := sha256.New()
	written, err := io.Copy(gotHash, download.Body)
	if err != nil {
		t.Fatal(err)
	}
	if download.StatusCode != http.StatusOK || written != int64(len(payload)) || !bytes.Equal(gotHash.Sum(nil), wantHash[:]) {
		t.Fatalf("download status=%d, bytes=%d, hash=%x, want full original file", download.StatusCode, written, gotHash.Sum(nil))
	}

	const start = 25*1024*1024 - 8
	const end = start + 31
	request, err := http.NewRequest(http.MethodGet, server.URL+attachment.DownloadURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	ranged, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer ranged.Body.Close()
	part, err := io.ReadAll(io.LimitReader(ranged.Body, 1024))
	if err != nil {
		t.Fatal(err)
	}
	wantRange := fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload))
	if ranged.StatusCode != http.StatusPartialContent || ranged.Header.Get("Content-Range") != wantRange || !bytes.Equal(part, payload[start:end+1]) {
		t.Fatalf("range status=%d, Content-Range=%q, bytes=%d, want original range", ranged.StatusCode, ranged.Header.Get("Content-Range"), len(part))
	}
}
