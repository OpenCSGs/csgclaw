package team

import (
	"context"
	"fmt"
	"strings"
)

type CreateTeamWithRoomInput struct {
	RoomID               string
	Channel              string
	Title                string
	LeadParticipantID    string
	MemberParticipantIDs []string
}

func (s *Service) CreateTeamWithRoom(ctx context.Context, adapter TeamChannelAdapter, input CreateTeamWithRoomInput) (TeamMeta, error) {
	if adapter == nil {
		return TeamMeta{}, fmt.Errorf("team adapter is required")
	}
	channel := strings.TrimSpace(input.Channel)
	if channel == "" {
		channel = adapter.Channel()
	}
	if !strings.EqualFold(channel, adapter.Channel()) {
		return TeamMeta{}, fmt.Errorf("unsupported team channel %q", channel)
	}

	memberParticipantIDs, err := uniqueParticipantIDs(input.MemberParticipantIDs)
	if err != nil {
		return TeamMeta{}, err
	}
	leadParticipantID, err := requireCanonicalParticipantID("lead_participant_id", input.LeadParticipantID)
	if err != nil {
		return TeamMeta{}, err
	}
	roomID := strings.TrimSpace(input.RoomID)
	title := strings.TrimSpace(input.Title)
	roomRef, err := adapter.EnsureRoom(ctx, EnsureRoomRequest{
		RoomID:               roomID,
		Title:                title,
		LeadParticipantID:    leadParticipantID,
		CreatorParticipantID: leadParticipantID,
		MemberParticipantIDs: memberParticipantIDs,
	})
	if err != nil {
		return TeamMeta{}, err
	}
	if roomID != "" && len(memberParticipantIDs) > 0 {
		if err := adapter.AddMembers(ctx, AddMembersRequest{
			Room:                 roomRef,
			InviterParticipantID: leadParticipantID,
			MemberParticipantIDs: memberParticipantIDs,
		}); err != nil {
			return TeamMeta{}, err
		}
	}

	return s.CreateTeam(CreateTeamInput{
		RoomID:            roomRef.RoomID,
		Channel:           channel,
		Title:             title,
		LeadParticipantID: leadParticipantID,
	})
}

func uniqueTrimmedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func uniqueParticipantIDs(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value, err := requireCanonicalParticipantID("participant_id", value)
		if err != nil {
			return nil, err
		}
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, nil
}
