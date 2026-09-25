package execution

import (
	"fmt"
	"reflect"
	"strings"

	"csgclaw/internal/agentengine"
	channel "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/presentation"
)

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func (r *Runner) replyIntents(message channel.InboundMessage, sequence uint64, final bool, rendered presentation.Rendered) []channel.DeliveryIntent {
	var intents []channel.DeliveryIntent
	for i, card := range rendered.Cards {
		createID := fmt.Sprintf("%s:reply:%06d:create", message.TurnID, i)
		intent := baseIntent(message, createID, sequence)
		intent.Kind = channel.DeliveryCard
		intent.Card = card
		if i > 0 {
			intent.RelatedID = fmt.Sprintf("%s:reply:%06d:create", message.TurnID, i-1)
		}
		if r.deliveryExists(createID) {
			if !final {
				if lookup, ok := r.state.(interface {
					LatestCard(string) (channel.DeliveryIntent, bool)
				}); ok {
					if previous, found := lookup.LatestCard(createID); found && reflect.DeepEqual(previous.Card, card) {
						continue
					}
				}
			}
			intent.ID = fmt.Sprintf("%s:reply:%06d:update:%020d", message.TurnID, i, sequence)
			if final {
				intent.ID = fmt.Sprintf("%s:reply:%06d:final", message.TurnID, i)
			}
			intent.Kind = channel.DeliveryCardUpdate
			intent.RelatedID = createID
		}
		intents = append(intents, intent)
	}
	return intents
}

func interactionPolicy(message channel.InboundMessage) agentengine.InteractionPolicy {
	if isCommentReply(message) {
		return agentengine.InteractionSkipUserInput
	}
	return agentengine.InteractionResolve
}

func (r *Runner) enqueueProcessStart(message channel.InboundMessage) error {
	intent := baseIntent(message, message.TurnID+":cot:create", 0)
	intent.Kind = channel.DeliveryCOTCreate
	intent.OriginMessageID = message.Source.MessageID
	if err := r.state.Enqueue(intent); err != nil {
		return err
	}
	return r.enqueueProcess(message, 0, presentation.NewProcess(message.TurnID, message.ConversationKey).Start())
}

func (r *Runner) enqueueProcess(message channel.InboundMessage, sequence uint64, events []channel.COTEvent) error {
	if len(events) == 0 {
		return nil
	}
	intent := baseIntent(message, fmt.Sprintf("%s:cot:events:%020d", message.TurnID, sequence), sequence)
	intent.Kind = channel.DeliveryCOTUpdate
	intent.RelatedID = message.TurnID + ":cot:create"
	intent.Events = events
	if err := r.state.AppendTurnDeliveries(message.TurnID, sequence, intent); err != nil {
		return err
	}
	r.notify()
	return nil
}

func (r *Runner) finishProcess(message channel.InboundMessage, p *presentation.Process) {
	r.refreshInteractions(message)
	r.interactionMu.Lock()
	defer r.interactionMu.Unlock()
	record, _ := r.state.Get(message.TurnID)
	status := agentengine.TurnStatus(record.Status)
	intent := baseIntent(message, message.TurnID+":cot:complete", record.LastSequence+1)
	intent.Kind = channel.DeliveryCOTComplete
	intent.RelatedID = message.TurnID + ":cot:create"
	for _, item := range r.interactions {
		if item.message.TurnID == message.TurnID && !item.finished && !item.request.Detached {
			intent.Events = append(intent.Events, presentation.Waiting(item.request.ID, true))
		}
	}
	intent.Events = append(intent.Events, p.Finish(status)...)
	lastEvents := intent
	lastEvents.ID = message.TurnID + ":cot:final-events"
	lastEvents.Kind = channel.DeliveryCOTUpdate
	if err := r.state.Enqueue(lastEvents); err != nil {
		r.logFinalizeError(message, err)
	}
	intent.Events = nil
	intent.Reason = "done"
	if status != agentengine.TurnSucceeded {
		intent.Reason = "error"
	}
	if err := r.state.Enqueue(intent); err != nil {
		r.logFinalizeError(message, err)
	}
	r.notify()
}
