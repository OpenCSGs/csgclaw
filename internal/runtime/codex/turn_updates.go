package codex

import (
	"context"
	"errors"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/diagnostics"
)

const (
	turnUpdateBatchDelay = 500 * time.Millisecond
	turnUpdateTimeout    = 2 * time.Second
)

type appServerTurnUpdates struct {
	inputs         contract.TurnInputUpdates
	timer          *time.Timer
	due            <-chan time.Time
	through        uint64
	waitCompaction bool
}

func (u *appServerTurnUpdates) schedule() {
	if u.inputs.Ready == nil || u.due != nil {
		return
	}
	if u.timer == nil {
		u.timer = time.NewTimer(turnUpdateBatchDelay)
	} else {
		u.timer.Reset(turnUpdateBatchDelay)
	}
	u.due = u.timer.C
}

func (u *appServerTurnUpdates) stop() {
	if u.timer != nil {
		u.timer.Stop()
	}
}

func (u *appServerTurnUpdates) flush(ctx context.Context, live *liveSession, waiter *appServerTurnWaiter) {
	u.due = nil
	turnID := waiter.currentTurnID()
	_, completed := waiter.terminalResult()
	if completed || ctx.Err() != nil || turnID == "" || live.appClient == nil || waiter.hasStructuredOutputBoundary() {
		return
	}
	live.mu.Lock()
	compacting := live.contextUsage[waiter.threadID].Compacting
	live.mu.Unlock()
	u.waitCompaction = compacting
	if compacting {
		return
	}
	through, text := u.inputs.Latest(u.through)
	if text == "" || through <= u.through {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, turnUpdateTimeout)
	defer cancel()
	_, err := live.appClient.probe(requestCtx, "turn/steer", map[string]any{
		"threadId":       waiter.threadID,
		"expectedTurnId": turnID,
		"input":          []map[string]any{{"type": "text", "text": text}},
	})
	diagnostics.From(ctx).Capture("runtime.turn_update", map[string]any{"through": through, "accepted": err == nil})
	if err == nil {
		u.through = through
		return
	}
	var rpcErr appServerRPCError
	if errors.As(err, &rpcErr) && rpcErr.Code == appServerRPCMethodNotFound {
		u.inputs.Ready = nil
	}
}
