package agentengine

import "context"

// ConversationInterface executes conversations for the Agent selected by
// Conversations. Phase 1 intentionally exposes only the operation used by the
// Session API.
type ConversationInterface interface {
	Run(ctx context.Context, request TurnRequest, sink EventSink) TurnResult

	// Run cancellation is intentionally context-based. Enable Cancel only when
	// an independent caller must address and cancel an active turn by
	// ConversationKey and TurnID; do not add it while callers can cancel their
	// own Run context.
	// Cancel(ctx context.Context, key ConversationKey, turnID TurnID) error

	// Later phases can enable these operations when a migrated caller needs
	// them. They stay visible here instead of being implemented as misleading
	// no-op methods.
	// Reset(ctx context.Context, key ConversationKey) error
	// Resolve(ctx context.Context, resolution InteractionResolution) error
}

// EventSink receives ordered progress for one turn.
type EventSink interface {
	Emit(ctx context.Context, event TurnEvent) error
}

// EventSinkFunc adapts a function to EventSink.
type EventSinkFunc func(context.Context, TurnEvent) error

func (f EventSinkFunc) Emit(ctx context.Context, event TurnEvent) error {
	return f(ctx, event)
}

// ConversationKey is an opaque, caller-owned conversation identity.
type ConversationKey string

// TurnID is an opaque, caller-generated identity for one Run request.
type TurnID string

// InputPartKind identifies one normalized turn input.
type InputPartKind string

const (
	InputPartText InputPartKind = "text"
	InputPartFile InputPartKind = "file"
)

// InputPart is text or a caller-authorized file.
// Text parts set Text and leave File nil.
// File parts set File and leave Text empty.
type InputPart struct {
	Kind InputPartKind
	Text string
	File *InputFile
}

// InputFile is a caller-authorized file source.
// The Engine treats SourcePath as opaque; the Runtime Adapter decides how to
// mount, copy, or expose it.
type InputFile struct {
	ID         string
	SourcePath string
	Name       string
	MediaType  string
	SizeBytes  int64
	SHA256     string
}

// TurnRequest contains conversation identity and caller-normalized input.
// File sources have already been authorized and resolved outside Engine.
type TurnRequest struct {
	ID              TurnID
	ConversationKey ConversationKey
	Input           []InputPart

	// Deferred until a caller needs them:
	// Continuation ContinuationPolicy
	// Admission    ConversationAdmission
	// Interaction  InteractionPolicy
}

// TurnEventKind identifies progress emitted during a turn.
type TurnEventKind string

const (
	TurnEventTextDelta      TurnEventKind = "text_delta"
	TurnEventToolCallStart  TurnEventKind = "tool_call_start"
	TurnEventToolCallUpdate TurnEventKind = "tool_call_update"
)

// TurnEvent contains either a text delta or a tool activity.
type TurnEvent struct {
	Kind TurnEventKind
	Text string
	Tool *ToolActivity

	// Deferred until Channel migration:
	// Thought     string
	// Interaction *InteractionRequest
	// Output      *OutputItem
}

// ToolActivity contains the fields required by the Session SSE renderer.
type ToolActivity struct {
	ID            string
	Kind          string
	Title         string
	Status        string
	InputSummary  string
	OutputSummary string
	Payload       any
}

// TurnStatus is the terminal state of one turn.
type TurnStatus string

const (
	TurnSucceeded TurnStatus = "succeeded"
	TurnFailed    TurnStatus = "failed"
	TurnCanceled  TurnStatus = "canceled"
)

// ErrorCode is a stable failure category used by callers for transport
// mapping.
type ErrorCode string

const (
	ErrorInvalidRequest            ErrorCode = "invalid_request"
	ErrorAgentUnavailable          ErrorCode = "agent_unavailable"
	ErrorRuntimeAdapterUnavailable ErrorCode = "runtime_adapter_unavailable"
	ErrorConversationBusy          ErrorCode = "conversation_busy"
	ErrorFileUnavailable           ErrorCode = "file_unavailable"
	ErrorInteractionUnsupported    ErrorCode = "interaction_unsupported"
	ErrorRuntimeFailed             ErrorCode = "runtime_failed"
)

// TurnError is the normalized failure carried by TurnResult.
type TurnError struct {
	Code    ErrorCode
	Message string
}

func (e *TurnError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// TurnResult is the terminal outcome of one turn.
type TurnResult struct {
	Status TurnStatus
	Output string
	Error  *TurnError

	// Deferred until strict continuation is implemented:
	// Dispatched bool
}
