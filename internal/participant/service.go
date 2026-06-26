package participant

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"
	"unicode"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
)

type Service struct {
	store  *Store
	agents *agent.Service
	im     *im.Service
}

type Option func(*Service)

func NewService(store *Store, opts ...Option) *Service {
	if store == nil {
		store = NewMemoryStore(nil)
	}
	s := &Service{store: store}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func WithAgentService(agentSvc *agent.Service) Option {
	return func(s *Service) {
		s.agents = agentSvc
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
	if s == nil || s.store == nil {
		return apitypes.Participant{}, fmt.Errorf("participant store is required")
	}
	normalized, err := s.normalizeCreateRequest(req)
	if err != nil {
		return apitypes.Participant{}, err
	}
	if _, ok := s.store.Get(normalized.Channel, normalized.ID); ok {
		return apitypes.Participant{}, fmt.Errorf("participant %s:%s already exists", normalized.Channel, normalized.ID)
	}

	if normalized.Type == TypeAgent {
		agentID, err := s.ensureAgentBinding(ctx, normalized)
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
	manager, err := s.agents.EnsureManager(ctx, false)
	if err != nil {
		return apitypes.Participant{}, err
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

	name := strings.TrimSpace(manager.Name)
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

func (s *Service) Update(_ context.Context, channel, id string, req UpdateRequest) (apitypes.Participant, bool, error) {
	if s == nil || s.store == nil {
		return apitypes.Participant{}, false, fmt.Errorf("participant store is required")
	}
	channel = normalizeChannel(channel)
	id = strings.TrimSpace(id)
	if channel == "" || id == "" {
		return apitypes.Participant{}, false, fmt.Errorf("channel and id are required")
	}
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
		if err := s.deleteUnreferencedCSGClawAgentUser(deleted); err != nil {
			return deleted, true, err
		}
	}
	return deleted, true, nil
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

func (s *Service) deleteUnreferencedCSGClawAgentUser(deleted apitypes.Participant) error {
	if s == nil || s.store == nil || s.im == nil {
		return nil
	}
	if deleted.Channel != ChannelCSGClaw || deleted.Type != TypeAgent || deleted.ChannelUserKind != ChannelUserKindLocalUserID {
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
	if err := s.im.DeleteUser(userID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("delete CSGClaw user %q: %w", userID, err)
	}
	return nil
}

type normalizedCreateRequest struct {
	ID               string
	Channel          string
	Type             string
	Name             string
	ChannelAppRef    string
	ChannelAppConfig map[string]any
	ChannelUser      ChannelUserSpec
	AgentBinding     AgentBindingSpec
	AgentID          string
	Metadata         map[string]any
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
		default:
			return normalizedCreateRequest{}, fmt.Errorf("agent_binding.mode must be one of %q, %q, or %q", BindingModeCreate, BindingModeReuse, BindingModeNone)
		}
	}

	return normalizedCreateRequest{
		ID:               id,
		Channel:          channel,
		Type:             typ,
		Name:             name,
		ChannelAppRef:    strings.TrimSpace(req.ChannelAppRef),
		ChannelAppConfig: cloneMap(req.ChannelAppConfig),
		ChannelUser:      channelUser,
		AgentBinding:     binding,
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

func (s *Service) ensureAgentBinding(ctx context.Context, req normalizedCreateRequest) (string, error) {
	switch req.AgentBinding.Mode {
	case BindingModeNone:
		return "", nil
	case BindingModeReuse:
		if s.agents == nil {
			return "", fmt.Errorf("agent service is required")
		}
		if _, ok := s.agents.Agent(req.AgentBinding.AgentID); !ok {
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
		if existing, ok := s.agents.Agent(agentID); ok {
			return existing.ID, nil
		}
		spec := agent.CreateAgentSpec{}
		if req.AgentBinding.Agent != nil {
			spec = *req.AgentBinding.Agent
		}
		spec.ID = agentID
		if strings.TrimSpace(spec.Name) == "" {
			spec.Name = req.Name
		}
		if strings.TrimSpace(spec.Role) == "" {
			spec.Role = agent.RoleWorker
		}
		created, err := s.agents.Create(ctx, agent.CreateRequest{Spec: spec})
		if err != nil {
			return "", err
		}
		return created.ID, nil
	default:
		return "", fmt.Errorf("agent_binding.mode must be one of %q, %q, or %q", BindingModeCreate, BindingModeReuse, BindingModeNone)
	}
}

func (s *Service) ensureChannelIdentity(_ context.Context, req normalizedCreateRequest) error {
	if req.Channel != ChannelCSGClaw || s.im == nil {
		return nil
	}
	role := "member"
	if req.Type == TypeAgent {
		role = agent.RoleWorker
	}
	if _, _, err := s.im.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID:   req.ChannelUser.Ref,
		Name: req.Name,
		Role: role,
	}); err != nil {
		return err
	}
	_, _, err := s.im.UpdateAgentUser(im.UpdateAgentUserRequest{
		ID:   req.ChannelUser.Ref,
		Name: req.Name,
		Role: role,
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
