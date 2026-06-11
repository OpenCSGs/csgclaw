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
