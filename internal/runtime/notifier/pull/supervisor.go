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

// Supervisor reconciles per-agent pull loops: each notifier agent with pull delivery enabled
// runs its own polling goroutine (interval from config) while a 1s reconcile tick discovers
// agents added, removed, or reconfigured.
type Supervisor struct {
	Agents  *agent.Service
	Deliver notifier.Fanouter
	Relay   *notifier.RelayClient
	Log     *slog.Logger

	reloadMu     sync.Mutex
	lastReload   time.Time
	reloadPeriod time.Duration

	mu       sync.Mutex
	loopStop map[string]context.CancelFunc
}

// NewSupervisor wires notifier pull delivery over the agent store and IM fanout.
func NewSupervisor(agents *agent.Service, d notifier.Fanouter) *Supervisor {
	return &Supervisor{
		Agents:       agents,
		Deliver:      d,
		Relay:        &notifier.RelayClient{},
		Log:          slog.Default(),
		loopStop:     make(map[string]context.CancelFunc),
		reloadPeriod: 10 * time.Second,
	}
}

// Run blocks until ctx is cancelled. A 1s reconcile ticker matches the previous global worker
// discovery rate; each agent loop sleeps its own poll_interval between fetches.
func (s *Supervisor) Run(ctx context.Context) {
	if s == nil || s.Agents == nil || s.Deliver == nil {
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
	if err := s.Agents.Reload(); err != nil && s.Log != nil {
		s.Log.Debug("notifier pull: agent reload", "error", err)
	}
}

func agentRuntimeRunning(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), string(sandbox.StateRunning))
}

func (s *Supervisor) desiredPullAgentIDs() map[string]struct{} {
	out := make(map[string]struct{})
	for _, a := range s.Agents.List() {
		if !notifier.IsDeliveryWorker(a.Role, a.RuntimeKind) {
			continue
		}
		cfg := notifier.ConfigFromAgentRuntimeOptions(a.RuntimeOptions)
		if !cfg.AllowsPull() {
			continue
		}
		if !agentRuntimeRunning(a.Status) {
			continue
		}
		out[a.ID] = struct{}{}
	}
	return out
}

func (s *Supervisor) syncLoops(parentCtx context.Context) {
	desired := s.desiredPullAgentIDs()

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
		go s.agentPullLoop(st.ctx, st.id)
	}
}

func (s *Supervisor) agentPullLoop(ctx context.Context, agentID string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		a, ok := s.Agents.Agent(agentID)
		if !ok {
			return
		}
		if !notifier.IsDeliveryWorker(a.Role, a.RuntimeKind) {
			return
		}
		cfg := notifier.ConfigFromAgentRuntimeOptions(a.RuntimeOptions)
		if !cfg.AllowsPull() {
			return
		}
		if !agentRuntimeRunning(a.Status) {
			return
		}
		if err := s.pullAgent(ctx, a, cfg); err != nil && s.Log != nil {
			s.Log.Info("notifier pull failed", "agent_id", a.ID, "error", err)
		}
		interval := cfg.PollIntervalDuration()
		if interval <= 0 {
			interval = 30 * time.Second
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

func (s *Supervisor) pullAgent(ctx context.Context, a agent.Agent, cfg notifier.Config) error {
	msgs, _, err := s.Relay.FetchInbox(ctx, cfg, 50, "")
	if err != nil {
		return err
	}
	var ackIDs []string
	for _, m := range msgs {
		raw, ct, err := notifier.DecodePayload(m)
		if err != nil {
			if s.Log != nil {
				s.Log.Warn("notifier inbox decode skipped", "agent_id", a.ID, "msg_id", m.ID, "error", err)
			}
			continue
		}
		content := notifier.FormatPayloadAsChatContent(raw, ct, nil)
		if err := s.Deliver.DeliverNotifierFanout(a.ID, content); err != nil {
			return err
		}
		if strings.TrimSpace(m.ID) != "" {
			ackIDs = append(ackIDs, strings.TrimSpace(m.ID))
		}
	}
	if err := s.Relay.Ack(ctx, cfg, ackIDs); err != nil {
		return err
	}
	return nil
}

// NewWorker is an alias for [NewSupervisor] (legacy name).
func NewWorker(agents *agent.Service, d notifier.Fanouter) *Supervisor {
	return NewSupervisor(agents, d)
}
