package participantprovider

import (
	"fmt"
	"log/slog"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/participant"
)

const feishuAdminParticipantID = "admin"

type ParticipantConfigProvider struct {
	path string
}

func New(path string) *ParticipantConfigProvider {
	return &ParticipantConfigProvider{path: strings.TrimSpace(path)}
}

func (p *ParticipantConfigProvider) BotConfig(participantID string) (feishu.AppConfig, bool) {
	item, ok := p.getParticipant(strings.TrimSpace(participantID))
	if !ok {
		return feishu.AppConfig{}, false
	}
	app, ok := appConfigFromParticipant(item)
	return app, ok
}

func (p *ParticipantConfigProvider) BotConfigForAgent(agentID string) (string, feishu.AppConfig, bool) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "", feishu.AppConfig{}, false
	}
	store, err := p.openStore()
	if err != nil {
		slog.Warn("read feishu participant config failed", "agent_id", agentID, "error", err)
		return "", feishu.AppConfig{}, false
	}
	items := store.List(participant.ListOptions{
		Channel: participant.ChannelFeishu,
		Type:    participant.TypeAgent,
		AgentID: agentID,
	})
	if len(items) == 0 {
		return "", feishu.AppConfig{}, false
	}

	canonicalID := agent.ParticipantIDForAgent("", agentID)
	var fallback *apitypes.Participant
	for i := range items {
		app, ok := appConfigFromParticipant(items[i])
		if !ok {
			continue
		}
		if strings.TrimSpace(items[i].ID) == canonicalID {
			if fallback != nil {
				slog.Warn("multiple feishu participants configured for agent; using canonical participant",
					"agent_id", agentID,
					"participant_id", canonicalID,
					"ignored_participant_id", fallback.ID)
			}
			return items[i].ID, app, true
		}
		if fallback == nil {
			candidate := items[i]
			fallback = &candidate
		}
	}
	if fallback == nil {
		return "", feishu.AppConfig{}, false
	}
	app, ok := appConfigFromParticipant(*fallback)
	if !ok {
		return "", feishu.AppConfig{}, false
	}
	slog.Debug("using feishu participant for agent",
		"agent_id", agentID,
		"participant_id", fallback.ID,
		"canonical_participant_id", canonicalID)
	return fallback.ID, app, true
}

func (p *ParticipantConfigProvider) DefaultAdminOpenID() (string, bool) {
	store, err := p.openStore()
	if err != nil {
		slog.Warn("read feishu admin participant config failed", "error", err)
		return "", false
	}
	return p.defaultAdminOpenIDFromStore(store)
}

func (p *ParticipantConfigProvider) MentionOpenID(participantID string) (string, bool) {
	item, ok := p.getParticipant(strings.TrimSpace(participantID))
	if !ok {
		return "", false
	}
	return openIDFromHumanParticipant(item)
}

func (p *ParticipantConfigProvider) Snapshot() feishu.Snapshot {
	store, err := p.openStore()
	if err != nil {
		slog.Warn("read feishu participant snapshot failed", "error", err)
		return feishu.Snapshot{}
	}
	snapshot := feishu.Snapshot{Bots: make(map[string]feishu.AppConfig)}
	if adminOpenID, ok := p.defaultAdminOpenIDFromStore(store); ok {
		snapshot.AdminOpenID = adminOpenID
	}
	for _, item := range store.List(participant.ListOptions{Channel: participant.ChannelFeishu, Type: participant.TypeAgent}) {
		app, ok := appConfigFromParticipant(item)
		if !ok {
			continue
		}
		snapshot.Bots[strings.TrimSpace(item.ID)] = app
	}
	if len(snapshot.Bots) == 0 {
		snapshot.Bots = nil
	}
	return snapshot
}

func (p *ParticipantConfigProvider) defaultAdminOpenIDFromStore(store *participant.Store) (string, bool) {
	if store == nil {
		return "", false
	}
	for _, id := range []string{feishuAdminParticipantID, "pt-admin"} {
		if item, ok := store.Get(participant.ChannelFeishu, id); ok {
			if openID, ok := openIDFromHumanParticipant(item); ok {
				return openID, true
			}
		}
	}
	for _, item := range store.List(participant.ListOptions{Channel: participant.ChannelFeishu, Type: participant.TypeHuman}) {
		name := strings.ToLower(strings.TrimSpace(item.Name))
		id := strings.ToLower(strings.TrimSpace(item.ID))
		if name != "admin" && !strings.HasPrefix(id, "pt-admin") {
			continue
		}
		if openID, ok := openIDFromHumanParticipant(item); ok {
			return openID, true
		}
	}
	return "", false
}

func (p *ParticipantConfigProvider) getParticipant(participantID string) (apitypes.Participant, bool) {
	if participantID == "" {
		return apitypes.Participant{}, false
	}
	store, err := p.openStore()
	if err != nil {
		slog.Warn("read feishu participant config failed", "participant_id", participantID, "error", err)
		return apitypes.Participant{}, false
	}
	return store.Get(participant.ChannelFeishu, participantID)
}

func (p *ParticipantConfigProvider) openStore() (*participant.Store, error) {
	if p == nil {
		return nil, fmt.Errorf("feishu participant config provider is nil")
	}
	return participant.NewStore(p.path)
}

func appConfigFromParticipant(item apitypes.Participant) (feishu.AppConfig, bool) {
	if strings.TrimSpace(item.Channel) != participant.ChannelFeishu ||
		strings.TrimSpace(item.Type) != participant.TypeAgent ||
		strings.TrimSpace(item.ChannelUserKind) != participant.ChannelUserKindAppID {
		return feishu.AppConfig{}, false
	}
	appID := channelAppConfigString(item.ChannelAppConfig, "app_id")
	appSecret := channelAppConfigString(item.ChannelAppConfig, participant.ChannelAppConfigAppSecretKey)
	if appID == "" || appSecret == "" {
		return feishu.AppConfig{}, false
	}
	return feishu.AppConfig{AppID: appID, AppSecret: appSecret}, true
}

func openIDFromHumanParticipant(item apitypes.Participant) (string, bool) {
	if strings.TrimSpace(item.Type) != participant.TypeHuman ||
		strings.TrimSpace(item.ChannelUserKind) != participant.ChannelUserKindOpenID {
		return "", false
	}
	openID := strings.TrimSpace(item.ChannelUserRef)
	return openID, openID != ""
}

func channelAppConfigString(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	value, ok := values[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
