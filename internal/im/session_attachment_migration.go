package im

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Older direct sessions kept their batch references here and shared the room
// object and blob directories. Publish each batch in the session store before
// the room collector can discard its old files.
func migrateLegacySessionAttachments(statePath string) error {
	store := sessionAttachmentStore(statePath)
	entries, err := os.ReadDir(string(store))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var lock *os.File
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if lock == nil {
			lock, err = store.lockFile("migration.lock", true)
			if err != nil {
				return err
			}
			defer lock.Close()
		}
		path := filepath.Join(string(store), entry.Name())
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue // Another process already moved the manifest to refs.
		}
		if err != nil {
			return err
		}
		var ids []string
		if err := json.Unmarshal(data, &ids); err != nil {
			return fmt.Errorf("read legacy session attachment batch %s: %w", entry.Name(), err)
		}
		if len(ids) == 0 || entry.Name() != ids[0]+".json" {
			return fmt.Errorf("invalid legacy session attachment batch %s", entry.Name())
		}
		for _, id := range ids {
			if err := migrateLegacySessionFile(roomAttachmentStore(statePath), store, id); err != nil {
				return fmt.Errorf("migrate legacy session attachment %q: %w", id, err)
			}
		}
		refs := filepath.Join(string(store), "refs")
		if err := os.MkdirAll(refs, 0700); err != nil {
			return err
		}
		published := filepath.Join(refs, entry.Name())
		if err := os.Rename(path, published); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if _, statErr := os.Stat(published); statErr == nil {
					continue
				}
			}
			return err
		}
	}
	return nil
}

func migrateLegacySessionFile(rooms, sessions attachmentStore, id string) error {
	if !validAttachmentID(id) {
		return fmt.Errorf("invalid attachment ID")
	}
	object, err := readAttachmentObject(rooms.objectPath(id))
	if errors.Is(err, errAttachmentNotFound) {
		// An older room collector may already have removed this attachment.
		return nil
	}
	if err != nil {
		return err
	}
	if object.ID != id || object.RoomID != "" || object.SessionID == "" || object.BlobSHA256 != object.SHA256 {
		return fmt.Errorf("invalid session attachment metadata")
	}
	sha := object.BlobSHA256
	source := rooms.blobPath(sha)
	// Preserve the room's immutable bytes when a room message shares this hash.
	// A hard link also avoids copying a large file again on a migration retry.
	if damaged, err := inspectExistingAttachmentBlob(context.Background(), source, sha, object.SizeBytes); err != nil {
		return err
	} else if damaged != nil {
		return fmt.Errorf("invalid legacy attachment blob %s", sha)
	}
	dest := sessions.blobPath(sha)
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	if err := os.Link(source, dest); errors.Is(err, os.ErrExist) {
		if damaged, err := inspectExistingAttachmentBlob(context.Background(), dest, sha, object.SizeBytes); err != nil {
			return err
		} else if damaged != nil {
			return fmt.Errorf("invalid session attachment blob %s", sha)
		}
	} else if err != nil {
		// The stores can reside on different volumes.
		f, err := os.Open(source)
		if errors.Is(err, os.ErrNotExist) {
			// The mutable workspace file may still exist.
			return writeAttachmentObject(sessions.objectPath(id), object)
		}
		if err != nil {
			return err
		}
		defer f.Close()
		if err := atomicWriteReader(dest, f, 0600); err != nil {
			return err
		}
	}
	return writeAttachmentObject(sessions.objectPath(id), object)
}

// MigrateSessionWorkspaces binds old session download IDs to their existing
// workspace files. An unresolved agent or file keeps its original metadata
// and bytes for a later retry. Migration never recreates a user-deleted file.
func (s *Service) MigrateSessionWorkspaces(resolve func(string) (string, error)) error {
	if s == nil || s.statePath == "" || resolve == nil {
		return nil
	}
	store := sessionAttachmentStore(s.statePath)
	dir := filepath.Join(string(store), "refs")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) || err == nil && len(entries) == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	lock, err := store.lockFile("migration.lock", true)
	if err != nil {
		return err
	}
	defer lock.Close()
	entries, err = os.ReadDir(dir)
	if err != nil {
		return err
	}
	var complete []string
	converted, unresolved := map[string]bool{}, map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var ids []string
		if err := json.Unmarshal(data, &ids); err != nil {
			return err
		}
		if len(ids) == 0 || entry.Name() != ids[0]+".json" {
			return fmt.Errorf("invalid session attachment manifest %q", entry.Name())
		}
		all := true
		for _, id := range ids {
			object, err := store.readObject(id)
			if errors.Is(err, errAttachmentNotFound) {
				continue // Already removed by a previous room-store version.
			}
			if err != nil {
				return err
			}
			convertedFile, err := store.bindSessionWorkspace(object, resolve)
			if err != nil {
				return err
			}
			if convertedFile {
				converted[object.SHA256] = true
			} else {
				unresolved[object.SHA256] = true
				all = false
			}
		}
		if all {
			complete = append(complete, path)
		}
	}
	for sha := range converted {
		if !unresolved[sha] {
			if err := removeMissingOK(os.Remove(store.blobPath(sha))); err != nil {
				return err
			}
		}
	}
	// Remove manifests only after releasing their last session-store blobs,
	// so an interrupted startup can safely repeat the conversion.
	for _, path := range complete {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

func (store attachmentStore) bindSessionWorkspace(object attachmentObject, resolve func(string) (string, error)) (bool, error) {
	if object.WorkspaceRoot != "" {
		return true, nil // A previous migration already updated the index.
	}
	if !object.validSessionWorkspacePath() {
		return false, nil
	}
	root, err := resolve(object.CreatedBy)
	if err != nil || !filepath.IsAbs(root) {
		return false, nil
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return false, nil
	}
	object.WorkspaceRoot, object.BlobSHA256 = root, ""
	file, err := sessionFile(object)
	if err != nil {
		return false, err
	}
	opened, err := file.Open()
	if err != nil {
		return false, nil // The workspace may be unavailable or the file deleted.
	}
	opened.Close()
	return true, writeAttachmentObject(store.objectPath(object.ID), object)
}
