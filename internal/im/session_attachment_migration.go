package im

import (
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
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
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
	if damaged, err := inspectExistingAttachmentBlob(source, sha, object.SizeBytes); err != nil {
		return err
	} else if damaged != nil {
		return fmt.Errorf("invalid legacy attachment blob %s", sha)
	}
	dest := sessions.blobPath(sha)
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	if err := os.Link(source, dest); errors.Is(err, os.ErrExist) {
		if damaged, err := inspectExistingAttachmentBlob(dest, sha, object.SizeBytes); err != nil {
			return err
		} else if damaged != nil {
			return fmt.Errorf("invalid session attachment blob %s", sha)
		}
	} else if err != nil {
		// The stores can reside on different volumes.
		f, err := os.Open(source)
		if errors.Is(err, os.ErrNotExist) {
			return nil
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
