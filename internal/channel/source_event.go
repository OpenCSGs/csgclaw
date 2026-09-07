package channel

import "csgclaw/internal/apitypes"

type MessageAttachment = apitypes.MessageAttachment

// Event is the transport-neutral message delivered from a Channel source to a
// Binding-scoped ingress worker.
type Event struct {
	Channel       string              `json:"channel,omitempty"`
	ParticipantID string              `json:"participant_id,omitempty"`
	MessageID     string              `json:"message_id"`
	RoomID        string              `json:"room_id"`
	Locale        string              `json:"locale,omitempty"`
	ChatType      string              `json:"chat_type"`
	Text          string              `json:"text"`
	Attachments   []MessageAttachment `json:"attachments,omitempty"`
	Mentions      []string            `json:"mentions,omitempty"`
	Mentioned     bool                `json:"mentioned,omitempty"`
	ThreadRootID  string              `json:"thread_root_id,omitempty"`
	ThreadContext *ThreadContext      `json:"thread_context,omitempty"`
}

type ThreadContext struct {
	RootMessageID string                 `json:"root_message_id"`
	Context       []ThreadContextMessage `json:"context,omitempty"`
	Summary       ThreadContextSummary   `json:"summary"`
}

type ThreadContextMessage struct {
	ID          string              `json:"id,omitempty"`
	SenderID    string              `json:"sender_id,omitempty"`
	Content     string              `json:"content,omitempty"`
	CreatedAt   string              `json:"created_at,omitempty"`
	Attachments []MessageAttachment `json:"attachments,omitempty"`
}

type ThreadContextSummary struct {
	RootExcerpt  string `json:"root_excerpt,omitempty"`
	MessageCount int    `json:"message_count,omitempty"`
	BeforeCount  int    `json:"before_count,omitempty"`
	AfterCount   int    `json:"after_count,omitempty"`
}
