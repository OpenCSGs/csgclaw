package apitypes

import "time"

type Participant struct {
	ID              string         `json:"id"`
	Channel         string         `json:"channel"`
	Type            string         `json:"type"`
	Name            string         `json:"name"`
	Avatar          string         `json:"avatar,omitempty"`
	ChannelUserRef  string         `json:"channel_user_ref,omitempty"`
	ChannelUserKind string         `json:"channel_user_kind,omitempty"`
	ChannelAppRef   string         `json:"channel_app_ref,omitempty"`
	AgentID         string         `json:"agent_id,omitempty"`
	LifecycleStatus string         `json:"lifecycle_status"`
	Presence        string         `json:"presence,omitempty"`
	Mentionable     bool           `json:"mentionable"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type ParticipantRef struct {
	Channel string `json:"channel,omitempty"`
	ID      string `json:"id"`
}
