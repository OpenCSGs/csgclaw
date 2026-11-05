package im

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sessionAttachmentSource(name, content string) AttachmentSource {
	return AttachmentSource{Name: name, MediaType: "text/plain", SizeBytes: int64(len(content)), Reader: io.NopCloser(strings.NewReader(content))}
}

func TestSessionAttachmentFileChecksOwnershipAndReadsSnapshot(t *testing.T) {
	s, room := newAttachmentTestService(t)
	workspace := t.TempDir()
	stored, err := s.StoreSessionResponseAttachments("worker", "session", "response", workspace, []AttachmentSource{sessionAttachmentSource("input.txt", "original")})
	if err != nil {
		t.Fatal(err)
	}
	file := stored[0]
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(file.WorkspacePath)), []byte("workspace edit"), 0600); err != nil {
		t.Fatal(err)
	}
	roomMessage, err := s.DeliverMessage(DeliverMessageRequest{
		RoomID: room.ID, SenderID: AdminUserID, MessageID: "room-output", AttachmentSources: []AttachmentSource{sessionAttachmentSource("input.txt", "original")},
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err = NewServiceFromPath(s.statePath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.SessionAttachmentFile("worker", "session", file.ID)
	if err != nil || got.Attachment.ID != file.ID {
		t.Fatalf("lookup after restart: %+v, %v", got.Attachment, err)
	}
	data, err := os.ReadFile(got.Path)
	if err != nil || string(data) != "original" {
		t.Fatalf("snapshot: %q, %v", data, err)
	}
	for _, tc := range []struct{ name, agentID, sessionID, attachmentID string }{
		{"different agent", "another-worker", "session", file.ID},
		{"different session", "worker", "another-session", file.ID},
		{"room attachment", AdminUserID, "session", roomMessage.Attachments[0].ID},
		{"empty session", AdminUserID, "", roomMessage.Attachments[0].ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.SessionAttachmentFile(tc.agentID, tc.sessionID, tc.attachmentID); !errors.Is(err, errAttachmentNotFound) {
				t.Fatalf("unauthorized lookup: %v", err)
			}
		})
	}
	// Older objects retain their download but have no stored session identity.
	// Their workspace path alone must not authorize use in a new request.
	path := attachmentObjectPath(s.statePath, file.ID)
	object, err := readAttachmentObject(path)
	if err != nil {
		t.Fatal(err)
	}
	object.SessionID = ""
	if err := writeAttachmentObject(path, object); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionAttachmentFile("worker", "session", file.ID); !errors.Is(err, errAttachmentNotFound) {
		t.Fatalf("unscoped object: %v", err)
	}
	if _, err := s.AttachmentFile(file.ID); err != nil {
		t.Fatalf("existing download: %v", err)
	}
}

func TestSessionResponseAttachmentsRetainDownloadsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	// A response can include the maximum input set and additional generated files.
	var files []AttachmentSource
	for i := range MaxAttachmentsPerMessage + 1 {
		files = append(files, sessionAttachmentSource(fmt.Sprintf("document-%d.txt", i), "contents"))
	}
	workspace := filepath.Join(t.TempDir(), "workspace")
	attachments, err := s.StoreSessionResponseAttachments("worker", "session", "response", workspace, files)
	if err != nil || len(attachments) != len(files) {
		t.Fatalf("stored %d attachments: %v", len(attachments), err)
	}
	s, err = NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if rooms := s.Bootstrap().Rooms; len(rooms) != 0 {
		t.Fatalf("direct response created rooms: %+v", rooms)
	}
	// A room checkpoint collects abandoned attachments. Live direct-session
	// files have an independent reference even when no room mentions them.
	if _, err := s.CreateRoom(CreateRoomRequest{Title: "other", CreatorID: AdminUserID}); err != nil {
		t.Fatal(err)
	}
	// Onboarding can save the bootstrap from a separate Service instance.
	if err := EnsureBootstrapState(path); err != nil {
		t.Fatal(err)
	}
	tokens := make(map[string]bool)
	for i, attachment := range attachments {
		got, err := s.AttachmentFile(attachment.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Attachment.DownloadURL != attachment.DownloadURL || got.Attachment.Name != files[i].Name ||
			got.Attachment.WorkspacePath != attachment.WorkspacePath {
			t.Fatalf("unavailable or unexpected metadata for file %d", i)
		}
		download, err := url.Parse(attachment.DownloadURL)
		if err != nil {
			t.Fatal(err)
		}
		token := download.Query().Get("token")
		if tokens[token] || download.Path != "/api/v1/attachments/"+attachment.ID || !got.AuthorizesDownloadToken(token) || got.AuthorizesDownloadToken("invalid") {
			t.Fatalf("download capability mismatch for file %d", i)
		}
		tokens[token] = true
		wantPath := filepath.Join(".csgclaw", "attachments", "sessions", "session", "response", attachment.ID+"-"+attachment.Name)
		if attachment.WorkspacePath != filepath.ToSlash(wantPath) {
			t.Fatalf("unexpected workspace path %q", attachment.WorkspacePath)
		}
		localPath := filepath.Join(workspace, wantPath)
		saved, err := os.ReadFile(localPath)
		if err != nil || string(saved) != "contents" {
			t.Fatalf("workspace file %q: contents=%q, error=%v", localPath, saved, err)
		}
		if err := os.WriteFile(localPath, []byte("edited copy"), 0600); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(got.Path)
		if err != nil || string(data) != "contents" {
			t.Fatalf("downloaded contents=%q, error=%v", data, err)
		}
	}
}

func TestDeletingRoomAttachmentRetainsSharedSessionDownload(t *testing.T) {
	s, room := newAttachmentTestService(t)
	roomMessage, err := s.DeliverMessage(DeliverMessageRequest{
		RoomID: room.ID, SenderID: AdminUserID, MessageID: "room-output", AttachmentSources: []AttachmentSource{sessionAttachmentSource("room.txt", "shared")},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := s.StoreSessionResponseAttachments("worker", "session", "response", t.TempDir(), []AttachmentSource{sessionAttachmentSource("original.txt", "shared")})
	if err != nil {
		t.Fatal(err)
	}
	roomFile := roomMessage.Attachments[0]
	if roomFile.SHA256 != stored[0].SHA256 || roomFile.ID == stored[0].ID {
		t.Fatal("expected separate attachment IDs backed by the same blob")
	}
	if err := s.DeleteRoomAttachment(room.ID, roomFile.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AttachmentFile(stored[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AttachmentFile(roomFile.ID); err == nil {
		t.Error("deleted room attachment still available")
	}
}

func TestSessionResponseAttachmentCommitFailureReclaimsPartialBatch(t *testing.T) {
	s, _ := newAttachmentTestService(t)
	// Occupying the second blob's destination with a directory causes its
	// installation to fail after the first object has already been written.
	broken := attachmentBlobPath(s.statePath, fmt.Sprintf("%x", sha256.Sum256([]byte("broken"))))
	if err := os.MkdirAll(broken, 0700); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	attachments, err := s.StoreSessionResponseAttachments("worker", "session", "response", workspace, []AttachmentSource{
		sessionAttachmentSource("partial.txt", "partial"), sessionAttachmentSource("broken.txt", "broken"),
	})
	if err == nil || len(attachments) != 0 {
		t.Fatalf("expected batch publication failure, got %d files, error=%v", len(attachments), err)
	}
	if _, err := os.Stat(attachmentBlobPath(s.statePath, fmt.Sprintf("%x", sha256.Sum256([]byte("partial"))))); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("partial blob remains: %v", err)
	}
	if err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			t.Errorf("workspace file remains after failed publication: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	objects, err := os.ReadDir(filepath.Join(filepath.Dir(s.statePath), assetsDirName, assetObjectsDirName))
	if err != nil || len(objects) != 0 {
		t.Fatalf("partial attachment objects=%v, err=%v", objects, err)
	}
}

func TestSessionAttachmentCollectionRemovesCorruptOrphans(t *testing.T) {
	s, _ := newAttachmentTestService(t)
	state := s.Bootstrap()
	stored, err := s.StoreSessionResponseAttachments("worker", "session", "response", t.TempDir(), []AttachmentSource{sessionAttachmentSource("live.txt", "live")})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate corrupt metadata left without a room reference by a previous
	// installation. It must not prevent collection of other abandoned bytes.
	orphan := attachmentObjectPath(s.statePath, "att-abandoned")
	if err := os.WriteFile(orphan, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	blob := attachmentBlobPath(s.statePath, fmt.Sprintf("%x", sha256.Sum256([]byte("unused"))))
	if err := os.MkdirAll(filepath.Dir(blob), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blob, []byte("unused"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveBootstrap(s.statePath, state); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{orphan, blob} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("orphan was not removed: %s, error=%v", path, err)
		}
	}
	if _, err := s.AttachmentFile(stored[0].ID); err != nil {
		t.Fatalf("bootstrap checkpoint removed a live session file: %v", err)
	}
}

func TestSessionAttachmentRejectsWorkspaceEscape(t *testing.T) {
	s, _ := newAttachmentTestService(t)
	workspace, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, ".csgclaw")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	stored, err := s.StoreSessionResponseAttachments("worker", "session", "response", workspace, []AttachmentSource{sessionAttachmentSource("report.txt", "bytes")})
	if err == nil || len(stored) != 0 {
		t.Fatalf("expected unsafe directory rejection, got %d files, error=%v", len(stored), err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside directory: %v, error=%v", entries, err)
	}
}

func TestConcurrentSessionAttachmentsWithSameFilename(t *testing.T) {
	s, _ := newAttachmentTestService(t)
	workspace := t.TempDir()
	type publication struct {
		content string
		files   []MessageAttachment
		err     error
	}
	const count = 4
	results := make(chan publication, count)
	for i := range count {
		go func() {
			content := fmt.Sprintf("content %d", i)
			files, err := s.StoreSessionResponseAttachments("worker", "session", "response", workspace, []AttachmentSource{sessionAttachmentSource("report.txt", content)})
			results <- publication{content, files, err}
		}()
	}
	for range count {
		result := <-results
		if result.err != nil || len(result.files) != 1 {
			t.Errorf("published %d files, error=%v", len(result.files), result.err)
			continue
		}
		file := result.files[0]
		content, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(file.WorkspacePath)))
		if err != nil || string(content) != result.content {
			t.Errorf("workspace content=%q, error=%v", content, err)
		}
		if _, err := s.AttachmentFile(file.ID); err != nil {
			t.Error(err)
		}
	}
}

func TestSessionWorkspaceFileKeepsDirectoryAcrossSymlinkSwap(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "response")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	reader := &gatedAttachmentReader{Reader: strings.NewReader("contents"), entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- writeSessionWorkspaceFile(dir, "report.txt", reader) }()
	<-reader.entered
	original := filepath.Join(workspace, "original")
	outside := t.TempDir()
	swapErr := os.Rename(path, original)
	if swapErr == nil {
		swapErr = os.Symlink(outside, path)
	}
	close(reader.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if swapErr != nil {
		t.Skipf("directory replacement unavailable: %v", swapErr)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("unexpected files outside workspace: %v, %v", entries, err)
	}
	if content, err := os.ReadFile(filepath.Join(original, "report.txt")); err != nil || string(content) != "contents" {
		t.Fatalf("original directory: %q, %v", content, err)
	}
}
