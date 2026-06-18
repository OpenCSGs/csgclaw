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
			_ = w.handleEvent(ctx, evt, eventCh)
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
		_, err := w.flushTurn(ctx, evt.RoomID, evt.ThreadRootID, renderer)
		return err
	}
	sessionID, err := w.sessionID(ctx, evt)
	if err != nil {
		renderer := runtimebridge.NewTurnRenderer()
		renderer.SetPromptError(err.Error())
		cleanupProcessingReaction(ctx)
		_, err := w.flushTurn(ctx, evt.RoomID, evt.ThreadRootID, renderer)
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
		if generatedRootID == "" {
			return w.flushTurn(ctx, evt.RoomID, turnRootID, renderer)
		}
		if w.canUpdateGeneratedTurnRoot(evt) {
			return w.flushTurnByUpdatingRoot(ctx, evt.RoomID, generatedRootID, renderer)
		}
		if len(renderer.FinalMessages()) == 0 {
			return w.flushTurnWithEmptyCompletion(ctx, evt.RoomID, generatedRootID, renderer)
		}
		return w.flushTurn(ctx, evt.RoomID, turnRootID, renderer)
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

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-runtimeEvents:
			if !ok {
				return fmt.Errorf("runtime event sink closed")
			}
			if !matchesSession(event, w.binding.RuntimeID, sessionID) {
				continue
			}
			if renderedActivity, ok := renderer.RenderActivity(event, localChannel, evt.RoomID, w.binding.BotID); ok {
				threadRootID, err := ensureActivityThreadRoot()
				if err != nil {
					return err
				}
				if err := w.sendActivity(ctx, evt.RoomID, threadRootID, renderedActivity); err != nil {
					return err
				}
			}
			renderer.ApplyText(event)
			if isTerminalEvent(event.Kind) && promptReturned {
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
			if result.err != nil {
				renderer.SetPromptError(result.err.Error())
				_, err := flushTurn()
				return err
			}
			settleTimer.Reset(w.service.promptSettle)
		case <-settleTimer.C:
			if promptReturned {
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
	_, err := w.flushTurn(ctx, evt.RoomID, evt.ThreadRootID, renderer)
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

func (w *worker) flushTurn(ctx context.Context, roomID, threadRootID string, renderer *runtimebridge.TurnRenderer) (string, error) {
	return w.flushTurnMessages(ctx, roomID, threadRootID, false, renderer)
}

func (w *worker) flushTurnWithEmptyCompletion(ctx context.Context, roomID, threadRootID string, renderer *runtimebridge.TurnRenderer) (string, error) {
	return w.flushTurnMessages(ctx, roomID, threadRootID, true, renderer)
}

func (w *worker) flushTurnByUpdatingRoot(ctx context.Context, roomID, rootMessageID string, renderer *runtimebridge.TurnRenderer) (string, error) {
	messages := renderer.FinalMessages()
	if len(messages) == 0 {
		messages = []string{turnCompleteText}
	}
	if len(messages) == 0 {
		return strings.TrimSpace(rootMessageID), nil
	}
	if err := w.updateMessage(ctx, roomID, rootMessageID, messages[0]); err != nil {
		slog.Warn("codex bridge update generated turn root failed; sending completion inside activity thread",
			"bot_id", w.binding.BotID,
			"room_id", strings.TrimSpace(roomID),
			"message_id", strings.TrimSpace(rootMessageID),
			"text_bytes", len(strings.TrimSpace(messages[0])),
			"error", err,
		)
		return w.flushMessages(ctx, roomID, rootMessageID, messages)
	}
	if len(messages) > 1 {
		if _, err := w.flushMessages(ctx, roomID, rootMessageID, messages[1:]); err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(rootMessageID), nil
}

func (w *worker) flushTurnMessages(ctx context.Context, roomID, threadRootID string, includeEmptyCompletion bool, renderer *runtimebridge.TurnRenderer) (string, error) {
	messages := renderer.FinalMessages()
	if len(messages) == 0 && includeEmptyCompletion {
		messages = []string{turnCompleteText}
	}
	return w.flushMessages(ctx, roomID, threadRootID, messages)
}

func (w *worker) flushMessages(ctx context.Context, roomID, threadRootID string, messages []string) (string, error) {
	var firstSentMessageID string
	for _, text := range messages {
		req := SendMessageRequest{
			RoomID:       roomID,
			Text:         text,
			ThreadRootID: strings.TrimSpace(threadRootID),
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

func (w *worker) canUpdateGeneratedTurnRoot(evt BotEvent) bool {
	if !strings.EqualFold(strings.TrimSpace(evt.Channel), "feishu") {
		return false
	}
	_, ok := w.service.client.(MessageUpdater)
	return ok
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

func (w *worker) sendActivity(ctx context.Context, roomID, threadRootID string, activity runtimebridge.RenderedActivity) error {
	_, err := w.sendMessageRequest(ctx, SendMessageRequest{
		RoomID:       roomID,
		Text:         activity.Text,
		ThreadRootID: strings.TrimSpace(threadRootID),
	})
	return err
}

func (w *worker) updateMessage(ctx context.Context, roomID, messageID, text string) error {
	text = strings.TrimSpace(text)
	messageID = strings.TrimSpace(messageID)
	if text == "" || messageID == "" {
		return nil
	}
	updater, ok := w.service.client.(MessageUpdater)
	if !ok {
		return fmt.Errorf("message update is not supported")
	}
	updateStartedAt := time.Now()
	resp, err := updater.UpdateMessage(ctx, w.binding.BotID, UpdateMessageRequest{
		RoomID:    strings.TrimSpace(roomID),
		MessageID: messageID,
		Text:      text,
	})
	if err != nil {
		slog.Debug("codex bridge update message failed",
			"bot_id", w.binding.BotID,
			"room_id", strings.TrimSpace(roomID),
			"message_id", messageID,
			"text_bytes", len(text),
			"duration", time.Since(updateStartedAt),
			"error", err,
		)
		return err
	}
	slog.Debug("codex bridge update message completed",
		"bot_id", w.binding.BotID,
		"room_id", strings.TrimSpace(roomID),
		"message_id", messageID,
		"updated_message_id", strings.TrimSpace(resp.MessageID),
		"text_bytes", len(text),
		"duration", time.Since(updateStartedAt),
	)
	return nil
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
	text := strings.TrimSpace(evt.Text)
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
	return true
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
	if channel == "" || strings.EqualFold(channel, localChannel) {
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
		if content == "" {
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
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
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
