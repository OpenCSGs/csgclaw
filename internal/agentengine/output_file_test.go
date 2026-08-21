package agentengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
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
	firstMetadata := store.registerTurnFiles("agent-a", "conversation-a", "turn-1", []*OutputFile{first})[0]
	secondMetadata := store.registerTurnFiles("agent-a", "conversation-b", "turn-1", []*OutputFile{second})[0]
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

func outputFileTestRequest(name string, content []byte) FileCreateRequest {
	digest := sha256.Sum256(content)
	return FileCreateRequest{
		Name: name, MIMEType: "application/pdf", SizeBytes: int64(len(content)), SHA256: hex.EncodeToString(digest[:]),
	}
}
