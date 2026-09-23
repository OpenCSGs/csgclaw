package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/im"
)

type downstreamFiles struct {
	agentengine.ConversationInterface
	files agentengine.FileInterface
}

func (s downstreamFiles) Files() agentengine.FileInterface                       { return s.files }
func (s downstreamFiles) Conversations(string) agentengine.ConversationInterface { return s }

type downstreamParticipant struct{ id string }

func (p downstreamParticipant) Get(_, _ string) (apitypes.Participant, bool) {
	return apitypes.Participant{ChannelUserRef: p.id}, true
}

func TestGeneratedLargeFileDeliveryPreviewAndDownload(t *testing.T) {
	const size = int64(291 * 1024 * 1024)
	ctx := context.Background()
	root := t.TempDir()
	source, err := os.Create(filepath.Join(root, "video.mp4"))
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
	files := agentengine.NewFileStore().Scope("worker")
	created, err := files.Create(ctx, agentengine.FileCreateRequest{Name: "video.mp4", MIMEType: "video/mp4", SizeBytes: size}, source)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Delete(ctx, created.ID)
	statePath := filepath.Join(root, "im", "state.json")
	service, err := im.NewServiceFromPath(statePath)
	if err != nil {
		t.Fatal(err)
	}
	worker, _, err := service.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "worker", Name: "worker", Role: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := service.CreateRoom(im.CreateRoomRequest{Title: "Large file", CreatorID: "user-admin", MemberIDs: []string{worker.ID}})
	if err != nil {
		t.Fatal(err)
	}
	store, err := delivery.NewIMTranscriptStore(service, downstreamParticipant{worker.ID}, downstreamFiles{files: files})
	if err != nil {
		t.Fatal(err)
	}
	err = delivery.NewTranscriptRenderer(store).Complete(ctx, channel.TurnContext{AgentID: "worker", ParticipantID: "worker", RoomID: room.ID, TurnID: "large-file"}, agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Ready", Files: []agentengine.OutputFile{created}})
	if err != nil {
		t.Fatal(err)
	}
	// Downloads must survive both engine cleanup and an IM restart.
	if err := files.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	service, err = im.NewServiceFromPath(statePath)
	if err != nil {
		t.Fatal(err)
	}
	room, ok := service.Room(room.ID)
	if !ok {
		t.Fatal("room missing")
	}
	var attachment im.MessageAttachment
	for _, message := range room.Messages {
		if message.ID == "large-file-final" && len(message.Attachments) == 1 {
			attachment = message.Attachments[0]
		}
	}
	if attachment.SizeBytes != size {
		t.Fatalf("generated attachment size=%d, want %d", attachment.SizeBytes, size)
	}
	server := httptest.NewServer((&Handler{im: service, serverAccessToken: "test-token"}).Routes())
	defer server.Close()
	response, err := http.Get(server.URL + attachment.DownloadURL)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	downloaded, copyErr := io.Copy(hash, response.Body)
	response.Body.Close()
	if copyErr != nil || response.StatusCode != http.StatusOK || downloaded != size || hex.EncodeToString(hash.Sum(nil)) != created.SHA256 {
		t.Fatalf("download status=%d, bytes=%d, error=%v", response.StatusCode, downloaded, copyErr)
	}
	request, err := http.NewRequest(http.MethodGet, server.URL+attachment.DownloadURL+"&inline=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Range", "bytes=-4")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	tail, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusPartialContent || string(tail) != "TAIL" || response.Header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", size-4, size-1, size) {
		t.Fatalf("preview range status=%d, tail=%q, error=%v", response.StatusCode, tail, err)
	}
}
