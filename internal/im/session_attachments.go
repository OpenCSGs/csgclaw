package im

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// StoreSessionResponseAttachments saves direct-session files in the worker's
// workspace and publishes immutable downloads through the message attachment store.
// The caller owns the source readers.
func (s *Service) StoreSessionResponseAttachments(agentID, sessionID, responseID, workspaceRoot string, sources []AttachmentSource) (_ []MessageAttachment, err error) {
	if len(sources) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(agentID) == "" || !isAttachmentPathSegment(sessionID) || !isAttachmentPathSegment(responseID) {
		return nil, fmt.Errorf("agent, session and response IDs are required and session and response IDs must be path segments")
	}
	if s == nil || s.statePath == "" || !filepath.IsAbs(workspaceRoot) {
		return nil, fmt.Errorf("persistent IM storage and an absolute workspace are required")
	}
	if err := s.ensureSessionAttachmentStore(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(workspaceRoot, 0700); err != nil {
		return nil, fmt.Errorf("create agent workspace: %w", err)
	}
	root, err := os.OpenRoot(workspaceRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	// Keep both the stable ID and its contents outside the room collector,
	// including the collector shipped by versions that predate direct sessions.
	store := sessionAttachmentStore(s.statePath)
	files, err := store.prepare(sources)
	if err != nil {
		return nil, err
	}
	defer discardPreparedAttachments(files)
	relativeDir := filepath.Join(".csgclaw", "attachments", "sessions", sessionID, responseID)
	if err := root.MkdirAll(relativeDir, 0o700); err != nil {
		return nil, fmt.Errorf("create workspace attachment directory: %w", err)
	}
	dir, err := root.OpenRoot(relativeDir)
	if err != nil {
		return nil, fmt.Errorf("open workspace attachment directory: %w", err)
	}
	defer dir.Close()
	var names []string
	defer func() {
		if err != nil {
			for _, name := range names {
				if removeErr := dir.Remove(name); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
					err = errors.Join(err, removeErr)
				}
			}
		}
	}()
	// Copy file bytes before acquiring the IM lock. Each attachment ID is
	// unique, including when several concurrent responses use the same name.
	ids := make([]string, 0, len(files))
	for i, file := range files {
		name := file.object.ID + "-" + file.object.SafeName
		source, err := os.Open(file.path)
		if err != nil {
			return nil, err
		}
		writeErr := writeSessionWorkspaceFile(dir, name, source)
		source.Close()
		if writeErr != nil {
			return nil, fmt.Errorf("write attachment workspace file: %w", writeErr)
		}
		names = append(names, name)
		files[i].object.SessionID = sessionID
		files[i].object.WorkspacePath = filepath.ToSlash(filepath.Join(relativeDir, name))
		ids = append(ids, file.object.ID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	attachments, err := store.commit("", responseID, agentID, files)
	if err != nil {
		return nil, errors.Join(err, store.collectSessionAttachments())
	}
	// A committed batch owns its references independently of room messages.
	// Publish the references together only after all objects have been stored.
	data, err := json.Marshal(ids)
	if err == nil {
		err = atomicWriteFile(filepath.Join(string(store), "refs", ids[0]+".json"), data, 0600)
	}
	if err != nil {
		return nil, errors.Join(err, store.collectSessionAttachments())
	}
	return attachments, nil
}

// SessionAttachmentFile resolves an attachment ID against its original agent
// and session before opening access to the immutable download.
func (s *Service) SessionAttachmentFile(agentID, sessionID, id string) (AttachmentFile, error) {
	agentID = strings.TrimSpace(agentID)
	if s == nil || s.statePath == "" || agentID == "" || !isAttachmentPathSegment(sessionID) {
		return AttachmentFile{}, errAttachmentNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	store := sessionAttachmentStore(s.statePath)
	object, err := store.readObject(id)
	if err != nil {
		return AttachmentFile{}, err
	}
	if object.RoomID != "" || object.CreatedBy != agentID || object.SessionID != sessionID {
		return AttachmentFile{}, errAttachmentNotFound
	}
	return store.blobFile(object)
}

// Hold the directory open across the copy, rename and any rollback so a
// concurrent symlink replacement cannot redirect the publication.
func writeSessionWorkspaceFile(dir *os.Root, name string, source io.Reader) error {
	tempName := ".attachment-" + rand.Text()
	tmp, err := dir.OpenFile(tempName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer dir.Remove(tempName)
	if err := writeAndSyncAttachmentTemp(tmp, source, 0o600, nil); err != nil {
		return err
	}
	return dir.Rename(tempName, name)
}

func isAttachmentPathSegment(value string) bool {
	return filepath.IsLocal(value) && value != "." && value != ".." && !strings.ContainsAny(value, "/\\")
}

func (s *Service) ensureSessionAttachmentStore() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Direct sessions may write the first attachment before a room exists.
	// The bootstrap index must be present before staging any bytes for restart.
	if _, err := os.Stat(s.statePath); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := checkMissingBootstrap(s.statePath); err != nil {
		return err
	}
	return SaveBootstrap(s.statePath, s.bootstrapLocked())
}

func (store attachmentStore) collectSessionAttachments() error {
	live := make(map[string]struct{})
	dir := filepath.Join(string(store), "refs")
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		var ids []string
		if err := json.Unmarshal(data, &ids); err != nil {
			return fmt.Errorf("read session response attachment references: %w", err)
		}
		for _, id := range ids {
			live[id] = struct{}{}
		}
	}
	return store.collect(live, make(map[string]struct{}))
}
