package binding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/channel/feishu/delivery"
	"csgclaw/internal/channel/feishu/execution"
	"csgclaw/internal/channel/feishu/files"
	"csgclaw/internal/channel/feishu/ingress"
	"csgclaw/internal/channel/feishu/interaction"
	feishustate "csgclaw/internal/channel/feishu/state"
	"csgclaw/internal/channel/feishu/transport"
	"csgclaw/internal/im"
)

type PipelineFactoryOptions struct {
	Engine    agentengine.Interface
	Transport transport.Factory
	MediaRoot string
}

// PipelineFactory creates one cohesive transport -> bounded ingress -> Engine
// -> in-memory delivery worker per stable App Binding.
type PipelineFactory struct {
	engine    agentengine.Interface
	transport transport.Factory
	mediaRoot string
}

func NewPipelineFactory(options PipelineFactoryOptions) (*PipelineFactory, error) {
	if options.Engine == nil {
		return nil, fmt.Errorf("feishu worker factory: agent engine is required")
	}
	if options.Transport == nil {
		options.Transport = transport.NewFactory()
	}
	if strings.TrimSpace(options.MediaRoot) == "" {
		return nil, fmt.Errorf("feishu worker factory: media root is required")
	}
	return &PipelineFactory{
		engine:    options.Engine,
		transport: options.Transport,
		mediaRoot: options.MediaRoot,
	}, nil
}

func (f *PipelineFactory) NewWorker(resolved Resolved) (Worker, error) {
	if f == nil || f.engine == nil || f.transport == nil {
		return nil, fmt.Errorf("feishu worker factory is not configured")
	}
	if strings.TrimSpace(resolved.Binding.ID) == "" || strings.TrimSpace(resolved.Binding.AgentID) == "" {
		return nil, fmt.Errorf("feishu worker binding and agent IDs are required")
	}
	return &pipelineWorker{factory: f, resolved: resolved}, nil
}

type pipelineWorker struct {
	factory  *PipelineFactory
	resolved Resolved

	lifecycleMu sync.Mutex
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	adapter     transport.Adapter
	intake      *ingress.Intake
	runner      *execution.Runner
	dispatcher  *delivery.Dispatcher
	monitorDone chan struct{}
}

func (w *pipelineWorker) Start(ctx context.Context) error {
	if w == nil {
		return fmt.Errorf("feishu worker is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	w.mu.Lock()
	if w.cancel != nil {
		w.mu.Unlock()
		return nil
	}
	workerCtx, cancel := context.WithCancel(ctx)
	w.mu.Unlock()

	slog.Debug("start Feishu binding worker", resolvedLogAttrs(w.resolved)...)
	sink := &deferredSink{}
	slog.Debug("create Feishu transport adapter", resolvedLogAttrs(w.resolved)...)
	adapter, err := w.factory.transport.New(transport.Config{
		AppID:     w.resolved.App.AppID,
		AppSecret: w.resolved.App.AppSecret,
	}, sink)
	if err != nil {
		cancel()
		slog.Warn("create Feishu transport adapter failed", resolvedLogAttrs(w.resolved, "error", err)...)
		return err
	}
	var intake *ingress.Intake
	var runner *execution.Runner
	fail := func(cause error) error {
		return errors.Join(cause, cleanupFailedStart(cancel, adapter, intake, runner))
	}
	store := feishustate.NewStore()
	dispatcher, err := delivery.NewDispatcher(delivery.DispatcherOptions{
		State:   store,
		Adapter: adapter,
		Files: delivery.NewEngineFileResolver(
			w.factory.engine.Conversations(w.resolved.Binding.AgentID).Files(),
		),
	})
	if err != nil {
		return fail(err)
	}
	stagingRoot := filepath.Join(w.factory.mediaRoot, bindingDirectory(w.resolved.Binding.ID))
	if err := files.CleanupStaging(stagingRoot); err != nil {
		return fail(fmt.Errorf("clean Feishu attachment staging for binding %q: %w", w.resolved.Binding.ID, err))
	}
	slog.Debug("cleaned Feishu attachment staging", resolvedLogAttrs(w.resolved, "staging_root", stagingRoot)...)
	preparer := &files.Preparer{
		Downloader: adapter,
		Files:      w.factory.engine.Conversations(w.resolved.Binding.AgentID).Files(),
		Root:       stagingRoot,
	}
	runner, err = execution.NewRunner(execution.RunnerOptions{
		Engine:   w.factory.engine,
		State:    store,
		Files:    preparer,
		Notifier: dispatcher,
	})
	if err != nil {
		return fail(err)
	}
	interactions, err := interaction.NewHandler(w.factory.engine)
	if err != nil {
		return fail(err)
	}
	comments, _ := adapter.(transport.CommentAdapter)
	messages, _ := adapter.(transport.MessageAdapter)
	intake, err = ingress.NewIntake(ingress.IntakeOptions{
		Binding:      w.resolved.Binding,
		State:        store,
		Runner:       runner,
		Interactions: interactions,
		Notifier:     dispatcher,
		Comments:     comments,
		Messages:     messages,
	})
	if err != nil {
		return fail(err)
	}
	if preparer, ok := adapter.(transport.IdentityPreparer); ok {
		identity, err := preparer.PrepareIdentity(workerCtx)
		if err != nil {
			return fail(fmt.Errorf("prepare Feishu binding %q bot identity: %w", w.resolved.Binding.ID, err))
		}
		if strings.TrimSpace(identity.OpenID) == "" {
			return fail(fmt.Errorf("prepare Feishu binding %q: bot open_id is required for exact mention filtering", w.resolved.Binding.ID))
		}
		slog.Debug("prepared Feishu binding bot identity",
			resolvedLogAttrs(w.resolved,
				"bot_open_id", identity.OpenID,
				"bot_name", identity.Name,
			)...)
		intake.SetIdentity(identity)
	}
	sink.Set(intake)
	if err := adapter.Start(workerCtx); err != nil {
		slog.Warn("start Feishu transport adapter failed", resolvedLogAttrs(w.resolved, "error", err)...)
		return fail(err)
	}
	identity := adapter.Identity()
	if strings.TrimSpace(identity.OpenID) == "" {
		return fail(fmt.Errorf("start feishu binding %q: bot open_id is required for exact mention filtering", w.resolved.Binding.ID))
	}
	slog.Debug("started Feishu transport adapter",
		resolvedLogAttrs(w.resolved,
			"bot_open_id", identity.OpenID,
			"bot_name", identity.Name,
		)...)
	intake.SetIdentity(identity)
	// Events observed during adapter.Start may enter the bounded memory buffer,
	// but execution starts only after the binding identity is ready.
	if err := intake.Start(workerCtx); err != nil {
		return fail(err)
	}
	if err := intake.Activate(); err != nil {
		return fail(fmt.Errorf("activate Feishu binding %q ingress: %w", w.resolved.Binding.ID, err))
	}
	slog.Debug("activated Feishu ingress intake", resolvedLogAttrs(w.resolved)...)
	if err := dispatcher.Start(workerCtx); err != nil {
		return fail(err)
	}
	slog.Debug("started Feishu delivery dispatcher", resolvedLogAttrs(w.resolved)...)

	w.mu.Lock()
	w.ctx = workerCtx
	w.cancel = cancel
	w.adapter = adapter
	w.intake = intake
	w.runner = runner
	w.dispatcher = dispatcher
	w.monitorDone = make(chan struct{})
	go func() { defer close(w.monitorDone); runner.Monitor(workerCtx) }()
	w.mu.Unlock()
	slog.Debug("Feishu binding worker ready", resolvedLogAttrs(w.resolved)...)
	return nil
}

func cleanupFailedStart(cancel context.CancelFunc, adapter transport.Adapter, intake *ingress.Intake, runner *execution.Runner) error {
	if cancel != nil {
		cancel()
	}
	if intake != nil {
		intake.Close()
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()
	var closeErr error
	if runner != nil {
		if err := runner.Wait(closeCtx); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("wait for Feishu executions after startup failure: %w", err))
		}
	}
	if adapter != nil {
		if err := adapter.Close(closeCtx); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close Feishu adapter after startup failure: %w", err))
		}
	}
	return closeErr
}

func (w *pipelineWorker) Close(ctx context.Context) error {
	if w == nil {
		return nil
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	w.mu.Lock()
	cancel := w.cancel
	adapter := w.adapter
	intake := w.intake
	runner := w.runner
	dispatcher := w.dispatcher
	monitorDone := w.monitorDone
	w.ctx = nil
	w.cancel = nil
	w.adapter = nil
	w.intake = nil
	w.runner = nil
	w.dispatcher = nil
	w.mu.Unlock()
	if cancel == nil {
		return nil
	}
	slog.Debug("close Feishu binding worker", resolvedLogAttrs(w.resolved)...)
	cancel()
	if intake != nil {
		intake.Close()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx := ctx
	waitCancel := func() {}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		waitCtx, waitCancel = context.WithTimeout(ctx, 5*time.Second)
	}
	defer waitCancel()
	var closeErr error
	if runner != nil {
		if err := runner.Wait(waitCtx); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("wait for Feishu executions: %w", err))
		}
	}
	if monitorDone != nil {
		<-monitorDone
	}
	if dispatcher != nil {
		dispatcher.Close()
	}
	if adapter != nil {
		if err := adapter.Close(waitCtx); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}
	if closeErr != nil {
		slog.Warn("close Feishu binding worker failed", resolvedLogAttrs(w.resolved, "error", closeErr)...)
	} else {
		slog.Debug("closed Feishu binding worker", resolvedLogAttrs(w.resolved)...)
	}
	return closeErr
}

func (w *pipelineWorker) HandleLocalMessage(ctx context.Context, event feishu.MessageEvent) error {
	if w == nil {
		return fmt.Errorf("feishu worker is required")
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	if event.Type != feishu.MessageEventTypeMessageCreated || event.Message == nil {
		return nil
	}
	if strings.TrimSpace(event.MentionBotID) != strings.TrimSpace(w.resolved.Binding.ParticipantID) {
		return nil
	}
	w.mu.Lock()
	intake := w.intake
	w.mu.Unlock()
	if intake == nil {
		return fmt.Errorf("feishu binding %q intake is not ready", w.resolved.Binding.ID)
	}
	mapped, err := localTransportEvent(event)
	if err != nil {
		return err
	}
	return intake.HandleEvent(ctx, mapped)
}

func localTransportEvent(event feishu.MessageEvent) (transport.Event, error) {
	message := event.Message
	if message == nil {
		return transport.Event{}, fmt.Errorf("local Feishu message is required")
	}
	messageID := strings.TrimSpace(message.ID)
	chatID := strings.TrimSpace(event.RoomID)
	if messageID == "" || chatID == "" {
		return transport.Event{}, fmt.Errorf("local Feishu message and room IDs are required")
	}
	createdAt := message.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	mapped := &transport.Message{
		ID:          messageID,
		ChatID:      chatID,
		ChatType:    transport.ChatGroup,
		Sender:      transport.Identity{OpenID: strings.TrimSpace(message.SenderID)},
		SenderType:  transport.SenderBot,
		Text:        message.Content,
		ContentType: "text",
		CreatedAt:   createdAt,
	}
	for _, mention := range message.Mentions {
		mapped.Mentions = append(mapped.Mentions, transport.Mention{
			OpenID: strings.TrimSpace(mention.ID), Name: strings.TrimSpace(mention.Name),
		})
	}
	if relation := message.RelatesTo; relation != nil && strings.TrimSpace(relation.RelType) == im.RelationTypeThread {
		mapped.ThreadID = strings.TrimSpace(relation.EventID)
		mapped.RootID = mapped.ThreadID
		mapped.ParentID = mapped.ThreadID
	}
	return transport.Event{
		Kind:       transport.EventMessage,
		EventID:    messageID,
		OccurredAt: createdAt,
		Message:    mapped,
	}, nil
}

type deferredSink struct {
	mu     sync.RWMutex
	target transport.Sink
}

func (s *deferredSink) Set(target transport.Sink) {
	s.mu.Lock()
	s.target = target
	s.mu.Unlock()
}

func (s *deferredSink) HandleEvent(ctx context.Context, event transport.Event) error {
	s.mu.RLock()
	target := s.target
	s.mu.RUnlock()
	if target == nil {
		return fmt.Errorf("feishu ingress is not ready")
	}
	return target.HandleEvent(ctx, event)
}

func bindingDirectory(bindingID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(bindingID)))
	return hex.EncodeToString(sum[:])
}

var _ WorkerFactory = (*PipelineFactory)(nil)
var _ LocalMessageWorker = (*pipelineWorker)(nil)
