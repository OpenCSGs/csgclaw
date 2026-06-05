package participant

import (
	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
)

const (
	ChannelCSGClaw = "csgclaw"
	ChannelFeishu  = "feishu"

	TypeHuman        = "human"
	TypeAgent        = "agent"
	TypeNotification = "notification"

	ChannelUserKindLocalUserID = "local_user_id"
	ChannelUserKindOpenID      = "open_id"

	BindingModeCreate = "create"
	BindingModeReuse  = "reuse"
	BindingModeNone   = "none"

	LifecycleStatusActive = "active"
)

type Participant = apitypes.Participant

type ChannelUserSpec struct {
	Ref  string `json:"ref,omitempty"`
	Kind string `json:"kind,omitempty"`
}

type AgentBindingSpec struct {
	Mode    string                 `json:"mode,omitempty"`
	AgentID string                 `json:"agent_id,omitempty"`
	Agent   *agent.CreateAgentSpec `json:"agent,omitempty"`
}

type CreateRequest struct {
	ID            string           `json:"id,omitempty"`
	Channel       string           `json:"channel,omitempty"`
	Type          string           `json:"type"`
	Name          string           `json:"name"`
	Avatar        string           `json:"avatar,omitempty"`
	ChannelAppRef string           `json:"channel_app_ref,omitempty"`
	ChannelUser   ChannelUserSpec  `json:"channel_user,omitempty"`
	AgentBinding  AgentBindingSpec `json:"agent_binding,omitempty"`
	Metadata      map[string]any   `json:"metadata,omitempty"`
}

type UpdateRequest struct {
	Name        *string        `json:"name,omitempty"`
	Avatar      *string        `json:"avatar,omitempty"`
	Mentionable *bool          `json:"mentionable,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type ListOptions struct {
	Channel string
	Type    string
	AgentID string
}

type DeleteOptions struct {
	DeleteAgent string
}

const DeleteAgentIfUnreferenced = "if_unreferenced"
