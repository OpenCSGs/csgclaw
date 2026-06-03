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

func (s *Service) List(channel, role, botType string) ([]Bot, error) {
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
		if !shouldIncludeBotInList(b, normalizedChannel, botType) {
			continue
		}
		if normalizedChannel != "" && b.Channel != normalizedChannel {
			continue
		}
		if normalizedRole != "" && b.Role != normalizedRole {
			continue
		}
		filtered = append(filtered, b)
	}
	filtered = s.refreshBotAvailability(filtered)
	if normalizedChannel == string(ChannelFeishu) && NormalizeBotType(botType) != BotTypeNotification {
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
		if IsNotificationBot(b) {
			refreshed = append(refreshed, s.presentNotificationBot(b))
			continue
		}
		agentID := strings.TrimSpace(b.AgentID)
		b.Available = false
		if agentID != "" {
			if a, ok := s.agents.Agent(agentID); ok {
				b.AgentID = a.ID
				b.Available = strings.EqualFold(strings.TrimSpace(a.Status), "running")
				b.RuntimeKind = strings.TrimSpace(a.RuntimeKind)
				b.Image = strings.TrimSpace(a.Image)
				b.Avatar = strings.TrimSpace(a.Avatar)
				b.Status = strings.TrimSpace(a.Status)
				b.Provider = strings.TrimSpace(a.AgentProfile.Provider)
				b.ModelID = strings.TrimSpace(a.AgentProfile.ModelID)
				b.ProfileComplete = a.ProfileComplete || a.AgentProfile.ProfileComplete
				b.EnvRestartRequired = a.AgentProfile.EnvRestartRequired
				b.ImageUpgradeRequired = a.AgentProfile.ImageUpgradeRequired
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
	deleted, ok, err := s.store.GetByChannelID(channel, id)
	if err != nil {
		return err
	}
	target, err := s.deletionTarget(ctx, channel, id, deleted, ok)
	if err != nil {
		return err
	}
	userDeleted, err := s.deleteChannelUser(target)
	if err != nil {
		return err
	}
	agentDeleted, err := s.deleteBackingAgent(ctx, target)
	if err != nil {
		if userDeleted {
			return fmt.Errorf("bot %q partially deleted: channel user removed, but backing agent cleanup failed; retry delete to finish cleanup: %w", id, err)
		}
		return err
	}
	if ok {
		if _, deletedOK, err := s.store.DeleteByChannelID(channel, id); err != nil {
			if userDeleted || agentDeleted {
				return fmt.Errorf("bot %q partially deleted: backing resources were removed, but bot state cleanup failed; retry delete to finish cleanup: %w", id, err)
			}
			return err
		} else if !deletedOK {
			return nil
		}
	}
	return nil
}

func (s *Service) deletionTarget(ctx context.Context, channel, id string, stored Bot, storedOK bool) (Bot, error) {
	if storedOK {
		return stored, nil
	}

	target := Bot{
		ID:      id,
		Channel: channel,
		AgentID: id,
		UserID:  id,
	}
	if s.agents != nil {
		if a, ok := s.agents.Agent(id); ok {
			target.AgentID = a.ID
			target.Role = strings.ToLower(strings.TrimSpace(a.Role))
		}
	}
	if target.Role == "" {
		target.Role = string(RoleWorker)
	}
	switch strings.TrimSpace(channel) {
	case string(ChannelCSGClaw):
		target.UserID = id
	case string(ChannelFeishu):
		target.UserID = ""
		if s.feishu != nil {
			if user, ok, err := s.feishu.ResolveBotUser(ctx, id); err != nil {
				return Bot{}, fmt.Errorf("resolve feishu user for bot %q: %w", id, err)
			} else if ok {
				target.UserID = strings.TrimSpace(user.ID)
			}
		}
	}
	return target, nil
}

func (s *Service) deleteBackingAgent(ctx context.Context, target Bot) (bool, error) {
	if IsNotificationBot(target) {
		return false, nil
	}
	if s == nil || s.agents == nil {
		return false, nil
	}
	agentID := strings.TrimSpace(target.AgentID)
	if agentID == "" {
		return false, nil
	}
	role := strings.ToLower(strings.TrimSpace(target.Role))
	if role == "" {
		if a, ok := s.agents.Agent(agentID); ok {
			role = strings.ToLower(strings.TrimSpace(a.Role))
		}
	}
	if role != string(RoleWorker) {
		return false, nil
	}
	for _, b := range s.store.List() {
		if sameChannelBot(target, b) {
			continue
		}
		if strings.TrimSpace(b.AgentID) == agentID {
			return false, nil
		}
	}
	if err := s.agents.Delete(ctx, agentID); err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("delete backing agent %q: %w", agentID, err)
	}
	return true, nil
}

func (s *Service) deleteChannelUser(target Bot) (bool, error) {
	if IsNotificationBot(target) {
		botID := strings.TrimSpace(target.ID)
		userID := strings.TrimSpace(target.UserID)
		if userID != "" && botID != "" && userID != botID {
			return false, nil
		}
	}
	if s.channelUserStillReferenced(target) {
		return false, nil
	}
	userID := strings.TrimSpace(target.UserID)
	if userID == "" {
		return false, nil
	}
	switch strings.TrimSpace(target.Channel) {
	case string(ChannelCSGClaw):
		if s.im == nil {
			return false, nil
		}
		if err := s.im.DeleteUser(userID); err != nil {
			if isNotFoundError(err) {
				return false, nil
			}
			return false, fmt.Errorf("delete csgclaw user %q: %w", userID, err)
		}
		return true, nil
	case string(ChannelFeishu):
		if s.feishu == nil {
			return false, nil
		}
		if err := s.feishu.DeleteUser(userID); err != nil {
			if isNotFoundError(err) {
				return false, nil
			}
			return false, fmt.Errorf("delete feishu user %q: %w", userID, err)
		}
		return true, nil
	}
	return false, nil
}

func sameChannelBot(a, b Bot) bool {
	return strings.TrimSpace(a.Channel) == strings.TrimSpace(b.Channel) &&
		strings.TrimSpace(a.ID) == strings.TrimSpace(b.ID)
}

func (s *Service) channelUserStillReferenced(target Bot) bool {
	if s == nil {
		return false
	}
	userID := strings.TrimSpace(target.UserID)
	if userID == "" {
		userID = strings.TrimSpace(target.ID)
	}
	if userID == "" {
		return false
	}
	deletingAgentID := strings.TrimSpace(target.AgentID)
	if deletingAgentID == "" {
		deletingAgentID = strings.TrimSpace(target.ID)
	}
	if s.agents != nil {
		// The backing agent still exists in the store until deleteBackingAgent runs.
		// Only treat other agents as references.
		if _, ok := s.agents.Agent(userID); ok && userID != deletingAgentID {
			return true
		}
		if botID := strings.TrimSpace(target.ID); botID != "" && botID != userID {
			if _, ok := s.agents.Agent(botID); ok && botID != deletingAgentID {
				return true
			}
		}
	}
	if s.store == nil {
		return false
	}
	for _, b := range s.store.List() {
		if sameChannelBot(target, b) {
			continue
		}
		if strings.TrimSpace(b.UserID) == userID {
			return true
		}
		if !IsNotificationBot(b) && strings.TrimSpace(b.AgentID) == userID {
			return true
		}
	}
	return false
}

func isNotFoundError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "not found")
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
		created, err = s.agents.Create(ctx, agent.CreateRequest{
			Spec: agent.CreateAgentSpec{
				ID:             normalized.ID,
				Name:           normalized.Name,
				Description:    normalized.Description,
				Image:          normalized.Image,
				Avatar:         normalized.Avatar,
				Role:           agent.RoleWorker,
				RuntimeKind:    normalized.RuntimeKind,
				FromTemplate:   normalized.FromTemplate,
				RuntimeOptions: utils.CloneAnyMap(normalized.RuntimeOptions),
				AgentProfile:   agentProfileFromBotRequest(normalized.AgentProfile),
			},
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
		Avatar:      strings.TrimSpace(created.Avatar),
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
			Avatar:      created.Avatar,
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
			Avatar: created.Avatar,
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
