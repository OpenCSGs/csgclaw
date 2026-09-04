package contract

import "context"

// RuntimeConversation is the Runtime-facing execution boundary. Only the Adapter
// knows native sessions, request brokers and workspace materialization.
type RuntimeConversation interface {
	Run(context.Context, TurnRequest, EventSink) TurnResult
	Reset(context.Context, ConversationKey) *TurnError
	Resolve(context.Context, InteractionRequest, InteractionResolution) *TurnError
}

// ConversationProvider is implemented by registered Runtimes that execute turns.
// Absence means unsupported; Engine never selects another Runtime as fallback.
type ConversationProvider interface {
	Conversation(runtimeID string) RuntimeConversation
}
