package bot

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/im"
	"csgclaw/internal/utils"
)

type Service struct {
	store  *Store
	agents *agent.Service
	im     *im.Service
	imBus  *im.Bus
	imProv *im.Provisioner
	feishu *feishu.Service
}

func NewService(store *Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("bot store is required")
	}
	return &Service{store: store}, nil
}

func NewServiceWithDependencies(store *Store, agentSvc *agent.Service, imSvc *im.Service, feishuSvc ...*feishu.Service) (*Service, error) {
	s, err := NewService(store)
	if err != nil {
		return nil, err
	}
	s.SetDependencies(agentSvc, imSvc, feishuSvc...)
	return s, nil
}

func (s *Service) SetDependencies(agentSvc *agent.Service, imSvc *im.Service, feishuSvc ...*feishu.Service) {
	if s == nil {
		return
	}
	s.agents = agentSvc
	s.im = imSvc
	s.imProv = im.NewProvisioner(imSvc, s.imBus)
	if len(feishuSvc) > 0 {
		s.feishu = feishuSvc[0]
	}
}

func (s *Service) SetIMBus(bus *im.Bus) {
	if s == nil {
		return
	}
	s.imBus = bus
	s.imProv = im.NewProvisioner(s.im, bus)
}

func (s *Service) List(channel, role string) ([]Bot, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("bot store is required")
	}

	all := s.store.List()
	normalizedChannel := ""
	if strings.TrimSpace(channel) != "" {
		normalized, err := NormalizeChannel(channel)
		if err != nil {
			return nil, err
		}
		normalizedChannel = string(normalized)
	}

	normalizedRole := ""
	if strings.TrimSpace(role) != "" {
		normalized, err := NormalizeRole(role)
		if err != nil {
			return nil, err
		}
		normalizedRole = string(normalized)
	}

	filtered := make([]Bot, 0, len(all))
	for _, b := range all {
		if normalizedChannel != "" && b.Channel != normalizedChannel {
			continue
		}
		if normalizedRole != "" && b.Role != normalizedRole {
			continue
		}
		filtered = append(filtered, b)
	}
	filtered = s.refreshBotAvailability(filtered)
	if normalizedChannel == string(ChannelFeishu) {
		var err error
		filtered, err = s.appendConfiguredFeishuBots(context.Background(), filtered, normalizedRole)
		if err != nil {
			return nil, err
		}
	}
	return filtered, nil
}

func (s *Service) refreshBotAvailability(bots []Bot) []Bot {
	if s == nil || s.agents == nil {
		return bots
	}
	refreshed := make([]Bot, 0, len(bots))
	for _, b := range bots {
		agentID := strings.TrimSpace(b.AgentID)
		b.Available = false
		if agentID != "" {
			if a, ok := s.agents.Agent(agentID); ok {
				b.AgentID = a.ID
				b.Available = strings.EqualFold(strings.TrimSpace(a.Status), "running")
			}
		}
		refreshed = append(refreshed, b)
	}
	return refreshed
}

func (s *Service) appendConfiguredFeishuBots(ctx context.Context, bots []Bot, role string) ([]Bot, error) {
	if s.feishu == nil {
		return bots, nil
	}
	apps := s.feishu.AppConfigs()
	if len(apps) == 0 {
		return bots, nil
	}

	seen := make(map[string]struct{}, len(bots))
	for _, b := range bots {
		id := strings.TrimSpace(b.ID)
		seen[id] = struct{}{}
		seen[configuredFeishuBotDisplayID(id)] = struct{}{}
	}

	configuredIDs := make([]string, 0, len(apps))
	for id := range apps {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		configuredIDs = append(configuredIDs, id)
	}
	slices.Sort(configuredIDs)

	for _, id := range configuredIDs {
		displayID := configuredFeishuBotDisplayID(id)
		if displayID == "" {
			continue
		}
		if _, ok := seen[displayID]; ok {
			continue
		}
		botRole := string(RoleWorker)
		if id == agent.ManagerUserID {
			botRole = string(RoleManager)
		}
		if role != "" && botRole != role {
			continue
		}
		agentID := ""
		available := false
		if s.agents != nil {
			if a, ok := s.agents.Agent(id); ok {
				agentID = a.ID
				available = strings.EqualFold(strings.TrimSpace(a.Status), "running")
			}
		}
		openID, appName, err := s.feishu.ResolveBotOpenID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("resolve configured feishu bot %q open_id: %w", id, err)
		}
		name := strings.TrimSpace(appName)
		if name == "" {
			name = displayID
		}
		bots = append(bots, Bot{
			ID:        id,
			Name:      name,
			Role:      botRole,
			Channel:   string(ChannelFeishu),
			AgentID:   agentID,
			UserID:    strings.TrimSpace(openID),
			Available: available,
		})
		seen[displayID] = struct{}{}
	}
	return bots, nil
}

func configuredFeishuBotDisplayID(id string) string {
	id = strings.TrimSpace(id)
	return strings.TrimPrefix(id, "u-")
}

func (s *Service) Delete(ctx context.Context, channel, id string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("bot store is required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("bot id is required")
	}
	if strings.TrimSpace(channel) == "" {
		channel = string(ChannelCSGClaw)
	}
	deleted, ok, err := s.store.DeleteByChannelID(channel, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("bot %q not found", id)
	}
	if err := s.deleteChannelUser(deleted); err != nil {
		return err
	}
	if s.agents == nil {
		return nil
	}
	if r := strings.ToLower(strings.TrimSpace(deleted.Role)); r != string(RoleWorker) {
		return nil
	}
	agentID := strings.TrimSpace(deleted.AgentID)
	if agentID == "" {
		return nil
	}
	for _, b := range s.store.List() {
		if strings.TrimSpace(b.AgentID) == agentID {
			return nil
		}
	}
	if err := s.agents.Delete(ctx, agentID); err != nil {
		return fmt.Errorf("delete backing agent %q: %w", agentID, err)
	}
	return nil
}

func (s *Service) deleteChannelUser(deleted Bot) error {
	if strings.TrimSpace(deleted.Channel) != string(ChannelCSGClaw) || s.im == nil {
		return nil
	}
	userID := strings.TrimSpace(deleted.UserID)
	if userID == "" {
		userID = strings.TrimSpace(deleted.ID)
	}
	if userID == "" {
		return nil
	}
	if err := s.im.DeleteUser(userID); err != nil && !strings.Contains(err.Error(), "user not found") {
		return fmt.Errorf("delete csgclaw user %q: %w", userID, err)
	}
	return nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Bot, error) {
	if s == nil || s.store == nil {
		return Bot{}, fmt.Errorf("bot store is required")
	}

	normalized, err := NormalizeCreateRequest(req)
	if err != nil {
		return Bot{}, err
	}
	if s.agents == nil {
		return Bot{}, fmt.Errorf("agent service is required")
	}
	switch normalized.Role {
	case string(RoleManager):
		return s.createManager(ctx, normalized, false)
	case string(RoleWorker):
		return s.createWorker(ctx, normalized)
	default:
		return Bot{}, fmt.Errorf("role must be one of %q or %q", RoleManager, RoleWorker)
	}
}

func (s *Service) CreateManager(ctx context.Context, req CreateRequest, forceRecreateAgent bool) (Bot, error) {
	if s == nil || s.store == nil {
		return Bot{}, fmt.Errorf("bot store is required")
	}
	req.Role = string(RoleManager)
	normalized, err := NormalizeCreateRequest(req)
	if err != nil {
		return Bot{}, err
	}
	if s.agents == nil {
		return Bot{}, fmt.Errorf("agent service is required")
	}
	return s.createManager(ctx, normalized, forceRecreateAgent)
}

func (s *Service) createWorker(ctx context.Context, normalized CreateRequest) (Bot, error) {
	var err error
	if existing, ok := s.findByChannelName(normalized.Channel, normalized.Name); ok {
		return Bot{}, fmt.Errorf("bot name %q already exists in channel %q with id %q", normalized.Name, normalized.Channel, existing.ID)
	}

	created, ok := s.agents.Agent(workerAgentID(normalized))
	if ok {
		if !strings.EqualFold(strings.TrimSpace(created.Role), agent.RoleWorker) {
			return Bot{}, fmt.Errorf("agent id %q already exists with role %q", created.ID, created.Role)
		}
	} else {
		created, err = s.agents.CreateWorker(ctx, agent.CreateAgentSpec{
			ID:             normalized.ID,
			Name:           normalized.Name,
			Description:    normalized.Description,
			Image:          normalized.Image,
			Role:           agent.RoleWorker,
			RuntimeKind:    normalized.RuntimeKind,
			FromTemplate:   normalized.FromTemplate,
			RuntimeOptions: utils.CloneAnyMap(normalized.RuntimeOptions),
			AgentProfile:   agentProfileFromBotRequest(normalized.AgentProfile),
		})
		if err != nil {
			return Bot{}, err
		}
	}

	userID, userCreatedAt, err := s.ensureChannelUser(ctx, normalized.Channel, created)
	if err != nil {
		// TODO: compensate by deleting the agent/box created above once agent deletion
		// semantics are safe to call from bot creation.
		return Bot{}, err
	}

	createdAt := userCreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = created.CreatedAt.UTC()
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	b := Bot{
		ID:          created.ID,
		Name:        created.Name,
		Description: normalized.Description,
		Role:        normalized.Role,
		Channel:     normalized.Channel,
		AgentID:     created.ID,
		UserID:      userID,
		Available:   true,
		CreatedAt:   createdAt,
	}
	if _, ok, err := s.store.GetByChannelID(b.Channel, b.ID); err != nil {
		return Bot{}, err
	} else if ok {
		if err := s.store.Save(b); err != nil {
			return Bot{}, err
		}
		return b, nil
	}
	if err := s.store.Save(b); err != nil {
		return Bot{}, err
	}
	return b, nil
}

func agentProfileFromBotRequest(req *apitypes.CreateAgentProfile) agent.AgentProfile {
	if req == nil {
		return agent.AgentProfile{}
	}
	return agent.AgentProfile{
		Name:            req.Name,
		Description:     req.Description,
		Provider:        req.Provider,
		BaseURL:         req.BaseURL,
		APIKey:          req.APIKey,
		Headers:         req.Headers,
		ModelID:         req.ModelID,
		ReasoningEffort: req.ReasoningEffort,
		EnableFastMode:  req.EnableFastMode,
		RequestOptions:  req.RequestOptions,
		Env:             req.Env,
		ProfileComplete: req.ProfileComplete,
	}
}

func (s *Service) createManager(ctx context.Context, normalized CreateRequest, forceRecreateAgent bool) (Bot, error) {
	if normalized.ID != "" && normalized.ID != agent.ManagerUserID {
		return Bot{}, fmt.Errorf("manager bot id must be %q", agent.ManagerUserID)
	}

	manager, ok := s.agents.Agent(agent.ManagerUserID)
	if forceRecreateAgent || !ok || strings.ToLower(strings.TrimSpace(manager.Role)) != agent.RoleManager {
		ensured, err := s.agents.EnsureManager(ctx, forceRecreateAgent)
		if err != nil {
			return Bot{}, err
		}
		manager = ensured
	}

	userID, userCreatedAt, err := s.ensureChannelUser(ctx, normalized.Channel, manager)
	if err != nil {
		return Bot{}, err
	}

	createdAt := userCreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = manager.CreatedAt.UTC()
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	b := Bot{
		ID:          manager.ID,
		Name:        normalized.Name,
		Description: normalized.Description,
		Role:        string(RoleManager),
		Channel:     normalized.Channel,
		AgentID:     manager.ID,
		UserID:      userID,
		Available:   true,
		CreatedAt:   createdAt,
	}
	if _, ok, err := s.store.GetByChannelID(b.Channel, b.ID); err != nil {
		return Bot{}, err
	} else if ok {
		if err := s.store.Save(b); err != nil {
			return Bot{}, err
		}
		return b, nil
	}
	if err := s.store.Save(b); err != nil {
		return Bot{}, err
	}
	return b, nil
}

func workerAgentID(req CreateRequest) string {
	if id := strings.TrimSpace(req.ID); id != "" {
		return id
	}
	return fmt.Sprintf("u-%s", strings.TrimSpace(req.Name))
}

func (s *Service) findByChannelName(channel, name string) (Bot, bool) {
	channel = strings.TrimSpace(channel)
	name = strings.TrimSpace(name)
	for _, existing := range s.store.List() {
		if !strings.EqualFold(strings.TrimSpace(existing.Channel), channel) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(existing.Name), name) {
			return existing, true
		}
	}
	return Bot{}, false
}

func (s *Service) ensureChannelUser(ctx context.Context, channelName string, created agent.Agent) (string, time.Time, error) {
	switch channelName {
	case string(ChannelCSGClaw):
		if s.imProv == nil {
			return "", time.Time{}, fmt.Errorf("im provisioner is required")
		}
		result, err := s.imProv.EnsureAgentUser(ctx, im.AgentIdentity{
			ID:          created.ID,
			Name:        created.Name,
			Description: created.Description,
			Handle:      deriveAgentHandle(created),
			Role:        displayRole(created),
		})
		if err != nil {
			return "", time.Time{}, fmt.Errorf("failed to ensure im user: %w", err)
		}
		return result.User.ID, result.User.CreatedAt, nil
	case string(ChannelFeishu):
		if s.feishu == nil {
			return "", time.Time{}, fmt.Errorf("feishu service is required")
		}
		user, err := s.feishu.EnsureUser(feishu.CreateUserRequest{
			ID:     created.ID,
			Name:   created.Name,
			Handle: deriveAgentHandle(created),
			Role:   displayRole(created),
		})
		if err != nil {
			return "", time.Time{}, fmt.Errorf("failed to ensure feishu user: %w", err)
		}
		return user.ID, user.CreatedAt, nil
	default:
		return "", time.Time{}, fmt.Errorf("channel must be one of %q or %q", ChannelCSGClaw, ChannelFeishu)
	}
}

func deriveAgentHandle(a agent.Agent) string {
	if strings.EqualFold(strings.TrimSpace(a.Role), agent.RoleWorker) &&
		strings.EqualFold(strings.TrimSpace(a.RuntimeKind), agent.RuntimeKindNotifier) {
		if handle, ok := sanitizeHandle(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(a.Name), " ", "-"))); ok {
			return handle
		}
		return "notifier"
	}
	if handle, ok := sanitizeHandle(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(a.Name), " ", "-"))); ok {
		return handle
	}
	if handle, ok := sanitizeHandle(strings.ToLower(strings.TrimPrefix(strings.TrimSpace(a.ID), "u-"))); ok {
		return handle
	}
	switch strings.ToLower(strings.TrimSpace(a.Role)) {
	case agent.RoleManager:
		return "manager"
	case agent.RoleWorker:
		return "worker"
	default:
		return "agent"
	}
}

func displayRole(a agent.Agent) string {
	switch strings.ToLower(strings.TrimSpace(a.Role)) {
	case agent.RoleManager:
		return "manager"
	case agent.RoleWorker:
		return "Worker"
	default:
		return "Agent"
	}
}

func sanitizeHandle(input string) (string, bool) {
	var b strings.Builder
	hasAlphaNum := false
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			hasAlphaNum = true
			b.WriteRune(r)
			continue
		}
		if r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 || !hasAlphaNum {
		return "", false
	}
	return b.String(), true
}
