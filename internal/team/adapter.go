package team

import "context"

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
	TeamID       string   `json:"team_id,omitempty"`
	RoomID       string   `json:"room_id,omitempty"`
	Title        string   `json:"title,omitempty"`
	LeadBotID    string   `json:"lead_bot_id"`
	CreatorBotID string   `json:"creator_bot_id,omitempty"`
	MemberBotIDs []string `json:"member_bot_ids,omitempty"`
}

type AddMembersRequest struct {
	Room         RoomRef  `json:"room"`
	InviterBotID string   `json:"inviter_bot_id,omitempty"`
	MemberBotIDs []string `json:"member_bot_ids"`
}

type SendMessageRequest struct {
	Room           RoomRef `json:"room"`
	SenderBotID    string  `json:"sender_bot_id"`
	Kind           string  `json:"kind,omitempty"`
	Content        string  `json:"content"`
	IdempotencyKey string  `json:"idempotency_key,omitempty"`
}
