package agentengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestOutputFileSnapshotsOnceAndUsesOpaqueIdentity(t *testing.T) {
	content := []byte("immutable output")
	store := NewFileStore()
	files := store.Scope("agent-a")
	request := outputFileTestRequest("报告.pdf", content)
	first, err := files.Create(context.Background(), request, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	second, err := files.Create(context.Background(), request, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.ID, "file-") || len(first.ID) != len("file-")+32 || first.ID == second.ID || first.ID == "sha256:"+first.SHA256 {
		t.Fatalf("file IDs = %q, %q", first.ID, second.ID)
	}
	for attempt := 0; attempt < 2; attempt++ {
		download, err := files.Get(context.Background(), first.ID)
		if err != nil {
			t.Fatal(err)
		}
		got, readErr := io.ReadAll(download.Content)
		closeErr := download.Content.Close()
		if download.Metadata != first.OutputFileMetadata || readErr != nil || closeErr != nil || !bytes.Equal(got, content) {
			t.Fatalf("attempt %d metadata=%+v content=%q read=%v close=%v", attempt, download.Metadata, got, readErr, closeErr)
		}
	}
}

func TestOutputFileRejectsUnsafeNames(t *testing.T) {
	content := []byte("content")
	files := NewFileStore().Scope("agent-a")
	invalidNames := []string{
		"", ".", "..", "nested/report.pdf", `nested\report.pdf`, "line\nbreak.pdf", "tab\tname.pdf", string([]byte{0xff, 0xfe}), strings.Repeat("a", 256),
	}
	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			request := outputFileTestRequest(name, content)
			if _, err := files.Create(context.Background(), request, bytes.NewReader(content)); err == nil {
				t.Fatalf("Create(%q) error = nil", name)
			}
		})
	}
	request := outputFileTestRequest("报告 2026.pdf", content)
	if _, err := files.Create(context.Background(), request, bytes.NewReader(content)); err != nil {
		t.Fatalf("valid Unicode name error = %v", err)
	}
}

func TestFileStoreTurnCleanupUsesConversationIdentity(t *testing.T) {
	store := NewFileStore()
	content := []byte("same turn ID")
	first, err := newOutputFile(context.Background(), OutputFileMetadata{Name: "first.txt", MediaType: "text/plain", SizeBytes: int64(len(content))}, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	second, err := newOutputFile(context.Background(), OutputFileMetadata{Name: "second.txt", MediaType: "text/plain", SizeBytes: int64(len(content))}, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	firstFiles, firstStoreErr := store.registerTurnFiles("agent-a", "conversation-a", "turn-1", []*OutputFile{first})
	if firstStoreErr != nil {
		t.Fatal(firstStoreErr)
	}
	secondFiles, secondStoreErr := store.registerTurnFiles("agent-a", "conversation-b", "turn-1", []*OutputFile{second})
	if secondStoreErr != nil {
		t.Fatal(secondStoreErr)
	}
	firstMetadata := firstFiles[0]
	secondMetadata := secondFiles[0]
	store.deleteTurn("agent-a", "conversation-a", "turn-1")
	if _, err := store.Scope("agent-a").Get(context.Background(), firstMetadata.ID); ErrorCodeOf(err) != ErrorFileNotFound {
		t.Fatalf("first Get error = %v", err)
	}
	download, err := store.Scope("agent-a").Get(context.Background(), secondMetadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = download.Content.Close()
}

func TestFileStoreGetLeaseSurvivesDelete(t *testing.T) {
	store := NewFileStore()
	files := store.Scope("agent-a")
	content := []byte("leased content")
	created, err := files.Create(context.Background(), outputFileTestRequest("lease.txt", content), bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	download, err := files.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := files.Delete(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Get(context.Background(), created.ID); ErrorCodeOf(err) != ErrorFileNotFound {
		t.Fatalf("Get after Delete error = %v", err)
	}
	got, readErr := io.ReadAll(download.Content)
	closeErr := download.Content.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(got, content) {
		t.Fatalf("leased content = %q, read=%v close=%v", got, readErr, closeErr)
	}
	store.mu.RLock()
	remaining := store.bytesByAgent["agent-a"]
	store.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("remaining bytes = %d, want 0", remaining)
	}
}

func TestFileStoreConcurrentGetAndDelete(t *testing.T) {
	store := NewFileStore()
	files := store.Scope("agent-a")
	content := bytes.Repeat([]byte("x"), 4096)
	for iteration := 0; iteration < 50; iteration++ {
		created, err := files.Create(context.Background(), outputFileTestRequest("race.txt", content), bytes.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		getErr := make(chan error, 1)
		deleteErr := make(chan error, 1)
		go func() {
			<-start
			download, err := files.Get(context.Background(), created.ID)
			if err != nil {
				if code := ErrorCodeOf(err); code != ErrorFileNotFound && code != ErrorFileUnavailable {
					getErr <- err
					return
				}
				getErr <- nil
				return
			}
			got, readErr := io.ReadAll(download.Content)
			closeErr := download.Content.Close()
			if readErr != nil || closeErr != nil || !bytes.Equal(got, content) {
				getErr <- errors.Join(readErr, closeErr, errors.New("content changed"))
				return
			}
			getErr <- nil
		}()
		go func() {
			<-start
			deleteErr <- files.Delete(context.Background(), created.ID)
		}()
		close(start)
		if err := <-getErr; err != nil {
			t.Fatalf("iteration %d Get error = %v", iteration, err)
		}
		if err := <-deleteErr; err != nil {
			t.Fatalf("iteration %d Delete error = %v", iteration, err)
		}
	}
}

func TestFileStoreResolvedInputSurvivesDelete(t *testing.T) {
	store := NewFileStore()
	files := store.Scope("agent-a")
	content := []byte("resolved input")
	created, err := files.Create(context.Background(), outputFileTestRequest("input.txt", content), bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	file, release, resolveErr := store.resolve("agent-a", created.ID)
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	if err := files.Delete(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	reader, err := file.open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	release()
	if readErr != nil || closeErr != nil || !bytes.Equal(got, content) {
		t.Fatalf("resolved content = %q, read=%v close=%v", got, readErr, closeErr)
	}
}

func TestFileStoreEnforcesFileAndAgentLimits(t *testing.T) {
	store := NewFileStore()
	store.maxAgentBytes = 8
	files := store.Scope("agent-a")
	content := []byte("12345678")
	first, err := files.Create(context.Background(), outputFileTestRequest("first.txt", content), bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := files.Create(context.Background(), outputFileTestRequest("second.txt", []byte("x")), strings.NewReader("x")); ErrorCodeOf(err) != ErrorFileUnavailable {
		t.Fatalf("aggregate quota error = %v", err)
	}
	if err := files.Delete(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Create(context.Background(), outputFileTestRequest("second.txt", []byte("x")), strings.NewReader("x")); err != nil {
		t.Fatalf("Create after quota release error = %v", err)
	}
	tooLarge := FileCreateRequest{Name: "large.bin", MIMEType: "application/octet-stream", SizeBytes: maxFileSizeBytes + 1}
	if _, err := files.Create(context.Background(), tooLarge, strings.NewReader("")); ErrorCodeOf(err) != ErrorInvalidRequest {
		t.Fatalf("per-file quota error = %v", err)
	}
}

func TestFileStoreNormalizesStorageAndContentErrors(t *testing.T) {
	files := NewFileStore().Scope("agent-a")
	request := FileCreateRequest{Name: "failed.txt", MIMEType: "text/plain", SizeBytes: 1}
	if _, err := files.Create(context.Background(), request, errorReader{err: errors.New("read /private/secret")}); ErrorCodeOf(err) != ErrorFileUnavailable || strings.Contains(err.Error(), "/private/secret") {
		t.Fatalf("storage error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := files.Create(canceled, request, strings.NewReader("x")); ErrorCodeOf(err) != ErrorCanceled {
		t.Fatalf("canceled Create error = %v", err)
	}

	store := NewFileStore()
	storedFiles := store.Scope("agent-a")
	content := []byte("content")
	created, err := storedFiles.Create(context.Background(), outputFileTestRequest("content.txt", content), bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	store.mu.RLock()
	path := store.byAgent["agent-a"][created.ID].file.snapshot.path
	store.mu.RUnlock()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := storedFiles.Get(context.Background(), created.ID); ErrorCodeOf(err) != ErrorFileUnavailable || strings.Contains(err.Error(), path) {
		t.Fatalf("content error = %v", err)
	}
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func outputFileTestRequest(name string, content []byte) FileCreateRequest {
	digest := sha256.Sum256(content)
	return FileCreateRequest{
		Name: name, MIMEType: "application/pdf", SizeBytes: int64(len(content)), SHA256: hex.EncodeToString(digest[:]),
	}
}
