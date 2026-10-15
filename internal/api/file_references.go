package api

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/im"
	"csgclaw/internal/utils/filebrowse"
)

var errFileReference = errors.New("file reference unavailable")

// FileReference is a source identity, never a client-supplied host filename.
type FileReference struct {
	Source       string `json:"source"`
	AgentID      string `json:"agent_id,omitempty"`
	Path         string `json:"path,omitempty"`
	AttachmentID string `json:"attachment_id,omitempty"`
}

// Referenced sources are resolved after the IM idempotency check. The request
// owns opened handles until storage finishes, including quota/error paths.
func (h *Handler) resolveFileReferences(roomID, callerAgentID string, refs []FileReference) ([]im.MessageAttachmentUpload, func(), error) {
	var handles []*os.File
	var snapshots []string
	closeAll := func() {
		for _, handle := range handles {
			_ = handle.Close()
		}
		for _, snapshot := range snapshots {
			_ = os.Remove(snapshot)
		}
	}
	fail := func(err error) ([]im.MessageAttachmentUpload, func(), error) {
		closeAll()
		return nil, func() {}, fmt.Errorf("%w: %v", errFileReference, err)
	}
	if h.im == nil {
		return fail(fmt.Errorf("IM service unavailable"))
	}
	room, ok := h.im.Room(roomID)
	if !ok {
		return fail(im.ErrRoomNotFound)
	}
	seen := make(map[string]bool)
	var uploads []im.MessageAttachmentUpload
	for _, ref := range refs {
		key := ref.Source + "\x00" + ref.AgentID + "\x00" + ref.Path + "\x00" + ref.AttachmentID
		if seen[key] {
			continue
		}
		seen[key] = true
		if len(uploads) >= im.MaxAttachmentsPerMessage {
			return fail(fmt.Errorf("too many referenced files"))
		}
		var handle *os.File
		var err error
		var name, mediaType string
		switch ref.Source {
		case "workspace":
			if h.svc == nil || h.workspace == nil {
				return fail(fmt.Errorf("workspace unavailable"))
			}
			item, exists := h.svc.Agent(ref.AgentID)
			if !exists {
				return fail(errAgentWorkspaceNotFound)
			}
			// Scoped runtimes cannot use message attachments to bypass another
			// Agent's workspace boundary. The human UI can explicitly share files.
			if callerAgentID != "" && item.ID != callerAgentID {
				return fail(fmt.Errorf("source workspace is outside Agent scope"))
			}
			member := false
			for _, id := range room.Members {
				if h.participantBridgeTargetForRoomMember(item.ID).matches(id) {
					member = true
					break
				}
			}
			if !member {
				return fail(fmt.Errorf("source agent is not a room member"))
			}
			root, rootErr := h.conversationWorkspaceRoot(item.ID, roomID)
			if rootErr != nil {
				return fail(rootErr)
			}
			handle, err = filebrowse.OpenFile(root, ref.Path)
			name = filepath.Base(filepath.FromSlash(ref.Path))
			mediaType = mime.TypeByExtension(filepath.Ext(name))
			if err != nil {
				handle, err = filebrowse.ArchiveDirectory(root, ref.Path, im.MaxAttachmentFileBytes)
				if err == nil {
					snapshots = append(snapshots, handle.Name())
					name += ".zip"
					mediaType = "application/zip"
				}
			}
		case "attachment":
			file, fileErr := h.im.RoomAttachmentFile(roomID, ref.AttachmentID)
			if fileErr != nil {
				return fail(fileErr)
			}
			handle, err = os.Open(file.Path)
			name, mediaType = file.Attachment.Name, file.MediaType
		default:
			return fail(fmt.Errorf("invalid reference source"))
		}
		if err != nil {
			return fail(err)
		}
		handles = append(handles, handle)
		info, err := handle.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return fail(fmt.Errorf("source is not a readable file"))
		}
		if strings.TrimSpace(name) == "" {
			return fail(fmt.Errorf("source filename required"))
		}
		uploads = append(uploads, im.MessageAttachmentUpload{Name: name, MediaType: mediaType, SizeBytes: info.Size(), Open: func() (io.ReadCloser, error) { return handle, nil }})
	}
	return uploads, closeAll, nil
}
