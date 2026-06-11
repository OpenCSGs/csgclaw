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
	return s.store.Get(channel, id)
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
		ID:              normalized.ID,
		Channel:         normalized.Channel,
		Type:            normalized.Type,
		Name:            normalized.Name,
		Avatar:          normalized.Avatar,
		ChannelUserRef:  normalized.ChannelUser.Ref,
		ChannelUserKind: normalized.ChannelUser.Kind,
		ChannelAppRef:   normalized.ChannelAppRef,
		AgentID:         normalized.AgentID,
		LifecycleStatus: LifecycleStatusActive,
		Mentionable:     true,
		Metadata:        cloneMetadata(normalized.Metadata),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.store.Save(created); err != nil {
		return apitypes.Participant{}, err
	}
	return created, nil
}

func (s *Service) EnsureBootstrapAdmin(_ context.Context) (apitypes.Participant, error) {
	if s == nil || s.store == nil {
		return apitypes.Participant{}, fmt.Errorf("participant store is required")
	}

	now := time.Now().UTC()
	createdAt := now
	existing, ok := s.store.Get(ChannelCSGClaw, im.AdminUserID)
	legacyExisting, legacyOK := s.store.Get(ChannelCSGClaw, legacyAdminParticipantID)
	source := existing
	hasLegacySource := false
	if !ok && legacyOK && isLegacyAdminParticipant(legacyExisting) {
		source = legacyExisting
		hasLegacySource = true
	}
	if (ok || hasLegacySource) && !source.CreatedAt.IsZero() {
		createdAt = source.CreatedAt.UTC()
	}

	name := strings.TrimSpace(source.Name)
	if name == "" {
		name = "admin"
	}
	avatar := strings.TrimSpace(source.Avatar)
	metadata := map[string]any(nil)
	if ok || hasLegacySource {
		metadata = cloneMetadata(source.Metadata)
	}
	if s.im != nil {
		if _, _, err := s.im.EnsureAgentUser(im.EnsureAgentUserRequest{
			ID:     im.AdminUserID,
			Name:   "admin",
			Handle: "admin",
			Role:   "admin",
			Avatar: avatar,
		}); err != nil {
			return apitypes.Participant{}, err
		}
	}

	item := apitypes.Participant{
		ID:              im.AdminUserID,
		Channel:         ChannelCSGClaw,
		Type:            TypeHuman,
		Name:            name,
		Avatar:          avatar,
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
	if legacyOK && isLegacyAdminParticipant(legacyExisting) {
		if _, _, err := s.store.Delete(ChannelCSGClaw, legacyAdminParticipantID); err != nil {
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
	legacyExisting, legacyOK := s.store.Get(ChannelCSGClaw, agent.ManagerUserID)
	legacyItems := s.legacyManagerParticipants()
	source := existing
	if !ok && legacyOK && isLegacyManagerParticipant(legacyExisting) {
		source = legacyExisting
	} else if !ok && len(legacyItems) > 0 {
		source = legacyItems[0]
	}
	hasLegacySource := legacyOK && isLegacyManagerParticipant(legacyExisting) || len(legacyItems) > 0
	if (ok || hasLegacySource) && !source.CreatedAt.IsZero() {
		createdAt = source.CreatedAt.UTC()
	}

	name := strings.TrimSpace(manager.Name)
	if name == "" {
		name = agent.ManagerName
	}
	avatar := strings.TrimSpace(manager.Avatar)
	metadata := map[string]any(nil)
	if ok || hasLegacySource {
		metadata = cloneMetadata(source.Metadata)
		if avatar == "" {
			avatar = strings.TrimSpace(source.Avatar)
		}
	}
	if s.im != nil {
		if _, _, err := s.im.EnsureAgentUser(im.EnsureAgentUserRequest{
			ID:     agent.ManagerParticipantID,
			Name:   name,
			Handle: "manager",
			Role:   agent.RoleManager,
			Avatar: avatar,
		}); err != nil {
			return apitypes.Participant{}, err
		}
	}

	item := apitypes.Participant{
		ID:              agent.ManagerParticipantID,
		Channel:         ChannelCSGClaw,
		Type:            TypeAgent,
		Name:            name,
		Avatar:          avatar,
		ChannelUserRef:  agent.ManagerParticipantID,
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
	if legacyOK && isLegacyManagerParticipant(legacyExisting) {
		if _, _, err := s.store.Delete(ChannelCSGClaw, agent.ManagerUserID); err != nil {
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
	bootstrapAdminParticipantID = "admin"
	legacyAdminParticipantID    = "u-admin"
)

func isLegacyAdminParticipant(item apitypes.Participant) bool {
	if strings.TrimSpace(item.ID) != legacyAdminParticipantID {
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
		if strings.TrimSpace(item.ID) == agent.ManagerParticipantID || strings.TrimSpace(item.ID) == agent.ManagerUserID {
			continue
		}
		if strings.TrimSpace(item.AgentID) == agent.ManagerUserID ||
			strings.TrimSpace(item.ChannelUserRef) == agent.ManagerUserID ||
			strings.EqualFold(strings.TrimSpace(item.Name), agent.ManagerName) {
			out = append(out, item)
		}
	}
	return out
}

func isLegacyManagerParticipant(item apitypes.Participant) bool {
	if strings.TrimSpace(item.ID) != agent.ManagerUserID {
		return false
	}
	if strings.TrimSpace(item.Channel) != ChannelCSGClaw {
		return false
	}
	if strings.TrimSpace(item.AgentID) == agent.ManagerUserID {
		return true
	}
	if strings.TrimSpace(item.ChannelUserRef) == agent.ManagerUserID {
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
	item, ok := s.store.Get(channel, id)
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
	if req.Avatar != nil {
		item.Avatar = strings.TrimSpace(*req.Avatar)
	}
	if req.Mentionable != nil {
		item.Mentionable = *req.Mentionable
	}
	if req.Metadata != nil {
		item.Metadata = cloneMetadata(req.Metadata)
	}
	item.UpdatedAt = time.Now().UTC()
	syncChannelUser := req.Name != nil || req.Avatar != nil
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
		ID:     userID,
		Name:   item.Name,
		Role:   role,
		Avatar: item.Avatar,
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

	existing, ok := s.store.Get(channel, id)
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

	deleted, ok, err := s.store.Delete(channel, id)
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
	ID            string
	Channel       string
	Type          string
	Name          string
	Avatar        string
	ChannelAppRef string
	ChannelUser   ChannelUserSpec
	AgentBinding  AgentBindingSpec
	AgentID       string
	Metadata      map[string]any
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
			channelUser.Ref = defaultAgentID(id)
		} else {
			channelUser.Ref = id
		}
	}
	if channelUser.Kind == "" {
		switch channel {
		case ChannelCSGClaw:
			channelUser.Kind = ChannelUserKindLocalUserID
		case ChannelFeishu:
			channelUser.Kind = ChannelUserKindOpenID
		}
	}
	if channelUser.Ref == "" {
		return normalizedCreateRequest{}, fmt.Errorf("channel_user.ref is required")
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
		ID:            id,
		Channel:       channel,
		Type:          typ,
		Name:          name,
		Avatar:        strings.TrimSpace(req.Avatar),
		ChannelAppRef: strings.TrimSpace(req.ChannelAppRef),
		ChannelUser:   channelUser,
		AgentBinding:  binding,
		Metadata:      cloneMetadata(req.Metadata),
	}, nil
}

func (s *Service) resolveParticipantID(channel, typ string, req CreateRequest) (string, error) {
	if id := slugify(req.ID); id != "" {
		return id, nil
	}
	stable := strings.TrimSpace(req.ChannelUser.Ref)
	if stable == "" {
		stable = strings.TrimSpace(req.AgentBinding.AgentID)
	}
	if strings.HasPrefix(stable, "u-") && typ == TypeAgent {
		stable = strings.TrimPrefix(stable, "u-")
	}
	if slug := slugify(stable); slug != "" {
		if _, ok := s.store.Get(channel, slug); !ok {
			return slug, nil
		}
		return slug + "-" + randomSuffix(), nil
	}
	return typ + "-" + randomSuffix(), nil
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
		return req.AgentBinding.AgentID, nil
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
	_, _, err := s.im.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID:     req.ChannelUser.Ref,
		Name:   req.Name,
		Handle: req.ID,
		Role:   role,
		Avatar: req.Avatar,
	})
	return err
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
	return "u-" + strings.TrimSpace(participantID)
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
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
