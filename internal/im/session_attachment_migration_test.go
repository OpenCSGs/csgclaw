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
	files, err := store.prepare(t.Context(), sources)
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

func writeLegacyWorkspaceFile(t *testing.T, workspace string, attachment MessageAttachment, content string) {
	t.Helper()
	path := filepath.Join(workspace, filepath.FromSlash(attachment.WorkspacePath))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func migrateTestSessionWorkspace(t *testing.T, s *Service, workspace string) {
	t.Helper()
	if err := s.MigrateSessionWorkspaces(func(agentID string) (string, error) {
		if agentID != "worker" {
			t.Fatalf("unexpected agent: %s", agentID)
		}
		return workspace, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLegacySessionAttachmentMigrationRetriesAndPreservesDownloads(t *testing.T) {
	s, room := newAttachmentTestService(t)
	message, err := s.DeliverMessage(DeliverMessageRequest{RoomID: room.ID, SenderID: AdminUserID, MessageID: "room-file",
		AttachmentSources: []AttachmentSource{sessionAttachmentSource("room.txt", "shared")}})
	if err != nil {
		t.Fatal(err)
	}
	stored, manifest := legacySessionBatch(t, s, "shared", "private")
	workspace := t.TempDir()
	writeLegacyWorkspaceFile(t, workspace, stored[0], "updated shared")
	writeLegacyWorkspaceFile(t, workspace, stored[1], "private")
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
	for range 2 {
		migrateTestSessionWorkspace(t, s, workspace)
	}
	sessions := sessionAttachmentStore(s.statePath)
	for _, a := range stored {
		if _, err := os.Stat(sessions.blobPath(a.SHA256)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("session snapshot after migration: %v", err)
		}
	}
	// An HTTP download resolves its file before opening it. The first room
	// checkpoint after migration may run between those two operations.
	inFlight, err := s.AttachmentFile(stored[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveBootstrap(s.statePath, s.Bootstrap()); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(inFlight.Path); err != nil || string(data) != "private" {
		t.Fatalf("download spanning the first checkpoint: %q, %v", data, err)
	}
	if err := migrateLegacySessionAttachments(s.statePath); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RoomAttachmentFile(room.ID, message.Attachments[0].ID); err != nil {
		t.Fatalf("room with matching content lost its attachment: %v", err)
	}
	for i, content := range []string{"updated shared", "private"} {
		old := stored[i]
		got, err := s.SessionAttachmentFile("worker", "session", old.ID)
		if err != nil {
			t.Fatal(err)
		}
		link, err := url.Parse(old.DownloadURL)
		if err != nil {
			t.Fatal(err)
		}
		if got.Attachment.DownloadURL != old.DownloadURL || got.Attachment.ID != old.ID || !got.AuthorizesDownloadToken(link.Query().Get("token")) {
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
	workspace := t.TempDir()
	writeLegacyWorkspaceFile(t, workspace, files[1], "edited after original removed")
	writeLegacyWorkspaceFile(t, workspace, files[2], "retained")
	migrateTestSessionWorkspace(t, s, workspace)
	if missing, err := s.AttachmentFile(files[1].ID); err != nil || missing.Attachment.DownloadURL != files[1].DownloadURL {
		t.Fatalf("workspace bytes with collected snapshot: %v", err)
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

func TestLegacySessionMigrationKeepsUnresolvedWorkspaceAndSharedBlob(t *testing.T) {
	s, _ := newAttachmentTestService(t)
	files, manifest := legacySessionBatch(t, s, "shared", "shared")
	workspace := t.TempDir()
	writeLegacyWorkspaceFile(t, workspace, files[0], "edited")
	if err := migrateLegacySessionAttachments(s.statePath); err != nil {
		t.Fatal(err)
	}
	store := sessionAttachmentStore(s.statePath)
	if err := s.MigrateSessionWorkspaces(func(string) (string, error) { return "", errors.New("agent unavailable") }); err != nil {
		t.Fatal(err)
	}
	migrateTestSessionWorkspace(t, s, workspace)
	// One file remains unresolved. Its immutable session-store bytes and
	// manifest must survive, but migration must not repopulate its workspace.
	missingPath := filepath.Join(workspace, filepath.FromSlash(files[1].WorkspacePath))
	if _, err := os.Stat(missingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing workspace path: %v", err)
	}
	if _, err := s.AttachmentFile(files[1].ID); !errors.Is(err, ErrSessionAttachmentNotFound) {
		t.Fatalf("legacy snapshot exposed as a download: %v", err)
	}
	refsPath := filepath.Join(string(store), "refs", filepath.Base(manifest))
	for _, p := range []string{store.blobPath(files[0].SHA256), refsPath} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
	writeLegacyWorkspaceFile(t, workspace, files[1], "recovered by owner")
	migrateTestSessionWorkspace(t, s, workspace)
	for _, p := range []string{store.blobPath(files[0].SHA256), refsPath} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("obsolete migration data: %s, %v", p, err)
		}
	}
}
