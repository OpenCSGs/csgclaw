package agentengine

import "context"

// ConversationInterface executes conversations for the Agent selected by
// Conversations.
type ConversationInterface interface {
	Files() FileInterface
	Run(ctx context.Context, request TurnRequest, sink EventSink) TurnResult
	Cancel(ctx context.Context, key ConversationKey, turnID TurnID) error
	Reset(ctx context.Context, key ConversationKey) error
	Resolve(ctx context.Context, resolution InteractionResolution) error
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

// AdmissionPolicy controls how Run behaves when the Conversation already has
// an active Turn or atomic control operation.
type AdmissionPolicy string

const (
	AdmissionRejectIfBusy AdmissionPolicy = "reject_if_busy"
	AdmissionWait         AdmissionPolicy = "wait"
	AdmissionSupersede    AdmissionPolicy = "supersede"
)

// ContinuationPolicy controls whether a missing Runtime-native conversation may
// be created.
type ContinuationPolicy string

const (
	ContinuationCreateOrResume  ContinuationPolicy = "create_or_resume"
	ContinuationRequireExisting ContinuationPolicy = "require_existing"
)

// InteractionPolicy controls how blocking Runtime interactions are handled.
type InteractionPolicy string

const (
	InteractionResolve       InteractionPolicy = "resolve"
	InteractionReject        InteractionPolicy = "reject"
	InteractionSkipUserInput InteractionPolicy = "skip_user_input"
)

// InputPartKind identifies one normalized turn input.
type InputPartKind string

const (
	InputPartText InputPartKind = "text"
	InputPartFile InputPartKind = "file"
)

// InputPart is text or a caller-authorized file.
type InputPart struct {
	Kind InputPartKind `json:"kind"`
	Text string        `json:"text,omitempty"`
	File *InputFile    `json:"file,omitempty"`
}

// InputFile references one immutable Agent-scoped Conversation file resource.
// Runtime Adapters receive the resolved snapshot only after Engine admission.
type InputFile struct {
	ID string `json:"id"`

	file *OutputFile
}

// TurnRequest contains conversation identity and caller-normalized input.
type TurnRequest struct {
	ID              TurnID             `json:"id"`
	ConversationKey ConversationKey    `json:"conversation_key"`
	Input           []InputPart        `json:"input"`
	Admission       AdmissionPolicy    `json:"admission,omitempty"`
	Continuation    ContinuationPolicy `json:"continuation,omitempty"`
	Interaction     InteractionPolicy  `json:"interaction,omitempty"`
}

// TurnEventKind identifies progress emitted during a turn.
type TurnEventKind string

const (
	TurnEventTextDelta          TurnEventKind = "text_delta"
	TurnEventThoughtDelta       TurnEventKind = "thought_delta"
	TurnEventToolCallStart      TurnEventKind = "tool_call_start"
	TurnEventToolCallUpdate     TurnEventKind = "tool_call_update"
	TurnEventActivityUpdate     TurnEventKind = "activity_update"
	TurnEventInteractionRequest TurnEventKind = "interaction_request"
	TurnEventOutputItem         TurnEventKind = "output_item"
)

// TurnEvent is the replay-safe envelope for one normalized Runtime event.
// Sequence starts at one and increases monotonically for each TurnID.
type TurnEvent struct {
	TurnID      TurnID              `json:"turn_id"`
	Sequence    uint64              `json:"sequence"`
	Kind        TurnEventKind       `json:"kind"`
	Text        string              `json:"text,omitempty"`
	Thought     string              `json:"thought,omitempty"`
	Tool        *ToolActivity       `json:"tool,omitempty"`
	Activity    *ActivityUpdate     `json:"activity,omitempty"`
	Interaction *InteractionRequest `json:"interaction,omitempty"`
	Output      *OutputItem         `json:"output,omitempty"`
}

// ToolActivity contains normalized tool progress. Payload must be JSON-compatible.
type ToolActivity struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Title         string `json:"title,omitempty"`
	Status        string `json:"status,omitempty"`
	InputSummary  string `json:"input_summary,omitempty"`
	OutputSummary string `json:"output_summary,omitempty"`
	Payload       any    `json:"payload,omitempty"`
}

// ActivityUpdate carries Runtime-neutral progress not represented by a tool.
// Payload must be JSON-compatible.
type ActivityUpdate struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Title   string `json:"title,omitempty"`
	Status  string `json:"status,omitempty"`
	Payload any    `json:"payload,omitempty"`
}

// InteractionKind identifies a blocking Runtime request.
type InteractionKind string

const (
	InteractionPermission InteractionKind = "permission"
	InteractionUserInput  InteractionKind = "user_input"
)

// InteractionRequest is one pending interaction addressable through Resolve.
// Payload must be JSON-compatible.
type InteractionRequest struct {
	ID      string          `json:"id"`
	Kind    InteractionKind `json:"kind"`
	Title   string          `json:"title,omitempty"`
	Payload any             `json:"payload,omitempty"`
}

// InteractionResolution resolves exactly one pending interaction.
type InteractionResolution struct {
	ConversationKey ConversationKey              `json:"conversation_key"`
	InteractionID   string                       `json:"interaction_id"`
	OptionID        string                       `json:"option_id,omitempty"`
	Answers         map[string]InteractionAnswer `json:"answers,omitempty"`
	ResponderID     string                       `json:"responder_id,omitempty"`
}

// InteractionAnswer is a normalized answer for one Runtime question.
type InteractionAnswer struct {
	Values  []string `json:"values,omitempty"`
	Skipped bool     `json:"skipped,omitempty"`
}

// OutputItemKind identifies a detached, non-blocking output record.
type OutputItemKind string

const (
	OutputItemRequestUserInput OutputItemKind = "request_user_input"
	OutputItemResourceLink     OutputItemKind = "resource_link"
)

// OutputItem contains an already validated Runtime output record. Payload must
// be JSON-compatible and is decoded according to Kind by an HTTP client.
type OutputItem struct {
	Kind    OutputItemKind `json:"kind"`
	Payload any            `json:"payload"`
}

// TurnStatus is the terminal state of one turn.
type TurnStatus string

const (
	TurnSucceeded TurnStatus = "succeeded"
	TurnFailed    TurnStatus = "failed"
	TurnCanceled  TurnStatus = "canceled"
)

// ErrorCode is a stable failure category used by callers for transport mapping.
type ErrorCode string

const (
	ErrorInvalidRequest              ErrorCode = "invalid_request"
	ErrorAgentUnavailable            ErrorCode = "agent_unavailable"
	ErrorRuntimeAdapterUnavailable   ErrorCode = "runtime_adapter_unavailable"
	ErrorConversationBusy            ErrorCode = "conversation_busy"
	ErrorConversationNotResumable    ErrorCode = "conversation_not_resumable"
	ErrorFileUnavailable             ErrorCode = "file_unavailable"
	ErrorFileNotFound                ErrorCode = "file_not_found"
	ErrorInteractionNotFound         ErrorCode = "interaction_not_found"
	ErrorInteractionUnsupported      ErrorCode = "interaction_unsupported"
	ErrorCanceled                    ErrorCode = "canceled"
	ErrorRuntimeFailed               ErrorCode = "runtime_failed"
	ErrorUnsupportedRuntimeProvision ErrorCode = "unsupported_runtime_provisioning"
)

// TurnError is a normalized failure returned by Engine operations.
type TurnError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *TurnError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// ErrorCodeOf returns the stable Engine code carried by err.
func ErrorCodeOf(err error) ErrorCode {
	for err != nil {
		if turnErr, ok := err.(*TurnError); ok {
			if turnErr == nil {
				return ""
			}
			return turnErr.Code
		}
		type unwrapper interface{ Unwrap() error }
		next, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = next.Unwrap()
	}
	return ""
}

// TurnResult is the JSON-safe terminal outcome of one turn. Files contains
// metadata only; callers retrieve metadata and content through
// Conversations(agentID).Files().Get(fileID).
type TurnResult struct {
	Status     TurnStatus   `json:"status"`
	Output     string       `json:"output,omitempty"`
	Files      []OutputFile `json:"files,omitempty"`
	Dispatched bool         `json:"dispatched"`
	Error      *TurnError   `json:"error,omitempty"`

	files []*OutputFile
}
