package runtime

import "context"

// MemoryDocument is the runtime-neutral readable memory surface exposed to
// CSGClaw. Content is generated runtime state and is therefore read-only.
type MemoryDocument struct {
	Enabled  bool   `json:"enabled"`
	Ready    bool   `json:"ready"`
	Name     string `json:"name"`
	Location string `json:"location,omitempty"`
	Content  string `json:"content"`
}

// MemoryController lets a runtime own both its memory configuration format
// and the location or representation of its readable memory document.
type MemoryController interface {
	ReadMemoryDocument(ctx context.Context, agentHome string, options map[string]any) (MemoryDocument, error)
	ConfigureMemory(options map[string]any, enabled bool) (map[string]any, error)
}
