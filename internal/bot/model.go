package bot

import (
	"fmt"
	"slices"
	"strings"

	"csgclaw/internal/apitypes"
)

type Role string

const (
	RoleManager Role = "manager"
	RoleWorker  Role = "worker"
)

type Channel string

const (
	ChannelCSGClaw Channel = "csgclaw"
	ChannelFeishu  Channel = "feishu"
)

type Bot = apitypes.Bot

type CreateRequest = apitypes.CreateBotRequest

func NormalizeCreateRequest(req CreateRequest) (CreateRequest, error) {
	req.ID = strings.TrimSpace(req.ID)
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.Image = strings.TrimSpace(req.Image)
	req.ModelID = strings.TrimSpace(req.ModelID)
	req.RuntimeKind = strings.TrimSpace(req.RuntimeKind)
	if req.Name == "" {
		return CreateRequest{}, fmt.Errorf("name is required")
	}

	role, err := NormalizeRole(req.Role)
	if err != nil {
		return CreateRequest{}, err
	}
	channel, err := NormalizeChannel(req.Channel)
	if err != nil {
		return CreateRequest{}, err
	}
	req.Role = string(role)
	req.Channel = string(channel)
	return req, nil
}

func ValidateCreateRequest(req CreateRequest) error {
	_, err := NormalizeCreateRequest(req)
	return err
}

func NormalizeBot(b Bot) (Bot, error) {
	b.ID = strings.TrimSpace(b.ID)
	b.Name = strings.TrimSpace(b.Name)
	b.Description = strings.TrimSpace(b.Description)
	b.AgentID = strings.TrimSpace(b.AgentID)
	b.UserID = strings.TrimSpace(b.UserID)
	if b.ID == "" {
		return Bot{}, fmt.Errorf("id is required")
	}
	if b.Name == "" {
		return Bot{}, fmt.Errorf("name is required")
	}

	role, err := NormalizeRole(b.Role)
	if err != nil {
		return Bot{}, err
	}
	channel, err := NormalizeChannel(b.Channel)
	if err != nil {
		return Bot{}, err
	}
	b.Role = string(role)
	b.Channel = string(channel)
	b.Available = true
	return b, nil
}

func ValidateBot(b Bot) error {
	_, err := NormalizeBot(b)
	return err
}

func NormalizeRole(role string) (Role, error) {
	switch Role(strings.ToLower(strings.TrimSpace(role))) {
	case RoleManager:
		return RoleManager, nil
	case RoleWorker:
		return RoleWorker, nil
	default:
		return "", fmt.Errorf("role must be one of %q or %q", RoleManager, RoleWorker)
	}
}

func NormalizeChannel(channel string) (Channel, error) {
	switch Channel(strings.ToLower(strings.TrimSpace(channel))) {
	case "", ChannelCSGClaw:
		return ChannelCSGClaw, nil
	case ChannelFeishu:
		return ChannelFeishu, nil
	default:
		return "", fmt.Errorf("channel must be one of %q or %q", ChannelCSGClaw, ChannelFeishu)
	}
}

func sortedBotsFromMap(items map[string]Bot) []Bot {
	bots := make([]Bot, 0, len(items))
	for _, b := range items {
		bots = append(bots, b)
	}
	slices.SortFunc(bots, func(a, b Bot) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			if a.ID != b.ID {
				if a.ID < b.ID {
					return -1
				}
				return 1
			}
			if a.Channel < b.Channel {
				return -1
			}
			if a.Channel > b.Channel {
				return 1
			}
			return 0
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	})
	return bots
}
