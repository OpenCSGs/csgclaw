package worklease

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
)

const (
	DefaultTTLSeconds  = 15
	MinTTLSeconds      = 5
	MaxTTLSeconds      = 60
	TombstoneTTL       = 70 * time.Second
	JanitorInterval    = time.Second
	MaxThinkingBytes   = 16 * 1024
	StatusRateWindow   = time.Second
	MaxStatusPerWindow = 10
)

var (
	ErrParticipantNotFound = errors.New("participant not found")
	ErrRoomNotFound        = errors.New("room not found")
	ErrNotRoomMember       = errors.New("participant is not a room member")
	ErrConflict            = errors.New("work lease metadata conflicts with the active lease")
	ErrClosed              = errors.New("work lease is closed")
	ErrLeaseNotFound       = errors.New("active work lease not found")
	ErrInvalidStatus       = errors.New("invalid work lease status")
	ErrRateLimited         = errors.New("work lease status update rate limit exceeded")
	ErrTurnControlFailed   = errors.New("turn stop delivery failed")
	ErrTurnControlTimedOut = errors.New("turn stop delivery timed out")
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

type ParticipantWorkStatusReporter interface {
	UpdateStatus(ctx context.Context, participantID, leaseID string, request apitypes.ParticipantWorkStatusPatchRequest) (apitypes.ParticipantWorkUpdate, bool, error)
}

type ParticipantWorkFinisher interface {
	Finish(ctx context.Context, participantID, leaseID, outcome string) error
}

type ParticipantWorkController interface {
	ParticipantWorkReporter
	UpdateStatus(ctx context.Context, participantID, leaseID string, request apitypes.ParticipantWorkStatusPatchRequest) (apitypes.ParticipantWorkUpdate, bool, error)
	RequestStop(ctx context.Context, participantID string, request apitypes.ParticipantWorkStopRequest) (apitypes.ParticipantWorkStopResponse, error)
}

type ParticipantDirectory interface {
	Get(channel, id string) (apitypes.Participant, bool)
}

type IMService interface {
	ResolveUserID(userID string) string
	Room(roomID string) (im.Room, bool)
	User(userID string) (im.User, bool)
	DeliverMessage(req im.DeliverMessageRequest) (im.Message, error)
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
	participantID         string
	leaseID               string
	userID                string
	roomID                string
	threadRootID          string
	requestID             string
	kind                  string
	revision              uint64
	expiresAt             time.Time
	capabilities          []string
	status                *apitypes.ParticipantWorkStatus
	statusSequence        uint64
	stopRequestedAt       *time.Time
	stopState             string
	stopError             string
	statusWindowStartedAt time.Time
	statusWindowCount     int
}

type tombstone struct {
	lastRevision uint64
	rejectUntil  time.Time
}

type Registry struct {
	participants ParticipantDirectory
	im           IMService
	bus          *Bus
	controlBus   *ControlBus
	turnControls *TurnControlDispatcher
	epoch        string
	now          func() time.Time

	mu              sync.Mutex
	activeByKey     map[leaseKey]activeLease
	activeBySubject map[string]map[string]map[string]struct{}
	tombstones      map[leaseKey]tombstone
}

func NewRegistry(participants ParticipantDirectory, imService IMService, bus *Bus, opts ...Option) *Registry {
	registry := &Registry{
		participants:    participants,
		im:              imService,
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
	if registry.turnControls == nil && registry.controlBus != nil {
		registry.turnControls = NewTurnControlDispatcher(registry.controlBus)
	}
	return registry
}

func WithControlBus(bus *ControlBus) Option {
	return func(registry *Registry) {
		registry.controlBus = bus
	}
}

func WithTurnControlDispatcher(dispatcher *TurnControlDispatcher) Option {
	return func(registry *Registry) {
		registry.turnControls = dispatcher
	}
}

func (r *Registry) RegisterTurnController(participantID string, controller agentruntime.TurnController) func() {
	if r == nil || r.turnControls == nil {
		return func() {}
	}
	return r.turnControls.RegisterTurnController(r.canonicalParticipantID(participantID), controller)
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

func (r *Registry) UpdateStatus(
	_ context.Context,
	participantID,
	leaseID string,
	request apitypes.ParticipantWorkStatusPatchRequest,
) (apitypes.ParticipantWorkUpdate, bool, error) {
	if r == nil {
		return apitypes.ParticipantWorkUpdate{}, false, ErrUnavailable
	}
	participantID = r.canonicalParticipantID(participantID)
	leaseID = strings.TrimSpace(leaseID)
	if err := validateStatusPatch(request); err != nil {
		return apitypes.ParticipantWorkUpdate{}, false, err
	}
	key := leaseKey{participantID: participantID, leaseID: leaseID}
	now := r.now().UTC()

	r.mu.Lock()
	lease, ok := r.activeByKey[key]
	if !ok {
		_, closed := r.tombstones[key]
		r.mu.Unlock()
		if closed {
			return apitypes.ParticipantWorkUpdate{}, false, ErrClosed
		}
		return apitypes.ParticipantWorkUpdate{}, false, ErrLeaseNotFound
	}
	if lease.stopState == apitypes.ParticipantWorkStopStateRequested {
		update := r.updateFor(lease, apitypes.ParticipantWorkStateWorking, apitypes.ParticipantWorkReasonStopRequested)
		r.mu.Unlock()
		return update, true, nil
	}
	if request.Sequence <= lease.statusSequence {
		r.mu.Unlock()
		return apitypes.ParticipantWorkUpdate{}, false, nil
	}
	if !capabilitiesExtend(lease.capabilities, request.Capabilities) {
		r.mu.Unlock()
		return apitypes.ParticipantWorkUpdate{}, false, ErrConflict
	}
	if lease.statusWindowStartedAt.IsZero() || now.Sub(lease.statusWindowStartedAt) >= StatusRateWindow {
		lease.statusWindowStartedAt = now
		lease.statusWindowCount = 0
	}
	if lease.statusWindowCount >= MaxStatusPerWindow {
		r.mu.Unlock()
		return apitypes.ParticipantWorkUpdate{}, false, ErrRateLimited
	}
	lease.statusWindowCount++
	lease.capabilities = append([]string(nil), request.Capabilities...)
	lease.statusSequence = request.Sequence
	lease.status = &apitypes.ParticipantWorkStatus{
		Sequence: request.Sequence,
		Phase:    request.Phase,
		Stage:    request.Stage,
		Thinking: cloneThinking(request.Thinking),
	}
	lease.revision++
	r.activeByKey[key] = lease
	update := r.updateFor(lease, apitypes.ParticipantWorkStateWorking, apitypes.ParticipantWorkReasonStatusUpdated)
	r.mu.Unlock()

	r.publish(update)
	return update, true, nil
}

func (r *Registry) RequestStop(
	ctx context.Context,
	participantID string,
	request apitypes.ParticipantWorkStopRequest,
) (apitypes.ParticipantWorkStopResponse, error) {
	if r == nil || r.participants == nil || r.im == nil {
		return apitypes.ParticipantWorkStopResponse{}, ErrUnavailable
	}
	normalized, err := r.validate(ParticipantWorkLease{
		ParticipantID: participantID,
		LeaseID:       request.LeaseID,
		RoomID:        request.RoomID,
		RequestID:     request.RequestID,
		Kind:          apitypes.ParticipantWorkKindAgentTurn,
	})
	if err != nil {
		return apitypes.ParticipantWorkStopResponse{}, err
	}
	key := leaseKey{participantID: normalized.participantID, leaseID: strings.TrimSpace(request.LeaseID)}
	now := r.now().UTC()

	var workUpdate *apitypes.ParticipantWorkUpdate
	r.mu.Lock()
	lease, ok := r.activeByKey[key]
	if !ok {
		_, closed := r.tombstones[key]
		r.mu.Unlock()
		if closed {
			return apitypes.ParticipantWorkStopResponse{}, ErrClosed
		}
		return apitypes.ParticipantWorkStopResponse{}, ErrLeaseNotFound
	}
	if lease.roomID != normalized.roomID || lease.requestID != normalized.requestID {
		r.mu.Unlock()
		return apitypes.ParticipantWorkStopResponse{}, ErrConflict
	}
	if !slices.Contains(lease.capabilities, apitypes.ParticipantWorkCapabilityTurnStopV1) {
		r.mu.Unlock()
		return apitypes.ParticipantWorkStopResponse{}, ErrConflict
	}
	if lease.stopRequestedAt == nil ||
		lease.stopState == apitypes.ParticipantWorkStopStateFailed ||
		lease.stopState == apitypes.ParticipantWorkStopStateTimedOut {
		requestedAt := now
		lease.stopRequestedAt = &requestedAt
		lease.stopState = apitypes.ParticipantWorkStopStateRequested
		lease.stopError = ""
		lease.revision++
		r.activeByKey[key] = lease
		update := r.updateFor(lease, apitypes.ParticipantWorkStateWorking, apitypes.ParticipantWorkReasonStopRequested)
		workUpdate = &update
	}
	requestedAt := *lease.stopRequestedAt
	response := apitypes.ParticipantWorkStopResponse{
		Accepted:      true,
		RegistryEpoch: r.epoch,
		ParticipantID: lease.participantID,
		UserID:        lease.userID,
		RoomID:        lease.roomID,
		ThreadRootID:  lease.threadRootID,
		LeaseID:       lease.leaseID,
		RequestID:     lease.requestID,
		State:         "stop_requested",
		RequestedAt:   requestedAt,
	}
	ref := agentruntime.TurnRef{
		RegistryEpoch: r.epoch,
		ParticipantID: lease.participantID,
		RoomID:        lease.roomID,
		LeaseID:       lease.leaseID,
		RequestID:     lease.requestID,
		RequestedAt:   requestedAt,
	}
	r.mu.Unlock()

	if workUpdate != nil {
		r.publish(*workUpdate)
	}
	if r.turnControls == nil {
		return response, r.recordTurnControlFailure(key, requestedAt, agentruntime.ErrTurnControlUnavailable)
	}
	if err := r.turnControls.StopTurn(ctx, ref); err != nil {
		return response, r.recordTurnControlFailure(key, requestedAt, err)
	}
	return response, nil
}

func (r *Registry) Stop(ctx context.Context, participantID, leaseID string) error {
	return r.Finish(ctx, participantID, leaseID, apitypes.ParticipantWorkOutcomeReleased)
}

func (r *Registry) Finish(_ context.Context, participantID, leaseID, outcome string) error {
	if r == nil {
		return ErrUnavailable
	}
	outcome = strings.TrimSpace(outcome)
	if outcome == "" {
		outcome = apitypes.ParticipantWorkOutcomeReleased
	}
	if outcome != apitypes.ParticipantWorkOutcomeReleased &&
		outcome != apitypes.ParticipantWorkOutcomeStopped &&
		outcome != apitypes.ParticipantWorkOutcomeStopTimedOut {
		return ErrInvalidStatus
	}
	participantID = r.canonicalParticipantID(participantID)
	leaseID = strings.TrimSpace(leaseID)
	key := leaseKey{participantID: participantID, leaseID: leaseID}
	now := r.now().UTC()

	var update *apitypes.ParticipantWorkUpdate
	r.mu.Lock()
	if lease, ok := r.activeByKey[key]; ok {
		if outcome != apitypes.ParticipantWorkOutcomeReleased && lease.stopRequestedAt == nil {
			r.mu.Unlock()
			return ErrConflict
		}
		delete(r.activeByKey, key)
		r.removeSubjectLocked(lease.roomID, lease.participantID, key.leaseID)
		lease.revision++
		r.tombstones[key] = tombstone{lastRevision: lease.revision, rejectUntil: now.Add(TombstoneTTL)}
		reason := apitypes.ParticipantWorkReasonReleased
		switch outcome {
		case apitypes.ParticipantWorkOutcomeStopped:
			reason = apitypes.ParticipantWorkReasonStopped
			lease.stopState = apitypes.ParticipantWorkStopStateStopped
			lease.stopError = ""
		case apitypes.ParticipantWorkOutcomeStopTimedOut:
			reason = apitypes.ParticipantWorkReasonStopTimedOut
			lease.stopState = apitypes.ParticipantWorkStopStateTimedOut
			lease.stopError = "runtime did not confirm turn stop before the control timeout"
		default:
			lease.stopState = ""
			lease.stopError = ""
		}
		event := r.updateFor(lease, apitypes.ParticipantWorkStateIdle, reason)
		update = &event
	} else {
		closed := r.tombstones[key]
		closed.rejectUntil = maxTime(closed.rejectUntil, now.Add(TombstoneTTL))
		r.tombstones[key] = closed
	}
	r.mu.Unlock()

	if update != nil {
		r.publish(*update)
		if outcome == apitypes.ParticipantWorkOutcomeStopped {
			r.recordParticipantTurnStopped(*update)
		}
	}
	return nil
}

func (r *Registry) recordTurnControlFailure(key leaseKey, requestedAt time.Time, cause error) error {
	state := apitypes.ParticipantWorkStopStateFailed
	reason := apitypes.ParticipantWorkReasonStopFailed
	publicErr := ErrTurnControlFailed
	if errors.Is(cause, context.DeadlineExceeded) || errors.Is(cause, context.Canceled) {
		state = apitypes.ParticipantWorkStopStateTimedOut
		reason = apitypes.ParticipantWorkReasonStopTimedOut
		publicErr = ErrTurnControlTimedOut
	}
	errorText := strings.TrimSpace(cause.Error())
	if len(errorText) > 1024 {
		errorText = errorText[:1024]
	}

	var update *apitypes.ParticipantWorkUpdate
	r.mu.Lock()
	if lease, ok := r.activeByKey[key]; ok && lease.stopRequestedAt != nil && lease.stopRequestedAt.Equal(requestedAt) {
		lease.stopState = state
		lease.stopError = errorText
		lease.revision++
		r.activeByKey[key] = lease
		event := r.updateFor(lease, apitypes.ParticipantWorkStateWorking, reason)
		update = &event
	}
	r.mu.Unlock()
	if update != nil {
		r.publish(*update)
	}
	return fmt.Errorf("%w: %v", publicErr, cause)
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
	update := apitypes.ParticipantWorkUpdate{
		RegistryEpoch:   r.epoch,
		LeaseID:         lease.leaseID,
		ParticipantID:   lease.participantID,
		UserID:          lease.userID,
		RoomID:          lease.roomID,
		ThreadRootID:    lease.threadRootID,
		RequestID:       lease.requestID,
		Kind:            lease.kind,
		State:           state,
		Reason:          reason,
		Revision:        lease.revision,
		ExpiresAt:       lease.expiresAt,
		Capabilities:    append([]string(nil), lease.capabilities...),
		StopRequestedAt: cloneTime(lease.stopRequestedAt),
		StopState:       lease.stopState,
		StopError:       lease.stopError,
	}
	if lease.status != nil {
		update.Status = &apitypes.ParticipantWorkStatus{
			Sequence: lease.status.Sequence,
			Phase:    lease.status.Phase,
			Stage:    lease.status.Stage,
			Thinking: cloneThinking(lease.status.Thinking),
		}
	}
	return update
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
func validateStatusPatch(request apitypes.ParticipantWorkStatusPatchRequest) error {
	if request.Sequence == 0 {
		return ErrInvalidStatus
	}
	seen := make(map[string]struct{}, len(request.Capabilities))
	for _, capability := range request.Capabilities {
		if capability != apitypes.ParticipantWorkCapabilityThinkingStatusV1 &&
			capability != apitypes.ParticipantWorkCapabilityTurnStopV1 &&
			capability != apitypes.ParticipantWorkCapabilityStageV1 {
			return ErrInvalidStatus
		}
		if _, duplicate := seen[capability]; duplicate {
			return ErrInvalidStatus
		}
		seen[capability] = struct{}{}
	}
	if request.Phase != apitypes.ParticipantWorkPhaseWorking &&
		request.Phase != apitypes.ParticipantWorkPhaseThinking {
		return ErrInvalidStatus
	}
	if request.Stage != "" {
		if !slices.Contains(request.Capabilities, apitypes.ParticipantWorkCapabilityStageV1) ||
			!validStageForPhase(request.Stage, request.Phase) {
			return ErrInvalidStatus
		}
	}
	if request.Phase == apitypes.ParticipantWorkPhaseWorking {
		if request.Thinking != nil {
			return ErrInvalidStatus
		}
		return nil
	}
	if !slices.Contains(request.Capabilities, apitypes.ParticipantWorkCapabilityThinkingStatusV1) {
		return ErrInvalidStatus
	}
	if request.Stage == apitypes.ParticipantWorkStageThinking &&
		(request.Thinking == nil || strings.TrimSpace(request.Thinking.Text) == "") {
		return ErrInvalidStatus
	}
	if request.Thinking == nil {
		return nil
	}
	if request.Stage != "" && request.Stage != apitypes.ParticipantWorkStageThinking {
		return ErrInvalidStatus
	}
	if request.Thinking.Format != apitypes.ParticipantThinkingFormatPlainText ||
		!utf8.ValidString(request.Thinking.Text) ||
		len([]byte(request.Thinking.Text)) > MaxThinkingBytes {
		return ErrInvalidStatus
	}
	return nil
}

func validStageForPhase(stage, phase string) bool {
	switch stage {
	case apitypes.ParticipantWorkStagePreparingReply,
		apitypes.ParticipantWorkStageThinking:
		return phase == apitypes.ParticipantWorkPhaseThinking
	case apitypes.ParticipantWorkStageRunningTool,
		apitypes.ParticipantWorkStageGeneratingReply:
		return phase == apitypes.ParticipantWorkPhaseWorking
	default:
		return false
	}
}

func capabilitiesExtend(current, next []string) bool {
	for _, capability := range current {
		if !slices.Contains(next, capability) {
			return false
		}
	}
	return true
}

func cloneThinking(source *apitypes.ParticipantThinkingStatus) *apitypes.ParticipantThinkingStatus {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}

func cloneTime(source *time.Time) *time.Time {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}
