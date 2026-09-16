package im

import (
	"fmt"
	"sort"
	"strings"

	"csgclaw/internal/apitypes"
)

// ListRoomAttachments uses the same live references as attachment garbage collection.
// It includes thread replies and retained context, regardless of transcript paging.
func (s *Service) ListRoomAttachments(roomID string, opts apitypes.RoomAttachmentListOptions) (apitypes.RoomAttachmentList, error) {
	if opts.From < 0 || opts.Limit < 0 || opts.Limit > 200 {
		return apitypes.RoomAttachmentList{}, fmt.Errorf("from must be nonnegative and limit must be between 1 and 200")
	}
	if opts.Limit == 0 {
		opts.Limit = 50
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	room, ok := s.rooms[strings.TrimSpace(roomID)]
	if !ok {
		return apitypes.RoomAttachmentList{}, ErrRoomNotFound
	}
	items := roomAttachmentReferences(room)
	query, messageID := strings.ToLower(strings.TrimSpace(opts.Query)), strings.TrimSpace(opts.MessageID)
	filtered := make([]apitypes.RoomAttachment, 0, len(items))
	for _, item := range items {
		if (query == "" || strings.Contains(strings.ToLower(item.Name), query)) && (messageID == "" || item.MessageID == messageID) {
			filtered = append(filtered, item)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].ID > filtered[j].ID
		}
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	start := min(opts.From, len(filtered))
	end := start + min(opts.Limit, len(filtered)-start)
	return apitypes.RoomAttachmentList{Items: filtered[start:end], Total: len(filtered), From: opts.From, Limit: opts.Limit}, nil
}

// RoomAttachmentFile rejects an ID unless it is still referenced by this room.
func (s *Service) RoomAttachmentFile(roomID, attachmentID string) (AttachmentFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	room, ok := s.rooms[strings.TrimSpace(roomID)]
	if !ok {
		return AttachmentFile{}, ErrRoomNotFound
	}
	for _, item := range roomAttachmentReferences(room) {
		if item.ID == strings.TrimSpace(attachmentID) {
			return s.AttachmentFile(item.ID)
		}
	}
	return AttachmentFile{}, fmt.Errorf("attachment not found in room")
}

func roomAttachmentReferences(room *Room) []apitypes.RoomAttachment {
	seen := make(map[string]bool)
	items := make([]apitypes.RoomAttachment, 0)
	collect := func(messages []Message) {
		for _, message := range messages {
			for _, a := range message.Attachments {
				if a.ID == "" || seen[a.ID] {
					continue
				}
				seen[a.ID] = true
				items = append(items, apitypes.RoomAttachment{ID: a.ID, Name: a.Name, Kind: a.Kind, MediaType: a.MediaType, SizeBytes: a.SizeBytes, SHA256: a.SHA256, CreatedAt: a.CreatedAt, MessageID: message.ID, SenderID: message.SenderID})
			}
		}
	}
	collect(room.Messages)
	for _, thread := range room.Threads {
		collect(thread.Context)
	}
	return items
}
