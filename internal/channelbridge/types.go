package channelbridge

import "context"

type BotEvent struct {
	Channel       string            `json:"channel,omitempty"`
	ParticipantID string            `json:"participant_id,omitempty"`
	MessageID     string            `json:"message_id"`
	RoomID        string            `json:"room_id"`
	ChatType      string            `json:"chat_type"`
	Text          string            `json:"text"`
	Mentions      []string          `json:"mentions,omitempty"`
	ThreadRootID  string            `json:"thread_root_id,omitempty"`
	ThreadContext *BotThreadContext `json:"thread_context,omitempty"`
}

type BotThreadContext struct {
	RootMessageID string                    `json:"root_message_id"`
	Context       []BotThreadContextMessage `json:"context,omitempty"`
	Summary       BotThreadContextSummary   `json:"summary"`
}

type BotThreadContextMessage struct {
	ID        string `json:"id,omitempty"`
	SenderID  string `json:"sender_id,omitempty"`
	Content   string `json:"content,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type BotThreadContextSummary struct {
	RootExcerpt  string `json:"root_excerpt,omitempty"`
	MessageCount int    `json:"message_count,omitempty"`
	BeforeCount  int    `json:"before_count,omitempty"`
	AfterCount   int    `json:"after_count,omitempty"`
}

type SendMessageRequest struct {
	RoomID       string `json:"room_id"`
	Text         string `json:"text"`
	ThreadRootID string `json:"thread_root_id,omitempty"`
}

type SendMessageResponse struct {
	MessageID string `json:"message_id"`
}

type UpdateMessageRequest struct {
	RoomID    string `json:"room_id,omitempty"`
	MessageID string `json:"message_id"`
	Text      string `json:"text"`
}

type UpdateMessageResponse struct {
	MessageID string `json:"message_id"`
}

type AddMessageReactionRequest struct {
	MessageID string `json:"message_id"`
	EmojiType string `json:"emoji_type,omitempty"`
}

type AddMessageReactionResponse struct {
	ReactionID string `json:"reaction_id"`
}

type DeleteMessageReactionRequest struct {
	MessageID  string `json:"message_id"`
	ReactionID string `json:"reaction_id"`
}

type BotClient interface {
	StreamEvents(ctx context.Context, botID, lastEventID string) (<-chan BotEvent, <-chan error)
	SendMessage(ctx context.Context, botID string, req SendMessageRequest) (SendMessageResponse, error)
}

type MessageUpdater interface {
	UpdateMessage(ctx context.Context, botID string, req UpdateMessageRequest) (UpdateMessageResponse, error)
}

type MessageReactor interface {
	AddMessageReaction(ctx context.Context, botID string, req AddMessageReactionRequest) (AddMessageReactionResponse, error)
	DeleteMessageReaction(ctx context.Context, botID string, req DeleteMessageReactionRequest) error
}
