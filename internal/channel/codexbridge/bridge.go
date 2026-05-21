package codexbridge

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	runtimecodex "csgclaw/internal/runtime/codex"

	acp "github.com/coder/acp-go-sdk"
)

const (
	defaultQueueSize    = 32
	defaultSeenWindow   = 256
	defaultPromptSettle = 150 * time.Millisecond
)

type Binding struct {
	BotID      string
	RuntimeID  string
	SessionID  string
	PromptMeta map[string]any
}

type SessionPrompter interface {
	Prompt(ctx context.Context, handle runtimecodex.SessionHandle, req acp.PromptRequest) (acp.PromptResponse, error)
}

type ConversationSessionEnsurer interface {
	EnsureSession(ctx context.Context, handle runtimecodex.SessionHandle, conversationKey string) (string, error)
}

type Service struct {
	client         BotClient
	prompter       SessionPrompter
	events         *EventSink
	reconnectDelay time.Duration
	queueSize      int
	seenWindow     int
	promptSettle   time.Duration

	mu      sync.Mutex
	workers map[string]*worker
}

func NewService(client BotClient, prompter SessionPrompter, events *EventSink) *Service {
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
		w.cancel()
		<-w.done
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
	sessionID, err := w.sessionID(ctx, evt)
	if err != nil {
		renderer := newTurnRenderer()
		renderer.promptError = strings.TrimSpace(err.Error())
		_, err := w.flushTurn(ctx, evt.RoomID, evt.ThreadRootID, renderer)
		return err
	}
	req := acp.PromptRequest{
		SessionId: acp.SessionId(sessionID),
		Prompt:    []acp.ContentBlock{acp.TextBlock(w.promptText(evt))},
		Meta:      cloneMeta(w.binding.PromptMeta),
	}
	renderer := newTurnRenderer()

	type promptResult struct {
		err error
	}
	promptDone := make(chan promptResult, 1)
	go func() {
		_, err := w.service.prompter.Prompt(ctx, runtimecodex.SessionHandle{RuntimeID: w.binding.RuntimeID}, req)
		promptDone <- promptResult{err: err}
	}()

	var toolMessages []string
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
				return fmt.Errorf("codex event sink closed")
			}
			if !matchesSession(event, w.binding.RuntimeID, sessionID) {
				continue
			}
			for _, text := range renderer.Apply(event) {
				toolMessages = append(toolMessages, text)
			}
			if isTerminalEvent(event.Kind) && promptReturned {
				return w.flushTurnWithToolAttachments(ctx, evt, renderer, toolMessages)
			}
		case result := <-promptDone:
			promptReturned = true
			if result.err != nil {
				renderer.promptError = strings.TrimSpace(result.err.Error())
				return w.flushTurnWithToolAttachments(ctx, evt, renderer, toolMessages)
			}
			settleTimer.Reset(w.service.promptSettle)
		case <-settleTimer.C:
			if promptReturned {
				return w.flushTurnWithToolAttachments(ctx, evt, renderer, toolMessages)
			}
		}
	}
}

func (w *worker) flushTurnWithToolAttachments(ctx context.Context, evt BotEvent, renderer *turnRenderer, toolMessages []string) error {
	responseMessageID, err := w.flushTurn(ctx, evt.RoomID, evt.ThreadRootID, renderer)
	if err != nil {
		return err
	}
	threadRootID := toolAttachmentThreadRootID(evt, responseMessageID)
	for _, text := range toolMessages {
		if _, err := w.sendMessage(ctx, evt.RoomID, threadRootID, text); err != nil {
			return err
		}
	}
	return nil
}

func (w *worker) flushTurn(ctx context.Context, roomID, threadRootID string, renderer *turnRenderer) (string, error) {
	var firstMessageID string
	for _, text := range renderer.FinalMessages() {
		messageID, err := w.sendMessage(ctx, roomID, threadRootID, text)
		if err != nil {
			return "", err
		}
		if firstMessageID == "" {
			firstMessageID = messageID
		}
	}
	return firstMessageID, nil
}

func (w *worker) sendMessage(ctx context.Context, roomID, threadRootID, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	resp, err := w.service.client.SendMessage(ctx, w.binding.BotID, SendMessageRequest{
		RoomID:       roomID,
		Text:         text,
		ThreadRootID: strings.TrimSpace(threadRootID),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.MessageID), nil
}

func toolAttachmentThreadRootID(evt BotEvent, responseMessageID string) string {
	if rootID := strings.TrimSpace(evt.ThreadRootID); rootID != "" {
		return rootID
	}
	if responseID := strings.TrimSpace(responseMessageID); responseID != "" {
		return responseID
	}
	return strings.TrimSpace(evt.MessageID)
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
	if key == "" || evt.ThreadContext == nil {
		return text
	}

	w.mu.Lock()
	_, sent := w.contextSent[key]
	if !sent {
		w.contextSent[key] = struct{}{}
	}
	w.mu.Unlock()
	if sent {
		return text
	}

	contextText := formatHiddenThreadContext(evt.ThreadContext)
	if contextText == "" {
		return text
	}
	if text == "" {
		return contextText
	}
	return contextText + "\n\nCurrent thread message:\n" + text
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
