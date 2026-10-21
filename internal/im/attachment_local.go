package im

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalRoomAttachmentFile provides a named, editable copy for native applications.
// Blobs are content-addressed and must never be handed to editors directly.
func (s *Service) LocalRoomAttachmentFile(roomID, attachmentID string) (AttachmentFile, error) {
	roomID, attachmentID = strings.TrimSpace(roomID), strings.TrimSpace(attachmentID)
	file, err := s.RoomAttachmentFile(roomID, attachmentID)
	if err != nil {
		return AttachmentFile{}, err
	}
	dir := filepath.Join(filepath.Dir(s.statePath), assetsDirName, "open", file.Attachment.ID)
	local := filepath.Join(dir, file.SafeName)
	if info, err := os.Lstat(local); err == nil {
		if !info.Mode().IsRegular() {
			return AttachmentFile{}, errors.New("local attachment is not a regular file")
		}
		file.Path = local
		return file, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return AttachmentFile{}, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return AttachmentFile{}, err
	}
	source, err := os.Open(file.Path)
	if err != nil {
		return AttachmentFile{}, err
	}
	defer source.Close()
	temporary, err := os.CreateTemp(dir, ".opening-*")
	if err != nil {
		return AttachmentFile{}, err
	}
	defer os.Remove(temporary.Name())
	_, copyErr := io.Copy(temporary, source)
	if err := errors.Join(copyErr, temporary.Close()); err != nil {
		return AttachmentFile{}, err
	}
	// Recheck the live reference after copying, then publish without replacing
	// another opener's copy or an external editor's changes.
	s.mu.RLock()
	defer s.mu.RUnlock()
	room, ok := s.rooms[roomID]
	if !ok {
		return AttachmentFile{}, ErrRoomNotFound
	}
	live := false
	for _, item := range roomAttachmentReferences(room) {
		if item.ID == attachmentID {
			live = true
			break
		}
	}
	if !live {
		return AttachmentFile{}, ErrRoomAttachmentNotFound
	}
	if err := os.Link(temporary.Name(), local); err != nil && !errors.Is(err, os.ErrExist) {
		return AttachmentFile{}, err
	}
	info, err := os.Lstat(local)
	if err != nil {
		return AttachmentFile{}, err
	}
	if !info.Mode().IsRegular() {
		return AttachmentFile{}, errors.New("local attachment is not a regular file")
	}
	file.Path = local
	return file, nil
}

func cleanupLocalAttachmentFiles(statePath string, live map[string]struct{}) error {
	root := filepath.Join(filepath.Dir(statePath), assetsDirName, "open")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if _, ok := live[entry.Name()]; !ok {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
