package agentengine

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelcap"
	"strings"
)

func (c *conversations) Compact(ctx context.Context, req contract.CompactRequest) (contract.CompactResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil || c.engine == nil || c.engine.runtimes == nil || c.agentID == "" {
		return contract.CompactResult{}, &TurnError{Code: ErrorInvalidRequest, Message: "agent conversation is required"}
	}
	req.ConversationKey = ConversationKey(strings.TrimSpace(string(req.ConversationKey)))
	if req.ConversationKey == "" || req.Usage.SessionID == "" {
		return contract.CompactResult{}, &TurnError{Code: ErrorInvalidRequest, Message: "existing conversation is required"}
	}
	if !modelcap.ManualCompactionEligible(&req.Usage) {
		return contract.CompactResult{Status: "unnecessary"}, nil
	}
	identity := conversationIdentity{agentID: c.agentID, key: req.ConversationKey}
	// Check active turns and acquire control under the same lock. Unlike Reset,
	// compaction never cancels or waits behind an existing turn.
	c.engine.mu.Lock()
	c.engine.ensureState()
	for other := range c.engine.active {
		if other.agentID == c.agentID {
			c.engine.mu.Unlock()
			return contract.CompactResult{Status: "busy"}, nil
		}
	}
	if c.engine.controls[identity] != nil || c.engine.active[identity] != nil {
		c.engine.mu.Unlock()
		return contract.CompactResult{Status: "busy"}, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	control := &conversationControl{done: make(chan struct{}), cancel: cancel}
	c.engine.controls[identity] = control
	c.engine.mu.Unlock()
	defer c.engine.endControl(identity, control)
	adapter, release, err := c.engine.runtimes.conversationRuntime(ctx, c.agentID)
	if err != nil {
		return contract.CompactResult{}, err
	}
	if release != nil {
		defer release()
	}
	compactor, ok := adapter.(contract.ConversationCompactor)
	if !ok {
		return contract.CompactResult{Status: "unsupported"}, nil
	}
	return compactor.Compact(ctx, req)
}
