package agentengine

import (
	"context"
	"encoding/json"
	"time"
)

// ConversationInterface executes conversations for the Agent selected by
// Interface.Conversations. Conversation keys are caller-owned identities, not
// persisted Engine resources, so this interface intentionally uses lifecycle
// verbs rather than misleading CRUD operations.
type ConversationInterface interface {
	// Run admits and dispatches one turn and emits ordered progress.
	// TurnResult is the only outcome before and after Runtime dispatch.
	Run(ctx context.Context, request TurnRequest, sink EventSink) TurnResult

	// Cancel idempotently cancels one queued or running execution.
	Cancel(ctx context.Context, key ConversationKey) error

	// Reset serializes with turns for the same ConversationKey.
	Reset(ctx context.Context, key ConversationKey) error

	// Resolve resolves one active interaction exactly once.
	Resolve(ctx context.Context, resolution InteractionResolution) error
}

// ConversationRuntime adapts one selected Agent Runtime to direct,
// Channel-neutral turns. The adapter owns native conversation mapping and
// persistence.
type ConversationRuntime interface {
	// Run executes one already-admitted turn and reports one terminal result.
	Run(ctx context.Context, request RuntimeTurnRequest, sink EventSink) TurnResult

	// Cancel requests cancellation of one dispatched Runtime turn.
	Cancel(ctx context.Context, key ConversationKey) error

	// Reset atomically replaces the native conversation mapping.
	Reset(ctx context.Context, key ConversationKey) error

	// Resolve routes an answer to one pending Runtime interaction.
	Resolve(ctx context.Context, resolution InteractionResolution) error
}

// EventSink receives ordered, bounded, non-terminal events for one turn.
// A sink is neither an event bus nor a transcript store.
type EventSink interface {
	Emit(ctx context.Context, event TurnEvent) error
}

// ConversationKey is an opaque, caller-owned conversation identity.
type ConversationKey string

// InteractionID identifies one interaction awaited by a running turn.
type InteractionID string

// ContinuationPolicy selects mapping creation or strict continuation.
type ContinuationPolicy string

const (
	// ContinuationCreateOrResume creates a missing native mapping or resumes it.
	ContinuationCreateOrResume ContinuationPolicy = "create_or_resume"
	// ContinuationRequireExisting rejects a missing native mapping.
	ContinuationRequireExisting ContinuationPolicy = "require_existing"
)

// ConversationAdmission selects waiting or fail-fast admission for a busy key.
type ConversationAdmission string

const (
	// ConversationAdmissionWait queues behind the active turn.
	ConversationAdmissionWait ConversationAdmission = "wait"
	// ConversationAdmissionRejectIfBusy returns ErrorConversationBusy immediately.
	ConversationAdmissionRejectIfBusy ConversationAdmission = "reject_if_busy"
)

// InteractionPolicy declares how the caller handles blocking interactions.
type InteractionPolicy string

const (
	// InteractionResolve lets the caller resolve a pending interaction.
	InteractionResolve InteractionPolicy = "resolve"
	// InteractionReject terminates when the Runtime requests interaction.
	InteractionReject InteractionPolicy = "reject"
	// InteractionSkipUserInput skips user input and safely denies permissions.
	InteractionSkipUserInput InteractionPolicy = "skip_user_input"
)

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
	ConversationKey ConversationKey
	Continuation    ContinuationPolicy
	Admission       ConversationAdmission
	Interaction     InteractionPolicy
	Input           []InputPart
}

// RuntimeTurnRequest contains only state needed by an already-selected Runtime
// after admission.
type RuntimeTurnRequest struct {
	ConversationKey ConversationKey
	Continuation    ContinuationPolicy
	Interaction     InteractionPolicy
	Input           []InputPart
}

// InteractionResolution carries one answer for an active interaction.
// Secret answer values must not be logged or included in a transcript.
type InteractionResolution struct {
	ConversationKey ConversationKey
	InteractionID   InteractionID
	Answer          InteractionAnswer
}

// InteractionAnswer represents either a permission choice or answers to a
// native user-input request.
type InteractionAnswer struct {
	OptionID string
	Answers  map[string][]string
}

// TurnEventKind identifies one ordered progress event.
type TurnEventKind string

const (
	TurnEventTextDelta           TurnEventKind = "text_delta"
	TurnEventThoughtDelta        TurnEventKind = "thought_delta"
	TurnEventActivity            TurnEventKind = "activity"
	TurnEventInteractionRequired TurnEventKind = "interaction_required"
	TurnEventOutput              TurnEventKind = "output"
)

// TurnEvent is one ordered piece of live turn progress.
// Exactly one payload matching Kind is populated.
type TurnEvent struct {
	Sequence    uint64
	OccurredAt  time.Time
	Kind        TurnEventKind
	Text        string
	Activity    *ActivityEvent
	Interaction *InteractionRequest
	Output      *OutputItem
}

// ActivityKind identifies the work represented by a live activity.
type ActivityKind string

const (
	ActivityToolCall ActivityKind = "tool_call"
	ActivityPlan     ActivityKind = "plan"
)

// ActivityStatus is the observed state of a live activity.
type ActivityStatus string

const (
	ActivityPending   ActivityStatus = "pending"
	ActivityRunning   ActivityStatus = "running"
	ActivityCompleted ActivityStatus = "completed"
	ActivityFailed    ActivityStatus = "failed"
	ActivityCanceled  ActivityStatus = "canceled"
)

// ActivityEvent describes one Runtime-neutral live activity. Repeated events
// with the same ID update the same activity in sequence order.
type ActivityEvent struct {
	ID            string
	Kind          ActivityKind
	ToolName      string
	Title         string
	Status        ActivityStatus
	InputSummary  string
	OutputSummary string
	StartedAt     time.Time
	CompletedAt   *time.Time
}

// InteractionKind identifies one blocking Runtime request.
type InteractionKind string

const (
	InteractionPermission InteractionKind = "permission"
	InteractionUserInput  InteractionKind = "user_input"
)

// InteractionRequest describes a blocking permission or user-input request.
type InteractionRequest struct {
	ID        InteractionID
	Kind      InteractionKind
	Title     string
	Options   []InteractionOption
	Questions []InputQuestion
	ExpiresAt *time.Time
}

// InteractionOption is one permission or action choice.
type InteractionOption struct {
	ID    string
	Kind  string
	Label string
	Scope string
}

// InputQuestion is one typed question shown to a user.
type InputQuestion struct {
	ID         string
	Header     string
	Prompt     string
	Options    []InputOption
	AllowOther bool
	Secret     bool
}

// InputOption is one selectable answer to an InputQuestion.
type InputOption struct {
	Label       string
	Description string
}

// OutputKind identifies final text or decoded CSGClaw structured output.
type OutputKind string

const (
	OutputText          OutputKind = "text"
	OutputResourceLink  OutputKind = "resource_link"
	OutputDeferredInput OutputKind = "request_user_input"
)

// OutputItem is text or one validated record decoded from
// ::csgclaw-output::. Raw control lines never cross this boundary.
type OutputItem struct {
	Kind         OutputKind
	Text         string
	ResourceLink *ResourceLink
	InputRequest *DeferredInputRequest
}

// ResourceLink is a validated link decoded from CSGClaw structured output.
type ResourceLink struct {
	Name        string
	Title       string
	URI         string
	Description string
	MediaType   string
	SizeBytes   *uint64
	Annotations json.RawMessage
	Metadata    json.RawMessage
	Icons       []ResourceIcon
}

// ResourceIcon is one optional icon associated with a ResourceLink.
type ResourceIcon struct {
	Source    string
	MediaType string
	Sizes     []string
	Theme     string
}

// DeferredInputRequest completes the current turn and asks the Channel to
// create a follow-up turn after the user answers.
type DeferredInputRequest struct {
	Questions        []InputQuestion
	AutoResolveAfter time.Duration
}

// TurnStatus is the terminal status of one turn.
type TurnStatus string

const (
	TurnSucceeded TurnStatus = "succeeded"
	TurnFailed    TurnStatus = "failed"
	TurnCanceled  TurnStatus = "canceled"
)

// ErrorCode is a stable Engine failure category.
type ErrorCode string

const (
	ErrorInvalidRequest            ErrorCode = "invalid_request"
	ErrorAgentUnavailable          ErrorCode = "agent_unavailable"
	ErrorRuntimeAdapterUnavailable ErrorCode = "runtime_adapter_unavailable"
	ErrorConversationBusy          ErrorCode = "conversation_busy"
	ErrorAdmissionExhausted        ErrorCode = "admission_exhausted"
	ErrorConversationNotResumable  ErrorCode = "conversation_not_resumable"
	ErrorFileUnavailable           ErrorCode = "file_unavailable"
	ErrorInteractionUnsupported    ErrorCode = "interaction_unsupported"
	ErrorRuntimeFailed             ErrorCode = "runtime_failed"
)

// TurnError is the normalized failure carried by TurnResult.
type TurnError struct {
	Code       ErrorCode
	Message    string
	RetryAfter time.Duration
}

// TurnResult is the sole outcome before and after Runtime dispatch.
type TurnResult struct {
	Dispatched bool
	Status     TurnStatus
	Output     []OutputItem
	Error      *TurnError
}
