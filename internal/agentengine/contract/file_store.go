package contract

import (
	"context"
	"errors"
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
	file, err := NewOutputFile(ctx, OutputFileMetadata{
		Name: request.Name, MediaType: request.MIMEType, SizeBytes: request.SizeBytes, SHA256: request.SHA256,
	}, content)
	if err != nil {
		var invalid *invalidOutputFileError
		if errors.As(err, &invalid) {
			return OutputFile{}, &TurnError{Code: ErrorInvalidRequest, Message: invalid.Error()}
		}
		if ctx != nil && ctx.Err() != nil {
			return OutputFile{}, &TurnError{Code: ErrorCanceled, Message: "file upload was canceled"}
		}
		return OutputFile{}, &TurnError{Code: ErrorFileUnavailable, Message: "file content could not be stored"}
	}
	if storeErr := f.store.put(f.agentID, storedFile{file: file}); storeErr != nil {
		file.Cleanup()
		return OutputFile{}, storeErr
	}
	return file.Metadata(), nil
}

func (f *agentFiles) Get(ctx context.Context, fileID string) (FileContent, error) {
	fileID = strings.TrimSpace(fileID)
	if f == nil || f.store == nil || f.agentID == "" || fileID == "" {
		return FileContent{}, &TurnError{Code: ErrorInvalidRequest, Message: "agent ID and file ID are required"}
	}
	f.store.mu.RLock()
	file, ok := f.store.byAgent[f.agentID][fileID]
	if !ok {
		f.store.mu.RUnlock()
		return FileContent{}, &TurnError{Code: ErrorFileNotFound, Message: fmt.Sprintf("file %q was not found", fileID)}
	}
	release, retained := file.file.snapshot.retain()
	f.store.mu.RUnlock()
	if !retained {
		return FileContent{}, &TurnError{Code: ErrorFileUnavailable, Message: "file content is unavailable"}
	}
	content, err := file.file.Open(ctx)
	release()
	if err != nil {
		if ctx != nil && ctx.Err() != nil {
			return FileContent{}, &TurnError{Code: ErrorCanceled, Message: "file download was canceled"}
		}
		return FileContent{}, &TurnError{Code: ErrorFileUnavailable, Message: "file content is unavailable"}
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
	file.file.Cleanup()
	return nil
}

func (s *FileStore) put(agentID string, file storedFile) *TurnError {
	if file.file == nil || file.file.snapshot == nil {
		return &TurnError{Code: ErrorFileUnavailable, Message: "file content is unavailable"}
	}
	s.mu.Lock()
	if s.byAgent == nil {
		s.byAgent = make(map[string]map[string]storedFile)
	}
	files := s.byAgent[agentID]
	if files == nil {
		files = make(map[string]storedFile)
		s.byAgent[agentID] = files
	}
	if _, exists := files[file.file.ID]; exists {
		s.mu.Unlock()
		return &TurnError{Code: ErrorFileUnavailable, Message: "file ID collision"}
	}
	files[file.file.ID] = file
	s.mu.Unlock()
	return nil
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

func (s *FileStore) RegisterTurnFiles(agentID string, conversationKey ConversationKey, turnID TurnID, files []*OutputFile) ([]OutputFile, *TurnError) {
	result := make([]OutputFile, 0, len(files))
	registered := make([]string, 0, len(files))
	for _, file := range files {
		if file == nil || file.snapshot == nil {
			continue
		}
		if err := s.put(agentID, storedFile{file: file, conversationKey: conversationKey, turnID: turnID}); err != nil {
			for _, fileID := range registered {
				stored, ok := s.delete(agentID, fileID)
				if ok {
					stored.file.Cleanup()
				}
			}
			for _, pending := range files {
				pending.Cleanup()
			}
			return nil, err
		}
		registered = append(registered, file.ID)
		result = append(result, file.Metadata())
	}
	return result, nil
}

func (s *FileStore) Resolve(agentID, fileID string) (*OutputFile, func(), *TurnError) {
	agentID = strings.TrimSpace(agentID)
	fileID = strings.TrimSpace(fileID)
	s.mu.RLock()
	file, ok := s.byAgent[agentID][fileID]
	if !ok {
		s.mu.RUnlock()
		return nil, nil, &TurnError{Code: ErrorFileNotFound, Message: fmt.Sprintf("file %q was not found", fileID)}
	}
	release, retained := file.file.snapshot.retain()
	s.mu.RUnlock()
	if !retained {
		return nil, nil, &TurnError{Code: ErrorFileUnavailable, Message: "file content is unavailable"}
	}
	return file.file, release, nil
}

func (s *FileStore) DeleteTurn(agentID string, conversationKey ConversationKey, turnID TurnID) {
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
		file.Cleanup()
	}
}

// DeleteAgent removes every snapshot owned by one Agent.
func (s *FileStore) DeleteAgent(agentID string) {
	s.mu.Lock()
	files := s.byAgent[agentID]
	delete(s.byAgent, agentID)
	s.mu.Unlock()
	for _, file := range files {
		file.file.Cleanup()
	}
}

type UnavailableFiles struct{}

func (UnavailableFiles) Create(context.Context, FileCreateRequest, io.Reader) (OutputFile, error) {
	return OutputFile{}, &TurnError{Code: ErrorAgentUnavailable, Message: "file store is unavailable"}
}
func (UnavailableFiles) Get(context.Context, string) (FileContent, error) {
	return FileContent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "file store is unavailable"}
}
func (UnavailableFiles) Delete(context.Context, string) error {
	return &TurnError{Code: ErrorAgentUnavailable, Message: "file store is unavailable"}
}
