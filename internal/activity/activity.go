package activity

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrActionNotFound           = errors.New("action request not found")
	ErrActionInvalidOption      = errors.New("action option is invalid")
	ErrActionAlreadyDecided     = errors.New("action request already decided")
	ErrActionGone               = errors.New("action request is no longer pending")
	ErrUserInputNotFound        = errors.New("user input request not found")
	ErrUserInputInvalidResponse = errors.New("user input response is invalid")
	ErrUserInputAlreadyResolved = errors.New("user input request already resolved")
	ErrUserInputGone            = errors.New("user input request is no longer pending")
)

type ActionStatus string

const (
	ActionKindPermission = "permission"

	ActionOptionScopeAgent = "agent"

	ActionStatusPending  ActionStatus = "pending"
	ActionStatusAllowed  ActionStatus = "allowed"
	ActionStatusRejected ActionStatus = "rejected"
	ActionStatusExpired  ActionStatus = "expired"
	ActionStatusCanceled ActionStatus = "canceled"
)

type ActionOptionSnapshot struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Scope string `json:"scope,omitempty"`
}

type ActionDecisionSnapshot struct {
	OptionID  string    `json:"option_id,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	DecidedAt time.Time `json:"decided_at"`
}

type ActivitySnapshot struct {
	ID          string                  `json:"id"`
	Kind        string                  `json:"kind"`
	Title       string                  `json:"title"`
	Status      ActionStatus            `json:"status"`
	RequestedAt time.Time               `json:"requested_at"`
	ExpiresAt   time.Time               `json:"expires_at"`
	Options     []ActionOptionSnapshot  `json:"options,omitempty"`
	Decision    *ActionDecisionSnapshot `json:"decision,omitempty"`
}

type ExecutionRef struct {
	RuntimeKind string
	RuntimeID   string
	SessionID   string
	TurnID      string
	ToolCallID  string
	ToolKind    string
}

type ActivityDecisionRequest struct {
	Channel    string
	ActivityID string
	OptionID   string
}

type ActivityDecider interface {
	Decide(ctx context.Context, req ActivityDecisionRequest) (ActivitySnapshot, error)
}

type UserInputStatus string

const (
	UserInputStatusPending     UserInputStatus = "pending"
	UserInputStatusAnswered    UserInputStatus = "answered"
	UserInputStatusSkipped     UserInputStatus = "skipped"
	UserInputStatusExpired     UserInputStatus = "expired"
	UserInputStatusCanceled    UserInputStatus = "canceled"
	UserInputStatusInterrupted UserInputStatus = "interrupted"
)

type UserInputOptionSnapshot struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// RequestUserInputOption matches Codex's request_user_input wire type.
type RequestUserInputOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// RequestUserInputQuestion matches Codex's request_user_input wire type.
type RequestUserInputQuestion struct {
	ID       string                   `json:"id"`
	Header   string                   `json:"header"`
	Question string                   `json:"question"`
	IsOther  bool                     `json:"isOther"`
	IsSecret bool                     `json:"isSecret"`
	Options  []RequestUserInputOption `json:"options"`
}

// RequestUserInputArgs matches Codex's request_user_input arguments.
type RequestUserInputArgs struct {
	Questions        []RequestUserInputQuestion `json:"questions"`
	AutoResolutionMS *uint64                    `json:"autoResolutionMs,omitempty"`
}

// RequestUserInputAnswer matches one Codex request_user_input answer.
type RequestUserInputAnswer struct {
	Answers []string `json:"answers"`
}

// RequestUserInputResponse matches Codex's request_user_input response.
type RequestUserInputResponse struct {
	Answers map[string]RequestUserInputAnswer `json:"answers"`
}

// ResourceLink matches the resource_link content block exposed by Codex.
// Rendering uses Name, Title, URI, Description, and the first safe icon while
// retaining the remaining source-compatible fields for future renderers.
type ResourceLink struct {
	Type        string           `json:"type"`
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	URI         string           `json:"uri"`
	Description string           `json:"description,omitempty"`
	MIMEType    string           `json:"mimeType,omitempty"`
	Size        *uint64          `json:"size,omitempty"`
	Annotations json.RawMessage  `json:"annotations,omitempty"`
	Meta        json.RawMessage  `json:"_meta,omitempty"`
	Icons       []map[string]any `json:"icons,omitempty"`
}

// StructuredOutputArtifact is the normalized result of decoding CSGClaw
// control records from one completed command output.
type StructuredOutputArtifact struct {
	RequestUserInput *RequestUserInputArgs `json:"requestUserInput,omitempty"`
	ResourceLinks    []ResourceLink        `json:"resourceLinks,omitempty"`
}

type UserInputQuestionSnapshot struct {
	ID       string                    `json:"id"`
	Header   string                    `json:"header"`
	Question string                    `json:"question"`
	Options  []UserInputOptionSnapshot `json:"options,omitempty"`
	IsOther  bool                      `json:"is_other,omitempty"`
	IsSecret bool                      `json:"is_secret,omitempty"`
}

type UserInputAnswerSnapshot struct {
	Answered    bool   `json:"answered"`
	OptionIndex int    `json:"option_index,omitempty"`
	OptionLabel string `json:"option_label,omitempty"`
	Text        string `json:"text,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Skipped     bool   `json:"skipped,omitempty"`
}

type UserInputSnapshot struct {
	ID            string                             `json:"id"`
	Channel       string                             `json:"channel,omitempty"`
	RoomID        string                             `json:"room_id,omitempty"`
	ThreadRootID  string                             `json:"thread_root_id,omitempty"`
	Status        UserInputStatus                    `json:"status"`
	Questions     []UserInputQuestionSnapshot        `json:"questions"`
	Answers       map[string]UserInputAnswerSnapshot `json:"answers,omitempty"`
	RequestedAt   time.Time                          `json:"requested_at"`
	ResolvedAt    *time.Time                         `json:"resolved_at,omitempty"`
	AutoResolveAt *time.Time                         `json:"auto_resolve_at,omitempty"`
	ResponderID   string                             `json:"responder_id,omitempty"`
}

type UserInputResponseRequest struct {
	Channel          string                                         `json:"channel,omitempty"`
	ActivityID       string                                         `json:"activity_id,omitempty"`
	RoomID           string                                         `json:"room_id,omitempty"`
	ResponderID      string                                         `json:"responder_id,omitempty"`
	Response         RequestUserInputResponse                       `json:"response"`
	RecordTranscript func(context.Context, UserInputSnapshot) error `json:"-"`
}

type UserInputResponder interface {
	Respond(ctx context.Context, req UserInputResponseRequest) (UserInputSnapshot, error)
	Get(requestID string) (UserInputSnapshot, bool)
}

type RuntimeEventKind string

const (
	RuntimeEventUserMessageDelta  RuntimeEventKind = "user_message_delta"
	RuntimeEventTextDelta         RuntimeEventKind = "text_delta"
	RuntimeEventThoughtDelta      RuntimeEventKind = "thought_delta"
	RuntimeEventToolCallStart     RuntimeEventKind = "tool_call_start"
	RuntimeEventToolCallUpdate    RuntimeEventKind = "tool_call_update"
	RuntimeEventPlanUpdate        RuntimeEventKind = "plan_update"
	RuntimeEventActionRequest     RuntimeEventKind = "action_request"
	RuntimeEventActionDecision    RuntimeEventKind = "action_decision"
	RuntimeEventUserInputRequest  RuntimeEventKind = "user_input_request"
	RuntimeEventUserInputResolved RuntimeEventKind = "user_input_resolved"
	RuntimeEventStructuredOutput  RuntimeEventKind = "structured_output"
	RuntimeEventPromptCompleted   RuntimeEventKind = "prompt_completed"
	RuntimeEventPromptFailed      RuntimeEventKind = "prompt_failed"
)

type RuntimeEvent struct {
	RuntimeKind       string
	RuntimeID         string
	SessionID         string
	TurnID            string
	Kind              RuntimeEventKind
	ReceivedAt        time.Time
	MessageID         string
	Text              string
	ToolCallID        string
	ToolKind          string
	ToolTitle         string
	ToolStatus        string
	ToolInputSummary  string
	ToolOutputSummary string
	ActionID          string
	ActionStatus      string
	ActionOptionID    string
	ActionOptionKind  string
	UserInputID       string
	UserInputStatus   string
	StopReason        string
	Error             string
	Payload           any
}

type RuntimeEventSink interface {
	Publish(RuntimeEvent)
}

type RuntimeEventSubscriber interface {
	Subscribe(runtimeID string) (<-chan RuntimeEvent, func())
}

func RuntimeEventRequiresReliableDelivery(event RuntimeEvent) bool {
	switch event.Kind {
	case RuntimeEventTextDelta,
		RuntimeEventActionRequest, RuntimeEventActionDecision,
		RuntimeEventUserInputRequest, RuntimeEventUserInputResolved,
		RuntimeEventStructuredOutput:
		return true
	case RuntimeEventPromptCompleted, RuntimeEventPromptFailed:
		return true
	default:
		return false
	}
}
