package agentengine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const maxOutputFileNameBytes = 255

type invalidOutputFileError struct {
	message string
}

func (e *invalidOutputFileError) Error() string {
	return e.message
}

func invalidOutputFile(message string) error {
	return &invalidOutputFileError{message: message}
}

// OutputFileMetadata describes one Runtime file authorized for delivery by a
// Channel Adapter. It deliberately contains no host or Runtime path.
type OutputFileMetadata struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

// OutputFile is JSON-safe metadata for one immutable Engine file resource.
// Snapshot access stays behind FileInterface and never exposes a host path.
type OutputFile struct {
	OutputFileMetadata

	snapshot *outputFileSnapshot
}

func newOutputFile(ctx context.Context, metadata OutputFileMetadata, source io.Reader) (*OutputFile, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if source == nil {
		return nil, invalidOutputFile("output file source is required")
	}
	name, err := normalizeOutputFileName(metadata.Name)
	if err != nil {
		return nil, err
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(metadata.MediaType))
	if err != nil || strings.TrimSpace(mediaType) == "" {
		return nil, invalidOutputFile("output file media type is invalid")
	}
	if metadata.SizeBytes < 0 {
		return nil, invalidOutputFile("output file size must be non-negative")
	}
	expectedHash := strings.ToLower(strings.TrimSpace(metadata.SHA256))
	if expectedHash != "" {
		digest, err := hex.DecodeString(expectedHash)
		if err != nil || len(digest) != sha256.Size {
			return nil, invalidOutputFile("output file SHA-256 is invalid")
		}
	}

	snapshot, actualHash, err := createOutputFileSnapshot(ctx, source, metadata.SizeBytes)
	if err != nil {
		return nil, err
	}
	if expectedHash != "" && actualHash != expectedHash {
		snapshot.cleanup()
		return nil, invalidOutputFile("output file SHA-256 does not match")
	}
	id, err := newOutputFileID()
	if err != nil {
		snapshot.cleanup()
		return nil, err
	}
	return &OutputFile{
		OutputFileMetadata: OutputFileMetadata{
			ID: id, Name: name, MediaType: strings.ToLower(mediaType), SizeBytes: metadata.SizeBytes, SHA256: actualHash,
		},
		snapshot: snapshot,
	}, nil
}

func (f *OutputFile) open(ctx context.Context) (io.ReadCloser, error) {
	if f == nil || f.snapshot == nil || f.snapshot.path == "" {
		return nil, fmt.Errorf("output file is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.snapshot.openVerified(ctx, f.SizeBytes, f.SHA256)
}

func (f *OutputFile) metadata() OutputFile {
	if f == nil {
		return OutputFile{}
	}
	return OutputFile{OutputFileMetadata: f.OutputFileMetadata}
}

func (f *OutputFile) cleanup() {
	if f == nil || f.snapshot == nil {
		return
	}
	f.snapshot.cleanup()
}

func normalizeOutputFileName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" || !utf8.ValidString(name) || len([]byte(name)) > maxOutputFileNameBytes {
		return "", invalidOutputFile(fmt.Sprintf("output file name must be valid UTF-8 with 1 to %d bytes", maxOutputFileNameBytes))
	}
	if name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
		return "", invalidOutputFile("output file name must be a basename without path components")
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", invalidOutputFile("output file name must not contain control characters")
		}
	}
	return name, nil
}

func newOutputFileID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate output file ID: %w", err)
	}
	return "file-" + hex.EncodeToString(value), nil
}

type outputFileSnapshot struct {
	mu      sync.Mutex
	path    string
	removed bool
	leases  int
}

func createOutputFileSnapshot(ctx context.Context, source io.Reader, expectedSize int64) (*outputFileSnapshot, string, error) {
	if expectedSize < 0 {
		return nil, "", invalidOutputFile("output file size must be non-negative")
	}
	snapshot, err := os.CreateTemp("", "csgclaw-output-")
	if err != nil {
		return nil, "", fmt.Errorf("create output file snapshot: %w", err)
	}
	path := snapshot.Name()
	cleanup := func() {
		_ = snapshot.Close()
		_ = os.Remove(path)
	}
	hash := sha256.New()
	written, extra, err := copyExactOutputFile(ctx, io.MultiWriter(snapshot, hash), source, expectedSize)
	if err != nil {
		cleanup()
		return nil, "", fmt.Errorf("snapshot output file: %w", err)
	}
	if written != expectedSize || extra {
		cleanup()
		return nil, "", fmt.Errorf("output file size changed: got %d, want %d", written, expectedSize)
	}
	if err := snapshot.Sync(); err != nil {
		cleanup()
		return nil, "", fmt.Errorf("sync output file snapshot: %w", err)
	}
	if err := snapshot.Chmod(0o400); err != nil {
		cleanup()
		return nil, "", fmt.Errorf("protect output file snapshot: %w", err)
	}
	if err := snapshot.Close(); err != nil {
		_ = os.Remove(path)
		return nil, "", fmt.Errorf("close output file snapshot: %w", err)
	}
	result := &outputFileSnapshot{path: path}
	runtime.SetFinalizer(result, func(snapshot *outputFileSnapshot) {
		snapshot.cleanup()
	})
	return result, hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *outputFileSnapshot) cleanup() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.removed = true
	path := s.takeRemovalLocked()
	s.mu.Unlock()
	removeOutputFileSnapshot(path)
}

func (s *outputFileSnapshot) retain() (func(), bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.removed || s.path == "" {
		return nil, false
	}
	s.leases++
	return s.release, true
}

func (s *outputFileSnapshot) openVerified(ctx context.Context, expectedSize int64, expectedHash string) (io.ReadCloser, error) {
	if s == nil {
		return nil, fmt.Errorf("output file is unavailable")
	}
	s.mu.Lock()
	if s.path == "" {
		s.mu.Unlock()
		return nil, fmt.Errorf("output file is unavailable")
	}
	source, err := os.Open(s.path)
	if err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("open output file snapshot: %w", err)
	}
	s.leases++
	s.mu.Unlock()
	closeWithRelease := func() {
		_ = source.Close()
		s.release()
	}
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != expectedSize {
		closeWithRelease()
		return nil, fmt.Errorf("output file snapshot metadata changed")
	}
	hash := sha256.New()
	written, extra, err := copyExactOutputFile(ctx, hash, source, expectedSize)
	if err != nil {
		closeWithRelease()
		return nil, fmt.Errorf("read output file snapshot: %w", err)
	}
	if written != expectedSize || extra || hex.EncodeToString(hash.Sum(nil)) != expectedHash {
		closeWithRelease()
		return nil, fmt.Errorf("output file snapshot content changed")
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		closeWithRelease()
		return nil, fmt.Errorf("rewind output file delivery reader: %w", err)
	}
	return &leasedOutputFile{File: source, snapshot: s}, nil
}

func (s *outputFileSnapshot) release() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.leases > 0 {
		s.leases--
	}
	path := s.takeRemovalLocked()
	s.mu.Unlock()
	removeOutputFileSnapshot(path)
}

func (s *outputFileSnapshot) takeRemovalLocked() string {
	if !s.removed || s.leases != 0 || s.path == "" {
		return ""
	}
	path := s.path
	s.path = ""
	return path
}

func removeOutputFileSnapshot(path string) {
	if path == "" {
		return
	}
	_ = os.Chmod(path, 0o600)
	_ = os.Remove(path)
}

func copyExactOutputFile(ctx context.Context, destination io.Writer, source io.Reader, expectedSize int64) (int64, bool, error) {
	reader := contextReader{ctx: ctx, reader: source}
	written, err := io.CopyN(destination, reader, expectedSize)
	if err != nil {
		return written, false, err
	}
	extra, err := io.ReadAll(io.LimitReader(reader, 1))
	if err != nil {
		return written, false, err
	}
	return written, len(extra) != 0, nil
}

type leasedOutputFile struct {
	*os.File
	snapshot *outputFileSnapshot
}

func (f *leasedOutputFile) Close() error {
	if f == nil || f.File == nil {
		return nil
	}
	closeErr := f.File.Close()
	f.File = nil
	if f.snapshot != nil {
		f.snapshot.release()
	}
	f.snapshot = nil
	return closeErr
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
