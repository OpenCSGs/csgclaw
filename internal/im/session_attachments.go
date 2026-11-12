package im

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StoreSessionResponseAttachments publishes one file per attachment in the
// worker's workspace. IM stores only the metadata needed to find and authorize it.
func (s *Service) StoreSessionResponseAttachments(ctx context.Context, agentID, sessionID, responseID, workspaceRoot string, sources []AttachmentSource) (_ []MessageAttachment, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(sources) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(agentID) == "" || !isAttachmentPathSegment(sessionID) || !isAttachmentPathSegment(responseID) {
		return nil, fmt.Errorf("invalid session attachment identity")
	}
	if s == nil || s.statePath == "" || !filepath.IsAbs(workspaceRoot) {
		return nil, fmt.Errorf("persistent IM storage and an absolute workspace are required")
	}
	batch := pendingSessionBatch{Session: sessionID, Response: responseID}
	for _, source := range sources {
		name, err := ValidateAttachmentName(source.Name)
		if err != nil {
			return nil, err
		}
		if source.Reader == nil || source.SizeBytes < 0 {
			return nil, fmt.Errorf("invalid attachment source for %q", name)
		}
		batch.Files = append(batch.Files, pendingSessionFile{Name: name})
	}
	if err := s.ensureSessionAttachmentStore(); err != nil {
		return nil, err
	}
	batch.Workspace, err = canonicalSessionWorkspace(workspaceRoot)
	if err != nil {
		return nil, err
	}
	dir, err := openCanonicalSessionDir(batch.Workspace, batch.relativeDir(), true)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	store := sessionAttachmentStore(s.statePath)
	lock, err := store.lockFile("publication.lock", false)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	key := rand.Text()
	if err := store.writePending(key, batch); err != nil {
		return nil, err
	}
	// A recoverable journal names the private workspace files even when the
	// process exits while still reading the first source.
	defer func() {
		if err != nil {
			err = errors.Join(err, store.recoverSessionBatch(key, batch, dir))
		}
	}()
	objects := make([]attachmentObject, len(sources))
	for i, source := range sources {
		tmp, err := dir.OpenFile(batch.tempName(key, i), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		object, err := writeAttachmentData(ctx, tmp, source, batch.Files[i].Name)
		tmp.Close()
		if err != nil {
			return nil, err
		}
		object.DownloadToken, err = newAttachmentDownloadToken()
		if err != nil {
			return nil, err
		}
		object.CreatedAt = time.Now().UTC()
		object.CreatedBy = strings.TrimSpace(agentID)
		object.MessageID = responseID
		object.SessionID = sessionID
		object.WorkspaceRoot = batch.Workspace
		object.WorkspacePath = filepath.ToSlash(filepath.Join(batch.relativeDir(), object.ID+"-"+object.SafeName))
		if i > 0 {
			object.SessionBatchID = objects[0].ID
		}
		objects[i] = object
		batch.Files[i] = pendingSessionFile{ID: object.ID, Name: object.SafeName, SHA: object.SHA256, Size: object.SizeBytes}
	}
	// Save the final names before moving them, so interrupted publications can
	// remove only their own files and retain any edits.
	if err := store.writePending(key, batch); err != nil {
		return nil, err
	}
	attachments := make([]MessageAttachment, len(objects))
	for i, object := range objects {
		name := filepath.Base(object.WorkspacePath)
		if _, err := dir.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				err = os.ErrExist
			}
			return nil, err
		}
		if err := dir.Rename(batch.tempName(key, i), name); err != nil {
			return nil, err
		}
		attachments[i] = attachmentFromObject(object)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current, err := openCanonicalSessionDir(batch.Workspace, batch.relativeDir(), false)
	if err != nil {
		return nil, err
	}
	currentInfo, err := current.Stat(".")
	current.Close()
	if err != nil {
		return nil, err
	}
	originalInfo, err := dir.Stat(".")
	if err != nil {
		return nil, err
	}
	if !os.SameFile(currentInfo, originalInfo) {
		return nil, errWorkspaceDirectoryChanged
	}
	// Write the first index last. Its presence commits the whole batch and
	// makes its cleanup idempotent across process exits.
	for i := len(objects) - 1; i >= 0; i-- {
		if err := writeAttachmentObject(store.objectPath(objects[i].ID), objects[i]); err != nil {
			return nil, err
		}
	}
	// Recovery can retire the journal if the process stops here.
	_ = os.Remove(store.pendingPath(key))
	return attachments, nil
}

func (o attachmentObject) validSessionWorkspacePath() bool {
	if o.RoomID != "" || strings.TrimSpace(o.CreatedBy) == "" || !isAttachmentPathSegment(o.SessionID) ||
		!isAttachmentPathSegment(o.MessageID) || !isAttachmentPathSegment(o.SafeName) {
		return false
	}
	expected := filepath.Join(".csgclaw", "attachments", "sessions", o.SessionID, o.MessageID, o.ID+"-"+o.SafeName)
	return o.WorkspacePath == filepath.ToSlash(expected)
}

var ErrSessionAttachmentNotFound = errAttachmentNotFound

// SessionAttachmentFile resolves an attachment ID against its original agent
// and session, then checks the current workspace file.
func (s *Service) SessionAttachmentFile(agentID, sessionID, id string) (AttachmentFile, error) {
	agentID = strings.TrimSpace(agentID)
	if s == nil || s.statePath == "" || agentID == "" || !isAttachmentPathSegment(sessionID) || !validAttachmentID(id) {
		return AttachmentFile{}, errAttachmentNotFound
	}
	store := sessionAttachmentStore(s.statePath)
	s.mu.RLock()
	object, err := store.readObject(id)
	s.mu.RUnlock()
	if err != nil {
		return AttachmentFile{}, err
	}
	if object.RoomID != "" || object.CreatedBy != agentID || object.SessionID != sessionID {
		return AttachmentFile{}, errAttachmentNotFound
	}
	file, err := sessionFile(object)
	if err != nil {
		return AttachmentFile{}, err
	}
	if err := file.refreshWorkspaceAttributes(); err != nil {
		return AttachmentFile{}, err
	}
	return file, nil
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
