package im

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// The previous writer committed direct-session objects with room assets, then
// published one top-level manifest containing the IDs in upload order.
func legacySessionBatch(t *testing.T, s *Service, contents ...string) ([]MessageAttachment, string) {
	t.Helper()
	store := roomAttachmentStore(s.statePath)
	var sources []AttachmentSource
	for _, text := range contents {
		sources = append(sources, sessionAttachmentSource("file.txt", text))
	}
	files, err := store.prepare(sources)
	if err != nil {
		t.Fatal(err)
	}
	defer discardPreparedAttachments(files)
	var ids []string
	for i := range files {
		files[i].object.SessionID = "session"
		files[i].object.WorkspacePath = ".csgclaw/attachments/sessions/session/response/" + files[i].object.ID + "-file.txt"
		ids = append(ids, files[i].object.ID)
	}
	originals, err := store.commit("", "response", "worker", files)
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(string(sessionAttachmentStore(s.statePath)), ids[0]+".json")
	data, err := json.Marshal(ids)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	return originals, manifest
}

func TestLegacySessionAttachmentMigrationRetriesAndPreservesDownloads(t *testing.T) {
	s, room := newAttachmentTestService(t)
	message, err := s.DeliverMessage(DeliverMessageRequest{RoomID: room.ID, SenderID: AdminUserID, MessageID: "room-file",
		AttachmentSources: []AttachmentSource{sessionAttachmentSource("room.txt", "shared")}})
	if err != nil {
		t.Fatal(err)
	}
	stored, manifest := legacySessionBatch(t, s, "shared", "private")
	rooms := roomAttachmentStore(s.statePath)
	broken := rooms.blobPath(stored[1].SHA256)
	if err := os.WriteFile(broken, []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveBootstrap(s.statePath, s.Bootstrap()); err == nil {
		t.Fatal("room checkpoint accepted corrupt legacy session bytes")
	}
	for _, path := range []string{manifest, rooms.objectPath(stored[0].ID), rooms.objectPath(stored[1].ID), broken} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("legacy source lost after failed migration: %v", err)
		}
	}
	if err := os.WriteFile(broken, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err = NewServiceFromPath(s.statePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manifest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old manifest after opening: %v", err)
	}
	if err := SaveBootstrap(s.statePath, s.Bootstrap()); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacySessionAttachments(s.statePath); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RoomAttachmentFile(room.ID, message.Attachments[0].ID); err != nil {
		t.Fatalf("room with matching content lost its attachment: %v", err)
	}
	for i, content := range []string{"shared", "private"} {
		old := stored[i]
		got, err := s.SessionAttachmentFile("worker", "session", old.ID)
		if err != nil {
			t.Fatal(err)
		}
		link, err := url.Parse(old.DownloadURL)
		if err != nil {
			t.Fatal(err)
		}
		if got.Attachment != old || !got.AuthorizesDownloadToken(link.Query().Get("token")) {
			t.Fatalf("historical download metadata changed for %s", old.ID)
		}
		if data, err := os.ReadFile(got.Path); err != nil || string(data) != content {
			t.Fatalf("download for %s after room GC: %q, %v", old.ID, data, err)
		}
	}
}

func TestLegacySessionAttachmentMigrationSkipsAlreadyCollectedFiles(t *testing.T) {
	s, room := newAttachmentTestService(t)
	files, manifest := legacySessionBatch(t, s, "lost object", "lost blob", "retained")
	rooms := roomAttachmentStore(s.statePath)
	for _, path := range []string{rooms.objectPath(files[0].ID), rooms.blobPath(files[1].SHA256)} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	s, err := NewServiceFromPath(s.statePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manifest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old manifest after opening: %v", err)
	}
	if _, err := s.ListMessages(room.ID); err != nil {
		t.Fatal(err)
	}
	if err := SaveBootstrap(s.statePath, s.Bootstrap()); err != nil {
		t.Fatal(err)
	}
	got, err := s.AttachmentFile(files[2].ID)
	if err != nil || got.Attachment.DownloadURL != files[2].DownloadURL {
		t.Fatalf("available historical download: %v", err)
	}
	if data, err := os.ReadFile(got.Path); err != nil || string(data) != "retained" {
		t.Fatalf("available file: %q, %v", data, err)
	}
}
