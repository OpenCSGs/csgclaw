package ingress

import (
	"fmt"
	"strings"

	"csgclaw/internal/agentengine"
	channeltypes "csgclaw/internal/channel"
	feishuctx "csgclaw/internal/channel/feishu/context"
	"csgclaw/internal/channel/feishu/interaction"
	feishustate "csgclaw/internal/channel/feishu/state"
	"csgclaw/internal/channel/feishu/transport"
)

type normalizedCardAction struct {
	source          channeltypes.Source
	turnID          string
	conversationKey string
	input           interaction.Input
	successText     string
	trusted         bool
}

const expiredCardActionText = "This card has expired and was not applied."

type activeTurnLookup interface {
	ActiveTurn(string) string
}

type cardRouteState interface {
	ResolveControlTarget(feishustate.ControlQuery) (feishustate.ControlTarget, bool)
}

func normalizeCardAction(binding channeltypes.Binding, event transport.Event, runner activeTurnLookup, state cardRouteState) (normalizedCardAction, error) {
	action := event.CardAction
	if action == nil {
		return normalizedCardAction{}, fmt.Errorf("Feishu card action payload is required")
	}
	eventID := strings.TrimSpace(event.EventID)
	if eventID == "" {
		return normalizedCardAction{}, fmt.Errorf("Feishu card action event ID is required")
	}
	chatID := strings.TrimSpace(action.ChatID)
	if chatID == "" {
		return normalizedCardAction{}, fmt.Errorf("Feishu card action chat ID is required")
	}
	threadID := strings.TrimSpace(action.ThreadID)
	operation := interaction.Operation(strings.ToLower(firstMapString(action.ActionValue, "operation", "action", "csgclaw_action")))
	if operation == "" && strings.EqualFold(firstMapString(action.ActionValue, "cmd"), "stop") {
		// The local CardKit renderer emits cmd=stop. The callback remains
		// bound to the trusted remote card message ID before it can cancel a Turn.
		operation = interaction.OperationCancel
	}
	successText := map[interaction.Operation]string{
		interaction.OperationCancel: "Canceled the active turn.",
		interaction.OperationReset:  "Cleared my internal history for this conversation. The IM room messages were not cleared.",
	}[operation]
	route, routed, err := trustedCardRoute(binding, action, state)
	if err != nil {
		return normalizedCardAction{}, err
	}
	card := normalizedCardAction{
		source: channeltypes.Source{
			Channel:   binding.Channel,
			BindingID: binding.ID,
			EventID:   eventID,
			MessageID: strings.TrimSpace(action.MessageID),
			ChatID:    chatID,
			ChatType:  strings.TrimSpace(string(action.ChatType)),
			ThreadID:  threadID,
		},
		turnID:      feishuctx.TurnID(binding.ID, eventID, action.MessageID),
		successText: expiredCardActionText,
	}
	if !routed {
		// Card action values are untrusted. Without a locally delivered card
		// record, no action is allowed to select an Agent, Conversation, or Turn.
		return card, nil
	}
	// The public Ingress callback intentionally does not manufacture a thread
	// ID. Reconstruct it only from the card delivery that CSGClaw recorded, so
	// card controls keep the same Engine conversation and reply target as the
	// originating card.
	threadID = route.intent.ThreadID
	card.source.ThreadID = threadID
	conversationKey := route.record.ConversationKey
	turnID := route.record.TurnID
	if operation == interaction.OperationCancel {
		turnID = route.record.TurnID
		if runner == nil || runner.ActiveTurn(route.record.ConversationKey) != route.record.TurnID {
			successText = "The requested turn is no longer active."
		}
	}
	if route.intent.Kind == channeltypes.DeliveryCOTCreate {
		if operation != interaction.OperationCancel {
			return card, nil
		}
	}
	card.conversationKey = conversationKey
	card.input = interaction.Input{
		InteractionID:   route.intent.InteractionID,
		ResponderID:     strings.TrimSpace(action.Operator.OpenID),
		OptionID:        firstMapString(action.ActionValue, "option_id"),
		FormValue:       action.FormValue,
		AgentID:         route.record.AgentID,
		ConversationKey: conversationKey,
		TurnID:          turnID,
		Action:          interaction.CardAction{Operation: operation},
	}
	card.successText = successText
	card.trusted = true
	return card, nil
}

type trustedCardRouteResult struct {
	intent channeltypes.DeliveryIntent
	record channeltypes.TurnRecord
}

func trustedCardRoute(binding channeltypes.Binding, action *transport.CardAction, state cardRouteState) (trustedCardRouteResult, bool, error) {
	if state == nil || action == nil || strings.TrimSpace(action.MessageID) == "" {
		return trustedCardRouteResult{}, false, nil
	}
	target, found := state.ResolveControlTarget(feishustate.ControlQuery{BindingID: binding.ID, AgentID: binding.AgentID, MessageID: strings.TrimSpace(action.MessageID), ChatID: strings.TrimSpace(action.ChatID), ThreadID: strings.TrimSpace(action.ThreadID), RequesterID: strings.TrimSpace(action.Operator.OpenID)})
	return trustedCardRouteResult{intent: target.Intent, record: target.Turn}, found, nil
}

func firstMapString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
				return text
			}
		}
	}
	return ""
}

func cardActionErrorText(err error) string {
	switch agentengine.ErrorCodeOf(err) {
	case agentengine.ErrorConversationBusy:
		return "This conversation is busy; the action was not applied."
	case agentengine.ErrorInteractionNotFound:
		return "This interaction is no longer active."
	case agentengine.ErrorAgentUnavailable:
		return "Agent is currently unavailable; the action was not applied."
	default:
		return "The card action could not be applied."
	}
}
