package agentengine

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelcap"
	"testing"
)

type compactTestRuntime struct {
	RuntimeConversation
	started, finish chan struct{}
}

func (r *compactTestRuntime) Compact(ctx context.Context, request contract.CompactRequest) (contract.CompactResult, error) {
	close(r.started)
	select {
	case <-r.finish:
		return contract.CompactResult{Status: "completed"}, nil
	case <-ctx.Done():
		return contract.CompactResult{}, ctx.Err()
	}
}
func TestManualCompactionAdmissionAndCancellation(t *testing.T) {
	runtime := &compactTestRuntime{started: make(chan struct{}), finish: make(chan struct{})}
	engine := &Engine{runtimes: directConversationResolver{runtime: runtime}}
	used := int64(50000)
	request := contract.CompactRequest{ConversationKey: "r", Usage: modelcap.ContextUsage{SessionID: "s", UsedTokens: &used, ContextWindow: 100000}}
	compactor := engine.Conversations("a").(contract.ConversationCompactor)
	done := make(chan error, 1)
	go func() { _, err := compactor.Compact(context.Background(), request); done <- err }()
	<-runtime.started
	result, err := compactor.Compact(context.Background(), request)
	if err != nil || result.Status != "busy" {
		t.Fatalf("duplicate admitted: %+v %v", result, err)
	}
	engine.interruptAgent("agent-a")
	if err := <-done; err == nil {
		t.Fatal("stop did not cancel compaction")
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if len(engine.controls) != 0 {
		t.Fatal("compaction leaked its admission lock")
	}
}
