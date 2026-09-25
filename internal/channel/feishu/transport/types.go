package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
)

var (
	ErrInvalidConfig    = errors.New("invalid feishu transport config")
	ErrNilSink          = errors.New("feishu transport sink is required")
	ErrAlreadyStarted   = errors.New("feishu transport is already started")
	ErrNotStarted       = errors.New("feishu transport is not started")
	ErrClosed           = errors.New("feishu transport is closed")
	ErrUnsupportedEvent = errors.New("unsupported feishu transport event")
	ErrPayloadTooLarge  = errors.New("feishu transport payload is too large")
)

// Config contains the credentials for one Feishu app binding.
//
// AppSecret must never be logged or included in an error.
type Config struct {
	AppID     string
	AppSecret string
}

type Identity struct {
	OpenID  string
	UserID  string
	UnionID string
	Name    string
}

type EventKind string

const (
	EventMessage    EventKind = "message"
	EventComment    EventKind = "comment"
	EventCardAction EventKind = "card_action"
)

type ChatType string

const (
	ChatP2P   ChatType = "p2p"
	ChatGroup ChatType = "group"
)

type ChatMode string

const (
	ModeP2P   ChatMode = "p2p"
	ModeGroup ChatMode = "group"
	ModeTopic ChatMode = "topic"
)

type SenderType string

const (
	SenderUser SenderType = "user"
	SenderBot  SenderType = "bot"
)

// Event is the transport-neutral event delivered to the Feishu ingress layer.
// EventID is the source event ID and Message.ID is the source message ID.
type Event struct {
	Kind       EventKind
	EventID    string
	OccurredAt time.Time

	Message    *Message
	Comment    *Comment
	CardAction *CardAction
}

// Comment identifies one document-comment notification. Content is fetched
// lazily outside the protocol callback so ingress remains fast and bounded.
type Comment struct {
	FileToken    string
	FileType     string
	CommentID    string
	ReplyID      string
	Operator     Identity
	MentionedBot bool
	CreatedAt    time.Time
}

type CommentTarget struct {
	FileToken string
	FileType  string
}

type CommentElement struct {
	Type     string
	Text     string
	URL      string
	PersonID string
}

type CommentReply struct {
	ID       string
	Elements []CommentElement
}

type CommentThread struct {
	Replies []CommentReply
	Quote   string
	IsWhole bool
}

type ReplyCommentRequest struct {
	Target    CommentTarget
	CommentID string
	Text      string
	TopLevel  bool
}

type Message struct {
	ID           string
	ChatID       string
	ChatType     ChatType
	ThreadID     string
	RootID       string
	ParentID     string
	Sender       Identity
	SenderType   SenderType
	Text         string
	ContentType  string
	RawContent   json.RawMessage
	Resources    []Resource
	Mentions     []Mention
	MentionedBot bool
	CreatedAt    time.Time
}

type Resource struct {
	Kind string
	ID   string
	Name string
	Size int64
	URL  string
}

type Mention struct {
	Key    string
	OpenID string
	UserID string
	Name   string
	IsBot  *bool
}

type CardAction struct {
	MessageID     string
	ChatID        string
	ChatType      ChatType
	Mode          ChatMode
	ThreadID      string
	Operator      Identity
	ActionValue   map[string]any
	FormValue     map[string]any
	RawContent    json.RawMessage
	CreatedAt     time.Time
	ExplicitScope string
	InheritScope  string
	Token         string
	Host          string
	DeliveryType  string
}

// Sink accepts normalized protocol events. Implementations should claim or
// enqueue the event promptly and must not run a long-lived Agent turn inline.
type Sink interface {
	HandleEvent(context.Context, Event) error
}

type SinkFunc func(context.Context, Event) error

func (f SinkFunc) HandleEvent(ctx context.Context, event Event) error {
	return f(ctx, event)
}

type SendCardRequest struct {
	ChatID string
	Card   map[string]any
	// IdempotencyKey is a process-local delivery ID. It is hashed into the UUID
	// sent to Feishu and is never sent verbatim.
	IdempotencyKey string
	ReplyTo        string
	ReplyInThread  bool
	ThreadID       string
}

type UploadImageRequest struct {
	MediaType string
	SizeBytes int64
	Content   io.Reader
}

type UploadFileRequest struct {
	Name      string
	SizeBytes int64
	Content   io.Reader
}

type UploadResult struct {
	Key string
}

type SendImageRequest struct {
	ChatID   string
	ImageKey string
	// IdempotencyKey is a process-local delivery ID. It is hashed into the UUID
	// sent to Feishu and is never sent verbatim.
	IdempotencyKey string
	ReplyTo        string
	ReplyInThread  bool
	ThreadID       string
}

type SendFileRequest struct {
	ChatID  string
	FileKey string
	// IdempotencyKey is a process-local delivery ID. It is hashed into the UUID
	// sent to Feishu and is never sent verbatim.
	IdempotencyKey string
	ReplyTo        string
	ReplyInThread  bool
	ThreadID       string
}

type UpdateCardRequest struct {
	MessageID string
	Card      map[string]any
}

type SendResult struct {
	MessageID string
}

type AddReactionRequest struct {
	MessageID string
	EmojiType string
}

type AddReactionResult struct {
	ReactionID string
}

type DeleteReactionRequest struct {
	MessageID  string
	ReactionID string
}

type DownloadResourceType string

const (
	DownloadImage DownloadResourceType = "image"
	DownloadFile  DownloadResourceType = "file"
)

type DownloadResourceRequest struct {
	MessageID       string
	FileKey         string
	Type            DownloadResourceType
	DestinationPath string
	MaxBytes        int64
}

type DownloadResourceResult struct {
	ContentType  string
	BytesWritten int64
}

// Adapter owns one Feishu app connection and its protocol operations.
// Start must receive the binding worker's long-lived context.
type Adapter interface {
	Start(context.Context) error
	Close(context.Context) error
	Identity() Identity

	SendCard(context.Context, SendCardRequest) (SendResult, error)
	UpdateCard(context.Context, UpdateCardRequest) error
	UploadImage(context.Context, UploadImageRequest) (UploadResult, error)
	UploadFile(context.Context, UploadFileRequest) (UploadResult, error)
	SendImage(context.Context, SendImageRequest) (SendResult, error)
	SendFile(context.Context, SendFileRequest) (SendResult, error)
	AddReaction(context.Context, AddReactionRequest) (AddReactionResult, error)
	DeleteReaction(context.Context, DeleteReactionRequest) error
	DownloadResource(context.Context, DownloadResourceRequest) (DownloadResourceResult, error)
}

// IdentityPreparer is implemented by transports that must establish the bot
// identity before opening ingress. Binding workers use it to preserve exact
// group-mention filtering during the connection startup window.
type IdentityPreparer interface {
	PrepareIdentity(context.Context) (Identity, error)
}

// CommentAdapter is optional because comment subscriptions are configured in
// the Feishu developer console. Production adapters implement it on the same
// connection/client owner as chat ingress; tests and deployments that do not
// enable document comments need not provide a second transport.
type CommentAdapter interface {
	ResolveCommentTarget(context.Context, string, string) (CommentTarget, bool, error)
	FetchComment(context.Context, CommentTarget, string) (CommentThread, error)
	ReplyToComment(context.Context, ReplyCommentRequest) error
}

// MessageAdapter is optional because quoted-message lookup depends on the
// Feishu app's read permissions. Ingress degrades to ID-only context when it
// is unavailable or the remote message cannot be read.
type MessageAdapter interface {
	FetchMessage(context.Context, string) (Message, bool, error)
}

type Factory interface {
	New(Config, Sink) (Adapter, error)
}

// FactoryFunc lets channel managers inject a test adapter without importing
// the protocol SDK outside this package.
type FactoryFunc func(Config, Sink) (Adapter, error)

func (f FactoryFunc) New(config Config, sink Sink) (Adapter, error) {
	return f(config, sink)
}
