// Package channel contains transport-neutral identities and records shared by
// channel adapters.
package channel

import (
	"strings"
	"time"

	"csgclaw/internal/agentengine"
)

// ChannelID identifies a channel implementation, such as csgclaw or feishu.
type ChannelID string

const (
	ChannelCSGClaw = "csgclaw"
	ChannelFeishu  = "feishu"
)

// BindingID is a stable channel binding identity, not a Runtime or Session ID.
type BindingID string

// SourceEventID is the source-side dedupe key for a message or action.
type SourceEventID string

// ActorRef is a source sender or interaction actor.
type ActorRef struct {
	ID          string
	DisplayName string
	IsBot       bool
}

// ConversationScope is channel context used to build an Engine ConversationKey.
type ConversationScope struct {
	BindingID BindingID
	RoomID    string
	ThreadID  string
	ReplyToID string
}

// Binding is the stable relationship between one external channel identity and
// one CSGClaw Agent. Runtime and native session identifiers deliberately do not
// belong here: they may change while the channel connection stays alive.
type Binding struct {
	ID            string `json:"id"`
	Channel       string `json:"channel"`
	AgentID       string `json:"agent_id"`
	ParticipantID string `json:"participant_id"`
	Enabled       bool   `json:"enabled,omitempty"`
}

// StableID returns the persistent binding identity, falling back to ParticipantID.
func (b Binding) StableID() BindingID {
	if id := strings.TrimSpace(b.ID); id != "" {
		return BindingID(id)
	}
	return BindingID(strings.TrimSpace(b.ParticipantID))
}

// TurnContext is channel-owned identity for rendering and source delivery.
// SourceMessageID is a dedupe key, not a TurnID.
type TurnContext struct {
	BindingID       BindingID
	ParticipantID   string
	AgentID         string
	RoomID          string
	Locale          string
	ChatType        string
	ThreadRootID    string
	SourceMessageID string
	ConversationKey agentengine.ConversationKey
	TurnID          agentengine.TurnID
}

// Outcome keeps the channel context next to the normalized Engine result.
type Outcome struct {
	Turn   TurnContext
	Result agentengine.TurnResult
}

// Source identifies the external event that produced one normalized inbound
// item.
type Source struct {
	Channel       string `json:"channel"`
	BindingID     string `json:"binding_id"`
	ParticipantID string `json:"participant_id,omitempty"`
	EventID       string `json:"event_id"`
	// DedupID identifies the logical channel item across multiple ingress
	// sources. For example, a Feishu message delivered by both local handoff
	// and WebSocket has different event IDs but the same message ID.
	DedupID   string `json:"dedup_id,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	ChatID    string `json:"chat_id,omitempty"`
	ChatType  string `json:"chat_type,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
	RootID    string `json:"root_id,omitempty"`
	ParentID  string `json:"parent_id,omitempty"`
}

// InboundFile is a channel-owned reference that must be downloaded and
// authorized before it is converted to an Agent Engine InputFile.
type InboundFile struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	URL       string `json:"url,omitempty"`
}

// InboundMessage is the transport-neutral input accepted by a channel
// execution runner.
type InboundMessage struct {
	Source          Source         `json:"source"`
	AgentID         string         `json:"agent_id"`
	ConversationKey string         `json:"conversation_key"`
	TurnID          string         `json:"turn_id"`
	Text            string         `json:"text,omitempty"`
	Files           []InboundFile  `json:"files,omitempty"`
	QuotedMessage   *QuotedMessage `json:"quoted_message,omitempty"`
	ReplyTarget     *ReplyTarget   `json:"reply_target,omitempty"`
}

// QuotedMessage is channel-fetched context for an ordinary reply or topic
// message. Its text remains untrusted user content when rendered for an Agent.
type QuotedMessage struct {
	ID         string `json:"id"`
	SenderID   string `json:"sender_id,omitempty"`
	SenderName string `json:"sender_name,omitempty"`
	SenderType string `json:"sender_type,omitempty"`
	Text       string `json:"text,omitempty"`
}

// ReplyTarget describes a non-chat response surface without importing a
// channel SDK type into the shared execution record. Chat replies continue to
// use Source.ChatID/MessageID/ThreadID and leave this field nil.
type ReplyTarget struct {
	Kind         string `json:"kind"`
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type,omitempty"`
	ParentID     string `json:"parent_id,omitempty"`
	TopLevel     bool   `json:"top_level,omitempty"`
}

const ReplyTargetComment = "comment"

// TurnStatus is the channel-side lifecycle of one Engine invocation.
type TurnStatus string

const (
	TurnAccepted  TurnStatus = "accepted"
	TurnRunning   TurnStatus = "running"
	TurnSucceeded TurnStatus = "succeeded"
	TurnFailed    TurnStatus = "failed"
	TurnCanceled  TurnStatus = "canceled"
)

// TurnRecord correlates an Engine Turn with process-local delivery intents. It
// does not replace Agent Engine admission or active-Turn state.
type TurnRecord struct {
	TurnID          string     `json:"turn_id"`
	AgentID         string     `json:"agent_id"`
	BindingID       string     `json:"binding_id"`
	ConversationKey string     `json:"conversation_key"`
	Status          TurnStatus `json:"status"`
	LastSequence    uint64     `json:"last_sequence,omitempty"`
}

// DeliveryKind identifies one channel-side effect.
type DeliveryKind string

const (
	DeliveryText           DeliveryKind = "text"
	DeliveryMarkdown       DeliveryKind = "markdown"
	DeliveryMarkdownUpdate DeliveryKind = "markdown_update"
	DeliveryCard           DeliveryKind = "card"
	DeliveryCardUpdate     DeliveryKind = "card_update"
	DeliveryReactionAdd    DeliveryKind = "reaction_add"
	DeliveryReactionDelete DeliveryKind = "reaction_delete"
	DeliveryCommentReply   DeliveryKind = "comment_reply"
)

// DeliveryStatus is the process-local state of one outbound delivery.
type DeliveryStatus string

const (
	DeliveryPending     DeliveryStatus = "pending"
	DeliveryDispatching DeliveryStatus = "dispatching"
	DeliveryDelivered   DeliveryStatus = "delivered"
	DeliveryFailed      DeliveryStatus = "failed"
)

// DeliveryIntent describes one process-local channel API call. ID is stable so
// streaming updates and dependencies can be correlated while a binding runs.
type DeliveryIntent struct {
	ID            string         `json:"id"`
	BindingID     string         `json:"binding_id"`
	TurnID        string         `json:"turn_id"`
	Sequence      uint64         `json:"sequence,omitempty"`
	Kind          DeliveryKind   `json:"kind"`
	Status        DeliveryStatus `json:"status"`
	ChatID        string         `json:"chat_id,omitempty"`
	MessageID     string         `json:"message_id,omitempty"`
	RelatedID     string         `json:"related_id,omitempty"`
	ReplyTo       string         `json:"reply_to,omitempty"`
	ThreadID      string         `json:"thread_id,omitempty"`
	ResourceID    string         `json:"resource_id,omitempty"`
	ResourceType  string         `json:"resource_type,omitempty"`
	ParentID      string         `json:"parent_id,omitempty"`
	TopLevel      bool           `json:"top_level,omitempty"`
	Text          string         `json:"text,omitempty"`
	Card          map[string]any `json:"card,omitempty"`
	EmojiType     string         `json:"emoji_type,omitempty"`
	ReactionID    string         `json:"reaction_id,omitempty"`
	Attempts      int            `json:"attempts,omitempty"`
	LastError     string         `json:"last_error,omitempty"`
	NextAttemptAt *time.Time     `json:"next_attempt_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}
