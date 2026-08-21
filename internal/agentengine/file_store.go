package agentengine

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
)

// FileInterface manages immutable files scoped to one Agent. Create and Get
// map directly to HTTP upload and download bodies.
type FileInterface interface {
	Create(ctx context.Context, request FileCreateRequest, content io.Reader) (OutputFile, error)
	Get(ctx context.Context, fileID string) (FileContent, error)
	Delete(ctx context.Context, fileID string) error
}

// FileContent combines authoritative metadata with an independent content
// stream. An HTTP adapter maps Metadata to response headers and Content to the
// response body.
type FileContent struct {
	Metadata OutputFileMetadata
	Content  io.ReadCloser
}

// FileCreateRequest declares metadata for one uploaded immutable file.
type FileCreateRequest struct {
	Name      string `json:"name"`
	MIMEType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256,omitempty"`
}

// FileStore owns process-local immutable snapshots indexed by Agent and FileID.
// It is reusable by the real Engine and contract-compatible test clients.
type FileStore struct {
	mu      sync.RWMutex
	byAgent map[string]map[string]storedFile
}

type storedFile struct {
	file            *OutputFile
	conversationKey ConversationKey
	turnID          TurnID
}

// NewFileStore creates an empty process-local Artifact Store.
func NewFileStore() *FileStore {
	return &FileStore{byAgent: make(map[string]map[string]storedFile)}
}

// Scope returns file operations for one Agent.
func (s *FileStore) Scope(agentID string) FileInterface {
	return &agentFiles{store: s, agentID: strings.TrimSpace(agentID)}
}

type agentFiles struct {
	store   *FileStore
	agentID string
}

func (f *agentFiles) Create(ctx context.Context, request FileCreateRequest, content io.Reader) (OutputFile, error) {
	if f == nil || f.store == nil || f.agentID == "" {
		return OutputFile{}, &TurnError{Code: ErrorInvalidRequest, Message: "agent ID is required"}
	}
	file, err := newOutputFile(ctx, OutputFileMetadata{
		Name: request.Name, MediaType: request.MIMEType, SizeBytes: request.SizeBytes, SHA256: request.SHA256,
	}, content)
	if err != nil {
		return OutputFile{}, &TurnError{Code: ErrorInvalidRequest, Message: err.Error()}
	}
	f.store.put(f.agentID, storedFile{file: file})
	return file.metadata(), nil
}

func (f *agentFiles) Get(ctx context.Context, fileID string) (FileContent, error) {
	file, err := f.stored(fileID)
	if err != nil {
		return FileContent{}, err
	}
	content, err := file.file.open(ctx)
	if err != nil {
		return FileContent{}, err
	}
	return FileContent{Metadata: file.file.OutputFileMetadata, Content: content}, nil
}

func (f *agentFiles) Delete(_ context.Context, fileID string) error {
	fileID = strings.TrimSpace(fileID)
	if f == nil || f.store == nil || f.agentID == "" || fileID == "" {
		return &TurnError{Code: ErrorInvalidRequest, Message: "agent ID and file ID are required"}
	}
	file, ok := f.store.delete(f.agentID, fileID)
	if !ok {
		return &TurnError{Code: ErrorFileNotFound, Message: fmt.Sprintf("file %q was not found", fileID)}
	}
	file.file.cleanup()
	return nil
}

func (f *agentFiles) stored(fileID string) (storedFile, error) {
	fileID = strings.TrimSpace(fileID)
	if f == nil || f.store == nil || f.agentID == "" || fileID == "" {
		return storedFile{}, &TurnError{Code: ErrorInvalidRequest, Message: "agent ID and file ID are required"}
	}
	file, ok := f.store.get(f.agentID, fileID)
	if !ok {
		return storedFile{}, &TurnError{Code: ErrorFileNotFound, Message: fmt.Sprintf("file %q was not found", fileID)}
	}
	return file, nil
}

func (s *FileStore) put(agentID string, file storedFile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files := s.byAgent[agentID]
	if files == nil {
		files = make(map[string]storedFile)
		s.byAgent[agentID] = files
	}
	files[file.file.ID] = file
}

func (s *FileStore) get(agentID, fileID string) (storedFile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	file, ok := s.byAgent[agentID][fileID]
	return file, ok
}

func (s *FileStore) delete(agentID, fileID string) (storedFile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files := s.byAgent[agentID]
	file, ok := files[fileID]
	if !ok {
		return storedFile{}, false
	}
	delete(files, fileID)
	if len(files) == 0 {
		delete(s.byAgent, agentID)
	}
	return file, true
}

func (s *FileStore) registerTurnFiles(agentID string, conversationKey ConversationKey, turnID TurnID, files []*OutputFile) []OutputFile {
	result := make([]OutputFile, 0, len(files))
	for _, file := range files {
		if file == nil || file.snapshot == nil {
			continue
		}
		s.put(agentID, storedFile{file: file, conversationKey: conversationKey, turnID: turnID})
		result = append(result, file.metadata())
	}
	return result
}

func (s *FileStore) resolve(agentID, fileID string) (*OutputFile, *TurnError) {
	file, ok := s.get(strings.TrimSpace(agentID), strings.TrimSpace(fileID))
	if !ok {
		return nil, &TurnError{Code: ErrorFileNotFound, Message: fmt.Sprintf("file %q was not found", fileID)}
	}
	return file.file, nil
}

func (s *FileStore) deleteTurn(agentID string, conversationKey ConversationKey, turnID TurnID) {
	var removed []*OutputFile
	s.mu.Lock()
	files := s.byAgent[agentID]
	for fileID, file := range files {
		if file.conversationKey == conversationKey && file.turnID == turnID {
			removed = append(removed, file.file)
			delete(files, fileID)
		}
	}
	if len(files) == 0 {
		delete(s.byAgent, agentID)
	}
	s.mu.Unlock()
	for _, file := range removed {
		file.cleanup()
	}
}

// DeleteAgent removes every snapshot owned by one Agent.
func (s *FileStore) DeleteAgent(agentID string) {
	s.mu.Lock()
	files := s.byAgent[agentID]
	delete(s.byAgent, agentID)
	s.mu.Unlock()
	for _, file := range files {
		file.file.cleanup()
	}
}

type unavailableFiles struct{}

func (unavailableFiles) Create(context.Context, FileCreateRequest, io.Reader) (OutputFile, error) {
	return OutputFile{}, &TurnError{Code: ErrorAgentUnavailable, Message: "file store is unavailable"}
}
func (unavailableFiles) Get(context.Context, string) (FileContent, error) {
	return FileContent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "file store is unavailable"}
}
func (unavailableFiles) Delete(context.Context, string) error {
	return &TurnError{Code: ErrorAgentUnavailable, Message: "file store is unavailable"}
}
