package apitypes

import "time"

// RoomAttachment describes a published file without workspace paths or download tokens.
type RoomAttachment struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	MediaType string    `json:"media_type"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
	MessageID string    `json:"message_id"`
	SenderID  string    `json:"sender_id"`
}

type RoomAttachmentListOptions struct {
	Query     string
	MessageID string
	From      int
	Limit     int
}

type RoomAttachmentList struct {
	Items []RoomAttachment `json:"items"`
	Total int              `json:"total"`
	From  int              `json:"from"`
	Limit int              `json:"limit"`
}
