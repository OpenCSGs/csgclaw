package agent

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/notifier"
)

// NotifierPullWorker periodically polls relay inbox for notifier agents with AllowsPull().
type NotifierPullWorker struct {
	Agents  *Service
	Deliver notifier.Fanouter
	Relay   *notifier.RelayClient
	Log     *slog.Logger

	mu       sync.Mutex
	lastPoll map[string]time.Time // last pull attempt (success or failure), for interval throttling
}

func NewNotifierPullWorker(agents *Service, d notifier.Fanouter) *NotifierPullWorker {
	return &NotifierPullWorker{
		Agents:   agents,
		Deliver:  d,
		Relay:    &notifier.RelayClient{},
		Log:      slog.Default(),
		lastPoll: make(map[string]time.Time),
	}
}

// Run blocks until ctx is cancelled; uses a base ticker and per-agent intervals.
func (w *NotifierPullWorker) Run(ctx context.Context) {
	if w == nil || w.Agents == nil || w.Deliver == nil {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *NotifierPullWorker) tick(ctx context.Context) {
	if err := w.Agents.Reload(); err != nil && w.Log != nil {
		w.Log.Debug("notifier pull: agent reload", "error", err)
	}
	for _, a := range w.Agents.List() {
		if !strings.EqualFold(strings.TrimSpace(a.Role), RoleNotifier) {
			continue
		}
		cfg := notifier.ParseConfigFromRequestOptions(a.AgentProfile.RequestOptions)
		if !cfg.AllowsPull() {
			continue
		}
		interval := cfg.PollIntervalDuration()
		w.mu.Lock()
		last := w.lastPoll[a.ID]
		w.mu.Unlock()
		if !last.IsZero() && time.Since(last) < interval {
			continue
		}
		err := w.pullAgent(ctx, a, cfg)
		if err != nil && w.Log != nil {
			// Transient relay errors are common in dev; Info avoids noisy WARN every poll_interval.
			w.Log.Info("notifier pull failed", "agent_id", a.ID, "error", err)
		}
		w.mu.Lock()
		w.lastPoll[a.ID] = time.Now().UTC()
		w.mu.Unlock()
	}
}

func (w *NotifierPullWorker) pullAgent(ctx context.Context, a Agent, cfg notifier.Config) error {
	msgs, _, err := w.Relay.FetchInbox(ctx, cfg.RemoteURL, cfg, 50, "")
	if err != nil {
		return err
	}
	var ackIDs []string
	for _, m := range msgs {
		raw, ct, err := notifier.DecodePayload(m)
		if err != nil {
			if w.Log != nil {
				w.Log.Warn("notifier inbox decode skipped", "agent_id", a.ID, "msg_id", m.ID, "error", err)
			}
			continue
		}
		md := notifier.FormatPayloadAsMarkdown(raw, ct)
		if err := w.Deliver.DeliverNotifierFanout(a.ID, md); err != nil {
			return err
		}
		if strings.TrimSpace(m.ID) != "" {
			ackIDs = append(ackIDs, strings.TrimSpace(m.ID))
		}
	}
	if err := w.Relay.Ack(ctx, cfg.RemoteURL, cfg, ackIDs); err != nil {
		return err
	}
	return nil
}
