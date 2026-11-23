package im

import (
	"fmt"
	"slices"
	"strings"
)

const MaxMessagePageSize = 200

type MessagePageOptions struct {
	Before, After        string
	Limit                int
	ThreadRootID         string
	IncludeThreadReplies bool
	PublicAutonomousOnly bool
}

type MessagePage struct {
	Messages      []Message `json:"messages"`
	Anchor        *Message  `json:"anchor,omitempty"`
	HasMore       bool      `json:"has_more"`
	CursorMissing bool      `json:"cursor_missing,omitempty"`
}

// ListMessagePage reads in transcript order. With no cursor it returns the
// newest page; before and after are exclusive message IDs. HasMore refers to
// the direction of the request. Only messages in the page are projected.
func (s *Service) ListMessagePage(roomID string, opts MessagePageOptions) (MessagePage, error) {
	if opts.Limit < 1 || opts.Limit > MaxMessagePageSize || (opts.Before != "" && opts.After != "") {
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
	start, end := 0, len(room.Messages)
	cursor := opts.Before
	if opts.After != "" {
		cursor = opts.After
	}
	if cursor != "" {
		index := end - 1
		for index >= 0 && room.Messages[index].ID != cursor {
			index--
		}
		if index < 0 || !inScope(room.Messages[index]) {
			page.CursorMissing = true
			return page, nil
		}
		if opts.After != "" {
			start = index + 1
			anchor := present(room.Messages[index])
			page.Anchor = &anchor
		} else {
			end = index
		}
	}
	collect := func(message Message) bool {
		if !inScope(message) {
			return true
		}
		if len(page.Messages) == opts.Limit {
			page.HasMore = true
			return false
		}
		page.Messages = append(page.Messages, present(message))
		return true
	}
	if opts.After != "" {
		for i := start; i < end && collect(room.Messages[i]); i++ {
		}
	} else {
		for i := end - 1; i >= start && collect(room.Messages[i]); i-- {
		}
		slices.Reverse(page.Messages)
	}
	return page, nil
}
