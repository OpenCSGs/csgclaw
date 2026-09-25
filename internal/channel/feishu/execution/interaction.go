package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	channel "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/interaction"
	"csgclaw/internal/channel/feishu/presentation"
)

type pendingInteraction struct {
	message  channel.InboundMessage
	request  agentengine.InteractionRequest
	created  time.Time
	finished bool
}

func interactionCreateID(turnID, id string) string { return turnID + ":interaction:" + id + ":create" }
func (r *Runner) registerInteraction(ctx context.Context, message channel.InboundMessage, request agentengine.InteractionRequest) error {
	card, err := presentation.InteractionCard(request)
	if err != nil {
		return err
	}
	r.interactionMu.Lock()
	defer r.interactionMu.Unlock()
	key := interactionCreateID(message.TurnID, request.ID)
	if r.interactions[key] != nil {
		return nil
	}
	for id, item := range r.interactions {
		if item.finished && time.Since(item.created) > time.Hour {
			delete(r.interactions, id)
		}
	}
	item := &pendingInteraction{message: message, request: request, created: time.Now()}
	intent := baseIntent(message, key, 0)
	intent.Kind = channel.DeliveryCard
	intent.Card = card
	intent.InteractionID = request.ID
	if err = r.state.Enqueue(intent); err != nil {
		return err
	}
	r.interactions[key] = item
	r.pinInteraction(key, true)
	if !request.Detached {
		wait := baseIntent(message, key+":waiting", 0)
		wait.Kind = channel.DeliveryCOTUpdate
		wait.RelatedID = message.TurnID + ":cot:create"
		wait.Events = []channel.COTEvent{presentation.Waiting(request.ID, false)}
		if err = r.state.Enqueue(wait); err != nil {
			return err
		}
	}
	r.notify()
	return nil
}

func (r *Runner) observeInteraction(ctx context.Context, message channel.InboundMessage, event agentengine.TurnEvent) error {
	if event.Interaction != nil {
		return r.registerInteraction(ctx, message, *event.Interaction)
	}
	if event.Activity != nil {
		r.refreshInteractions(message)
	}
	return nil
}

func (r *Runner) refreshInteractions(message channel.InboundMessage) {
	r.interactionMu.Lock()
	var keys []string
	for key, item := range r.interactions {
		if item.message.TurnID == message.TurnID && !item.finished {
			keys = append(keys, key)
		}
	}
	r.interactionMu.Unlock()
	for _, key := range keys {
		if err := r.refreshInteraction(key); err != nil {
			r.logFinalizeError(message, err)
		}
	}
}

func (r *Runner) refreshInteraction(key string) error {
	r.interactionMu.Lock()
	item := r.interactions[key]
	if item == nil || item.finished {
		r.interactionMu.Unlock()
		return nil
	}
	message, request := item.message, item.request
	r.interactionMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	current, err := r.engine.Conversations(message.AgentID).GetInteraction(ctx, agentengine.ConversationKey(message.ConversationKey), request.ID)
	if err != nil && agentengine.ErrorCodeOf(err) != agentengine.ErrorInteractionNotFound {
		return err
	}
	missing := err != nil
	terminal := missing
	switch snapshot := current.Payload.(type) {
	case activity.ActivitySnapshot:
		terminal = snapshot.Status != activity.ActionStatusPending
	case activity.UserInputSnapshot:
		terminal = snapshot.Status != activity.UserInputStatusPending
	}
	if !terminal {
		return nil
	}
	card, err := presentation.InteractionCard(current)
	if missing {
		card = presentation.Card("此确认已经过期。")
		err = nil
	}
	if err != nil {
		return err
	}
	r.interactionMu.Lock()
	defer r.interactionMu.Unlock()
	if item.finished {
		return nil
	}
	intent := baseIntent(message, strings.TrimSuffix(key, ":create")+":final", 1)
	intent.Kind = channel.DeliveryCardUpdate
	intent.RelatedID = key
	intent.Card = card
	if err = r.state.Enqueue(intent); err != nil {
		return err
	}
	item.finished = true
	r.pinInteraction(key, false)
	if !request.Detached {
		wait := baseIntent(message, key+":answered", 0)
		wait.Kind = channel.DeliveryCOTUpdate
		wait.RelatedID = message.TurnID + ":cot:create"
		wait.Events = []channel.COTEvent{presentation.Waiting(request.ID, true)}
		// A terminal COT is immutable; cancellation closes the waiting step in the
		// completion batch instead of appending after the complete request.
		if !r.deliveryExists(message.TurnID + ":cot:complete") {
			if err = r.state.Enqueue(wait); err != nil {
				return err
			}
		}
	}
	r.notify()
	return nil
}

// ResolveInteraction receives routing identities reconstructed from a locally
// delivered card, never from the submitted action value.
func (r *Runner) ResolveInteraction(ctx context.Context, input interaction.Input) error {
	release, err := r.acquireControl(ctx, input.ConversationKey)
	if err != nil {
		return err
	}
	defer release()
	key := interactionCreateID(input.TurnID, input.InteractionID)
	r.interactionMu.Lock()
	item := r.interactions[key]
	if item == nil || item.message.AgentID != input.AgentID || item.message.ConversationKey != input.ConversationKey || item.message.Source.SenderID == "" || item.message.Source.SenderID != input.ResponderID {
		r.interactionMu.Unlock()
		return fmt.Errorf("只有请求发起者可以处理此确认")
	}
	message, request := item.message, item.request
	currentTurn := r.latest[message.ConversationKey]
	workerCtx := r.workerContexts[message.ConversationKey]
	r.interactionMu.Unlock()
	if currentTurn != message.TurnID {
		return fmt.Errorf("此确认已经过期")
	}
	resolution := agentengine.InteractionResolution{ConversationKey: agentengine.ConversationKey(message.ConversationKey), InteractionID: request.ID, ResponderID: input.ResponderID, OptionID: input.OptionID}
	if request.Kind == agentengine.InteractionUserInput {
		answers, err := presentation.InteractionAnswers(request, input.FormValue)
		if err != nil {
			return err
		}
		resolution.Answers = answers
	}
	if err := r.engine.Conversations(message.AgentID).Resolve(ctx, resolution); err != nil {
		return err
	}
	if err := r.refreshInteraction(key); err != nil {
		return err
	}
	if request.Detached {
		// Engine owns answer validation and duplicate claims. The Channel owns
		// starting the follow-up Turn after a detached answer has been accepted.
		public := resolution.Answers
		if snapshot, ok := request.Payload.(activity.UserInputSnapshot); ok {
			for _, q := range snapshot.Questions {
				if q.IsSecret {
					public[q.ID] = agentengine.InteractionAnswer{Values: []string{"user_note: <redacted>"}}
				}
			}
		}
		raw, err := json.Marshal(public)
		if err != nil {
			return err
		}
		next := message
		next.TurnID = message.TurnID + ":answer:" + request.ID
		next.Source.EventID = next.TurnID
		next.Files = nil
		next.QuotedMessage = nil
		next.Text = "Continue using the user's answers to the previous questions:\n" + string(raw)
		if workerCtx == nil || workerCtx.Err() != nil {
			return fmt.Errorf("执行连接已关闭")
		}
		r.interactionMu.Lock()
		valid := r.latest[message.ConversationKey] == message.TurnID
		r.interactionMu.Unlock()
		if !valid {
			return fmt.Errorf("会话已经收到新的请求")
		}
		return r.submit(workerCtx, next)
	}
	return nil
}

// Monitor refreshes expiry and failed delivery independently of Runtime events.
// Its lifetime is the binding lifetime, including detached questions.
func (r *Runner) Monitor(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.interactionMu.Lock()
			items := make(map[string]pendingInteraction)
			for key, item := range r.interactions {
				if !item.finished {
					items[key] = *item
				}
			}
			r.interactionMu.Unlock()
			for key, item := range items {
				if ctx.Err() != nil {
					return
				}
				if lookup, ok := r.state.(interface {
					Delivery(string) (channel.DeliveryIntent, bool)
				}); ok {
					delivery, found := lookup.Delivery(key)
					if found && delivery.Status == channel.DeliveryFailed {
						notice := r.messageCardIntent(item.message, key+":delivery-error", 0, "确认卡片发送失败，请重新发送请求。")
						if err := r.state.Enqueue(notice); err != nil {
							r.logFinalizeError(item.message, err)
						}
						r.notify()
						if !item.request.Detached {
							cancelCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
							err := r.Cancel(cancelCtx, item.message.AgentID, item.message.ConversationKey, item.message.TurnID)
							cancel()
							if err != nil {
								r.logFinalizeError(item.message, err)
							}
						}
					}
				}
				if err := r.refreshInteraction(key); err != nil {
					r.logFinalizeError(item.message, err)
				}
			}
		}
	}
}

func (r *Runner) pinInteraction(key string, pin bool) {
	if store, ok := r.state.(interface{ Pin(string, bool) }); ok {
		store.Pin(key, pin)
	}
}
