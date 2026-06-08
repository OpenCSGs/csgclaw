package pull

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/channel/csgclaw/notification"
)

const csgclawChannel = "csgclaw"

// NotificationParticipant is the minimal participant view needed for pull delivery.
type NotificationParticipant struct {
	ID     string
	UserID string
}

// ParticipantLister lists notification participants eligible for pull delivery.
type ParticipantLister interface {
	Reload() error
	ListNotificationParticipants(channel string) ([]NotificationParticipant, error)
	// LookupNotificationParticipantForDelivery returns stored metadata with secrets, not API-redacted view.
	LookupNotificationParticipantForDelivery(channel, id string) (metadata map[string]any, userID string, ok bool)
}

// Supervisor reconciles per-participant pull loops for notification participants with pull delivery enabled.
type Supervisor struct {
	Participants ParticipantLister
	Deliver      notification.Fanouter
	Relay        *notification.RelayClient
	Log          *slog.Logger

	reloadMu     sync.Mutex
	lastReload   time.Time
	reloadPeriod time.Duration

	mu       sync.Mutex
	loopStop map[string]context.CancelFunc
}

// NewSupervisor wires notification pull delivery over participant metadata and IM fanout.
func NewSupervisor(participants ParticipantLister, d notification.Fanouter) *Supervisor {
	return &Supervisor{
		Participants: participants,
		Deliver:      d,
		Relay:        &notification.RelayClient{},
		Log:          slog.Default(),
		loopStop:     make(map[string]context.CancelFunc),
		reloadPeriod: 10 * time.Second,
	}
}

// Run blocks until ctx is cancelled.
func (s *Supervisor) Run(ctx context.Context) {
	if s == nil || s.Participants == nil || s.Deliver == nil {
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
	if err := s.Participants.Reload(); err != nil && s.Log != nil {
		s.Log.Debug("notification pull: participant reload", "error", err)
	}
}

func (s *Supervisor) desiredPullParticipantIDs() map[string]struct{} {
	out := make(map[string]struct{})
	channel := csgclawChannel
	items, err := s.Participants.ListNotificationParticipants(channel)
	if err != nil {
		if s.Log != nil {
			s.Log.Warn("notification pull: list participants", "error", err)
		}
		return out
	}
	for _, item := range items {
		// ListNotificationParticipants returns a public list view. Pull eligibility
		// must use stored metadata via LookupNotificationParticipantForDelivery.
		// Pull eligibility must use stored secrets via LookupNotificationParticipantForDelivery.
		flat, _, ok := s.Participants.LookupNotificationParticipantForDelivery(channel, item.ID)
		if !ok {
			continue
		}
		cfg := notification.ConfigFromMetadata(flat)
		if !cfg.PullDeliveryComplete() {
			if s.Log != nil && cfg.AllowsPull() {
				s.Log.Debug("notification pull: participant not ready", "participant_id", item.ID, "has_token", strings.TrimSpace(cfg.RemoteToken) != "")
			}
			continue
		}
		out[item.ID] = struct{}{}
	}
	return out
}

func (s *Supervisor) syncLoops(parentCtx context.Context) {
	desired := s.desiredPullParticipantIDs()

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
			s.Log.Info("notification pull loop started", "participant_id", st.id)
		}
		go s.participantPullLoop(st.ctx, st.id)
	}
}

func (s *Supervisor) lookupParticipant(participantID string) (NotificationParticipant, notification.Config, bool) {
	id := strings.TrimSpace(participantID)
	if id == "" {
		return NotificationParticipant{}, notification.Config{}, false
	}
	flat, userID, ok := s.Participants.LookupNotificationParticipantForDelivery(csgclawChannel, id)
	if !ok || len(flat) == 0 {
		return NotificationParticipant{}, notification.Config{}, false
	}
	cfg := notification.ConfigFromMetadata(flat)
	item := NotificationParticipant{ID: id, UserID: userID}
	return item, cfg, true
}

func (s *Supervisor) participantPullLoop(ctx context.Context, participantID string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		item, cfg, ok := s.lookupParticipant(participantID)
		if !ok {
			return
		}
		if !cfg.PullDeliveryComplete() {
			return
		}
		if err := s.pullParticipant(ctx, item, cfg); err != nil {
			if s.Log != nil {
				s.Log.Info("notification pull failed", "participant_id", item.ID, "error", err)
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

func (s *Supervisor) pullParticipant(ctx context.Context, item NotificationParticipant, cfg notification.Config) error {
	msgs, _, err := s.Relay.FetchInbox(ctx, cfg, 50, "")
	if err != nil {
		return err
	}
	memberID := strings.TrimSpace(item.UserID)
	if memberID == "" {
		memberID = strings.TrimSpace(item.ID)
	}
	var ackIDs []string
	var deliverErr error
	delivered := 0
	for _, m := range msgs {
		raw, ct, err := notification.DecodePayload(m)
		if err != nil {
			if s.Log != nil {
				s.Log.Warn("notification inbox decode skipped", "participant_id", item.ID, "msg_id", m.ID, "error", err)
			}
			continue
		}
		content := notification.FormatPayloadAsChatContent(raw, ct, nil)
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
			s.Log.Debug("notification pull ok", "participant_id", item.ID, "messages", 0)
		case delivered > 0:
			s.Log.Info("notification pull delivered", "participant_id", item.ID, "messages", delivered, "acked", len(ackIDs))
		case len(msgs) > 0 && delivered == 0:
			s.Log.Warn("notification pull: inbox had messages but none delivered", "participant_id", item.ID, "inbox", len(msgs))
		}
	}
	return deliverErr
}
