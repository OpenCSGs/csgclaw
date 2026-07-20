package worklease

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

const (
	DefaultTTLSeconds = 15
	MinTTLSeconds     = 5
	MaxTTLSeconds     = 60
	TombstoneTTL      = 70 * time.Second
	JanitorInterval   = time.Second
)

var (
	ErrParticipantNotFound = errors.New("participant not found")
	ErrRoomNotFound        = errors.New("room not found")
	ErrNotRoomMember       = errors.New("participant is not a room member")
	ErrConflict            = errors.New("work lease metadata conflicts with the active lease")
	ErrClosed              = errors.New("work lease is closed")
	ErrUnavailable         = errors.New("work lease service is not configured")
)

type ParticipantWorkLease struct {
	ParticipantID string
	LeaseID       string
	RoomID        string
	ThreadRootID  string
	RequestID     string
	Kind          string
	TTLSeconds    int
	TTLExplicit   bool
}

type ParticipantWorkReporter interface {
	StartOrRenew(ctx context.Context, lease ParticipantWorkLease) (apitypes.ParticipantWorkUpdate, error)
	Stop(ctx context.Context, participantID, leaseID string) error
}

type ParticipantDirectory interface {
	Get(channel, id string) (apitypes.Participant, bool)
}

type IMDirectory interface {
	ResolveUserID(userID string) string
	Room(roomID string) (im.Room, bool)
	User(userID string) (im.User, bool)
}

type Option func(*Registry)

func WithClock(now func() time.Time) Option {
	return func(registry *Registry) {
		if now != nil {
			registry.now = now
		}
	}
}

func WithEpoch(epoch string) Option {
	return func(registry *Registry) {
		if epoch = strings.TrimSpace(epoch); epoch != "" {
			registry.epoch = epoch
		}
	}
}

type leaseKey struct {
	participantID string
	leaseID       string
}

type activeLease struct {
	participantID string
	leaseID       string
	userID        string
	roomID        string
	threadRootID  string
	requestID     string
	kind          string
	revision      uint64
	expiresAt     time.Time
}

type tombstone struct {
	lastRevision uint64
	rejectUntil  time.Time
}

type Registry struct {
	participants ParticipantDirectory
	im           IMDirectory
	bus          *Bus
	epoch        string
	now          func() time.Time

	mu              sync.Mutex
	activeByKey     map[leaseKey]activeLease
	activeBySubject map[string]map[string]map[string]struct{}
	tombstones      map[leaseKey]tombstone
}

func NewRegistry(participants ParticipantDirectory, imDirectory IMDirectory, bus *Bus, opts ...Option) *Registry {
	registry := &Registry{
		participants:    participants,
		im:              imDirectory,
		bus:             bus,
		epoch:           NewID(),
		now:             time.Now,
		activeByKey:     make(map[leaseKey]activeLease),
		activeBySubject: make(map[string]map[string]map[string]struct{}),
		tombstones:      make(map[leaseKey]tombstone),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(registry)
		}
	}
	return registry
}

func (r *Registry) Epoch() string {
	if r == nil {
		return ""
	}
	return r.epoch
}

func (r *Registry) StartOrRenew(_ context.Context, request ParticipantWorkLease) (apitypes.ParticipantWorkUpdate, error) {
	if r == nil || r.participants == nil || r.im == nil {
		return apitypes.ParticipantWorkUpdate{}, ErrUnavailable
	}
	normalized, err := r.validate(request)
	if err != nil {
		return apitypes.ParticipantWorkUpdate{}, err
	}
	now := r.now().UTC()
	key := leaseKey{participantID: normalized.participantID, leaseID: strings.TrimSpace(request.LeaseID)}

	r.mu.Lock()
	if _, closed := r.tombstones[key]; closed {
		r.mu.Unlock()
		return apitypes.ParticipantWorkUpdate{}, ErrClosed
	}

	reason := apitypes.ParticipantWorkReasonStarted
	lease, exists := r.activeByKey[key]
	if exists {
		if !sameMetadata(lease, normalized) {
			r.mu.Unlock()
			return apitypes.ParticipantWorkUpdate{}, ErrConflict
		}
		lease.revision++
		reason = apitypes.ParticipantWorkReasonRenewed
	} else {
		lease = normalized
		lease.revision = 1
		r.addSubjectLocked(lease.roomID, lease.participantID, key.leaseID)
	}
	lease.expiresAt = now.Add(time.Duration(clampTTL(request.TTLSeconds, request.TTLExplicit)) * time.Second)
	r.activeByKey[key] = lease
	update := r.updateFor(lease, apitypes.ParticipantWorkStateWorking, reason)
	r.mu.Unlock()

	r.publish(update)
	return update, nil
}

func (r *Registry) Stop(_ context.Context, participantID, leaseID string) error {
	if r == nil {
		return ErrUnavailable
	}
	participantID = r.canonicalParticipantID(participantID)
	leaseID = strings.TrimSpace(leaseID)
	key := leaseKey{participantID: participantID, leaseID: leaseID}
	now := r.now().UTC()

	var update *apitypes.ParticipantWorkUpdate
	r.mu.Lock()
	if lease, ok := r.activeByKey[key]; ok {
		delete(r.activeByKey, key)
		r.removeSubjectLocked(lease.roomID, lease.participantID, key.leaseID)
		lease.revision++
		r.tombstones[key] = tombstone{lastRevision: lease.revision, rejectUntil: now.Add(TombstoneTTL)}
		event := r.updateFor(lease, apitypes.ParticipantWorkStateIdle, apitypes.ParticipantWorkReasonReleased)
		update = &event
	} else {
		closed := r.tombstones[key]
		closed.rejectUntil = maxTime(closed.rejectUntil, now.Add(TombstoneTTL))
		r.tombstones[key] = closed
	}
	r.mu.Unlock()

	if update != nil {
		r.publish(*update)
	}
	return nil
}

func (r *Registry) Sweep(now time.Time) {
	if r == nil {
		return
	}
	now = now.UTC()
	events := make([]apitypes.ParticipantWorkUpdate, 0)
	r.mu.Lock()
	for key, lease := range r.activeByKey {
		if lease.expiresAt.After(now) {
			continue
		}
		delete(r.activeByKey, key)
		r.removeSubjectLocked(lease.roomID, lease.participantID, key.leaseID)
		lease.revision++
		r.tombstones[key] = tombstone{lastRevision: lease.revision, rejectUntil: now.Add(TombstoneTTL)}
		events = append(events, r.updateFor(lease, apitypes.ParticipantWorkStateIdle, apitypes.ParticipantWorkReasonExpired))
	}
	for key, closed := range r.tombstones {
		if !closed.rejectUntil.After(now) {
			delete(r.tombstones, key)
		}
	}
	r.mu.Unlock()

	for _, event := range events {
		r.publish(event)
	}
}

func (r *Registry) Run(ctx context.Context) {
	if r == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(JanitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			r.Sweep(now)
		}
	}
}

func (r *Registry) ActiveCount(roomID, participantID string) int {
	if r == nil {
		return 0
	}
	participantID = r.canonicalParticipantID(participantID)
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.activeBySubject[strings.TrimSpace(roomID)][participantID])
}

func (r *Registry) validate(request ParticipantWorkLease) (activeLease, error) {
	item, ok := r.participants.Get(participant.ChannelCSGClaw, strings.TrimSpace(request.ParticipantID))
	if !ok || item.Channel != participant.ChannelCSGClaw || item.Type != participant.TypeAgent || item.LifecycleStatus != participant.LifecycleStatusActive {
		return activeLease{}, ErrParticipantNotFound
	}
	participantID := participant.CanonicalID(item.ID)
	userID := r.im.ResolveUserID(strings.TrimSpace(item.ChannelUserRef))
	if user, found := r.im.User(userID); found {
		userID = r.im.ResolveUserID(user.ID)
	} else {
		return activeLease{}, ErrParticipantNotFound
	}
	roomID := strings.TrimSpace(request.RoomID)
	room, ok := r.im.Room(roomID)
	if !ok {
		return activeLease{}, ErrRoomNotFound
	}
	isMember := slices.ContainsFunc(room.Members, func(memberID string) bool {
		return r.im.ResolveUserID(memberID) == userID
	})
	if !isMember {
		return activeLease{}, ErrNotRoomMember
	}
	return activeLease{
		participantID: participantID,
		leaseID:       strings.TrimSpace(request.LeaseID),
		userID:        userID,
		roomID:        roomID,
		threadRootID:  strings.TrimSpace(request.ThreadRootID),
		requestID:     strings.TrimSpace(request.RequestID),
		kind:          strings.TrimSpace(request.Kind),
	}, nil
}

func (r *Registry) canonicalParticipantID(id string) string {
	id = strings.TrimSpace(id)
	if r != nil && r.participants != nil {
		if item, ok := r.participants.Get(participant.ChannelCSGClaw, id); ok {
			return participant.CanonicalID(item.ID)
		}
	}
	return participant.CanonicalID(id)
}

func (r *Registry) addSubjectLocked(roomID, participantID, leaseID string) {
	byParticipant := r.activeBySubject[roomID]
	if byParticipant == nil {
		byParticipant = make(map[string]map[string]struct{})
		r.activeBySubject[roomID] = byParticipant
	}
	leases := byParticipant[participantID]
	if leases == nil {
		leases = make(map[string]struct{})
		byParticipant[participantID] = leases
	}
	leases[leaseID] = struct{}{}
}

func (r *Registry) removeSubjectLocked(roomID, participantID, leaseID string) {
	byParticipant := r.activeBySubject[roomID]
	leases := byParticipant[participantID]
	delete(leases, leaseID)
	if len(leases) == 0 {
		delete(byParticipant, participantID)
	}
	if len(byParticipant) == 0 {
		delete(r.activeBySubject, roomID)
	}
}

func (r *Registry) updateFor(lease activeLease, state, reason string) apitypes.ParticipantWorkUpdate {
	return apitypes.ParticipantWorkUpdate{
		RegistryEpoch: r.epoch,
		LeaseID:       lease.leaseID,
		ParticipantID: lease.participantID,
		UserID:        lease.userID,
		RoomID:        lease.roomID,
		ThreadRootID:  lease.threadRootID,
		RequestID:     lease.requestID,
		Kind:          lease.kind,
		State:         state,
		Reason:        reason,
		Revision:      lease.revision,
		ExpiresAt:     lease.expiresAt,
	}
}

func (r *Registry) publish(update apitypes.ParticipantWorkUpdate) {
	if r == nil || r.bus == nil {
		return
	}
	r.bus.Publish(Event{Type: EventTypeParticipantWorkUpdated, RoomID: update.RoomID, Work: update})
}

func sameMetadata(left, right activeLease) bool {
	return left.participantID == right.participantID &&
		left.userID == right.userID &&
		left.roomID == right.roomID &&
		left.threadRootID == right.threadRootID &&
		left.requestID == right.requestID &&
		left.kind == right.kind
}

func clampTTL(value int, explicit bool) int {
	if !explicit {
		return DefaultTTLSeconds
	}
	if value < MinTTLSeconds {
		return MinTTLSeconds
	}
	if value > MaxTTLSeconds {
		return MaxTTLSeconds
	}
	return value
}

func maxTime(left, right time.Time) time.Time {
	if left.After(right) {
		return left
	}
	return right
}
