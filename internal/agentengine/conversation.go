package agentengine

import "context"

// ConversationInterface executes conversations for the Agent selected by
// Conversations.
type ConversationInterface interface {
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
	Kind InputPartKind
	Text string
	File *InputFile
}

// InputFile is a caller-authorized file source.
// Engine validates only this neutral shape. Runtime Adapters own file access,
// copying, and Runtime-specific input conversion.
type InputFile struct {
	ID         string
	SourcePath string
	Name       string
	MediaType  string
	SizeBytes  int64
	SHA256     string
}

// TurnRequest contains conversation identity and caller-normalized input.
type TurnRequest struct {
	ID              TurnID
	ConversationKey ConversationKey
	Input           []InputPart
	Continuation    ContinuationPolicy
	Interaction     InteractionPolicy
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

// TurnEvent contains one normalized Runtime event. Sequence starts at one and
// increases monotonically for each Run.
type TurnEvent struct {
	Sequence    uint64
	Kind        TurnEventKind
	Text        string
	Thought     string
	Tool        *ToolActivity
	Activity    *ActivityUpdate
	Interaction *InteractionRequest
	Output      *OutputItem
}

// ToolActivity contains normalized tool progress.
type ToolActivity struct {
	ID            string
	Kind          string
	Title         string
	Status        string
	InputSummary  string
	OutputSummary string
	Payload       any
}

// ActivityUpdate carries Runtime-neutral progress not represented by a tool.
type ActivityUpdate struct {
	ID      string
	Kind    string
	Title   string
	Status  string
	Payload any
}

// InteractionKind identifies a blocking Runtime request.
type InteractionKind string

const (
	InteractionPermission InteractionKind = "permission"
	InteractionUserInput  InteractionKind = "user_input"
)

// InteractionRequest is one pending interaction addressable through Resolve.
type InteractionRequest struct {
	ID      string
	Kind    InteractionKind
	Title   string
	Payload any
}

// InteractionResolution resolves exactly one pending interaction.
type InteractionResolution struct {
	ConversationKey ConversationKey
	InteractionID   string
	OptionID        string
	Answers         map[string]InteractionAnswer
	ResponderID     string
}

// InteractionAnswer is a normalized answer for one Runtime question.
type InteractionAnswer struct {
	Values  []string
	Skipped bool
}

// OutputItemKind identifies a detached, non-blocking output record.
type OutputItemKind string

const (
	OutputItemRequestUserInput OutputItemKind = "request_user_input"
	OutputItemResourceLink     OutputItemKind = "resource_link"
)

// OutputItem contains an already validated Runtime output record.
type OutputItem struct {
	Kind    OutputItemKind
	Payload any
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
	ErrorInteractionNotFound         ErrorCode = "interaction_not_found"
	ErrorInteractionUnsupported      ErrorCode = "interaction_unsupported"
	ErrorCanceled                    ErrorCode = "canceled"
	ErrorRuntimeFailed               ErrorCode = "runtime_failed"
	ErrorUnsupportedRuntimeProvision ErrorCode = "unsupported_runtime_provisioning"
)

// TurnError is a normalized failure returned by Engine operations.
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

// ErrorCodeOf returns the stable Engine code carried by err.
func ErrorCodeOf(err error) ErrorCode {
	for err != nil {
		if turnErr, ok := err.(*TurnError); ok {
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

// TurnResult is the terminal outcome of one turn.
type TurnResult struct {
	Status     TurnStatus
	Output     string
	Dispatched bool
	Error      *TurnError
}
