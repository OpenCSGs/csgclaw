package codexbridge

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	csgclawchannel "csgclaw/internal/channel/csgclaw"
	"csgclaw/internal/channelbridge/runtimebridge"
	runtimecodex "csgclaw/internal/runtime/codex"
	"csgclaw/internal/slashcommand"
)

const (
	defaultQueueSize          = 32
	defaultSeenWindow         = 256
	defaultPromptSettle       = 150 * time.Millisecond
	localChannel              = csgclawchannel.ChannelID
	turnPlaceholderText       = "\u200b"
	turnCompleteText          = "Done."
	processingPinEmoji        = "Pin"
	processingReactionTimeout = 2 * time.Second
)

type Binding struct {
	BotID      string
	RuntimeID  string
	SessionID  string
	PromptMeta map[string]any
}

type SessionPrompter interface {
	Prompt(ctx context.Context, handle runtimecodex.SessionHandle, req runtimecodex.PromptRequest) (runtimecodex.PromptResponse, error)
}

type ConversationSessionEnsurer interface {
	EnsureSession(ctx context.Context, handle runtimecodex.SessionHandle, conversationKey string) (string, error)
}

type ConversationHistoryClearer interface {
	ResetConversationHistory(ctx context.Context, handle runtimecodex.SessionHandle, conversationKey string) error
}

type Service struct {
	client         BotClient
	prompter       SessionPrompter
	events         runtimecodex.SessionEventSubscriber
	reconnectDelay time.Duration
	queueSize      int
	seenWindow     int
	promptSettle   time.Duration

	mu      sync.Mutex
	workers map[string]*worker
}

func NewService(client BotClient, prompter SessionPrompter, events runtimecodex.SessionEventSubscriber) *Service {
	return &Service{
		client:         client,
		prompter:       prompter,
		events:         events,
		reconnectDelay: defaultReconnectDelay,
		queueSize:      defaultQueueSize,
		seenWindow:     defaultSeenWindow,
		promptSettle:   defaultPromptSettle,
		workers:        make(map[string]*worker),
	}
}

func (s *Service) StartBot(ctx context.Context, binding Binding) error {
	if s == nil {
		return fmt.Errorf("codex bridge service is required")
	}
	if s.client == nil {
		return fmt.Errorf("bot client is required")
	}
	if s.prompter == nil {
		return fmt.Errorf("session prompter is required")
	}
	if s.events == nil {
		return fmt.Errorf("event sink is required")
	}

	binding.BotID = strings.TrimSpace(binding.BotID)
	binding.RuntimeID = strings.TrimSpace(binding.RuntimeID)
	binding.SessionID = strings.TrimSpace(binding.SessionID)
	if binding.BotID == "" || binding.RuntimeID == "" {
		return fmt.Errorf("bot id and runtime id are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing := s.workers[binding.BotID]; existing != nil {
		if sameBinding(existing.binding, binding) {
			slog.Debug("codex bridge bot already running",
				"bot_id", binding.BotID,
				"runtime_id", binding.RuntimeID,
				"session_id", binding.SessionID,
			)
			return nil
		}
		existing.cancel()
	}

	if ctx == nil {
		ctx = context.Background()
	}
	workerCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	w := &worker{
		service:     s,
		binding:     binding,
		queue:       make(chan BotEvent, s.queueSize),
		queued:      make(map[string]struct{}),
		contextSent: make(map[string]struct{}),
		latest:      make(map[string]string),
		seen:        newRecentSet(s.seenWindow),
		cancel:      cancel,
		done:        make(chan struct{}),
	}
	s.workers[binding.BotID] = w
	slog.Debug("codex bridge bot started",
		"bot_id", binding.BotID,
		"runtime_id", binding.RuntimeID,
		"session_id", binding.SessionID,
	)
	go w.run(workerCtx)
	return nil
}

func (s *Service) StopBot(botID string) {
	if s == nil {
		return
	}
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return
	}

	s.mu.Lock()
	w := s.workers[botID]
	delete(s.workers, botID)
	s.mu.Unlock()

	if w != nil {
		slog.Debug("codex bridge bot stopping", "bot_id", botID)
		w.cancel()
		<-w.done
		slog.Debug("codex bridge bot stopped", "bot_id", botID)
	}
}

func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	workers := make([]*worker, 0, len(s.workers))
	for _, w := range s.workers {
		workers = append(workers, w)
	}
	s.workers = make(map[string]*worker)
	s.mu.Unlock()

	for _, w := range workers {
		w.cancel()
	}
	for _, w := range workers {
		<-w.done
	}
}

type worker struct {
	service *Service
	binding Binding
	queue   chan BotEvent
	queued  map[string]struct{}
	seen    *recentSet
	cancel  context.CancelFunc
	done    chan struct{}

	mu          sync.Mutex
	processing  string
	lastEvent   string
	latest      map[string]string
	contextSent map[string]struct{}
}

func (w *worker) run(ctx context.Context) {
	defer close(w.done)

	eventCh, cancelEvents := w.service.events.Subscribe(w.binding.RuntimeID)
	defer cancelEvents()

	go w.pumpEvents(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-w.queue:
			w.beginProcessing(eventDedupKey(evt))
			if !w.isSuperseded(evt) {
				_ = w.handleEvent(ctx, evt, eventCh)
			} else {
				slog.Debug("codex bridge skipped superseded message",
					"bot_id", w.binding.BotID,
					"runtime_id", w.binding.RuntimeID,
					"channel", strings.TrimSpace(evt.Channel),
					"room_id", strings.TrimSpace(evt.RoomID),
					"message_id", strings.TrimSpace(evt.MessageID),
					"thread_root_id", strings.TrimSpace(evt.ThreadRootID),
				)
			}
			w.finishProcessing(eventDedupKey(evt))
		}
	}
}

func (w *worker) pumpEvents(ctx context.Context) {
	for {
		events, errs := w.service.client.StreamEvents(ctx, w.binding.BotID, w.lastEventID())
		closed := false
		for !closed {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-events:
				if !ok {
					closed = true
					continue
				}
				if strings.TrimSpace(evt.MessageID) != "" {
					w.setLastEventID(evt.MessageID)
				}
				w.enqueue(ctx, evt)
			case err, ok := <-errs:
				if ok && err != nil {
					closed = true
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(w.service.reconnectDelay):
		}
	}
}

func (w *worker) enqueue(ctx context.Context, evt BotEvent) {
	if !w.accept(evt) {
		return
	}
	select {
	case <-ctx.Done():
	case w.queue <- evt:
	}
}

func (w *worker) handleEvent(ctx context.Context, evt BotEvent, runtimeEvents <-chan runtimecodex.SessionEvent) error {
	eventStartedAt := time.Now()
	cleanupProcessingReaction := w.startProcessingReaction(ctx, evt)
	defer cleanupProcessingReaction(context.Background())
	if cmd, ok, err := slashcommand.Parse(evt.Text); err == nil && ok && slashcommand.IsNewConversationCommand(cmd) {
		cleanupProcessingReaction(ctx)
		return w.handleConversationReset(ctx, evt)
	} else if err != nil {
		renderer := runtimebridge.NewTurnRenderer()
		renderer.SetPromptError(err.Error())
		cleanupProcessingReaction(ctx)
		_, err := w.flushTurn(ctx, evt.RoomID, "", renderer, codexFinalDeliveryMetadata(evt.MessageID))
		return err
	}
	sessionID, err := w.sessionID(ctx, evt)
	if err != nil {
		renderer := runtimebridge.NewTurnRenderer()
		renderer.SetPromptError(err.Error())
		cleanupProcessingReaction(ctx)
		_, err := w.flushTurn(ctx, evt.RoomID, "", renderer, codexFinalDeliveryMetadata(evt.MessageID))
		return err
	}
	req := runtimecodex.PromptRequest{
		SessionID: sessionID,
		Meta:      cloneMeta(w.binding.PromptMeta),
	}
	promptText := w.promptText(evt)
	req.Prompt = []runtimecodex.PromptContentBlock{runtimecodex.TextBlock(promptText)}
	slog.Debug("codex bridge prompt start",
		"bot_id", w.binding.BotID,
		"runtime_id", w.binding.RuntimeID,
		"session_id", sessionID,
		"channel", strings.TrimSpace(evt.Channel),
		"room_id", strings.TrimSpace(evt.RoomID),
		"message_id", strings.TrimSpace(evt.MessageID),
		"thread_root_id", strings.TrimSpace(evt.ThreadRootID),
		"prompt_bytes", len(promptText),
		"event_text_bytes", len(strings.TrimSpace(evt.Text)),
		"has_thread_context", evt.ThreadContext != nil,
	)
	renderer := runtimebridge.NewTurnRenderer()
	turnRootID := strings.TrimSpace(evt.ThreadRootID)
	var generatedRootID string

	ensureActivityThreadRoot := func() (string, error) {
		if turnRootID != "" {
			return turnRootID, nil
		}
		if generatedRootID != "" {
			return generatedRootID, nil
		}
		messageID, err := w.sendMessage(ctx, evt.RoomID, "", turnPlaceholderText)
		if err != nil {
			return "", err
		}
		generatedRootID = strings.TrimSpace(messageID)
		if generatedRootID == "" {
			return "", fmt.Errorf("create turn root message: empty message id")
		}
		return generatedRootID, nil
	}
	flushTurn := func() (string, error) {
		cleanupProcessingReaction(ctx)
		if w.isSuperseded(evt) {
			slog.Debug("codex bridge suppressed superseded turn final",
				"bot_id", w.binding.BotID,
				"runtime_id", w.binding.RuntimeID,
				"session_id", sessionID,
				"channel", strings.TrimSpace(evt.Channel),
				"room_id", strings.TrimSpace(evt.RoomID),
				"message_id", strings.TrimSpace(evt.MessageID),
				"thread_root_id", strings.TrimSpace(evt.ThreadRootID),
			)
			return "", nil
		}
		if generatedRootID != "" && len(renderer.FinalMessages()) == 0 {
			return w.flushTurnWithEmptyCompletion(ctx, evt.RoomID, generatedRootID, renderer, nil)
		}
		return w.flushTurn(ctx, evt.RoomID, "", renderer, codexFinalDeliveryMetadata(evt.MessageID))
	}

	type promptResult struct {
		err error
	}
	promptDone := make(chan promptResult, 1)
	go func() {
		promptStartedAt := time.Now()
		_, err := w.service.prompter.Prompt(ctx, runtimecodex.SessionHandle{RuntimeID: w.binding.RuntimeID}, req)
		slog.Debug("codex bridge prompt returned",
			"bot_id", w.binding.BotID,
			"runtime_id", w.binding.RuntimeID,
			"session_id", sessionID,
			"message_id", strings.TrimSpace(evt.MessageID),
			"duration", time.Since(promptStartedAt),
			"error", err,
		)
		promptDone <- promptResult{err: err}
	}()

	promptReturned := false
	settleTimer := time.NewTimer(time.Hour)
	defer settleTimer.Stop()
	if !settleTimer.Stop() {
		select {
		case <-settleTimer.C:
		default:
		}
	}

	handleRuntimeEvent := func(event runtimecodex.SessionEvent) (bool, error) {
		if !matchesSession(event, w.binding.RuntimeID, sessionID) {
			return false, nil
		}
		if w.isSuperseded(evt) {
			return isTerminalEvent(event.Kind), nil
		}
		if commentaryText, ok := codexCommentaryText(event); ok {
			slog.Debug("codex bridge captured commentary payload",
				"bot_id", w.binding.BotID,
				"runtime_id", w.binding.RuntimeID,
				"session_id", sessionID,
				"text_bytes", len(commentaryText),
			)
			return false, nil
		}
		if isCodexFinalTextEvent(event) {
			renderer.ApplyText(event)
		}
		if renderedActivity, ok := renderer.RenderActivity(event, localChannel, evt.RoomID, w.binding.BotID); ok {
			threadRootID := ""
			metadata := codexActivityDeliveryMetadata(event, evt.MessageID)
			if !isCodexToolDeliveryEvent(event) {
				var err error
				threadRootID, err = ensureActivityThreadRoot()
				if err != nil {
					return false, err
				}
			}
			if err := w.sendActivity(ctx, evt.RoomID, threadRootID, renderedActivity, metadata); err != nil {
				return false, err
			}
		}
		return isTerminalEvent(event.Kind), nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-runtimeEvents:
			if !ok {
				return fmt.Errorf("runtime event sink closed")
			}
			terminal, err := handleRuntimeEvent(event)
			if err != nil {
				return err
			}
			if terminal && promptReturned {
				slog.Debug("codex bridge terminal event flush",
					"bot_id", w.binding.BotID,
					"runtime_id", w.binding.RuntimeID,
					"session_id", sessionID,
					"message_id", strings.TrimSpace(evt.MessageID),
					"kind", string(event.Kind),
					"elapsed", time.Since(eventStartedAt),
				)
				_, err := flushTurn()
				return err
			}
		case result := <-promptDone:
			promptReturned = true
			if w.isSuperseded(evt) {
				cleanupProcessingReaction(ctx)
				// Prompt completion is published through the event sink before the
				// Prompt call returns, but buffered sink consumption is asynchronous.
				// Drain that terminal event before the next queued turn starts so
				// stale output cannot be mistaken for the newer turn.
				settleTimer.Reset(w.service.promptSettle)
				continue
			}
			if result.err != nil {
				renderer.SetPromptError(result.err.Error())
				_, err := flushTurn()
				return err
			}
			settleTimer.Reset(w.service.promptSettle)
		case <-settleTimer.C:
			if promptReturned {
				for {
					select {
					case event, ok := <-runtimeEvents:
						if !ok {
							return fmt.Errorf("runtime event sink closed")
						}
						terminal, err := handleRuntimeEvent(event)
						if err != nil {
							return err
						}
						if terminal {
							slog.Debug("codex bridge drained terminal event before settle flush",
								"bot_id", w.binding.BotID,
								"runtime_id", w.binding.RuntimeID,
								"session_id", sessionID,
								"message_id", strings.TrimSpace(evt.MessageID),
								"kind", string(event.Kind),
								"elapsed", time.Since(eventStartedAt),
							)
							_, err := flushTurn()
							return err
						}
						continue
					default:
					}
					break
				}
				if w.isSuperseded(evt) {
					cleanupProcessingReaction(ctx)
					return nil
				}
				if generatedRootID == "" && len(renderer.FinalMessages()) == 0 {
					settleTimer.Reset(w.service.promptSettle)
					continue
				}
				slog.Debug("codex bridge settle flush",
					"bot_id", w.binding.BotID,
					"runtime_id", w.binding.RuntimeID,
					"session_id", sessionID,
					"message_id", strings.TrimSpace(evt.MessageID),
					"elapsed", time.Since(eventStartedAt),
				)
				_, err := flushTurn()
				return err
			}
		}
	}
}

func (w *worker) handleConversationReset(ctx context.Context, evt BotEvent) error {
	roomID := strings.TrimSpace(evt.RoomID)
	if roomID == "" {
		return w.flushConversationResetError(ctx, evt, "room id is required")
	}
	resetter, ok := w.service.prompter.(ConversationHistoryClearer)
	if !ok {
		return w.flushConversationResetError(ctx, evt, "codex session prompter does not support conversation reset")
	}
	conversationKey := conversationKey(evt)
	if conversationKey == "" {
		return w.flushConversationResetError(ctx, evt, "conversation key is required")
	}
	if err := resetter.ResetConversationHistory(ctx, runtimecodex.SessionHandle{RuntimeID: w.binding.RuntimeID}, conversationKey); err != nil {
		return w.flushConversationResetError(ctx, evt, err.Error())
	}
	w.clearContextCache(conversationKey)
	_, err := w.sendMessage(ctx, roomID, evt.ThreadRootID, "Cleared my internal history for this conversation. The IM room messages were not cleared.")
	return err
}

func (w *worker) flushConversationResetError(ctx context.Context, evt BotEvent, message string) error {
	renderer := runtimebridge.NewTurnRenderer()
	renderer.SetPromptError(strings.TrimSpace(message))
	_, err := w.flushTurn(ctx, evt.RoomID, evt.ThreadRootID, renderer, nil)
	if err != nil {
		return err
	}
	return fmt.Errorf("%s", strings.TrimSpace(message))
}

func (w *worker) clearContextCache(conversationKey string) {
	conversationKey = strings.TrimSpace(conversationKey)
	if conversationKey == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.contextSent, conversationKey)
}

func (w *worker) flushTurn(ctx context.Context, roomID, threadRootID string, renderer *runtimebridge.TurnRenderer, metadata map[string]any) (string, error) {
	return w.flushTurnMessages(ctx, roomID, threadRootID, false, renderer, metadata)
}

func (w *worker) flushTurnWithEmptyCompletion(ctx context.Context, roomID, threadRootID string, renderer *runtimebridge.TurnRenderer, metadata map[string]any) (string, error) {
	return w.flushTurnMessages(ctx, roomID, threadRootID, true, renderer, metadata)
}

func (w *worker) flushTurnMessages(ctx context.Context, roomID, threadRootID string, includeEmptyCompletion bool, renderer *runtimebridge.TurnRenderer, metadata map[string]any) (string, error) {
	messages := renderer.FinalMessages()
	if len(messages) == 0 && includeEmptyCompletion {
		messages = []string{turnCompleteText}
	}
	return w.flushMessages(ctx, roomID, threadRootID, messages, metadata)
}

func (w *worker) flushMessages(ctx context.Context, roomID, threadRootID string, messages []string, metadata map[string]any) (string, error) {
	var firstSentMessageID string
	for _, text := range messages {
		req := SendMessageRequest{
			RoomID:       roomID,
			Text:         text,
			ThreadRootID: strings.TrimSpace(threadRootID),
			Metadata:     metadata,
		}
		messageID, err := w.sendMessageRequest(ctx, req)
		if err != nil {
			return "", err
		}
		if firstSentMessageID == "" {
			firstSentMessageID = messageID
		}
	}
	return firstSentMessageID, nil
}

func (w *worker) startProcessingReaction(ctx context.Context, evt BotEvent) func(context.Context) {
	if !strings.EqualFold(strings.TrimSpace(evt.Channel), "feishu") {
		return func(context.Context) {}
	}
	messageID := strings.TrimSpace(evt.MessageID)
	if messageID == "" {
		return func(context.Context) {}
	}
	reactor, ok := w.service.client.(MessageReactor)
	if !ok {
		return func(context.Context) {}
	}

	addCtx, cancelAdd := contextWithDefaultTimeout(ctx, processingReactionTimeout)
	resultCh := make(chan processingReactionResult, 1)
	go func() {
		defer cancelAdd()
		resp, err := reactor.AddMessageReaction(addCtx, w.binding.BotID, AddMessageReactionRequest{
			MessageID: messageID,
			EmojiType: processingPinEmoji,
		})
		if err != nil {
			slog.Debug("codex bridge add processing reaction failed",
				"bot_id", w.binding.BotID,
				"room_id", strings.TrimSpace(evt.RoomID),
				"message_id", messageID,
				"emoji_type", processingPinEmoji,
				"error", err,
			)
			resultCh <- processingReactionResult{}
			return
		}
		resultCh <- processingReactionResult{reactionID: strings.TrimSpace(resp.ReactionID)}
	}()

	var once sync.Once
	return func(cleanupCtx context.Context) {
		once.Do(func() {
			cancelAdd()
			go w.deleteProcessingReaction(cleanupCtx, evt, reactor, messageID, resultCh)
		})
	}
}

type processingReactionResult struct {
	reactionID string
}

func (w *worker) deleteProcessingReaction(cleanupCtx context.Context, evt BotEvent, reactor MessageReactor, messageID string, resultCh <-chan processingReactionResult) {
	waitCtx, cancelWait := context.WithTimeout(context.Background(), processingReactionTimeout)
	defer cancelWait()

	var result processingReactionResult
	select {
	case result = <-resultCh:
	case <-waitCtx.Done():
		slog.Debug("codex bridge processing reaction cleanup timed out",
			"bot_id", w.binding.BotID,
			"room_id", strings.TrimSpace(evt.RoomID),
			"message_id", messageID,
			"emoji_type", processingPinEmoji,
		)
		return
	}
	reactionID := strings.TrimSpace(result.reactionID)
	if reactionID == "" {
		return
	}

	deleteCtx, cancelDelete := contextWithDefaultTimeout(cleanupCtx, processingReactionTimeout)
	defer cancelDelete()
	if err := reactor.DeleteMessageReaction(deleteCtx, w.binding.BotID, DeleteMessageReactionRequest{
		MessageID:  messageID,
		ReactionID: reactionID,
	}); err != nil {
		slog.Debug("codex bridge delete processing reaction failed",
			"bot_id", w.binding.BotID,
			"room_id", strings.TrimSpace(evt.RoomID),
			"message_id", messageID,
			"reaction_id", reactionID,
			"emoji_type", processingPinEmoji,
			"error", err,
		)
	}
}

func contextWithDefaultTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func (w *worker) sendMessage(ctx context.Context, roomID, threadRootID, text string) (string, error) {
	return w.sendMessageRequest(ctx, SendMessageRequest{
		RoomID:       roomID,
		Text:         text,
		ThreadRootID: strings.TrimSpace(threadRootID),
	})
}

func (w *worker) sendActivity(ctx context.Context, roomID, threadRootID string, activity runtimebridge.RenderedActivity, metadata map[string]any) error {
	_, err := w.sendMessageRequest(ctx, SendMessageRequest{
		RoomID:       roomID,
		Text:         activity.Text,
		ThreadRootID: strings.TrimSpace(threadRootID),
		Metadata:     metadata,
	})
	return err
}

func (w *worker) sendMessageRequest(ctx context.Context, req SendMessageRequest) (string, error) {
	req.Text = strings.TrimSpace(req.Text)
	req.ThreadRootID = strings.TrimSpace(req.ThreadRootID)
	if req.Text == "" {
		return "", nil
	}
	mode := "create"
	if req.ThreadRootID != "" {
		mode = "reply"
	}
	sendStartedAt := time.Now()
	resp, err := w.service.client.SendMessage(ctx, w.binding.BotID, req)
	if err != nil {
		slog.Debug("codex bridge send message failed",
			"bot_id", w.binding.BotID,
			"room_id", strings.TrimSpace(req.RoomID),
			"thread_root_id", strings.TrimSpace(req.ThreadRootID),
			"mode", mode,
			"text_bytes", len(req.Text),
			"duration", time.Since(sendStartedAt),
			"error", err,
		)
		return "", err
	}
	slog.Debug("codex bridge send message completed",
		"bot_id", w.binding.BotID,
		"room_id", strings.TrimSpace(req.RoomID),
		"thread_root_id", strings.TrimSpace(req.ThreadRootID),
		"sent_message_id", strings.TrimSpace(resp.MessageID),
		"mode", mode,
		"text_bytes", len(req.Text),
		"duration", time.Since(sendStartedAt),
	)
	return strings.TrimSpace(resp.MessageID), nil
}

func isCodexFinalTextEvent(event runtimecodex.SessionEvent) bool {
	if event.Kind != runtimecodex.SessionEventTextDelta {
		return false
	}
	phase := codexEventPhase(event)
	return phase == "" || phase == "final_answer"
}

func isCodexToolDeliveryEvent(event runtimecodex.SessionEvent) bool {
	return event.Kind == runtimecodex.SessionEventToolCallStart || event.Kind == runtimecodex.SessionEventToolCallUpdate
}

func codexCommentaryText(event runtimecodex.SessionEvent) (string, bool) {
	if event.Kind != runtimecodex.SessionEventTextDelta {
		return "", false
	}
	if codexEventPhase(event) != "commentary" {
		return "", false
	}
	text := strings.TrimSpace(event.Text)
	return text, text != ""
}

func codexEventPhase(event runtimecodex.SessionEvent) string {
	payload, ok := event.Payload.(map[string]any)
	if !ok {
		return ""
	}
	phase, _ := payload["phase"].(string)
	return strings.TrimSpace(strings.ToLower(phase))
}

func codexFinalDeliveryMetadata(sourceMessageID string) map[string]any {
	sourceMessageID = strings.TrimSpace(sourceMessageID)
	entry := map[string]any{
		"delivery_kind":     "final",
		"request_id":        sourceMessageID,
		"source_message_id": sourceMessageID,
	}
	return map[string]any{
		"codex":    cloneStringAnyMap(entry),
		"openclaw": cloneStringAnyMap(entry),
	}
}

func codexActivityDeliveryMetadata(event runtimecodex.SessionEvent, sourceMessageID string) map[string]any {
	if !isCodexToolDeliveryEvent(event) {
		return nil
	}
	sourceMessageID = strings.TrimSpace(sourceMessageID)
	entry := map[string]any{
		"delivery_kind":     "tool",
		"request_id":        sourceMessageID,
		"source_message_id": sourceMessageID,
		"tool_call_id":      strings.TrimSpace(event.ToolCallID),
		"tool_kind":         strings.TrimSpace(event.ToolKind),
		"tool_status":       strings.TrimSpace(event.ToolStatus),
	}
	return map[string]any{
		"codex":    cloneStringAnyMap(entry),
		"openclaw": cloneStringAnyMap(entry),
	}
}

func cloneStringAnyMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func (w *worker) sessionID(ctx context.Context, evt BotEvent) (string, error) {
	sessionID := sessionIDForEvent(w.binding, evt)
	ensurer, ok := w.service.prompter.(ConversationSessionEnsurer)
	if !ok {
		return sessionID, nil
	}
	ensured, err := ensurer.EnsureSession(ctx, runtimecodex.SessionHandle{RuntimeID: w.binding.RuntimeID}, conversationKey(evt))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(ensured) != "" {
		return strings.TrimSpace(ensured), nil
	}
	return sessionID, nil
}

func (w *worker) promptText(evt BotEvent) string {
	text := joinHiddenContexts(strings.TrimSpace(evt.Text), formatAttachmentManifest(evt.Attachments))
	key := conversationKey(evt)

	contextText := joinHiddenContexts(
		formatHiddenChannelContext(w.binding, evt),
		formatHiddenThreadContext(evt.ThreadContext),
	)
	if contextText == "" {
		return text
	}

	if key != "" {
		w.mu.Lock()
		_, sent := w.contextSent[key]
		if !sent {
			w.contextSent[key] = struct{}{}
		}
		w.mu.Unlock()
		if sent {
			return text
		}
	}

	if text == "" {
		return contextText
	}
	currentLabel := "Current message:\n"
	if evt.ThreadContext != nil {
		currentLabel = "Current thread message:\n"
	}
	return contextText + "\n\n" + currentLabel + text
}

func (w *worker) accept(evt BotEvent) bool {
	key := eventDedupKey(evt)
	if key == "" {
		return true
	}
	scope := conversationKey(evt)

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.seen.Has(key) {
		return false
	}
	if _, ok := w.queued[key]; ok {
		return false
	}
	if w.processing == key {
		return false
	}
	w.queued[key] = struct{}{}
	if scope != "" {
		w.latest[scope] = key
	}
	return true
}

func (w *worker) isSuperseded(evt BotEvent) bool {
	key := eventDedupKey(evt)
	scope := conversationKey(evt)
	if key == "" || scope == "" {
		return false
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	latest := strings.TrimSpace(w.latest[scope])
	return latest != "" && latest != key
}

func (w *worker) beginProcessing(messageID string) {
	messageID = strings.TrimSpace(messageID)
	w.mu.Lock()
	delete(w.queued, messageID)
	w.processing = messageID
	w.mu.Unlock()
}

func (w *worker) finishProcessing(messageID string) {
	messageID = strings.TrimSpace(messageID)
	w.mu.Lock()
	if messageID != "" {
		w.seen.Add(messageID)
	}
	if w.processing == messageID {
		w.processing = ""
	}
	w.mu.Unlock()
}

func (w *worker) lastEventID() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastEvent
}

func (w *worker) setLastEventID(messageID string) {
	w.mu.Lock()
	w.lastEvent = strings.TrimSpace(messageID)
	w.mu.Unlock()
}

func matchesSession(event runtimecodex.SessionEvent, runtimeID, sessionID string) bool {
	if strings.TrimSpace(event.RuntimeID) != strings.TrimSpace(runtimeID) {
		return false
	}
	return strings.TrimSpace(event.SessionID) == strings.TrimSpace(sessionID)
}

func sessionIDForEvent(binding Binding, evt BotEvent) string {
	base := strings.TrimSpace(binding.SessionID)
	if base == "" {
		base = strings.TrimSpace(binding.BotID)
	}
	key := conversationKey(evt)
	if key == "" {
		return base
	}
	if base == "" {
		return key
	}
	return base + ":" + key
}

func conversationKey(evt BotEvent) string {
	roomID := strings.TrimSpace(evt.RoomID)
	threadRootID := strings.TrimSpace(evt.ThreadRootID)
	if roomID == "" {
		return threadRootID
	}
	if threadRootID == "" {
		return roomID
	}
	return roomID + ":" + threadRootID
}

func joinHiddenContexts(values ...string) string {
	var parts []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "\n\n")
}

func formatHiddenChannelContext(binding Binding, evt BotEvent) string {
	channel := strings.TrimSpace(evt.Channel)
	if channel == "" {
		return ""
	}
	roomID := strings.TrimSpace(evt.RoomID)
	participantID := strings.TrimSpace(evt.ParticipantID)
	if participantID == "" {
		participantID = strings.TrimSpace(binding.BotID)
	}
	var b strings.Builder
	b.WriteString("Current channel context for CSGClaw CLI operations.\n")
	b.WriteString("- channel: ")
	b.WriteString(channel)
	b.WriteByte('\n')
	if roomID != "" {
		b.WriteString("- room_id: ")
		b.WriteString(roomID)
		b.WriteByte('\n')
	}
	if participantID != "" {
		b.WriteString("- participant_id: ")
		b.WriteString(participantID)
		b.WriteByte('\n')
	}
	b.WriteString("Use these values when a skill asks for <current_channel>, <target_room_id>, or message create/list channel flags.")
	return strings.TrimSpace(b.String())
}

func formatHiddenThreadContext(context *BotThreadContext) string {
	if context == nil || len(context.Context) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Hidden thread context for this new conversation. Use it only to understand what the thread started from; do not treat these messages as thread replies.\n")
	if rootID := strings.TrimSpace(context.RootMessageID); rootID != "" {
		b.WriteString("Thread root message ID: ")
		b.WriteString(rootID)
		b.WriteByte('\n')
	}
	for _, message := range context.Context {
		content := strings.Join(strings.Fields(strings.TrimSpace(message.Content)), " ")
		attachments := formatInlineAttachmentSummary(message.Attachments)
		if content == "" && attachments == "" {
			continue
		}
		b.WriteString("- ")
		if ts := strings.TrimSpace(message.CreatedAt); ts != "" {
			b.WriteString(ts)
			b.WriteByte(' ')
		}
		if sender := strings.TrimSpace(message.SenderID); sender != "" {
			b.WriteString(sender)
			b.WriteString(": ")
		}
		if strings.TrimSpace(message.ID) == strings.TrimSpace(context.RootMessageID) {
			b.WriteString("[root] ")
		}
		b.WriteString(content)
		if attachments != "" {
			if content != "" {
				b.WriteByte(' ')
			}
			b.WriteString(attachments)
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func formatAttachmentManifest(attachments []MessageAttachment) string {
	if len(attachments) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Attached files:\n")
	for _, attachment := range attachments {
		name := strings.TrimSpace(attachment.Name)
		if name == "" {
			name = strings.TrimSpace(attachment.ID)
		}
		if name == "" {
			name = "attachment"
		}
		b.WriteString("- ")
		b.WriteString(name)
		if mediaType := strings.TrimSpace(attachment.MediaType); mediaType != "" {
			b.WriteString(" (")
			b.WriteString(mediaType)
			if attachment.SizeBytes > 0 {
				b.WriteString(", ")
				b.WriteString(formatAttachmentBytes(attachment.SizeBytes))
			}
			b.WriteString(")")
		} else if attachment.SizeBytes > 0 {
			b.WriteString(" (")
			b.WriteString(formatAttachmentBytes(attachment.SizeBytes))
			b.WriteString(")")
		}
		if path := strings.TrimSpace(attachment.WorkspacePath); path != "" {
			b.WriteString(" workspace_path=")
			b.WriteString(path)
		}
		if url := strings.TrimSpace(attachment.DownloadURL); url != "" {
			b.WriteString(" download_url=")
			b.WriteString(url)
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func formatInlineAttachmentSummary(attachments []MessageAttachment) string {
	if len(attachments) == 0 {
		return ""
	}
	names := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		name := strings.TrimSpace(attachment.Name)
		if name == "" {
			name = strings.TrimSpace(attachment.ID)
		}
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "[attachments]"
	}
	return "[attachments: " + strings.Join(names, ", ") + "]"
}

func formatAttachmentBytes(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(size)/(1024*1024))
}

func eventDedupKey(evt BotEvent) string {
	messageID := strings.TrimSpace(evt.MessageID)
	if messageID == "" {
		return ""
	}
	key := conversationKey(evt)
	if key == "" {
		return messageID
	}
	return key + ":" + messageID
}

func isTerminalEvent(kind runtimecodex.SessionEventKind) bool {
	return kind == runtimecodex.SessionEventPromptCompleted || kind == runtimecodex.SessionEventPromptFailed
}

func cloneMeta(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func sameBinding(left, right Binding) bool {
	return strings.TrimSpace(left.BotID) == strings.TrimSpace(right.BotID) &&
		strings.TrimSpace(left.RuntimeID) == strings.TrimSpace(right.RuntimeID) &&
		strings.TrimSpace(left.SessionID) == strings.TrimSpace(right.SessionID)
}

type recentSet struct {
	limit int
	order []string
	items map[string]struct{}
}

func newRecentSet(limit int) *recentSet {
	if limit <= 0 {
		limit = defaultSeenWindow
	}
	return &recentSet{
		limit: limit,
		items: make(map[string]struct{}),
	}
}

func (s *recentSet) Has(key string) bool {
	if s == nil || key == "" {
		return false
	}
	_, ok := s.items[key]
	return ok
}

func (s *recentSet) Add(key string) {
	if s == nil || key == "" {
		return
	}
	if _, ok := s.items[key]; ok {
		return
	}
	s.items[key] = struct{}{}
	s.order = append(s.order, key)
	if len(s.order) <= s.limit {
		return
	}
	oldest := s.order[0]
	s.order = s.order[1:]
	delete(s.items, oldest)
}
