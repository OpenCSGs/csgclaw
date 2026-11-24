package im

import (
	"fmt"
	"slices"
	"strings"
)

const MaxMessagePageSize = 200

type MessagePageOptions struct {
	Before               string
	Limit                int
	RefreshIDs           []string
	ThreadRootID         string
	IncludeThreadReplies bool
	PublicAutonomousOnly bool
}

type MessagePage struct {
	Messages      []Message `json:"messages"`
	Refreshed     []Message `json:"refreshed,omitempty"`
	HasMore       bool      `json:"has_more"`
	CursorMissing bool      `json:"cursor_missing,omitempty"`
}

// ListMessagePage reads in transcript order. With no cursor it returns the
// newest page. Before is an exclusive message ID. RefreshIDs selects extra
// snapshots without affecting the page boundary.
func (s *Service) ListMessagePage(roomID string, opts MessagePageOptions) (MessagePage, error) {
	if opts.Limit < 1 || opts.Limit > MaxMessagePageSize || len(opts.RefreshIDs) > MaxMessagePageSize {
		return MessagePage{}, fmt.Errorf("invalid message page options")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	room, ok := s.rooms[strings.TrimSpace(roomID)]
	if !ok {
		return MessagePage{}, ErrRoomNotFound
	}
	page := MessagePage{Messages: make([]Message, 0, opts.Limit)}
	publicOnly := opts.PublicAutonomousOnly && room.IsAutonomous()
	inScope := func(message Message) bool {
		if publicOnly && IsAgentActivityMessage(message) {
			return false
		}
		if opts.ThreadRootID != "" {
			return message.ID == opts.ThreadRootID || (isThreadReply(message) && message.RelatesTo.EventID == opts.ThreadRootID)
		}
		return opts.IncludeThreadReplies || !isThreadReply(message)
	}
	present := func(message Message) Message {
		message = s.presentMessageLocked(*room, message, "")
		if publicOnly {
			message = PublicMessageView(message)
		}
		return message
	}
	end := len(room.Messages)
	if opts.Before != "" {
		end--
		for end >= 0 && room.Messages[end].ID != opts.Before {
			end--
		}
		if end < 0 || !inScope(room.Messages[end]) {
			page.CursorMissing = true
			return page, nil
		}
	}
	for i := end - 1; i >= 0; i-- {
		message := room.Messages[i]
		if !inScope(message) {
			continue
		}
		if len(page.Messages) == opts.Limit {
			page.HasMore = true
			break
		}
		page.Messages = append(page.Messages, present(message))
	}
	slices.Reverse(page.Messages)
	if len(opts.RefreshIDs) > 0 {
		wanted := make(map[string]struct{}, len(opts.RefreshIDs))
		for _, id := range opts.RefreshIDs {
			wanted[id] = struct{}{}
		}
		for _, message := range page.Messages {
			delete(wanted, message.ID)
		}
		for i := len(room.Messages) - 1; i >= 0 && len(wanted) > 0; i-- {
			message := room.Messages[i]
			if _, ok := wanted[message.ID]; ok {
				delete(wanted, message.ID)
				if inScope(message) {
					page.Refreshed = append(page.Refreshed, present(message))
				}
			}
		}
	}
	return page, nil
}
