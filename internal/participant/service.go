package participant

import (
	"context"
	"crypto/rand"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentengine/lifecycle"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/identity"
	"csgclaw/internal/im"
	"csgclaw/internal/modelprovider"
	hub "csgclaw/internal/template"
	"csgclaw/internal/utils"
	"encoding/base32"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Service struct {
	mutations sync.Map // participant key -> completion channel
	lifecycle *lifecycle.Coordinator
	store     *Store
	agents    agentengine.AgentInterface
	im        *im.Service
}

type Option func(*Service)

func NewService(store *Store, opts ...Option) *Service {
	if store == nil {
		store = NewMemoryStore(nil)
	}
	s := &Service{store: store, lifecycle: &lifecycle.Coordinator{}}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func WithAgentEngine(engine agentengine.Interface) Option {
	return func(s *Service) {
		if engine != nil {
			s.agents = engine.Agents()
			if owner, ok := s.agents.(interface{ LifecycleCoordinator() *lifecycle.Coordinator }); ok {
				s.lifecycle = owner.LifecycleCoordinator()
			}
		}
	}
}

func WithIMService(imSvc *im.Service) Option {
	return func(s *Service) {
		s.im = imSvc
	}
}

func (s *Service) List(opts ListOptions) []apitypes.Participant {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.List(opts)
}

func (s *Service) Get(channel, id string) (apitypes.Participant, bool) {
	if s == nil || s.store == nil {
		return apitypes.Participant{}, false
	}
	item, _, ok := s.getByID(channel, id)
	return item, ok
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (apitypes.Participant, error) {
	item, _, err := s.CreateWithStatus(ctx, req)
	return item, err
}

// CreateWithStatus reports whether the request inserted a new participant.
func (s *Service) CreateWithStatus(ctx context.Context, req CreateRequest) (apitypes.Participant, bool, error) {
	if s == nil || s.store == nil {
		return apitypes.Participant{}, false, fmt.Errorf("participant store is required")
	}
	normalized, err := s.normalizeCreateRequest(req)
	if err != nil {
		return apitypes.Participant{}, false, err
	}
	normalized.reconcile = req.ReconcilesBinding()
	ctx, release, err := s.lockCreate(ctx, &normalized)
	if err != nil {
		return apitypes.Participant{}, false, err
	}
	defer release()
	if existing, ok := s.store.Get(normalized.Channel, normalized.ID); ok {
		if !normalized.reconcile || existing.Type != TypeAgent ||
			(req.ChannelUser.Ref != "" && normalized.ChannelUser.Ref != canonicalUserID(existing.ChannelUserRef)) ||
			(req.ChannelUser.Kind != "" && normalized.ChannelUser.Kind != existing.ChannelUserKind) {
			return apitypes.Participant{}, false, fmt.Errorf("participant %s:%s already exists", normalized.Channel, normalized.ID)
		}
		item, err := s.reconcileBinding(ctx, existing, normalized)
		return item, false, err
	}
	item, err := s.create(ctx, normalized)
	return item, err == nil, err
}

func (s *Service) reconcileBinding(ctx context.Context, existing apitypes.Participant, req normalizedCreateRequest) (_ apitypes.Participant, err error) {
	item := existing
	if req.reconcileBoundName {
		item.Name = req.Name
	}
	if s.im != nil && existing.ChannelUserKind == ChannelUserKindLocalUserID {
		user, ok := s.im.LocalUserByID(existing.ChannelUserRef)
		if !ok {
			return existing, im.ErrUserNotFound
		}
		if user.AgentState == "deleted" {
			return existing, im.ErrAgentIdentityDeleted
		}
		if req.reconcileBoundName {
			if owner, ok := s.im.User(item.Name); ok && owner.ID != user.ID {
				return existing, fmt.Errorf("name %q %w", item.Name, identity.ErrNameExists)
			}
		}
		if user.Name != item.Name {
			// Claim the IM name before updating the worker. A competing user
			// can acquire it between a read-only availability check and a write.
			if err := s.syncParticipantChannelUser(item); err != nil {
				return existing, err
			}
			defer func() {
				if err != nil {
					previous := existing
					previous.Name = user.Name
					err = errors.Join(err, s.syncParticipantChannelUser(previous))
				}
			}()
		}
	}
	if item.Name != existing.Name {
		item.UpdatedAt = time.Now().UTC()
		if err := s.store.Save(item); err != nil {
			return existing, err
		}
		defer func() {
			if err != nil {
				err = errors.Join(err, s.store.Save(existing))
			}
		}()
	}
	agentID, err := s.ensureAgentBinding(ctx, req, true)
	if err != nil {
		return existing, err
	}
	if item.AgentID == "" {
		item.AgentID = agentID
		item.UpdatedAt = time.Now().UTC()
		if err := s.store.Save(item); err != nil {
			return existing, err
		}
	}
	return item, nil
}

// Agent lifecycle mutations precede participant writes, including Feishu calls
// that already hold the same lease. Recheck the binding after both locks: a
// concurrent first create may have selected a different worker for this ID.
func (s *Service) lockCreate(ctx context.Context, req *normalizedCreateRequest) (context.Context, func(), error) {
	if req.Type != TypeAgent || req.AgentBinding.Mode == BindingModeNone {
		release, err := s.lockParticipant(ctx, req.Channel, req.ID)
		return ctx, release, err
	}
	requested := req.AgentBinding.AgentID
	if requested == "" {
		requested = defaultAgentID(req.ID)
	}
	// An explicit binding already identifies the worker. Pin it before reading
	// names so a concurrent participant PATCH is serialized with reconciliation.
	target := bindingTarget{id: requested, readOnly: true}
	var err error
	if req.reconcile {
		if bound, ok := s.store.Get(req.Channel, req.ID); ok && bound.AgentID != "" {
			target.id = bound.AgentID
		}
	} else {
		target, err = s.bindingTarget(ctx, req, requested)
		if err != nil {
			return ctx, nil, err
		}
	}
	for {
		lease := ctx
		var releaseAgent func()
		if target.readOnly {
			releaseAgent, err = s.lifecycle.Read(ctx, agent.CanonicalID(target.id))
		} else {
			lease, releaseAgent, err = s.lifecycle.Mutation(ctx, agent.CanonicalID(target.id))
		}
		if err != nil {
			return ctx, nil, err
		}
		releaseParticipant, err := s.lockParticipant(lease, req.Channel, req.ID)
		if err != nil {
			releaseAgent()
			return ctx, nil, err
		}
		release := func() {
			releaseParticipant()
			releaseAgent()
		}
		current, err := s.bindingTarget(lease, req, requested)
		if err != nil {
			release()
			return ctx, nil, err
		}
		if current.id != target.id || (target.readOnly && !current.readOnly) {
			release()
			target = current
			continue
		}
		if req.AgentBinding.Mode == BindingModeCreate {
			req.AgentBinding.AgentID = target.id
			req.reconcileBoundName = current.reconcileBoundName
		}
		return lease, release, nil
	}
}

type bindingTarget struct {
	id                 string
	readOnly           bool
	reconcileBoundName bool
}

func (s *Service) bindingTarget(ctx context.Context, req *normalizedCreateRequest, requested string) (bindingTarget, error) {
	var bound apitypes.Participant
	if req.reconcile {
		bound, _ = s.store.Get(req.Channel, req.ID)
		if bound.AgentID != "" {
			requested = bound.AgentID
		}
	}
	target := bindingTarget{id: requested, readOnly: req.AgentBinding.Mode == BindingModeReuse, reconcileBoundName: bound.AgentID != ""}
	// Historical selectors also match worker names. Pin the selected worker
	// until the participant is saved, including when deletion races the lookup.
	if s.agents != nil {
		selected, err := s.agents.Get(ctx, requested, agentengine.AgentGetOptions{})
		if err == nil {
			if !req.reconcile {
				return bindingTarget{id: selected.ID, readOnly: true}, nil
			}
			if selected.ID == agent.CanonicalID(requested) {
				nameMatches := selected.Spec.Name == req.Name
				// Replaying the creation of an alias must keep the shared worker's name.
				if target.reconcileBoundName && !nameMatches && bound.Name == req.Name && s.isSharedWorkerAlias(bound, selected.Spec.Name) {
					target.reconcileBoundName = false
				}
				target.readOnly = nameMatches || !target.reconcileBoundName
			}
			return target, nil
		}
		if agentengine.ErrorCodeOf(err) != agentengine.ErrorAgentNotFound {
			return bindingTarget{}, err
		}
	}
	return target, nil
}

func (s *Service) isSharedWorkerAlias(bound apitypes.Participant, workerName string) bool {
	return slices.ContainsFunc(s.store.List(ListOptions{Channel: bound.Channel, AgentID: bound.AgentID}), func(p apitypes.Participant) bool {
		return p.ID != bound.ID && strings.EqualFold(p.Name, workerName)
	})
}

// Serialize changes to the participant and its associated IM user.
func (s *Service) lockParticipant(ctx context.Context, channel, id string) (func(), error) {
	key := storeKey(channel, canonicalParticipantID(slugify(id)))
	done := make(chan struct{})
	for {
		active, occupied := s.mutations.LoadOrStore(key, done)
		if !occupied {
			return func() {
				s.mutations.Delete(key)
				close(done)
			}, nil
		}
		select {
		case <-active.(chan struct{}):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (s *Service) create(ctx context.Context, normalized normalizedCreateRequest) (apitypes.Participant, error) {
	if normalized.Channel == ChannelCSGClaw && s.im != nil {
		if err := s.im.ValidateAgentUser(im.EnsureAgentUserRequest{ID: normalized.ChannelUser.Ref, Name: normalized.Name}); err != nil {
			return apitypes.Participant{}, err
		}
	}
	if normalized.Type == TypeAgent && normalized.AgentBinding.Mode == BindingModeCreate && s.im != nil {
		agentID := normalized.AgentBinding.AgentID
		if agentID == "" {
			agentID = defaultAgentID(normalized.ID)
		}
		if err := s.im.ValidateAgentUser(im.EnsureAgentUserRequest{ID: agentID}); err != nil {
			return apitypes.Participant{}, err
		}
	}

	if normalized.Type == TypeAgent {
		agentID, err := s.ensureAgentBinding(ctx, normalized, false)
		if err != nil {
			return apitypes.Participant{}, err
		}
		normalized.AgentID = agentID
	}

	if err := s.ensureChannelIdentity(ctx, normalized); err != nil {
		return apitypes.Participant{}, err
	}

	now := time.Now().UTC()
	created := apitypes.Participant{
		ID:               normalized.ID,
		Channel:          normalized.Channel,
		Type:             normalized.Type,
		Name:             normalized.Name,
		ChannelUserRef:   normalized.ChannelUser.Ref,
		ChannelUserKind:  normalized.ChannelUser.Kind,
		ChannelAppRef:    normalized.ChannelAppRef,
		ChannelAppConfig: cloneMap(normalized.ChannelAppConfig),
		AgentID:          normalized.AgentID,
		LifecycleStatus:  LifecycleStatusActive,
		Mentionable:      true,
		Metadata:         cloneMetadata(normalized.Metadata),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.store.Save(created); err != nil {
		return apitypes.Participant{}, err
	}
	if err := s.syncParticipantChannelUser(created); err != nil {
		return created, err
	}
	return created, nil
}

func (s *Service) EnsureBootstrapAdmin(_ context.Context) (apitypes.Participant, error) {
	if s == nil || s.store == nil {
		return apitypes.Participant{}, fmt.Errorf("participant store is required")
	}

	now := time.Now().UTC()
	createdAt := now
	existing, ok := s.store.Get(ChannelCSGClaw, bootstrapAdminParticipantID)
	legacyExisting, legacyOK := s.store.Get(ChannelCSGClaw, legacyAdminParticipantID)
	legacyBareExisting, legacyBareOK := s.store.Get(ChannelCSGClaw, legacyBareAdminParticipantID)
	source := existing
	hasLegacySource := false
	if !ok && legacyOK && isLegacyAdminParticipant(legacyExisting) {
		source = legacyExisting
		hasLegacySource = true
	} else if !ok && legacyBareOK && isLegacyAdminParticipant(legacyBareExisting) {
		source = legacyBareExisting
		hasLegacySource = true
	}
	if (ok || hasLegacySource) && !source.CreatedAt.IsZero() {
		createdAt = source.CreatedAt.UTC()
	}

	name := strings.TrimSpace(source.Name)
	if name == "" {
		name = "admin"
	}
	metadata := map[string]any(nil)
	if ok || hasLegacySource {
		metadata = cloneMetadata(source.Metadata)
	}
	if s.im != nil {
		if _, _, err := s.im.EnsureAgentUser(im.EnsureAgentUserRequest{
			ID:   im.AdminUserID,
			Name: "admin",
			Role: "admin",
		}); err != nil {
			return apitypes.Participant{}, err
		}
	}

	item := apitypes.Participant{
		ID:              bootstrapAdminParticipantID,
		Channel:         ChannelCSGClaw,
		Type:            TypeHuman,
		Name:            name,
		ChannelUserRef:  im.AdminUserID,
		ChannelUserKind: ChannelUserKindLocalUserID,
		LifecycleStatus: LifecycleStatusActive,
		Mentionable:     true,
		Metadata:        metadata,
		CreatedAt:       createdAt,
		UpdatedAt:       now,
	}
	if err := s.store.Save(item); err != nil {
		return apitypes.Participant{}, err
	}
	if err := s.syncParticipantChannelUser(item); err != nil {
		return item, err
	}
	if legacyOK && isLegacyAdminParticipant(legacyExisting) {
		if _, _, err := s.store.Delete(ChannelCSGClaw, legacyAdminParticipantID); err != nil {
			return apitypes.Participant{}, err
		}
	}
	if legacyBareOK && isLegacyAdminParticipant(legacyBareExisting) {
		if _, _, err := s.store.Delete(ChannelCSGClaw, legacyBareAdminParticipantID); err != nil {
			return apitypes.Participant{}, err
		}
	}
	return item, nil
}

func (s *Service) EnsureBootstrapManager(ctx context.Context) (apitypes.Participant, error) {
	if s == nil || s.store == nil {
		return apitypes.Participant{}, fmt.Errorf("participant store is required")
	}
	if s.agents == nil {
		return apitypes.Participant{}, fmt.Errorf("agent service is required")
	}
	manager, err := s.agents.Get(ctx, agent.ManagerUserID, agentengine.AgentGetOptions{})
	if err != nil {
		manager, err = s.agents.Create(ctx, agentengine.AgentCreateRequest{
			ID: agent.ManagerUserID,
			Spec: agentengine.AgentSpec{
				Name: agent.ManagerName,
				Role: agentengine.AgentRoleManager,
			},
		})
		if err != nil {
			return apitypes.Participant{}, err
		}
	}
	now := time.Now().UTC()
	createdAt := manager.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = now
	}
	existing, ok := s.store.Get(ChannelCSGClaw, agent.ManagerParticipantID)
	legacyExisting, legacyOK := s.store.Get(ChannelCSGClaw, legacyManagerAgentID)
	legacyBareExisting, legacyBareOK := s.store.Get(ChannelCSGClaw, legacyManagerParticipantID)
	legacyItems := s.legacyManagerParticipants()
	source := existing
	if !ok && legacyOK && isLegacyManagerParticipant(legacyExisting) {
		source = legacyExisting
	} else if !ok && legacyBareOK && isLegacyManagerParticipant(legacyBareExisting) {
		source = legacyBareExisting
	} else if !ok && len(legacyItems) > 0 {
		source = legacyItems[0]
	}
	hasLegacySource := legacyOK && isLegacyManagerParticipant(legacyExisting) || legacyBareOK && isLegacyManagerParticipant(legacyBareExisting) || len(legacyItems) > 0
	if (ok || hasLegacySource) && !source.CreatedAt.IsZero() {
		createdAt = source.CreatedAt.UTC()
	}

	name := strings.TrimSpace(manager.Spec.Name)
	if name == "" {
		name = agent.ManagerName
	}
	metadata := map[string]any(nil)
	if ok || hasLegacySource {
		metadata = cloneMetadata(source.Metadata)
	}
	if s.im != nil {
		if _, _, err := s.im.EnsureAgentUser(im.EnsureAgentUserRequest{
			ID:   im.ManagerUserID,
			Name: name,
			Role: agent.RoleManager,
		}); err != nil {
			return apitypes.Participant{}, err
		}
	}

	item := apitypes.Participant{
		ID:              agent.ManagerParticipantID,
		Channel:         ChannelCSGClaw,
		Type:            TypeAgent,
		Name:            name,
		ChannelUserRef:  im.ManagerUserID,
		ChannelUserKind: ChannelUserKindLocalUserID,
		AgentID:         manager.ID,
		LifecycleStatus: LifecycleStatusActive,
		Mentionable:     true,
		Metadata:        metadata,
		CreatedAt:       createdAt,
		UpdatedAt:       now,
	}
	if err := s.store.Save(item); err != nil {
		return apitypes.Participant{}, err
	}
	if err := s.syncParticipantChannelUser(item); err != nil {
		return item, err
	}
	if legacyOK && isLegacyManagerParticipant(legacyExisting) {
		if _, _, err := s.store.Delete(ChannelCSGClaw, legacyManagerAgentID); err != nil {
			return apitypes.Participant{}, err
		}
	}
	if legacyBareOK && isLegacyManagerParticipant(legacyBareExisting) {
		if _, _, err := s.store.Delete(ChannelCSGClaw, legacyManagerParticipantID); err != nil {
			return apitypes.Participant{}, err
		}
	}
	for _, legacy := range legacyItems {
		if _, _, err := s.store.Delete(ChannelCSGClaw, legacy.ID); err != nil {
			return apitypes.Participant{}, err
		}
	}
	return item, nil
}

const (
	BootstrapAdminParticipantID  = "pt-admin"
	bootstrapAdminParticipantID  = BootstrapAdminParticipantID
	legacyBareAdminParticipantID = "admin"
	legacyAdminParticipantID     = "u-admin"
	legacyManagerParticipantID   = "manager"
	legacyManagerAgentID         = "u-manager"
)

func isLegacyAdminParticipant(item apitypes.Participant) bool {
	id := strings.TrimSpace(item.ID)
	if id != legacyAdminParticipantID && id != legacyBareAdminParticipantID {
		return false
	}
	if strings.TrimSpace(item.Channel) != ChannelCSGClaw {
		return false
	}
	return true
}

func (s *Service) legacyManagerParticipants() []apitypes.Participant {
	if s == nil || s.store == nil {
		return nil
	}
	var out []apitypes.Participant
	for _, item := range s.store.List(ListOptions{Channel: ChannelCSGClaw, Type: TypeAgent}) {
		if strings.TrimSpace(item.ID) == agent.ManagerParticipantID {
			continue
		}
		if strings.TrimSpace(item.AgentID) == agent.ManagerUserID ||
			strings.TrimSpace(item.AgentID) == legacyManagerAgentID ||
			strings.TrimSpace(item.ChannelUserRef) == agent.ManagerUserID ||
			strings.TrimSpace(item.ChannelUserRef) == legacyManagerAgentID ||
			strings.TrimSpace(item.ChannelUserRef) == legacyManagerParticipantID ||
			strings.EqualFold(strings.TrimSpace(item.Name), agent.ManagerName) {
			out = append(out, item)
		}
	}
	return out
}

func isLegacyManagerParticipant(item apitypes.Participant) bool {
	id := strings.TrimSpace(item.ID)
	if id != legacyManagerAgentID && id != legacyManagerParticipantID && id != agent.ManagerUserID {
		return false
	}
	if strings.TrimSpace(item.Channel) != ChannelCSGClaw {
		return false
	}
	if strings.TrimSpace(item.AgentID) == agent.ManagerUserID || strings.TrimSpace(item.AgentID) == legacyManagerAgentID {
		return true
	}
	if strings.TrimSpace(item.ChannelUserRef) == agent.ManagerUserID ||
		strings.TrimSpace(item.ChannelUserRef) == legacyManagerAgentID ||
		strings.TrimSpace(item.ChannelUserRef) == legacyManagerParticipantID {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(item.Name), agent.ManagerName)
}

func (s *Service) Update(ctx context.Context, channel, id string, req UpdateRequest) (apitypes.Participant, bool, error) {
	if s == nil || s.store == nil {
		return apitypes.Participant{}, false, fmt.Errorf("participant store is required")
	}
	channel = normalizeChannel(channel)
	id = strings.TrimSpace(id)
	if channel == "" || id == "" {
		return apitypes.Participant{}, false, fmt.Errorf("channel and id are required")
	}
	release, err := s.lockParticipant(ctx, channel, id)
	if err != nil {
		return apitypes.Participant{}, false, err
	}
	defer release()
	item, _, ok := s.getByID(channel, id)
	if !ok {
		return apitypes.Participant{}, false, nil
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return apitypes.Participant{}, false, fmt.Errorf("name is required")
		}
		item.Name = name
	}
	if req.ChannelUserRef != nil {
		if channel != ChannelFeishu {
			return apitypes.Participant{}, false, fmt.Errorf("channel_user_ref can only be updated for %s participants", ChannelFeishu)
		}
		item.ChannelUserRef = strings.TrimSpace(*req.ChannelUserRef)
	}
	if req.ChannelUserKind != nil {
		if channel != ChannelFeishu {
			return apitypes.Participant{}, false, fmt.Errorf("channel_user_kind can only be updated for %s participants", ChannelFeishu)
		}
		kind := strings.TrimSpace(*req.ChannelUserKind)
		if kind == "" {
			return apitypes.Participant{}, false, fmt.Errorf("channel_user_kind is required")
		}
		item.ChannelUserKind = kind
	}
	if req.ChannelAppConfig != nil {
		if channel != ChannelFeishu {
			return apitypes.Participant{}, false, fmt.Errorf("channel_app_config can only be updated for %s participants", ChannelFeishu)
		}
		item.ChannelAppConfig = cloneMap(req.ChannelAppConfig)
	}
	if req.AgentID != nil {
		if channel != ChannelFeishu {
			return apitypes.Participant{}, false, fmt.Errorf("agent_id can only be updated for %s participants", ChannelFeishu)
		}
		if item.Type != TypeAgent {
			return apitypes.Participant{}, false, fmt.Errorf("agent_id can only be updated for %s participants", TypeAgent)
		}
		item.AgentID = strings.TrimSpace(*req.AgentID)
	}
	if err := validateFeishuParticipantConfig(item.Channel, item.Type, item.ChannelUserRef, item.ChannelUserKind, item.ChannelAppConfig); err != nil {
		return apitypes.Participant{}, false, err
	}
	if req.Mentionable != nil {
		item.Mentionable = *req.Mentionable
	}
	if req.Metadata != nil {
		item.Metadata = cloneMetadata(req.Metadata)
	}
	item.UpdatedAt = time.Now().UTC()
	syncChannelUser := req.Name != nil
	if err := s.store.Save(item); err != nil {
		return apitypes.Participant{}, false, err
	}
	if syncChannelUser {
		if err := s.syncParticipantChannelUser(item); err != nil {
			return item, true, err
		}
	}
	return item, true, nil
}

func (s *Service) syncParticipantChannelUser(item apitypes.Participant) error {
	if s == nil || s.im == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(item.Channel), ChannelCSGClaw) {
		return nil
	}
	if strings.TrimSpace(item.ChannelUserKind) != ChannelUserKindLocalUserID {
		return nil
	}
	userID := strings.TrimSpace(item.ChannelUserRef)
	if userID == "" {
		userID = strings.TrimSpace(item.ID)
	}
	if userID == "" {
		return nil
	}

	role := ""
	if item.Type == TypeHuman {
		role = "admin"
	}
	if _, _, err := s.im.UpdateAgentUser(im.UpdateAgentUserRequest{
		ID:   userID,
		Name: item.Name,
		Role: role,
	}); err != nil {
		return fmt.Errorf("sync channel user: %w", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, channel, id string, opts DeleteOptions) (apitypes.Participant, bool, error) {
	if s == nil || s.store == nil {
		return apitypes.Participant{}, false, fmt.Errorf("participant store is required")
	}
	channel = normalizeChannel(channel)
	id = strings.TrimSpace(id)
	if channel == "" || id == "" {
		return apitypes.Participant{}, false, fmt.Errorf("channel and id are required")
	}

	existing, deleteID, ok := s.getByID(channel, id)
	if !ok {
		return apitypes.Participant{}, false, nil
	}
	deleteAgentMode := strings.TrimSpace(opts.DeleteAgent)
	if deleteAgentMode != "" && deleteAgentMode != DeleteAgentIfUnreferenced {
		return apitypes.Participant{}, false, fmt.Errorf("delete_agent must be %q", DeleteAgentIfUnreferenced)
	}
	if deleteAgentMode == DeleteAgentIfUnreferenced && strings.TrimSpace(existing.AgentID) != "" {
		if s.agents == nil {
			return apitypes.Participant{}, false, fmt.Errorf("agent service is required")
		}
		if selected, err := s.agents.Get(ctx, existing.AgentID, agentengine.AgentGetOptions{}); err != nil {
			return apitypes.Participant{}, false, err
		} else if selected.ArchivedAt == nil {
			return apitypes.Participant{}, false, fmt.Errorf("archive the agent before deleting it")
		}
		for _, item := range s.store.List(ListOptions{AgentID: existing.AgentID}) {
			if item.Channel == existing.Channel && item.ID == existing.ID {
				continue
			}
			return apitypes.Participant{}, false, fmt.Errorf("agent %q is still referenced by participant %s:%s", existing.AgentID, item.Channel, item.ID)
		}
	}

	deleted, ok, err := s.store.Delete(channel, deleteID)
	if err != nil || !ok {
		return deleted, ok, err
	}
	if deleteAgentMode == DeleteAgentIfUnreferenced && strings.TrimSpace(deleted.AgentID) != "" {
		if err := s.agents.Delete(ctx, deleted.AgentID); err != nil {
			return deleted, true, err
		}
		if err := s.deleteUnreferencedCSGClawParticipantUser(deleted); err != nil {
			return deleted, true, err
		}
	} else if deleted.Type == TypeNotification {
		if err := s.deleteUnreferencedCSGClawParticipantUser(deleted); err != nil {
			return deleted, true, err
		}
	}
	return deleted, true, nil
}

func (s *Service) DeleteAgent(ctx context.Context, agentID string) ([]apitypes.Participant, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("participant store is required")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	if s.agents == nil {
		return nil, fmt.Errorf("agent service is required")
	}

	ctx, release, err := s.lifecycle.Mutation(ctx, agent.CanonicalID(agentID))
	if err != nil {
		return nil, err
	}
	defer release()
	refs := s.store.List(ListOptions{AgentID: agentID})
	if err := s.agents.Delete(ctx, agentID); err != nil {
		return nil, err
	}

	deleted := make([]apitypes.Participant, 0, len(refs))
	for _, ref := range refs {
		item, ok, err := s.store.Delete(ref.Channel, ref.ID)
		if err != nil {
			return deleted, err
		}
		if !ok {
			continue
		}
		deleted = append(deleted, item)
	}
	for _, item := range deleted {
		if err := s.deleteUnreferencedCSGClawParticipantUser(item); err != nil {
			return deleted, err
		}
	}
	return deleted, nil
}

func (s *Service) RepairDanglingCSGClawAgentParticipants() ([]apitypes.Participant, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("participant store is required")
	}
	if s.agents == nil || s.im == nil {
		return nil, nil
	}

	deleted := []apitypes.Participant{}
	for _, item := range s.store.List(ListOptions{Channel: ChannelCSGClaw, Type: TypeAgent}) {
		if isManagerParticipant(item) {
			continue
		}
		agentID := strings.TrimSpace(item.AgentID)
		if agentID != "" {
			_, found, err := s.boundAgentByID(context.Background(), agentID)
			if err != nil {
				return deleted, fmt.Errorf("inspect agent %q while repairing participants: %w", agentID, err)
			}
			if found {
				continue
			}
			// A persisted binding with a live local identity can be restored by
			// participant create even when its worker record is missing.
			if item.ChannelUserKind == ChannelUserKindLocalUserID {
				if user, ok := s.im.LocalUserByID(item.ChannelUserRef); ok && user.AgentState != "deleted" {
					continue
				}
			}
		}
		removed, ok, err := s.store.Delete(item.Channel, item.ID)
		if err != nil {
			return deleted, err
		}
		if !ok {
			continue
		}
		deleted = append(deleted, removed)
		if err := s.deleteUnreferencedCSGClawParticipantUser(removed); err != nil {
			return deleted, err
		}
	}
	return deleted, nil
}

func isManagerParticipant(item apitypes.Participant) bool {
	return strings.TrimSpace(item.ID) == agent.ManagerParticipantID ||
		strings.TrimSpace(item.AgentID) == agent.ManagerUserID ||
		strings.TrimSpace(item.ChannelUserRef) == im.ManagerUserID
}

func (s *Service) getByID(channel, id string) (apitypes.Participant, string, bool) {
	if s == nil || s.store == nil {
		return apitypes.Participant{}, "", false
	}
	channel = normalizeChannel(channel)
	rawID := strings.TrimSpace(id)
	for _, candidate := range participantLookupIDs(rawID) {
		if item, ok := s.store.Get(channel, candidate); ok {
			return item, candidate, true
		}
	}
	return apitypes.Participant{}, "", false
}

func participantLookupIDs(id string) []string {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	typed := canonicalParticipantID(slugify(id))
	if typed == "" || typed == id {
		return []string{id}
	}
	return []string{typed, id}
}

func (s *Service) deleteUnreferencedCSGClawParticipantUser(deleted apitypes.Participant) error {
	if s == nil || s.store == nil || s.im == nil {
		return nil
	}
	if deleted.Channel != ChannelCSGClaw ||
		(deleted.Type != TypeAgent && deleted.Type != TypeNotification) ||
		deleted.ChannelUserKind != ChannelUserKindLocalUserID {
		return nil
	}
	userID := strings.TrimSpace(deleted.ChannelUserRef)
	if userID == "" {
		return nil
	}
	for _, item := range s.store.List(ListOptions{Channel: ChannelCSGClaw}) {
		if strings.TrimSpace(item.ChannelUserRef) == userID {
			return nil
		}
	}
	deleteUser := s.im.DeleteUser
	if deleted.Type == TypeAgent {
		deleteUser = func(id string) error { return s.im.RetireAgentUser(id, deleted.AgentID) }
	}
	if err := deleteUser(userID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("delete CSGClaw user %q: %w", userID, err)
	}
	return nil
}

type normalizedCreateRequest struct {
	reconcile          bool
	reconcileBoundName bool
	ID                 string
	Channel            string
	Type               string
	Name               string
	Avatar             string
	ChannelAppRef      string
	ChannelAppConfig   map[string]any
	ChannelUser        ChannelUserSpec
	AgentBinding       AgentBindingSpec
	AgentHubService    *hub.Service
	AgentID            string
	Metadata           map[string]any
}

func (s *Service) normalizeCreateRequest(req CreateRequest) (normalizedCreateRequest, error) {
	channel := normalizeChannel(req.Channel)
	if channel == "" {
		return normalizedCreateRequest{}, fmt.Errorf("channel must be one of %q or %q", ChannelCSGClaw, ChannelFeishu)
	}
	typ := normalizeType(req.Type)
	if typ == "" {
		return normalizedCreateRequest{}, fmt.Errorf("type must be one of %q, %q, or %q", TypeHuman, TypeAgent, TypeNotification)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return normalizedCreateRequest{}, fmt.Errorf("name is required")
	}

	id, err := s.resolveParticipantID(channel, typ, req)
	if err != nil {
		return normalizedCreateRequest{}, err
	}

	channelUser := ChannelUserSpec{
		Ref:  strings.TrimSpace(req.ChannelUser.Ref),
		Kind: strings.TrimSpace(req.ChannelUser.Kind),
	}
	if channelUser.Ref == "" && channel == ChannelCSGClaw {
		if typ == TypeAgent {
			channelUser.Ref = defaultUserID(id)
		} else {
			channelUser.Ref = defaultUserID(id)
		}
	}
	if channel == ChannelCSGClaw && channelUser.Ref != "" {
		channelUser.Ref = canonicalUserID(channelUser.Ref)
	}
	if channelUser.Kind == "" {
		switch channel {
		case ChannelCSGClaw:
			channelUser.Kind = ChannelUserKindLocalUserID
		case ChannelFeishu:
			channelUser.Kind = ChannelUserKindOpenID
		}
	}
	if channelUser.Ref == "" && !allowsEmptyFeishuChannelUserRef(channel, typ, channelUser.Kind, req.ChannelAppConfig) {
		return normalizedCreateRequest{}, fmt.Errorf("channel_user.ref is required")
	}
	if err := validateFeishuParticipantConfig(channel, typ, channelUser.Ref, channelUser.Kind, req.ChannelAppConfig); err != nil {
		return normalizedCreateRequest{}, err
	}
	if channel != ChannelFeishu && len(req.ChannelAppConfig) > 0 {
		return normalizedCreateRequest{}, fmt.Errorf("channel_app_config can only be set for %s participants", ChannelFeishu)
	}

	binding := req.AgentBinding
	binding.Mode = normalizeBindingMode(binding.Mode)
	binding.AgentID = strings.TrimSpace(binding.AgentID)
	if binding.Mode == "" {
		binding.Mode = BindingModeNone
	}
	switch typ {
	case TypeHuman, TypeNotification:
		if binding.Mode == BindingModeCreate {
			return normalizedCreateRequest{}, fmt.Errorf("%s participant cannot create an agent binding", typ)
		}
	case TypeAgent:
		switch binding.Mode {
		case BindingModeCreate:
		case BindingModeReuse:
			if binding.AgentID == "" {
				return normalizedCreateRequest{}, fmt.Errorf("agent_binding.agent_id is required for reuse")
			}
		case BindingModeNone:
			if channel == ChannelCSGClaw {
				return normalizedCreateRequest{}, fmt.Errorf("csgclaw agent participants require agent_binding.mode %q or %q", BindingModeCreate, BindingModeReuse)
			}
		default:
			return normalizedCreateRequest{}, fmt.Errorf("agent_binding.mode must be one of %q, %q, or %q", BindingModeCreate, BindingModeReuse, BindingModeNone)
		}
	}

	return normalizedCreateRequest{
		ID:               id,
		Channel:          channel,
		Type:             typ,
		Name:             name,
		Avatar:           strings.TrimSpace(req.Avatar),
		ChannelAppRef:    strings.TrimSpace(req.ChannelAppRef),
		ChannelAppConfig: cloneMap(req.ChannelAppConfig),
		ChannelUser:      channelUser,
		AgentBinding:     binding,
		AgentHubService:  req.AgentHubService,
		Metadata:         cloneMetadata(req.Metadata),
	}, nil
}

func allowsEmptyFeishuChannelUserRef(channel, typ, kind string, appConfig map[string]any) bool {
	return channel == ChannelFeishu &&
		typ == TypeAgent &&
		strings.TrimSpace(kind) == ChannelUserKindAppID &&
		strings.TrimSpace(feishuConfigString(appConfig, "app_id")) != ""
}

func validateFeishuParticipantConfig(channel, typ, channelUserRef, channelUserKind string, appConfig map[string]any) error {
	if channel != ChannelFeishu {
		return nil
	}
	switch strings.TrimSpace(channelUserKind) {
	case ChannelUserKindOpenID:
		if strings.TrimSpace(channelUserRef) == "" {
			return fmt.Errorf("channel_user.ref is required")
		}
	case ChannelUserKindAppID:
		if typ != TypeAgent {
			return fmt.Errorf("channel_user_kind %q requires participant type %q", ChannelUserKindAppID, TypeAgent)
		}
		if strings.TrimSpace(feishuConfigString(appConfig, "app_id")) == "" {
			return fmt.Errorf("channel_app_config.app_id is required")
		}
		if strings.TrimSpace(feishuConfigString(appConfig, ChannelAppConfigAppSecretKey)) == "" {
			return fmt.Errorf("channel_app_config.app_secret is required")
		}
	default:
		return fmt.Errorf("channel_user_kind must be one of %q or %q", ChannelUserKindOpenID, ChannelUserKindAppID)
	}
	return nil
}

func feishuConfigString(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	value, _ := values[key]
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func (s *Service) resolveParticipantID(channel, typ string, req CreateRequest) (string, error) {
	if rawID := strings.TrimSpace(req.ID); rawID != "" {
		id := slugify(rawID)
		return canonicalParticipantID(id), nil
	}
	stable := strings.TrimSpace(req.ChannelUser.Ref)
	if stable == "" {
		stable = strings.TrimSpace(req.AgentBinding.AgentID)
	}
	if slug := slugify(stable); slug != "" {
		if strings.HasPrefix(slug, "user-") {
			slug = strings.TrimPrefix(slug, "user-")
		}
		if strings.HasPrefix(slug, "agent-") {
			slug = strings.TrimPrefix(slug, "agent-")
		}
		if !strings.HasPrefix(slug, "pt-") {
			slug = "pt-" + strings.TrimPrefix(slug, "u-")
		}
		if _, ok := s.store.Get(channel, slug); !ok {
			return slug, nil
		}
		return slug + "-" + randomSuffix(), nil
	}
	return "pt-" + randomSuffix(), nil
}

func canonicalParticipantID(id string) string {
	id = strings.TrimSpace(id)
	switch id {
	case "", "admin", "u-admin", "user-admin":
		if id == "" {
			return ""
		}
		return bootstrapAdminParticipantID
	case "manager", "u-manager", "user-manager", "agent-manager":
		return agent.ManagerParticipantID
	}
	if strings.HasPrefix(id, "pt-") {
		return id
	}
	if suffix := trimLocalIdentityPrefixes(id); suffix != "" {
		return "pt-" + suffix
	}
	return "pt-" + id
}

func canonicalUserID(id string) string {
	id = strings.TrimSpace(id)
	switch id {
	case "", "admin", "u-admin", "pt-admin":
		if id == "" {
			return ""
		}
		return im.AdminUserID
	case "manager", "u-manager", "pt-manager", "agent-manager":
		return im.ManagerUserID
	}
	if strings.HasPrefix(id, "user-") {
		return id
	}
	if suffix := trimLocalIdentityPrefixes(id); suffix != "" {
		return "user-" + suffix
	}
	return "user-" + id
}

func trimLocalIdentityPrefixes(id string) string {
	id = strings.TrimSpace(id)
	for {
		next := id
		for _, prefix := range []string{"user-", "agent-", "pt-", "u-"} {
			if strings.HasPrefix(next, prefix) {
				next = strings.TrimPrefix(next, prefix)
				break
			}
		}
		if next == id {
			break
		}
		id = next
	}
	return strings.TrimSpace(id)
}

// Engine Get also accepts display names. Bindings refer to canonical IDs only.
func (s *Service) boundAgentByID(ctx context.Context, id string) (agentengine.Agent, bool, error) {
	selected, err := s.agents.Get(ctx, id, agentengine.AgentGetOptions{})
	if err != nil {
		if agentengine.ErrorCodeOf(err) == agentengine.ErrorAgentNotFound {
			return agentengine.Agent{}, false, nil
		}
		return agentengine.Agent{}, false, err
	}
	return selected, selected.ID == agent.CanonicalID(id), nil
}

func (s *Service) ensureAgentBinding(ctx context.Context, req normalizedCreateRequest, restoring bool) (string, error) {
	switch req.AgentBinding.Mode {
	case BindingModeNone:
		return "", nil
	case BindingModeReuse:
		if s.agents == nil {
			return "", fmt.Errorf("agent service is required")
		}
		if _, err := s.agents.Get(ctx, req.AgentBinding.AgentID, agentengine.AgentGetOptions{}); err != nil {
			return "", fmt.Errorf("agent %q not found", req.AgentBinding.AgentID)
		}
		return agent.CanonicalID(req.AgentBinding.AgentID), nil
	case BindingModeCreate:
		if s.agents == nil {
			return "", fmt.Errorf("agent service is required")
		}
		agentID := req.AgentBinding.AgentID
		if agentID == "" {
			agentID = defaultAgentID(req.ID)
		}
		if req.reconcile {
			owner, err := s.agents.Get(ctx, req.Name, agentengine.AgentGetOptions{})
			if err == nil && owner.ID != agent.CanonicalID(agentID) {
				return "", fmt.Errorf("agent name %q %w", req.Name, identity.ErrNameExists)
			}
			if err != nil && agentengine.ErrorCodeOf(err) != agentengine.ErrorAgentNotFound {
				return "", err
			}
		}
		if existing, found, err := s.boundAgentByID(ctx, agentID); err != nil {
			return "", err
		} else if found {
			return s.reconcileAgentName(ctx, req, existing)
		}
		if restoring && (req.AgentBinding.Agent == nil || strings.TrimSpace(req.AgentBinding.Agent.FromTemplate) == "") {
			return "", fmt.Errorf("restoring missing agent %q requires a template", agentID)
		}
		spec := agent.CreateAgentSpec{}
		if req.AgentBinding.Agent != nil {
			spec = *req.AgentBinding.Agent
		}
		spec.ID = agentID
		if req.reconcile || strings.TrimSpace(spec.Name) == "" {
			spec.Name = req.Name
		}
		if strings.TrimSpace(spec.Role) == "" {
			spec.Role = agent.RoleWorker
		}
		createCtx := agentengine.WithLocalTemplateService(ctx, req.AgentHubService)
		created, err := s.agents.Create(createCtx, participantEngineCreateRequest(spec))
		if err != nil {
			return "", err
		}
		return s.reconcileAgentName(ctx, req, created)
	default:
		return "", fmt.Errorf("agent_binding.mode must be one of %q, %q, or %q", BindingModeCreate, BindingModeReuse, BindingModeNone)
	}
}

func (s *Service) reconcileAgentName(ctx context.Context, req normalizedCreateRequest, worker agentengine.Agent) (string, error) {
	if !req.reconcile {
		return worker.ID, nil
	}
	role := agentengine.AgentRoleWorker
	if req.AgentBinding.Agent != nil && strings.EqualFold(req.AgentBinding.Agent.Role, agent.RoleManager) {
		role = agentengine.AgentRoleManager
	}
	if worker.Spec.Role != role {
		return "", fmt.Errorf("agent %q already exists with role %q", worker.ID, worker.Spec.Role)
	}
	if req.reconcileBoundName && worker.Spec.Name != req.Name {
		_, err := s.agents.Update(ctx, worker.ID, agentengine.AgentUpdateRequest{
			Spec: agentengine.AgentSpec{Name: req.Name}, FieldMask: []string{"name"}, ResourceVersion: worker.ResourceVersion,
		})
		if err != nil {
			return "", err
		}
	}
	return worker.ID, nil
}

func participantEngineCreateRequest(spec agent.CreateAgentSpec) agentengine.AgentCreateRequest {
	runtimeConfig := spec.RuntimeConfig()
	role := agentengine.AgentRoleWorker
	if strings.EqualFold(strings.TrimSpace(spec.Role), agent.RoleManager) {
		role = agentengine.AgentRoleManager
	}
	var mcpServers map[string]agentengine.MCPServerConfig
	if spec.MCPServers != nil {
		mcpServers = make(map[string]agentengine.MCPServerConfig, len(spec.MCPServers))
		for name, raw := range spec.MCPServers {
			config, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			mcpServers[name] = agentengine.MCPServerConfig(utils.CloneAnyMap(config))
		}
	}
	return agentengine.AgentCreateRequest{
		ID:           spec.ID,
		FromTemplate: spec.FromTemplate,
		Spec: agentengine.AgentSpec{
			Name:         spec.Name,
			Description:  spec.Description,
			Instructions: spec.Instructions,
			Skills:       slices.Clone(spec.Skills),
			Role:         role,
			Runtime: agentengine.RuntimeSpec{
				Adapter:     runtimeConfig.Name,
				Sandboxed:   runtimeConfig.Sandboxed,
				Image:       spec.Image,
				Credentials: maps.Clone(spec.RuntimeCredentials),
				InitShell:   spec.RuntimeInitShell,
				Options:     utils.CloneAnyMap(spec.RuntimeOptions),
			},
			Model: agentengine.ModelSpec{
				ImageGeneration: modelprovider.CloneImageGeneration(spec.AgentProfile.ImageGeneration),
				VideoGeneration: modelprovider.CloneVideoGeneration(spec.AgentProfile.VideoGeneration),
				APIProtocol:     spec.AgentProfile.APIProtocol,
				Name:            spec.AgentProfile.Name,
				Description:     spec.AgentProfile.Description,
				Provider:        spec.AgentProfile.Provider,
				ProviderID:      spec.AgentProfile.ModelProviderID,
				BaseURL:         spec.AgentProfile.BaseURL,
				APIKey:          spec.AgentProfile.APIKey,
				Headers:         maps.Clone(spec.AgentProfile.Headers),
				ModelID:         spec.AgentProfile.ModelID,
				ReasoningEffort: spec.AgentProfile.ReasoningEffort,
				FastMode:        spec.AgentProfile.EnableFastMode,
				Options:         utils.CloneAnyMap(spec.AgentProfile.RequestOptions),
				Env:             maps.Clone(spec.AgentProfile.Env),
			},
			MCPServers:   mcpServers,
			DesiredState: agentengine.AgentDesiredStateRunning,
		},
	}
}

func (s *Service) ensureChannelIdentity(_ context.Context, req normalizedCreateRequest) error {
	if req.Channel != ChannelCSGClaw || s.im == nil {
		return nil
	}
	role := "member"
	avatar := strings.TrimSpace(req.Avatar)
	if req.Type == TypeAgent {
		role = agent.RoleWorker
		avatar = s.defaultParticipantAvatar(avatar)
	}
	if _, _, err := s.im.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID:     req.ChannelUser.Ref,
		Name:   req.Name,
		Role:   role,
		Avatar: avatar,
	}); err != nil {
		return err
	}
	_, _, err := s.im.UpdateAgentUser(im.UpdateAgentUserRequest{
		ID:     req.ChannelUser.Ref,
		Name:   req.Name,
		Role:   role,
		Avatar: avatar,
	})
	return err
}

func csgclawHandleForParticipant(req normalizedCreateRequest) string {
	for _, value := range []string{req.ChannelUser.Ref, req.ID} {
		value = strings.TrimSpace(value)
		for {
			next := value
			for _, prefix := range []string{"user-", "pt-", "agent-", "u-"} {
				if strings.HasPrefix(next, prefix) {
					next = strings.TrimPrefix(next, prefix)
					break
				}
			}
			if next == value {
				break
			}
			value = next
		}
		if value != "" {
			return value
		}
	}
	return strings.TrimSpace(req.Name)
}

func normalizeChannel(channel string) string {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "", ChannelCSGClaw:
		return ChannelCSGClaw
	case ChannelFeishu:
		return ChannelFeishu
	default:
		return ""
	}
}

func normalizeType(typ string) string {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case TypeHuman:
		return TypeHuman
	case TypeAgent:
		return TypeAgent
	case TypeNotification:
		return TypeNotification
	default:
		return ""
	}
}

func normalizeBindingMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case BindingModeCreate:
		return BindingModeCreate
	case BindingModeReuse:
		return BindingModeReuse
	case "", BindingModeNone:
		return BindingModeNone
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

func defaultAgentID(participantID string) string {
	suffix := strings.TrimPrefix(strings.TrimSpace(participantID), "pt-")
	if suffix == "" {
		suffix = randomSuffix()
	}
	return "agent-" + suffix
}

func defaultUserID(participantID string) string {
	suffix := strings.TrimPrefix(strings.TrimSpace(participantID), "pt-")
	if suffix == "" {
		suffix = randomSuffix()
	}
	return "user-" + suffix
}

func slugify(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 48 {
		out = strings.Trim(out[:48], "-")
	}
	if len(out) < 2 {
		return ""
	}
	return out
}

func randomSuffix() string {
	var raw [5]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return strings.ToLower(strings.TrimRight(base32.StdEncoding.EncodeToString(raw[:]), "="))[:6]
}

func cloneMetadata(src map[string]any) map[string]any {
	return cloneMap(src)
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
