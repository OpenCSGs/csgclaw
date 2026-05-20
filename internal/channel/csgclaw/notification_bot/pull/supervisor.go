package pull

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/bot"
	"csgclaw/internal/channel/csgclaw/notification_bot"
)

// BotLister lists notification bots eligible for pull delivery.
type BotLister interface {
	Reload() error
	ListNotificationBots(channel string) ([]bot.Bot, error)
	// LookupNotificationBotForDelivery returns stored runtime_options (with secrets), not API-redacted view.
	LookupNotificationBotForDelivery(channel, id string) (runtimeOptions map[string]any, userID string, ok bool)
}

// Supervisor reconciles per-bot pull loops for notification bots with pull delivery enabled.
type Supervisor struct {
	Bots    BotLister
	Deliver notification_bot.Fanouter
	Relay   *notification_bot.RelayClient
	Log     *slog.Logger

	reloadMu     sync.Mutex
	lastReload   time.Time
	reloadPeriod time.Duration

	mu       sync.Mutex
	loopStop map[string]context.CancelFunc
}

// NewSupervisor wires notification pull delivery over the bot store and IM fanout.
func NewSupervisor(bots BotLister, d notification_bot.Fanouter) *Supervisor {
	return &Supervisor{
		Bots:         bots,
		Deliver:      d,
		Relay:        &notification_bot.RelayClient{},
		Log:          slog.Default(),
		loopStop:     make(map[string]context.CancelFunc),
		reloadPeriod: 10 * time.Second,
	}
}

// Run blocks until ctx is cancelled.
func (s *Supervisor) Run(ctx context.Context) {
	if s == nil || s.Bots == nil || s.Deliver == nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer s.stopAllLoops()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.maybeReloadFromDisk()
			s.syncLoops(ctx)
		}
	}
}

func (s *Supervisor) stopAllLoops() {
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.loopStop))
	for _, c := range s.loopStop {
		cancels = append(cancels, c)
	}
	s.loopStop = make(map[string]context.CancelFunc)
	s.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

func (s *Supervisor) maybeReloadFromDisk() {
	period := s.reloadPeriod
	if period <= 0 {
		period = 10 * time.Second
	}
	now := time.Now()
	s.reloadMu.Lock()
	if !s.lastReload.IsZero() && now.Sub(s.lastReload) < period {
		s.reloadMu.Unlock()
		return
	}
	s.lastReload = now
	s.reloadMu.Unlock()
	if err := s.Bots.Reload(); err != nil && s.Log != nil {
		s.Log.Debug("notification pull: bot reload", "error", err)
	}
}

func (s *Supervisor) desiredPullBotIDs() map[string]struct{} {
	out := make(map[string]struct{})
	channel := string(bot.ChannelCSGClaw)
	bots, err := s.Bots.ListNotificationBots(channel)
	if err != nil {
		if s.Log != nil {
			s.Log.Warn("notification pull: list bots", "error", err)
		}
		return out
	}
	for _, b := range bots {
		// ListNotificationBots returns API-redacted runtime_options (no remote_token).
		// Pull eligibility must use stored secrets via LookupNotificationBotForDelivery.
		flat, _, ok := s.Bots.LookupNotificationBotForDelivery(channel, b.ID)
		if !ok {
			continue
		}
		cfg := notification_bot.ConfigFromBotRuntimeOptions(flat)
		if !cfg.PullDeliveryComplete() {
			if s.Log != nil && cfg.AllowsPull() {
				s.Log.Debug("notification pull: bot not ready", "bot_id", b.ID, "has_token", strings.TrimSpace(cfg.RemoteToken) != "")
			}
			continue
		}
		out[b.ID] = struct{}{}
	}
	return out
}

func (s *Supervisor) syncLoops(parentCtx context.Context) {
	desired := s.desiredPullBotIDs()

	s.mu.Lock()
	for id, cancel := range s.loopStop {
		if _, ok := desired[id]; !ok {
			cancel()
			delete(s.loopStop, id)
		}
	}
	type loopStart struct {
		ctx context.Context
		id  string
	}
	var starts []loopStart
	for id := range desired {
		if _, ok := s.loopStop[id]; ok {
			continue
		}
		loopCtx, cancel := context.WithCancel(parentCtx)
		s.loopStop[id] = cancel
		starts = append(starts, loopStart{ctx: loopCtx, id: id})
	}
	s.mu.Unlock()
	for _, st := range starts {
		if s.Log != nil {
			s.Log.Info("notification pull loop started", "bot_id", st.id)
		}
		go s.botPullLoop(st.ctx, st.id)
	}
}

func (s *Supervisor) lookupBot(botID string) (bot.Bot, notification_bot.Config, bool) {
	id := strings.TrimSpace(botID)
	if id == "" {
		return bot.Bot{}, notification_bot.Config{}, false
	}
	flat, userID, ok := s.Bots.LookupNotificationBotForDelivery(string(bot.ChannelCSGClaw), id)
	if !ok || len(flat) == 0 {
		return bot.Bot{}, notification_bot.Config{}, false
	}
	cfg := notification_bot.ConfigFromBotRuntimeOptions(flat)
	b := bot.Bot{ID: id, UserID: userID, Channel: string(bot.ChannelCSGClaw)}
	return b, cfg, true
}

func (s *Supervisor) botPullLoop(ctx context.Context, botID string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		b, cfg, ok := s.lookupBot(botID)
		if !ok {
			return
		}
		if !cfg.PullDeliveryComplete() {
			return
		}
		if err := s.pullBot(ctx, b, cfg); err != nil {
			if s.Log != nil {
				s.Log.Info("notification pull failed", "bot_id", b.ID, "error", err)
			}
		}
		interval := cfg.PollIntervalDuration()
		if interval <= 0 {
			interval = 5 * time.Second
		}
		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !t.Stop() {
				<-t.C
			}
			return
		case <-t.C:
		}
	}
}

func (s *Supervisor) pullBot(ctx context.Context, b bot.Bot, cfg notification_bot.Config) error {
	msgs, _, err := s.Relay.FetchInbox(ctx, cfg, 50, "")
	if err != nil {
		return err
	}
	memberID := strings.TrimSpace(b.UserID)
	if memberID == "" {
		memberID = strings.TrimSpace(b.ID)
	}
	var ackIDs []string
	var deliverErr error
	delivered := 0
	for _, m := range msgs {
		raw, ct, err := notification_bot.DecodePayload(m)
		if err != nil {
			if s.Log != nil {
				s.Log.Warn("notification inbox decode skipped", "bot_id", b.ID, "msg_id", m.ID, "error", err)
			}
			continue
		}
		content := notification_bot.FormatPayloadAsChatContent(raw, ct, nil)
		if err := s.Deliver.DeliverFanout(memberID, content); err != nil {
			deliverErr = err
			break
		}
		delivered++
		if id := strings.TrimSpace(m.ID); id != "" {
			ackIDs = append(ackIDs, id)
		}
	}
	if len(ackIDs) > 0 {
		if err := s.Relay.Ack(ctx, cfg, ackIDs); err != nil {
			return err
		}
	}
	if s.Log != nil {
		switch {
		case len(msgs) == 0:
			s.Log.Debug("notification pull ok", "bot_id", b.ID, "messages", 0)
		case delivered > 0:
			s.Log.Info("notification pull delivered", "bot_id", b.ID, "messages", delivered, "acked", len(ackIDs))
		case len(msgs) > 0 && delivered == 0:
			s.Log.Warn("notification pull: inbox had messages but none delivered", "bot_id", b.ID, "inbox", len(msgs))
		}
	}
	return deliverErr
}
