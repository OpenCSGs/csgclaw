// Package interaction maps Channel UI identities to Engine-owned interactions.
// It owns transcript routing and continuation delivery, never Runtime requests.
package interaction

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
)

type participantResolver interface {
	Get(channel, id string) (apitypes.Participant, bool)
}
type eventSubmitter interface {
	Submit(channel.Binding, channel.Event) error
	IsCurrent(channel.BindingID, agentengine.ConversationKey, string) bool
}
type route struct {
	turn        channel.TurnContext
	requesterID string
	detached    bool
}

// Coordinator stores only the trusted UI-to-Engine route. Pending state,
// validation, duplicate decisions, expiry and cancellation belong to Engine.
type Coordinator struct {
	engine       agentengine.Interface
	participants participantResolver
	mu           sync.Mutex
	routes       map[string]route
	submitter    eventSubmitter
	lastPrune    time.Time
}

func NewCoordinator(engine agentengine.Interface, participants participantResolver) *Coordinator {
	if engine == nil || participants == nil {
		return nil
	}
	return &Coordinator{engine: engine, participants: participants, routes: make(map[string]route)}
}
func (c *Coordinator) SetSubmitter(submitter eventSubmitter) {
	c.mu.Lock()
	c.submitter = submitter
	c.mu.Unlock()
}

func (c *Coordinator) Bind(turn channel.TurnContext, request agentengine.InteractionRequest) (agentengine.InteractionRequest, error) {
	if c == nil || strings.TrimSpace(request.ID) == "" {
		return request, fmt.Errorf("interaction and coordinator are required")
	}
	c.mu.Lock()
	prune := time.Since(c.lastPrune) >= time.Minute
	if prune {
		c.lastPrune = time.Now()
	}
	c.mu.Unlock()
	if prune {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		c.Prune(ctx)
		cancel()
	}
	current, err := c.engine.Conversations(turn.AgentID).GetInteraction(context.Background(), turn.ConversationKey, request.ID)
	if err != nil {
		return request, err
	}
	request = current
	requester := strings.TrimSpace(turn.ParticipantID)
	if item, ok := c.participants.Get(string(channel.ChannelCSGClaw), requester); ok && item.ChannelUserRef != "" {
		requester = item.ChannelUserRef
	}
	c.mu.Lock()
	if previous, ok := c.routes[request.ID]; ok && (previous.turn.AgentID != turn.AgentID || previous.turn.ConversationKey != turn.ConversationKey) {
		c.mu.Unlock()
		return request, fmt.Errorf("interaction is already bound to another conversation")
	}
	c.routes[request.ID] = route{turn: turn, requesterID: requester, detached: request.Detached}
	c.mu.Unlock()
	return project(request, route{turn: turn, requesterID: requester, detached: request.Detached}), nil
}
func project(request agentengine.InteractionRequest, where route) agentengine.InteractionRequest {
	if snapshot, ok := request.Payload.(activity.UserInputSnapshot); ok {
		snapshot = activity.PublicUserInputSnapshot(snapshot)
		snapshot.Channel = string(channel.ChannelCSGClaw)
		snapshot.RoomID = where.turn.RoomID
		snapshot.ThreadRootID = where.turn.ThreadRootID
		snapshot.RequesterID = where.requesterID
		request.Payload = snapshot
	}
	return request
}
func (c *Coordinator) Project(event agentengine.TurnEvent) agentengine.TurnEvent {
	if event.Activity == nil {
		return event
	}
	where, ok := c.lookup(event.Activity.ID)
	if !ok {
		return event
	}
	update := *event.Activity
	update.Payload = project(agentengine.InteractionRequest{Payload: update.Payload}, where).Payload
	event.Activity = &update
	return event
}
func (c *Coordinator) lookup(id string) (route, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.routes[strings.TrimSpace(id)]
	return value, ok
}
func (c *Coordinator) read(ctx context.Context, id string) (agentengine.InteractionRequest, route, error) {
	where, ok := c.lookup(id)
	if !ok {
		return agentengine.InteractionRequest{}, route{}, activity.ErrUserInputNotFound
	}
	item, err := c.engine.Conversations(where.turn.AgentID).GetInteraction(ctx, where.turn.ConversationKey, id)
	return project(item, where), where, err
}
func (c *Coordinator) Get(id string) (activity.UserInputSnapshot, bool) {
	item, _, err := c.read(context.Background(), id)
	if err != nil {
		return activity.UserInputSnapshot{}, false
	}
	snapshot, ok := item.Payload.(activity.UserInputSnapshot)
	return snapshot, ok
}
func (c *Coordinator) Respond(ctx context.Context, req activity.UserInputResponseRequest) (activity.UserInputSnapshot, error) {
	item, where, err := c.read(ctx, req.ActivityID)
	if err != nil {
		return activity.UserInputSnapshot{}, activity.ErrUserInputNotFound
	}
	snapshot, ok := item.Payload.(activity.UserInputSnapshot)
	if !ok || req.Channel != string(channel.ChannelCSGClaw) || snapshot.RoomID != req.RoomID || strings.TrimSpace(req.ResponderID) == "" {
		return activity.UserInputSnapshot{}, activity.ErrUserInputNotFound
	}
	resolution := agentengine.InteractionResolution{ConversationKey: where.turn.ConversationKey, InteractionID: req.ActivityID, ResponderID: req.ResponderID, Answers: make(map[string]agentengine.InteractionAnswer, len(req.Response.Answers))}
	for id, answer := range req.Response.Answers {
		resolution.Answers[id] = agentengine.InteractionAnswer{Values: append([]string(nil), answer.Answers...), Skipped: len(answer.Answers) == 0}
	}
	if req.RecordTranscript != nil {
		resolution.BeforeResolve = func(ctx context.Context, request agentengine.InteractionRequest) error {
			projected := project(request, where)
			snapshot, ok := projected.Payload.(activity.UserInputSnapshot)
			if !ok {
				return fmt.Errorf("Engine returned an invalid user input snapshot")
			}
			if len(req.Response.Answers) == 0 {
				return nil
			}
			return req.RecordTranscript(ctx, snapshot)
		}
	}
	err = c.engine.Conversations(where.turn.AgentID).Resolve(ctx, resolution)
	updated, _, readErr := c.read(ctx, req.ActivityID)
	if readErr == nil {
		if value, ok := updated.Payload.(activity.UserInputSnapshot); ok {
			snapshot = value
		}
	}
	return snapshot, userInputError(err)
}
func (c *Coordinator) Decide(ctx context.Context, req activity.ActivityDecisionRequest) (activity.ActivitySnapshot, error) {
	if req.Channel != string(channel.ChannelCSGClaw) {
		return activity.ActivitySnapshot{}, activity.ErrActionNotFound
	}
	item, where, err := c.read(ctx, req.ActivityID)
	if err != nil {
		return activity.ActivitySnapshot{}, activity.ErrActionNotFound
	}
	snapshot, ok := item.Payload.(activity.ActivitySnapshot)
	if !ok {
		return snapshot, activity.ErrActionNotFound
	}
	err = c.engine.Conversations(where.turn.AgentID).Resolve(ctx, agentengine.InteractionResolution{ConversationKey: where.turn.ConversationKey, InteractionID: req.ActivityID, OptionID: req.OptionID})
	if updated, _, readErr := c.read(ctx, req.ActivityID); readErr == nil {
		if value, ok := updated.Payload.(activity.ActivitySnapshot); ok {
			snapshot = value
		}
	}
	switch agentengine.ErrorCodeOf(err) {
	case agentengine.ErrorInvalidRequest:
		return snapshot, activity.ErrActionInvalidOption
	case agentengine.ErrorInteractionNotFound:
		return snapshot, activity.ErrActionNotFound
	case agentengine.ErrorInteractionAlreadyResolved:
		return snapshot, activity.ErrActionAlreadyDecided
	case agentengine.ErrorInteractionGone:
		return snapshot, activity.ErrActionGone
	}
	return snapshot, err
}
func userInputError(err error) error {
	switch agentengine.ErrorCodeOf(err) {
	case agentengine.ErrorInvalidRequest:
		return fmt.Errorf("%w: %v", activity.ErrUserInputInvalidResponse, err)
	case agentengine.ErrorInteractionNotFound:
		return activity.ErrUserInputNotFound
	case agentengine.ErrorInteractionAlreadyResolved:
		return activity.ErrUserInputAlreadyResolved
	case agentengine.ErrorInteractionGone:
		return activity.ErrUserInputGone
	}
	return err
}

// Observe projects Runtime/Engine updates. A detached answer re-enters the
// Channel's normal ingress exactly once after its resolved card is persisted.
func (c *Coordinator) Observe(turn channel.TurnContext, event agentengine.TurnEvent) {
	if event.Activity == nil || event.Activity.Kind != string(activity.RuntimeEventUserInputResolved) {
		return
	}
	id := event.Activity.ID
	c.mu.Lock()
	where, ok := c.routes[id]
	submitter := c.submitter
	if !ok || !where.detached {
		c.mu.Unlock()
		return
	}
	// Claim the continuation, retaining the route for duplicate HTTP requests.
	where.detached = false
	c.routes[id] = where
	c.mu.Unlock()
	snapshot, ok := event.Activity.Payload.(activity.UserInputSnapshot)
	if !ok || snapshot.Status != activity.UserInputStatusAnswered || submitter == nil || !submitter.IsCurrent(turn.BindingID, turn.ConversationKey, turn.SourceMessageID) {
		return
	}
	response := activity.RequestUserInputResponse{Answers: make(map[string]activity.RequestUserInputAnswer, len(snapshot.Questions))}
	for _, question := range snapshot.Questions {
		answer := snapshot.Answers[question.ID]
		values := []string{}
		if answer.Answered {
			if answer.Secret {
				values = append(values, "user_note: <redacted>")
			} else {
				if answer.OptionLabel != "" {
					values = append(values, answer.OptionLabel)
				}
				if answer.Text != "" {
					values = append(values, "user_note: "+answer.Text)
				}
			}
		}
		response.Answers[question.ID] = activity.RequestUserInputAnswer{Answers: values}
	}
	body, err := json.Marshal(response)
	if err != nil {
		return
	}
	prompt := "The user answered the request_user_input emitted by the previous successful command. Continue the same workflow using this wire-compatible response JSON. Secret values are replaced with <redacted> before entering the model session:\n" + string(body)
	err = submitter.Submit(channel.Binding{ID: string(turn.BindingID), Channel: channel.ChannelCSGClaw, ParticipantID: turn.ParticipantID, AgentID: turn.AgentID, Enabled: true}, channel.Event{
		Channel: string(channel.ChannelCSGClaw), ParticipantID: turn.ParticipantID, MessageID: "structured-user-input-" + id, RoomID: turn.RoomID, Locale: turn.Locale, ChatType: turn.ChatType, Text: prompt, ThreadRootID: turn.ThreadRootID,
	})
	if err != nil {
		slog.Warn("submit user input continuation failed", "interaction_id", id, "error", err)
	}
}

// Prune forgets expired UI routes without retaining Runtime or secret state.
func (c *Coordinator) Prune(ctx context.Context) {
	c.mu.Lock()
	routes := make(map[string]route, len(c.routes))
	for id, r := range c.routes {
		routes[id] = r
	}
	c.mu.Unlock()
	for id, r := range routes {
		if ctx.Err() != nil {
			return
		}
		item, err := c.engine.Conversations(r.turn.AgentID).GetInteraction(ctx, r.turn.ConversationKey, id)
		stale := agentengine.ErrorCodeOf(err) == agentengine.ErrorInteractionNotFound
		if snapshot, ok := item.Payload.(activity.UserInputSnapshot); ok && snapshot.ResolvedAt != nil {
			stale = time.Since(*snapshot.ResolvedAt) > 10*time.Minute
		}
		if stale {
			c.mu.Lock()
			delete(c.routes, id)
			c.mu.Unlock()
		}
	}
}
