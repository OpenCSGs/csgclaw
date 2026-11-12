package im

import (
	"context"
	"errors"
	"io"
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

func TestSessionWorkspaceDownloadTracksCurrentFile(t *testing.T) {
	s, file, workspace := sessionWorkspaceFixture(t)
	target := filepath.Join(workspace, filepath.FromSlash(file.Attachment.WorkspacePath))
	if !file.WorkspaceMatches(workspace) || file.WorkspaceMatches(t.TempDir()) {
		t.Fatal("attachment did not identify its configured workspace")
	}
	const edited = "%PDF-1.7\nedited"
	if err := os.WriteFile(target, []byte(edited), 0600); err != nil {
		t.Fatal(err)
	}
	ref, err := s.SessionAttachmentFile("worker", "session", file.Attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Attachment.SizeBytes != int64(len(edited)) || ref.Attachment.MediaType != "application/pdf" || ref.Attachment.DownloadURL != file.Attachment.DownloadURL {
		t.Fatalf("edited file: %+v", ref.Attachment)
	}
	opened, err := file.Open()
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(opened)
	opened.Close()
	if err != nil || string(data) != edited {
		t.Fatalf("open descriptor: %q, %v", data, err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Open(); !errors.Is(err, ErrSessionAttachmentNotFound) {
		t.Fatalf("open deleted file: %v", err)
	}
	if _, err := s.SessionAttachmentFile("worker", "session", file.Attachment.ID); !errors.Is(err, ErrSessionAttachmentNotFound) {
		t.Fatalf("reference deleted file: %v", err)
	}
}

func TestSessionWorkspaceRejectsRedirects(t *testing.T) {
	for _, kind := range []string{"symlink", "directory", "fifo", "parent symlink", "root symlink"} {
		t.Run(kind, func(t *testing.T) {
			s, file, workspace := sessionWorkspaceFixture(t)
			target := filepath.Join(workspace, filepath.FromSlash(file.Attachment.WorkspacePath))
			if kind == "root symlink" || kind == "parent symlink" {
				path := workspace
				if kind == "parent symlink" {
					path = filepath.Dir(target)
				}
				moved := filepath.Join(t.TempDir(), "moved")
				if err := os.Rename(path, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, path); err != nil {
					t.Skipf("symlink: %v", err)
				}
			} else {
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "symlink":
					if err := os.Symlink(filepath.Base(target)+".other", target); err != nil {
						t.Skipf("symlink: %v", err)
					}
				case "directory":
					if err := os.Mkdir(target, 0700); err != nil {
						t.Fatal(err)
					}
				case "fifo":
					mkfifo, err := exec.LookPath("mkfifo")
					if err != nil {
						t.Skip("mkfifo unavailable")
					}
					if err := exec.Command(mkfifo, target).Run(); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := file.Open(); !errors.Is(err, ErrSessionAttachmentNotFound) {
				t.Fatalf("download: %v", err)
			}
			if _, err := s.SessionAttachmentFile("worker", "session", file.Attachment.ID); !errors.Is(err, ErrSessionAttachmentNotFound) {
				t.Fatalf("worker: %v", err)
			}
		})
	}
}

func TestSessionAttachmentUsesConfiguredWorkspaceSymlink(t *testing.T) {
	s, _ := newAttachmentTestService(t)
	root := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "current")
	if err := os.Symlink(root, workspace); err != nil {
		t.Skipf("symlink: %v", err)
	}
	stored, err := s.StoreSessionResponseAttachments(t.Context(), "worker", "session", "response", workspace, []AttachmentSource{sessionAttachmentSource("report", "content")})
	if err != nil {
		t.Fatal(err)
	}
	file, err := s.SessionAttachmentFile("worker", "session", stored[0].ID)
	if err != nil || !file.WorkspaceMatches(root) || !file.WorkspaceMatches(workspace) {
		t.Fatalf("configured workspace: %v", err)
	}
}
