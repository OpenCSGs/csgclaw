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
	// OnStarted is called once after prerequisites pass and before compaction work.
	// A failed callback must stop the operation where the native API permits it.
	OnStarted func() error
}
type CompactResult struct {
	Status string                           `json:"status"`
	Reason modelcap.CompactionFailureReason `json:"reason,omitempty"`
	Usage  *modelcap.ContextUsage           `json:"usage,omitempty"`
}

// ConversationCompactor is optional so unsupported runtimes fail explicitly.
type ConversationCompactor interface {
	Compact(context.Context, CompactRequest) (CompactResult, error)
}
