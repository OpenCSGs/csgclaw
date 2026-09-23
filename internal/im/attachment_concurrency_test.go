package im

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type gatedAttachmentReader struct {
	io.Reader
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *gatedAttachmentReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.entered); <-r.release })
	return r.Reader.Read(p)
}
func (r *gatedAttachmentReader) Close() error { return nil }

func TestAttachmentPreparationDoesNotBlockOtherRooms(t *testing.T) {
	for _, delivery := range []bool{false, true} {
		name := "upload"
		if delivery {
			name = "generated file"
		}
		t.Run(name, func(t *testing.T) {
			service, err := NewServiceFromPath(filepath.Join(t.TempDir(), "im", "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			room, err := service.CreateRoom(CreateRoomRequest{Title: "files", CreatorID: "user-admin"})
			if err != nil {
				t.Fatal(err)
			}
			other, err := service.CreateRoom(CreateRoomRequest{Title: "other", CreatorID: "user-admin"})
			if err != nil {
				t.Fatal(err)
			}
			reader := &gatedAttachmentReader{Reader: bytes.NewReader([]byte("data")), entered: make(chan struct{}), release: make(chan struct{})}
			finished := make(chan error, 1)
			go func() {
				if delivery {
					_, err := service.DeliverMessage(DeliverMessageRequest{RoomID: room.ID, SenderID: "user-admin", AttachmentSources: []AttachmentSource{{Name: "video.mp4", MediaType: "video/mp4", SizeBytes: 4, Reader: reader}}})
					finished <- err
				} else {
					_, err := service.CreateMessage(CreateMessageRequest{RoomID: room.ID, SenderID: "user-admin", Attachments: []MessageAttachmentUpload{{Name: "video.mp4", SizeBytes: 4, Open: func() (io.ReadCloser, error) { return reader, nil }}}})
					finished <- err
				}
			}()
			<-reader.entered
			otherDone := make(chan error, 1)
			go func() {
				_, err := service.CreateMessage(CreateMessageRequest{RoomID: other.ID, SenderID: "user-admin", Content: "still responsive"})
				otherDone <- err
			}()
			blocked := false
			select {
			case err := <-otherDone:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(time.Second):
				blocked = true
			}
			close(reader.release)
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
			if blocked {
				if err := <-otherDone; err != nil {
					t.Error(err)
				}
				t.Fatal("attachment preparation blocked another room")
			}
		})
	}
}

func newAttachmentTestService(t *testing.T) (*Service, Room) {
	t.Helper()
	service, err := NewServiceFromPath(filepath.Join(t.TempDir(), "im", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	room, err := service.CreateRoom(CreateRoomRequest{Title: "files", CreatorID: "user-admin"})
	if err != nil {
		t.Fatal(err)
	}
	return service, room
}

func TestConcurrentAttachmentUploadsDeduplicateMessagesAndBlobs(t *testing.T) {
	service, room := newAttachmentTestService(t)
	const workers = 16
	payload := []byte("shared attachment content")
	start := make(chan struct{})
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func(index int) {
			<-start
			message, _, err := service.CreateMessageOnce(CreateMessageRequest{RoomID: room.ID, SenderID: "user-admin", ClientMessageID: fmt.Sprintf("request-%d", index%4), Attachments: []MessageAttachmentUpload{{Name: "shared.bin", Data: payload}}})
			if err == nil && len(message.Attachments) != 1 {
				err = fmt.Errorf("missing attachment")
			}
			results <- err
		}(i)
	}
	close(start)
	for i := 0; i < workers; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	stored, ok := service.Room(room.ID)
	if !ok {
		t.Fatal("room missing")
	}
	count := 0
	for _, message := range stored.Messages {
		if message.ClientMessageID == "" {
			continue
		}
		count++
		attachment, err := service.AttachmentFile(message.Attachments[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(attachment.Path)
		if err != nil || !bytes.Equal(content, payload) {
			t.Fatalf("persisted bytes=%q, error=%v", content, err)
		}
	}
	if count != 4 {
		t.Fatalf("stored messages=%d, want 4", count)
	}
	pending, err := filepath.Glob(filepath.Join(filepath.Dir(service.statePath), assetsDirName, ".attachment-*"))
	if err != nil || len(pending) != 0 {
		t.Fatalf("staging leaked: %v, %v", pending, err)
	}
}

func TestAttachmentPreparationSurvivesConcurrentRoomCleanup(t *testing.T) {
	service, room := newAttachmentTestService(t)
	payload := []byte("shared attachment content")
	if _, err := service.CreateMessage(CreateMessageRequest{RoomID: room.ID, SenderID: "user-admin", Attachments: []MessageAttachmentUpload{{Name: "shared.bin", Data: payload}}}); err != nil {
		t.Fatal(err)
	}
	reader := &gatedAttachmentReader{Reader: bytes.NewReader([]byte("tail")), entered: make(chan struct{}), release: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		_, err := service.DeliverMessage(DeliverMessageRequest{RoomID: room.ID, SenderID: "user-admin", MessageID: "pending-files", AttachmentSources: []AttachmentSource{
			{Name: "shared.bin", SizeBytes: int64(len(payload)), Reader: io.NopCloser(bytes.NewReader(payload))},
			{Name: "tail.bin", SizeBytes: 4, Reader: reader},
		}})
		result <- err
	}()
	<-reader.entered
	cleared := make(chan error, 1)
	go func() { _, err := service.ClearRoomMessages(room.ID); cleared <- err }()
	select {
	case err := <-cleared:
		close(reader.release)
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		close(reader.release)
		<-result
		<-cleared
		t.Fatal("pending file blocked room cleanup")
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	stored, _ := service.Room(room.ID)
	for _, message := range stored.Messages {
		if message.ID != "pending-files" {
			continue
		}
		if len(message.Attachments) != 2 {
			t.Fatal("missing pending attachments")
		}
		file, err := service.AttachmentFile(message.Attachments[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(file.Path)
		if err != nil || !bytes.Equal(content, payload) {
			t.Fatalf("pending attachment lost: %q %v", content, err)
		}
		return
	}
	t.Fatal("pending message missing")
}

func TestDuplicateAttachmentReusesOpenBlobAndRepairsCorruption(t *testing.T) {
	service, room := newAttachmentTestService(t)
	request := CreateMessageRequest{RoomID: room.ID, SenderID: "user-admin", Attachments: []MessageAttachmentUpload{{Name: "shared.bin", Data: []byte("original")}}}
	message, err := service.CreateMessage(request)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := service.AttachmentFile(message.Attachments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	download, err := os.Open(blob.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer download.Close()
	before, err := download.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateMessage(request); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(blob.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("duplicate upload replaced a blob open for download")
	}
	download.Close()
	if err := os.WriteFile(blob.Path, []byte("corrupt!"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateMessage(request); err != nil {
		t.Fatal(err)
	}
	repaired, err := os.ReadFile(blob.Path)
	if err != nil || string(repaired) != "original" {
		t.Fatalf("blob repair=%q, error=%v", repaired, err)
	}
}

func TestFailedAttachmentPreparationDiscardsStaging(t *testing.T) {
	service, room := newAttachmentTestService(t)
	_, err := service.CreateMessage(CreateMessageRequest{RoomID: room.ID, SenderID: "user-admin", Attachments: []MessageAttachmentUpload{
		{Name: "valid.bin", Data: []byte("valid")},
		{Name: "truncated.bin", SizeBytes: 4, Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader([]byte("bad"))), nil }},
	}})
	if err == nil {
		t.Fatal("expected truncated upload to fail")
	}
	pending, err := filepath.Glob(filepath.Join(filepath.Dir(service.statePath), assetsDirName, ".attachment-*"))
	if err != nil || len(pending) != 0 {
		t.Fatalf("staging leaked: %v, %v", pending, err)
	}
	stored, _ := service.Room(room.ID)
	for _, message := range stored.Messages {
		if len(message.Attachments) != 0 {
			t.Fatal("failed upload published an attachment")
		}
	}
}

func TestAttachmentRetryDoesNotReopenConsumedSource(t *testing.T) {
	service, room := newAttachmentTestService(t)
	request := CreateMessageRequest{RoomID: room.ID, SenderID: "user-admin", ClientMessageID: "retry-key", Attachments: []MessageAttachmentUpload{{Name: "file.bin", Data: []byte("file")}}}
	original, _, err := service.CreateMessageOnce(request)
	if err != nil {
		t.Fatal(err)
	}
	opened := false
	request.Attachments = []MessageAttachmentUpload{{Name: "file.bin", SizeBytes: 4, Open: func() (io.ReadCloser, error) { opened = true; return nil, fmt.Errorf("consumed source") }}}
	repeated, created, err := service.CreateMessageOnce(request)
	if err != nil || created || opened || repeated.ID != original.ID {
		t.Fatalf("retry created=%t opened=%t message=%s error=%v", created, opened, repeated.ID, err)
	}
}
