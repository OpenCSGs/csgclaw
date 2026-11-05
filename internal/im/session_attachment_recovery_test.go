package im

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitForSessionWorkspaceCopy(t *testing.T, workspace, name string) string {
	t.Helper()
	pattern := filepath.Join(workspace, ".csgclaw", "attachments", "sessions", "session", "interrupted", "att-*-"+name)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if paths, err := filepath.Glob(pattern); err != nil {
			t.Fatal(err)
		} else if len(paths) == 1 {
			return paths[0]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("workspace copy never reached the commit")
	return ""
}

func TestSessionAttachmentRecoveryAfterProcessExit(t *testing.T) {
	const stateVar = "CSGCLAW_TEST_INTERRUPTED_IM"
	const workVar = "CSGCLAW_TEST_INTERRUPTED_WORKSPACE"
	if path := os.Getenv(stateVar); path != "" {
		s, err := NewServiceFromPath(path)
		if err != nil {
			t.Fatal(err)
		}
		workspace := os.Getenv(workVar)
		gate := &gatedAttachmentReader{Reader: strings.NewReader("first"), entered: make(chan struct{}), release: make(chan struct{})}
		go s.StoreSessionResponseAttachments(context.Background(), "worker", "session", "interrupted", workspace, []AttachmentSource{
			{Name: "one.txt", MediaType: "text/plain", SizeBytes: 5, Reader: gate},
			sessionAttachmentSource("two.txt", "original")})
		<-gate.entered
		s.mu.Lock() // Stop before committing the indexes.
		close(gate.release)
		target := waitForSessionWorkspaceCopy(t, workspace, "two.txt")
		if err := os.WriteFile(target, []byte("modified"), 0600); err != nil {
			t.Fatal(err)
		}
		os.Exit(73)
	}
	s, committed, workspace := sessionWorkspaceFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSessionAttachmentRecoveryAfterProcessExit$")
	configured := filepath.Join(t.TempDir(), "configured")
	hasAlias := os.Symlink(workspace, configured) == nil
	if !hasAlias {
		configured = workspace
	}
	cmd.Env = append(os.Environ(), stateVar+"="+s.statePath, workVar+"="+configured)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 73 {
		t.Fatalf("child: %s, %v", out, err)
	}
	first := waitForSessionWorkspaceCopy(t, workspace, "one.txt")
	edited := waitForSessionWorkspaceCopy(t, workspace, "two.txt")
	if hasAlias {
		if err := os.Remove(configured); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(t.TempDir(), configured); err != nil {
			t.Fatal(err)
		}
	}
	store := sessionAttachmentStore(s.statePath)
	for range 2 {
		reloaded, err := NewServiceFromPath(s.statePath)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(first); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("abandoned workspace copy remains: %v", err)
		}
		if data, err := os.ReadFile(edited); err != nil || string(data) != "modified" {
			t.Fatalf("edited file: %q, %v", data, err)
		}
		if _, err := reloaded.AttachmentFile(committed.Attachment.ID); err != nil {
			t.Fatalf("committed download: %v", err)
		}
		if pending, err := os.ReadDir(filepath.Join(string(store), "pending")); err != nil || len(pending) != 0 {
			t.Fatalf("pending records: %v, %v", pending, err)
		}
	}
}

func TestSessionAttachmentRecoveryKeepsCommittedBatch(t *testing.T) {
	s, file, workspace := sessionWorkspaceFixture(t)
	store := sessionAttachmentStore(s.statePath)
	att := file.Attachment
	// Recreate the journal present between the final index write and its removal.
	pending := pendingSessionBatch{Workspace: workspace, Session: "session", Response: "response",
		Files: []pendingSessionFile{{att.ID, att.Name, att.SHA256, att.SizeBytes}}}
	data, err := json.Marshal(pending)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(string(store), "pending", att.ID+".json")
	if err := atomicWriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServiceFromPath(s.statePath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed journal: %v", err)
	}
	for _, path := range []string{filepath.Join(workspace, filepath.FromSlash(att.WorkspacePath)), file.Path} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "original" {
			t.Fatalf("committed copy: %q, %v", data, err)
		}
	}
}

func TestSessionAttachmentRecoveryWaitsForActiveWriter(t *testing.T) {
	s, _ := newAttachmentTestService(t)
	workspace := t.TempDir()
	gate := &gatedAttachmentReader{Reader: strings.NewReader("current"), entered: make(chan struct{}), release: make(chan struct{})}
	published := make(chan error, 1)
	go func() {
		_, err := s.StoreSessionResponseAttachments(t.Context(), "worker", "session", "interrupted", workspace,
			[]AttachmentSource{{Name: "one.txt", SizeBytes: 7, Reader: gate}})
		published <- err
	}()
	<-gate.entered
	s.mu.Lock()
	close(gate.release)
	file := waitForSessionWorkspaceCopy(t, workspace, "one.txt")
	reopened := make(chan error, 1)
	go func() { _, err := NewServiceFromPath(s.statePath); reopened <- err }()
	select {
	case err := <-reopened:
		s.mu.Unlock()
		t.Fatalf("startup cleaned a live batch: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	s.mu.Unlock()
	if err := <-published; err != nil {
		t.Fatal(err)
	}
	if err := <-reopened; err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "current" {
		t.Fatalf("active writer's copy: %q, %v", data, err)
	}
}

func TestSessionAttachmentRecoveryDuringUpload(t *testing.T) {
	const stateVar = "CSGCLAW_TEST_UNFINISHED_UPLOAD"
	const workVar = "CSGCLAW_TEST_UNFINISHED_WORKSPACE"
	if path := os.Getenv(stateVar); path != "" {
		s, err := NewServiceFromPath(path)
		if err != nil {
			t.Fatal(err)
		}
		reader := &gatedAttachmentReader{Reader: strings.NewReader("two"), entered: make(chan struct{}), release: make(chan struct{})}
		go s.StoreSessionResponseAttachments(context.Background(), "worker", "session", "response", os.Getenv(workVar), []AttachmentSource{
			sessionAttachmentSource("one.txt", "one"), {Name: "two.txt", SizeBytes: 3, Reader: reader}})
		<-reader.entered
		os.Exit(72)
	}
	s, _ := newAttachmentTestService(t)
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSessionAttachmentRecoveryDuringUpload$")
	cmd.Env = append(os.Environ(), stateVar+"="+s.statePath, workVar+"="+workspace)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 72 {
		t.Fatalf("child: %s, %v", out, err)
	}
	dir := filepath.Join(workspace, ".csgclaw", "attachments", "sessions", "session", "response")
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 2 {
		t.Fatalf("expected staged files: %v, %v", entries, err)
	}
	for range 2 {
		if _, err := NewServiceFromPath(s.statePath); err != nil {
			t.Fatal(err)
		}
		if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
			t.Fatalf("unfinished upload: %v, %v", entries, err)
		}
	}
}
