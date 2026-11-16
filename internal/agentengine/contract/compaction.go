package contract

import (
	"context"
	"csgclaw/internal/modelcap"
)

// CompactRequest uses a trusted saved snapshot to select an existing native
// binding. Adapters must verify that binding belongs to ConversationKey.
type CompactRequest struct {
	RoomConfiguration *RoomConfiguration
	ConversationKey   ConversationKey
	Usage             modelcap.ContextUsage
}
type CompactResult struct {
	Status string                 `json:"status"`
	Usage  *modelcap.ContextUsage `json:"usage,omitempty"`
}

// ConversationCompactor is optional so unsupported runtimes fail explicitly.
type ConversationCompactor interface {
	Compact(context.Context, CompactRequest) (CompactResult, error)
}
