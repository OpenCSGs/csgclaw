package team

import (
	"context"
	"strings"
)

const (
	DefaultExecutionChannel = "csgclaw"
	FeishuExecutionChannel  = "feishu"
)

// TeamChannelAdapter is the narrow projection boundary between team
// orchestration and a concrete channel implementation.
type TeamChannelAdapter interface {
	Channel() string
	EnsureRoom(ctx context.Context, req EnsureRoomRequest) (RoomRef, error)
	AddMembers(ctx context.Context, req AddMembersRequest) error
	SendMessage(ctx context.Context, req SendMessageRequest) (MessageRef, error)
}

// ChannelAdapter remains as a compatibility alias during the MVP rollout.
type ChannelAdapter = TeamChannelAdapter

type AdapterRegistry struct {
	adapters map[string]TeamChannelAdapter
}

func NewAdapterRegistry(adapters ...TeamChannelAdapter) *AdapterRegistry {
	registry := &AdapterRegistry{adapters: make(map[string]TeamChannelAdapter)}
	for _, adapter := range adapters {
		registry.Register(adapter)
	}
	return registry
}

func (r *AdapterRegistry) Register(adapter TeamChannelAdapter) {
	if r == nil || adapter == nil {
		return
	}
	channel := NormalizeExecutionChannel(adapter.Channel())
	if channel == "" {
		return
	}
	r.adapters[channel] = adapter
}

func (r *AdapterRegistry) Adapter(channel string) (TeamChannelAdapter, bool) {
	channel = NormalizeExecutionChannel(channel)
	if channel == "" {
		channel = DefaultExecutionChannel
	}
	if r == nil || r.adapters == nil {
		return nil, false
	}
	adapter, ok := r.adapters[channel]
	return adapter, ok
}

func NormalizeExecutionChannel(channel string) string {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "", DefaultExecutionChannel:
		return DefaultExecutionChannel
	case FeishuExecutionChannel:
		return FeishuExecutionChannel
	default:
		return strings.ToLower(strings.TrimSpace(channel))
	}
}

type RoomRef struct {
	Channel string `json:"channel"`
	RoomID  string `json:"room_id"`
}

type MessageRef struct {
	Channel   string `json:"channel"`
	RoomID    string `json:"room_id"`
	MessageID string `json:"message_id,omitempty"`
}

type EnsureRoomRequest struct {
	TeamID               string   `json:"team_id,omitempty"`
	RoomID               string   `json:"room_id,omitempty"`
	Title                string   `json:"title,omitempty"`
	LeadParticipantID    string   `json:"lead_participant_id"`
	CreatorParticipantID string   `json:"creator_participant_id,omitempty"`
	MemberParticipantIDs []string `json:"member_participant_ids,omitempty"`
}

type AddMembersRequest struct {
	Room                 RoomRef  `json:"room"`
	InviterParticipantID string   `json:"inviter_participant_id,omitempty"`
	MemberParticipantIDs []string `json:"member_participant_ids"`
}

type SendMessageRequest struct {
	Room                RoomRef `json:"room"`
	SenderParticipantID string  `json:"sender_participant_id"`
	MentionID           string  `json:"mention_id,omitempty"`
	Kind                string  `json:"kind,omitempty"`
	Content             string  `json:"content"`
	IdempotencyKey      string  `json:"idempotency_key,omitempty"`
}
