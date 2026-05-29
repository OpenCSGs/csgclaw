package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"csgclaw/internal/channel/csgclaw/notification_bot"
	"csgclaw/internal/im"
	"csgclaw/internal/utils"
)

func (s *Service) Reload() error {
	if s == nil || s.store == nil {
		return fmt.Errorf("bot store is required")
	}
	return s.store.Reload()
}

func (s *Service) ListNotificationBots(channel string) ([]Bot, error) {
	return s.List(channel, "", BotTypeNotification)
}

func (s *Service) LookupNotificationBotForDelivery(channel, id string) (runtimeOptions map[string]any, userID string, ok bool) {
	if s == nil || s.store == nil {
		return nil, "", false
	}
	b, found, err := s.store.GetByChannelID(channel, id)
	if err != nil || !found || !IsNotificationBot(b) {
		return nil, "", false
	}
	userID = strings.TrimSpace(b.UserID)
	if userID == "" {
		userID = strings.TrimSpace(b.ID)
	}
	return notification_bot.FlatFromRuntimeOptionsMap(b.RuntimeOptions), userID, true
}

func (s *Service) GetNotificationBot(channel, id string) (Bot, error) {
	if s == nil || s.store == nil {
		return Bot{}, fmt.Errorf("bot store is required")
	}
	b, ok, err := s.store.GetByChannelID(channel, id)
	if err != nil {
		return Bot{}, err
	}
	if !ok || !IsNotificationBot(b) {
		return Bot{}, fmt.Errorf("notification bot %q not found", id)
	}
	return s.presentNotificationBot(b), nil
}

func (s *Service) CreateNotificationBot(ctx context.Context, req CreateRequest) (Bot, error) {
	if s == nil || s.store == nil {
		return Bot{}, fmt.Errorf("bot store is required")
	}
	req.Type = BotTypeNotification
	if strings.TrimSpace(req.Role) == "" {
		req.Role = string(RoleWorker)
	}
	normalized, err := NormalizeCreateRequest(req)
	if err != nil {
		return Bot{}, err
	}
	if normalized.Channel != string(ChannelCSGClaw) {
		return Bot{}, fmt.Errorf("notification bots are only supported on channel %q", ChannelCSGClaw)
	}
	if existing, ok := s.findByChannelName(normalized.Channel, normalized.Name); ok {
		return Bot{}, fmt.Errorf("bot name %q already exists in channel %q with id %q", normalized.Name, normalized.Channel, existing.ID)
	}
	botID := notificationBotID(normalized)
	if err := s.validateNotificationBotID(normalized.Channel, botID); err != nil {
		return Bot{}, err
	}

	userID, userCreatedAt, err := s.ensureChannelUserForBot(ctx, normalized.Channel, channelBotIdentity{
		ID:          botID,
		Name:        normalized.Name,
		Description: normalized.Description,
		Avatar:      normalized.Avatar,
		Role:        "Worker",
	})
	if err != nil {
		return Bot{}, err
	}
	createdAt := userCreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	runtimeOptions := utils.CloneAnyMap(normalized.RuntimeOptions)
	flat := notification_bot.NormalizeFlatForStorage(notification_bot.FlatFromRuntimeOptionsMap(runtimeOptions))
	b := Bot{
		ID:             botID,
		Name:           normalized.Name,
		Description:    normalized.Description,
		Avatar:         normalized.Avatar,
		Type:           BotTypeNotification,
		Role:           string(RoleWorker),
		Channel:        normalized.Channel,
		UserID:         userID,
		RuntimeOptions: flat,
		CreatedAt:      createdAt,
	}
	if err := s.store.Save(b); err != nil {
		return Bot{}, err
	}
	return s.presentNotificationBot(b), nil
}

func (s *Service) PatchNotificationBot(ctx context.Context, channel, id string, patch CreateRequest) (Bot, error) {
	if s == nil || s.store == nil {
		return Bot{}, fmt.Errorf("bot store is required")
	}
	existing, ok, err := s.store.GetByChannelID(channel, id)
	if err != nil {
		return Bot{}, err
	}
	if !ok || !IsNotificationBot(existing) {
		return Bot{}, fmt.Errorf("notification bot %q not found", id)
	}
	if name := strings.TrimSpace(patch.Name); name != "" {
		existing.Name = name
	}
	if desc := strings.TrimSpace(patch.Description); desc != "" {
		existing.Description = desc
	}
	if avatar := strings.TrimSpace(patch.Avatar); avatar != "" {
		existing.Avatar = avatar
	}
	if len(patch.RuntimeOptions) > 0 {
		merged := notification_bot.MergeRuntimeOptionsPatch(
			notification_bot.FlatFromRuntimeOptionsMap(existing.RuntimeOptions),
			patch.RuntimeOptions,
		)
		existing.RuntimeOptions = notification_bot.NormalizeFlatForStorage(merged)
	}
	if err := s.store.Save(existing); err != nil {
		return Bot{}, err
	}
	return s.presentNotificationBot(existing), nil
}

func (s *Service) presentNotificationBot(b Bot) Bot {
	b.Type = BotTypeNotification
	flat := notification_bot.FlatFromRuntimeOptionsMap(b.RuntimeOptions)
	b.Available = notification_bot.ProfileDeliveryComplete(flat)
	view := notification_bot.ViewRuntimeOptionsForAPI(b.RuntimeOptions)
	if len(view) > 0 {
		b.RuntimeOptions = view
	}
	return b
}

func notificationBotID(req CreateRequest) string {
	if id := strings.TrimSpace(req.ID); id != "" {
		return id
	}
	return NotificationBotIDPrefix + strings.TrimSpace(req.Name)
}

func (s *Service) validateNotificationBotID(channel, botID string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("bot store is required")
	}
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return fmt.Errorf("bot id is required")
	}
	if _, ok, err := s.store.GetByChannelID(channel, botID); err != nil {
		return err
	} else if ok {
		return fmt.Errorf("bot id %q already exists", botID)
	}
	if s.agents != nil {
		if a, ok := s.agents.Agent(botID); ok {
			return fmt.Errorf("bot id %q conflicts with existing agent %q (role %q)", botID, a.ID, a.Role)
		}
	}
	for _, b := range s.store.List() {
		if b.Channel != channel {
			continue
		}
		if IsNotificationBot(b) {
			continue
		}
		if strings.TrimSpace(b.ID) == botID || strings.TrimSpace(b.AgentID) == botID {
			return fmt.Errorf("bot id %q conflicts with existing channel bot %q", botID, b.ID)
		}
	}
	return nil
}

// BotByChannelID returns the stored bot record without API redaction.
func (s *Service) BotByChannelID(channel, id string) (Bot, bool, error) {
	if s == nil || s.store == nil {
		return Bot{}, false, fmt.Errorf("bot store is required")
	}
	return s.store.GetByChannelID(channel, id)
}

type channelBotIdentity struct {
	ID          string
	Name        string
	Description string
	Handle      string
	Role        string
	Avatar      string
}

func (s *Service) ensureChannelUserForBot(ctx context.Context, channelName string, identity channelBotIdentity) (string, time.Time, error) {
	handle := strings.TrimSpace(identity.Handle)
	if handle == "" {
		if h, ok := sanitizeHandle(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(identity.Name), " ", "-"))); ok {
			handle = h
		} else if h, ok := notificationBotHandleFromID(identity.ID); ok {
			handle = h
		} else {
			handle = "notification"
		}
	}
	switch channelName {
	case string(ChannelCSGClaw):
		if s.imProv == nil {
			return "", time.Time{}, fmt.Errorf("im provisioner is required")
		}
		result, err := s.imProv.EnsureAgentUser(ctx, im.AgentIdentity{
			ID:          identity.ID,
			Name:        identity.Name,
			Description: identity.Description,
			Handle:      handle,
			Role:        identity.Role,
			Avatar:      identity.Avatar,
		})
		if err != nil {
			return "", time.Time{}, fmt.Errorf("failed to ensure im user: %w", err)
		}
		return result.User.ID, result.User.CreatedAt, nil
	default:
		return "", time.Time{}, fmt.Errorf("notification bots are only supported on channel %q", ChannelCSGClaw)
	}
}

func notificationBotHandleFromID(id string) (string, bool) {
	stem := strings.TrimSpace(id)
	for _, prefix := range []string{NotificationBotIDPrefix, "u-"} {
		if h, ok := sanitizeHandle(strings.ToLower(strings.TrimPrefix(stem, prefix))); ok {
			return h, true
		}
	}
	return "", false
}
