// Package pull runs a background loop that fetches notifier inbox messages from the relay
// and fans them out to IM (same path as webhook delivery).
package pull

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/runtime/notifier"
	"csgclaw/internal/sandbox"
)

// Worker periodically polls the relay inbox for notifier agents with pull delivery enabled.
type Worker struct {
	Agents  *agent.Service
	Deliver notifier.Fanouter
	Relay   *notifier.RelayClient
	Log     *slog.Logger

	mu           sync.Mutex
	lastPoll     map[string]time.Time
	reloadMu     sync.Mutex
	lastReload   time.Time
	reloadPeriod time.Duration
}

// NewWorker wires a pull loop over the agent store and IM fanout.
func NewWorker(agents *agent.Service, d notifier.Fanouter) *Worker {
	return &Worker{
		Agents:       agents,
		Deliver:      d,
		Relay:        &notifier.RelayClient{},
		Log:          slog.Default(),
		lastPoll:     make(map[string]time.Time),
		reloadPeriod: 10 * time.Second,
	}
}

// Run blocks until ctx is cancelled; uses a 1s base ticker so poll_interval below 5s (e.g. 2s) works.
func (w *Worker) Run(ctx context.Context) {
	if w == nil || w.Agents == nil || w.Deliver == nil {
		return
	}
	ticker := time.NewTicker(time.Second)
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

func (w *Worker) maybeReloadFromDisk() {
	period := w.reloadPeriod
	if period <= 0 {
		period = 10 * time.Second
	}
	now := time.Now()
	w.reloadMu.Lock()
	if !w.lastReload.IsZero() && now.Sub(w.lastReload) < period {
		w.reloadMu.Unlock()
		return
	}
	w.lastReload = now
	w.reloadMu.Unlock()
	if err := w.Agents.Reload(); err != nil && w.Log != nil {
		w.Log.Debug("notifier pull: agent reload", "error", err)
	}
}

func agentRuntimeRunning(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), string(sandbox.StateRunning))
}

func (w *Worker) tick(ctx context.Context) {
	w.maybeReloadFromDisk()
	for _, a := range w.Agents.List() {
		if !notifier.IsDeliveryWorker(a.Role, a.RuntimeKind) {
			continue
		}
		cfg := notifier.ConfigFromAgentParts(a.RuntimeExtensions)
		if !cfg.AllowsPull() {
			continue
		}
		if !agentRuntimeRunning(a.Status) {
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
			w.Log.Info("notifier pull failed", "agent_id", a.ID, "error", err)
		}
		w.mu.Lock()
		w.lastPoll[a.ID] = time.Now().UTC()
		w.mu.Unlock()
	}
}

func (w *Worker) pullAgent(ctx context.Context, a agent.Agent, cfg notifier.Config) error {
	msgs, _, err := w.Relay.FetchInbox(ctx, cfg, 50, "")
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
		content := notifier.FormatPayloadAsChatContent(raw, ct)
		if err := w.Deliver.DeliverNotifierFanout(a.ID, content); err != nil {
			return err
		}
		if strings.TrimSpace(m.ID) != "" {
			ackIDs = append(ackIDs, strings.TrimSpace(m.ID))
		}
	}
	if err := w.Relay.Ack(ctx, cfg, ackIDs); err != nil {
		return err
	}
	return nil
}
