// Package interaction connects Agent Engine interaction events to the existing
// built-in IM question UI and Codex user-input broker.
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
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/channelbridge"
	"csgclaw/internal/channelbridge/runtimebridge"
	"csgclaw/internal/participant"
	runtimecodex "csgclaw/internal/runtime/codex"
)

type participantResolver interface {
	Get(channel, id string) (apitypes.Participant, bool)
}

type eventSubmitter interface {
	Submit(channel.Binding, channelbridge.BotEvent) error
	IsCurrent(channel.BindingID, agentengine.ConversationKey, string) bool
}

type agentProvider interface {
	Get(context.Context, string) (agentengine.Agent, error)
}

type sessionResolver interface {
	ExistingEngineSession(context.Context, string, string) (string, bool, error)
}

type Option func(*Coordinator)

// WithRuntimeIdentity lets detached requests participate in the same
// Runtime/session cancellation lifecycle as native Codex interactions.
func WithRuntimeIdentity(agents agentProvider, sessions sessionResolver) Option {
	return func(coordinator *Coordinator) {
		coordinator.agents = agents
		coordinator.sessions = sessions
	}
}

type detachedRequest struct {
	turn    channel.TurnContext
	binding channel.Binding
}

// Coordinator binds blocking Runtime interactions and activates detached
// request_user_input structured outputs. It intentionally reuses the existing
// broker and Web APIs instead of adding an Engine-specific browser endpoint.
type Coordinator struct {
	broker       runtimecodex.UserInputBroker
	participants participantResolver
	store        *delivery.IMTranscriptStore
	agents       agentProvider
	sessions     sessionResolver

	mu        sync.Mutex
	submitter eventSubmitter
	requests  map[string]detachedRequest
}

func NewCoordinator(
	broker runtimecodex.UserInputBroker,
	participants participantResolver,
	store *delivery.IMTranscriptStore,
	opts ...Option,
) *Coordinator {
	if broker == nil || participants == nil || store == nil {
		return nil
	}
	coordinator := &Coordinator{
		broker:       broker,
		participants: participants,
		store:        store,
		requests:     make(map[string]detachedRequest),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(coordinator)
		}
	}
	broker.AddDetachedHandler(coordinator.handleDetachedResolution)
	return coordinator
}

func (c *Coordinator) SetSubmitter(submitter eventSubmitter) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.submitter = submitter
	c.mu.Unlock()
}

func (c *Coordinator) Bind(requestID, channelID, roomID, threadRootID, requesterID string) (activity.UserInputSnapshot, error) {
	if c == nil || c.broker == nil {
		return activity.UserInputSnapshot{}, activity.ErrUserInputNotFound
	}
	return c.broker.Bind(requestID, channelID, roomID, threadRootID, c.channelUserID(requesterID))
}

func (c *Coordinator) Activate(
	ctx context.Context,
	turn channel.TurnContext,
	args activity.RequestUserInputArgs,
) (activity.UserInputSnapshot, error) {
	if c == nil || c.broker == nil {
		return activity.UserInputSnapshot{}, fmt.Errorf("structured user input is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return activity.UserInputSnapshot{}, ctx.Err()
	default:
	}
	questions := make([]activity.UserInputQuestionSnapshot, 0, len(args.Questions))
	for _, question := range args.Questions {
		options := make([]activity.UserInputOptionSnapshot, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, activity.UserInputOptionSnapshot{
				Label:       option.Label,
				Description: option.Description,
			})
		}
		questions = append(questions, activity.UserInputQuestionSnapshot{
			ID:       question.ID,
			Header:   question.Header,
			Question: question.Question,
			Options:  options,
			IsOther:  question.IsOther,
			IsSecret: question.IsSecret,
		})
	}
	var autoResolve time.Duration
	if args.AutoResolutionMS != nil {
		autoResolve = time.Duration(*args.AutoResolutionMS) * time.Millisecond
	}
	execution, err := c.execution(ctx, turn)
	if err != nil {
		return activity.UserInputSnapshot{}, err
	}
	execution.ToolCallID = "structured-output-" + strings.TrimSpace(turn.SourceMessageID)
	execution.ToolKind = "request_user_input"
	snapshot, err := c.broker.CreateDetached(runtimecodex.PendingUserInputRequest{
		Execution:   execution,
		Questions:   questions,
		RequestedAt: time.Now().UTC(),
		AutoResolve: autoResolve,
	}, runtimecodex.DetachedUserInputContext{
		Channel:         string(channel.ChannelCSGClaw),
		RoomID:          strings.TrimSpace(turn.RoomID),
		ThreadRootID:    strings.TrimSpace(turn.ThreadRootID),
		SourceMessageID: strings.TrimSpace(turn.SourceMessageID),
		RequesterID:     c.channelUserID(turn.ParticipantID),
	})
	if err != nil {
		return activity.UserInputSnapshot{}, err
	}
	c.mu.Lock()
	c.requests[snapshot.ID] = detachedRequest{
		turn: turn,
		binding: channel.Binding{
			ID:            string(turn.BindingID),
			Channel:       channel.ChannelCSGClaw,
			ParticipantID: turn.ParticipantID,
			AgentID:       turn.AgentID,
			Enabled:       true,
		},
	}
	c.mu.Unlock()
	return snapshot, nil
}

func (c *Coordinator) execution(ctx context.Context, turn channel.TurnContext) (activity.ExecutionRef, error) {
	if c.agents == nil || c.sessions == nil {
		return activity.ExecutionRef{}, fmt.Errorf("structured user input runtime identity is not configured")
	}
	selected, err := c.agents.Get(ctx, strings.TrimSpace(turn.AgentID))
	if err != nil {
		return activity.ExecutionRef{}, fmt.Errorf("resolve structured user input agent: %w", err)
	}
	runtimeID := strings.TrimSpace(selected.Status.RuntimeID)
	if runtimeID == "" {
		return activity.ExecutionRef{}, fmt.Errorf("structured user input runtime is unavailable")
	}
	sessionID, ok, err := c.sessions.ExistingEngineSession(ctx, runtimeID, strings.TrimSpace(string(turn.ConversationKey)))
	if err != nil {
		return activity.ExecutionRef{}, fmt.Errorf("resolve structured user input session: %w", err)
	}
	sessionID = strings.TrimSpace(sessionID)
	if !ok || sessionID == "" {
		return activity.ExecutionRef{}, fmt.Errorf("structured user input session is unavailable")
	}
	return activity.ExecutionRef{
		RuntimeKind: "codex",
		RuntimeID:   runtimeID,
		SessionID:   sessionID,
		TurnID:      string(turn.TurnID),
	}, nil
}

func (c *Coordinator) handleDetachedResolution(resolution runtimecodex.DetachedUserInputResolution) {
	if c == nil {
		return
	}
	c.mu.Lock()
	request, ok := c.requests[resolution.Snapshot.ID]
	if ok {
		delete(c.requests, resolution.Snapshot.ID)
	}
	submitter := c.submitter
	c.mu.Unlock()
	if !ok {
		return
	}

	snapshot := resolution.Snapshot
	if snapshot.Status == activity.UserInputStatusAnswered &&
		(submitter == nil || !submitter.IsCurrent(request.turn.BindingID, request.turn.ConversationKey, request.turn.SourceMessageID)) {
		snapshot.Status = activity.UserInputStatusInterrupted
		now := time.Now().UTC()
		snapshot.ResolvedAt = &now
	}
	if err := c.persistResolution(request.turn, snapshot); err != nil {
		slog.Warn("persist structured user input resolution failed", "request_id", resolution.Snapshot.ID, "error", err)
		return
	}
	if snapshot.Status != activity.UserInputStatusAnswered || submitter == nil {
		return
	}
	body, err := json.Marshal(activity.RedactSecretUserInputResponse(snapshot, resolution.Response))
	if err != nil {
		slog.Warn("encode structured user input continuation failed", "request_id", resolution.Snapshot.ID, "error", err)
		return
	}
	prompt := "The user answered the request_user_input emitted by the previous successful command. Continue the same workflow using this wire-compatible response JSON. Secret values are replaced with <redacted> before entering the model session:\n" + string(body)
	if err := submitter.Submit(request.binding, channelbridge.BotEvent{
		Channel:       string(channel.ChannelCSGClaw),
		ParticipantID: request.binding.ParticipantID,
		MessageID:     "structured-user-input-" + resolution.Snapshot.ID,
		RoomID:        request.turn.RoomID,
		Locale:        request.turn.Locale,
		ChatType:      request.turn.ChatType,
		Text:          prompt,
		ThreadRootID:  request.turn.ThreadRootID,
	}); err != nil {
		slog.Warn("submit structured user input continuation failed", "request_id", resolution.Snapshot.ID, "error", err)
	}
}

func (c *Coordinator) persistResolution(turn channel.TurnContext, snapshot activity.UserInputSnapshot) error {
	event := activity.RuntimeEvent{
		RuntimeKind:     "codex",
		RuntimeID:       string(turn.BindingID),
		SessionID:       string(turn.ConversationKey),
		TurnID:          string(turn.TurnID),
		Kind:            activity.RuntimeEventUserInputResolved,
		ReceivedAt:      time.Now().UTC(),
		UserInputID:     snapshot.ID,
		UserInputStatus: string(snapshot.Status),
		Payload:         snapshot,
	}
	rendered, ok := runtimebridge.NewTurnRenderer().RenderActivity(
		event,
		string(channel.ChannelCSGClaw),
		turn.RoomID,
		turn.ParticipantID,
	)
	if !ok {
		return fmt.Errorf("render structured user input resolution")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.store.DeliverRenderedActivity(ctx, turn, delivery.ActivityDelivery{
		MessageID:    rendered.MessageID,
		Text:         rendered.Text,
		Metadata:     rendered.Metadata,
		ThreadRootID: turn.ThreadRootID,
		Event: agentengine.TurnEvent{
			TurnID: turn.TurnID,
			Kind:   agentengine.TurnEventActivityUpdate,
			Activity: &agentengine.ActivityUpdate{
				ID:      snapshot.ID,
				Kind:    string(activity.RuntimeEventUserInputResolved),
				Status:  string(snapshot.Status),
				Payload: snapshot,
			},
		},
	})
}

func (c *Coordinator) channelUserID(participantID string) string {
	participantID = strings.TrimSpace(participantID)
	if c != nil && c.participants != nil {
		if item, ok := c.participants.Get(participant.ChannelCSGClaw, participantID); ok {
			if channelUserRef := strings.TrimSpace(item.ChannelUserRef); channelUserRef != "" {
				return channelUserRef
			}
		}
	}
	return participantID
}
