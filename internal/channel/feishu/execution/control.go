package execution

import (
	"context"
	"fmt"
	"log/slog"

	channel "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/interaction"
	feishustate "csgclaw/internal/channel/feishu/state"
)

type conversationControl struct {
	gate chan struct{}
	refs int
}

// acquireControl serializes admission, reset and answers within a conversation.
// References include waiting callers so an entry survives until every caller
// releases it. Engine calls never hold the map mutex.
func (r *Runner) acquireControl(ctx context.Context, key string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	r.controlMu.Lock()
	control := r.controls[key]
	if control == nil {
		control = &conversationControl{gate: make(chan struct{}, 1)}
		r.controls[key] = control
	}
	control.refs++
	r.controlMu.Unlock()

	releaseRef := func() {
		r.controlMu.Lock()
		control.refs--
		if control.refs == 0 {
			delete(r.controls, key)
		}
		r.controlMu.Unlock()
	}
	select {
	case control.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-control.gate
			releaseRef()
			return nil, err
		}
		return func() {
			<-control.gate
			releaseRef()
		}, nil
	case <-ctx.Done():
		releaseRef()
		return nil, ctx.Err()
	}
}

// CancelRequest keeps task cancellation independent from process presentation.
func (r *Runner) CancelRequest(ctx context.Context, request interaction.CancelRequest) (err error) {
	attrs := []any{"binding_id", request.BindingID, "agent_id", request.AgentID, "turn_id", request.TurnID, "conversation_key", request.ConversationKey, "message_id", request.MessageID}
	slog.Info("request Feishu cancellation", attrs...)
	defer func() {
		if err != nil {
			slog.Warn("Feishu cancellation rejected or failed", append(attrs, "error", err)...)
		} else {
			record, _ := r.state.Get(request.TurnID)
			slog.Info("Feishu cancellation handled", append(attrs, "status", record.Status)...)
		}
	}()
	release, err := r.acquireControl(ctx, request.ConversationKey)
	if err != nil {
		return err
	}
	defer release()
	target, found := r.state.ResolveControlTarget(feishustate.ControlQuery{BindingID: request.BindingID, AgentID: request.AgentID, MessageID: request.MessageID, ChatID: request.ChatID, ThreadID: request.ThreadID, RequesterID: request.RequesterID})
	if !found || request.RequesterID == "" || target.Intent.RequesterID == "" || target.Turn.TurnID != request.TurnID || target.Turn.ConversationKey != request.ConversationKey {
		return fmt.Errorf("无法确认此操作对应的任务或操作权限")
	}
	message := channel.InboundMessage{AgentID: request.AgentID, TurnID: request.TurnID, ConversationKey: request.ConversationKey, Source: channel.Source{BindingID: request.BindingID, MessageID: request.MessageID, ChatID: request.ChatID}}
	return r.cancelTarget(ctx, message, target.Turn)
}

// cancelTarget is called while the conversation control is held.
func (r *Runner) cancelTarget(ctx context.Context, message channel.InboundMessage, record channel.TurnRecord) error {
	switch record.Status {
	case channel.TurnSucceeded, channel.TurnFailed, channel.TurnCanceled:
	default:
		if r.ActiveTurn(message.ConversationKey) != message.TurnID {
			return fmt.Errorf("此任务已不再是当前执行的任务")
		}
		r.state.MarkCanceling(message.TurnID)
		r.notify()
		if err := r.Cancel(ctx, message.AgentID, message.ConversationKey, message.TurnID); err != nil {
			return err
		}
	}
	// Completion delivery owns retries and its failure must not report that task
	// cancellation failed. A successful completion is left unchanged.
	if err := r.state.RetryCOTCompletion(message.TurnID + ":cot:create"); err != nil {
		r.logFinalizeError(message, err)
	}
	r.notify()
	return nil
}

// Stop cancels the task captured when the control message was admitted.
func (r *Runner) Stop(ctx context.Context, message channel.InboundMessage) error {
	release, err := r.acquireControl(ctx, message.ConversationKey)
	if err != nil {
		return err
	}
	defer release()
	if message.TurnID == "" || r.ActiveTurn(message.ConversationKey) != message.TurnID {
		slog.Info("ignore Feishu stop without matching active task", messageLogAttrs(message)...)
		return nil
	}
	record, found := r.state.Get(message.TurnID)
	create, hasCreate := r.state.Delivery(message.TurnID + ":cot:create")
	if !found || !hasCreate || record.AgentID != message.AgentID || record.BindingID != message.Source.BindingID || record.ConversationKey != message.ConversationKey || create.ChatID != message.Source.ChatID || create.ThreadID != message.Source.ThreadID || message.Source.SenderID == "" || create.RequesterID != message.Source.SenderID {
		return fmt.Errorf("无法确认停止请求对应的任务或操作权限")
	}
	slog.Info("apply Feishu stop command", messageLogAttrs(message)...)
	return r.cancelTarget(ctx, message, record)
}
