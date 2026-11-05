package im

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sessionWorkspaceFixture(t *testing.T) (*Service, AttachmentFile, string) {
	t.Helper()
	s, _ := newAttachmentTestService(t)
	workspace := t.TempDir()
	files, err := s.StoreSessionResponseAttachments(t.Context(), "worker", "session", "response", workspace,
		[]AttachmentSource{sessionAttachmentSource("notes.txt", "original")})
	if err != nil {
		t.Fatal(err)
	}
	file, err := s.SessionAttachmentFile("worker", "session", files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, file, workspace
}

func TestRestoreSessionWorkspaceFile(t *testing.T) {
	s, file, workspace := sessionWorkspaceFixture(t)
	rel := filepath.FromSlash(file.Attachment.WorkspacePath)
	target := filepath.Join(workspace, rel)
	if err := os.WriteFile(target, []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSessionWorkspaceFile(t.Context(), workspace, file); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "edited" {
		t.Fatalf("existing file: %q, %v", got, err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{workspace, filepath.Join(t.TempDir(), "replacement")} {
		if err := RestoreSessionWorkspaceFile(t.Context(), root, file); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(filepath.Join(root, rel)); err != nil || string(got) != "original" {
			t.Fatalf("restored file: %q, %v", got, err)
		}
	}
	for _, tc := range []struct{ agent, session, id string }{
		{"other", "session", file.Attachment.ID}, {"worker", "other", file.Attachment.ID},
		{"worker", "session", "att-unknown"}, {"worker", "session", "../outside"},
	} {
		if _, err := s.SessionAttachmentFile(tc.agent, tc.session, tc.id); !errors.Is(err, ErrSessionAttachmentNotFound) {
			t.Errorf("attachment ownership: %v", err)
		}
	}
	download, err := s.AttachmentFile(file.Attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := RestoreSessionWorkspaceFile(t.Context(), workspace, download); !errors.Is(err, ErrSessionAttachmentNotFound) {
		t.Fatalf("unscoped download permitted restore: %v", err)
	}
}

func TestRestoreSessionWorkspaceRejectsUnsafeFiles(t *testing.T) {
	_, file, workspace := sessionWorkspaceFixture(t)
	rel := filepath.FromSlash(file.Attachment.WorkspacePath)
	target := filepath.Join(workspace, rel)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"symlink", "directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(filepath.Join(workspace, "missing"), target)
				if err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			case "directory":
				err = os.Mkdir(target, 0700)
			case "fifo":
				mkfifo, lookupErr := exec.LookPath("mkfifo")
				if lookupErr != nil {
					t.Skip("mkfifo unavailable")
				}
				err = exec.Command(mkfifo, target).Run()
			}
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(target)
			if err := RestoreSessionWorkspaceFile(t.Context(), workspace, file); err == nil || errors.Is(err, ErrSessionAttachmentNotFound) {
				t.Fatalf("restore to %s: %v", kind, err)
			}
		})
	}
	other := t.TempDir()
	redirect := filepath.Join(other, "redirect")
	if err := os.Mkdir(redirect, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(redirect, filepath.Join(other, ".csgclaw")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := RestoreSessionWorkspaceFile(t.Context(), other, file); err == nil {
		t.Fatal("restored through a symlink inside the workspace")
	}
	if files, err := os.ReadDir(redirect); err != nil || len(files) != 0 {
		t.Fatalf("redirect directory: %v, %v", files, err)
	}
}

func TestRestoreSessionWorkspaceKeepsConcurrentEdit(t *testing.T) {
	dir, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	const content = "original"
	gate := &gatedAttachmentReader{Reader: strings.NewReader(content), entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- restoreMissingSessionFile(t.Context(), dir, "notes.txt", gate, int64(len(content)), fmt.Sprintf("%x", sha256.Sum256([]byte(content))))
	}()
	<-gate.entered
	writeErr := dir.WriteFile("notes.txt", []byte("concurrent edit"), 0600)
	close(gate.release)
	if err := <-done; err != nil || writeErr != nil {
		t.Fatalf("write: %v, restore: %v", writeErr, err)
	}
	if got, err := dir.ReadFile("notes.txt"); err != nil || string(got) != "concurrent edit" {
		t.Fatalf("file after restore: %q, %v", got, err)
	}
}

func TestRestoreSessionWorkspaceRejectsCorruptSnapshot(t *testing.T) {
	_, file, _ := sessionWorkspaceFixture(t)
	root := t.TempDir()
	if err := os.WriteFile(file.Path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSessionWorkspaceFile(t.Context(), root, file); err == nil || errors.Is(err, ErrSessionAttachmentNotFound) {
		t.Fatalf("corrupt snapshot: %v", err)
	}
	path := filepath.Join(root, filepath.FromSlash(file.Attachment.WorkspacePath))
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed recovery left files: %v, %v", entries, err)
	}
}

func TestSessionAttachmentUsesConfiguredWorkspaceSymlink(t *testing.T) {
	s, _ := newAttachmentTestService(t)
	workspace := filepath.Join(t.TempDir(), "current")
	if err := os.Symlink(t.TempDir(), workspace); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	stored, err := s.StoreSessionResponseAttachments(t.Context(), "worker", "session", "response", workspace,
		[]AttachmentSource{sessionAttachmentSource("notes.txt", "original")})
	if err != nil {
		t.Fatal(err)
	}
	file, err := s.SessionAttachmentFile("worker", "session", stored[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace, filepath.FromSlash(file.Attachment.WorkspacePath))
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSessionWorkspaceFile(t.Context(), workspace, file); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "original" {
		t.Fatalf("restored file: %q, %v", data, err)
	}
}

func TestSessionAttachmentCancellationBeforeCommit(t *testing.T) {
	s, existing, workspace := sessionWorkspaceFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	gate := &gatedAttachmentReader{Reader: strings.NewReader("next"), entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := s.StoreSessionResponseAttachments(ctx, "worker", "session", "next", workspace, []AttachmentSource{
			{Name: "next.txt", MediaType: "text/plain", SizeBytes: 4, Reader: gate},
		})
		done <- err
	}()
	<-gate.entered
	s.mu.Lock()
	close(gate.release)
	responsePath := filepath.Join(workspace, ".csgclaw", "attachments", "sessions", "session", "next")
	deadline := time.Now().Add(5 * time.Second)
	for {
		matches, _ := filepath.Glob(filepath.Join(responsePath, "att-*-next.txt"))
		if len(matches) == 1 {
			break
		}
		if time.Now().After(deadline) {
			s.mu.Unlock()
			t.Fatal("workspace copy never reached commit")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	s.mu.Unlock()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("publication: %v", err)
	}
	if files, err := os.ReadDir(responsePath); err != nil || len(files) != 0 {
		t.Fatalf("uncommitted workspace files: %v, %v", files, err)
	}
	if _, err := s.AttachmentFile(existing.Attachment.ID); err != nil {
		t.Fatalf("committed file: %v", err)
	}
}

func TestRestoreSessionWorkspaceCancellation(t *testing.T) {
	root := t.TempDir()
	dir, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	gate := &gatedAttachmentReader{Reader: strings.NewReader("original"), entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- restoreMissingSessionFile(ctx, dir, "file.txt", gate, 8, "unused") }()
	<-gate.entered
	cancel()
	close(gate.release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("restoration: %v", err)
	}
	if files, err := os.ReadDir(root); err != nil || len(files) != 0 {
		t.Fatalf("canceled restoration left files: %v, %v", files, err)
	}
}
