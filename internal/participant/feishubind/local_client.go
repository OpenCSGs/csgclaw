package feishubind

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/participant"
)

type LocalClient struct {
	agents       *agent.Service
	participants *participant.Service
}

func NewLocalClient(agentSvc *agent.Service, participantSvc *participant.Service) *LocalClient {
	return &LocalClient{agents: agentSvc, participants: participantSvc}
}

func (c *LocalClient) ListParticipants(_ context.Context, channel, typ, agentID string) ([]apitypes.Participant, error) {
	if c == nil || c.participants == nil {
		return nil, fmt.Errorf("participant service is required")
	}
	return c.participants.List(participant.ListOptions{Channel: channel, Type: typ, AgentID: agentID}), nil
}

func (c *LocalClient) ListAgents(_ context.Context) ([]apitypes.Agent, error) {
	if c == nil || c.agents == nil {
		return nil, fmt.Errorf("agent service is required")
	}
	return presentAgents(c.agents.List()), nil
}

func (c *LocalClient) GetAgent(_ context.Context, id string) (apitypes.Agent, error) {
	if c == nil || c.agents == nil {
		return apitypes.Agent{}, fmt.Errorf("agent service is required")
	}
	id = strings.TrimSpace(id)
	got, ok := c.agents.Agent(id)
	if !ok {
		return apitypes.Agent{}, fmt.Errorf("agent %q not found", id)
	}
	return presentAgent(got), nil
}

func (c *LocalClient) CreateParticipant(ctx context.Context, req participant.CreateRequest) (apitypes.Participant, error) {
	if c == nil || c.participants == nil {
		return apitypes.Participant{}, fmt.Errorf("participant service is required")
	}
	return c.participants.Create(ctx, req)
}

func (c *LocalClient) UpdateParticipant(ctx context.Context, channel, id string, req participant.UpdateRequest) (apitypes.Participant, error) {
	if c == nil || c.participants == nil {
		return apitypes.Participant{}, fmt.Errorf("participant service is required")
	}
	updated, ok, err := c.participants.Update(ctx, channel, id, req)
	if err != nil {
		return apitypes.Participant{}, err
	}
	if !ok {
		return apitypes.Participant{}, fmt.Errorf("participant %s:%s not found", channel, id)
	}
	return updated, nil
}

func (c *LocalClient) RecreateAgent(ctx context.Context, id string) (apitypes.Agent, error) {
	if c == nil || c.agents == nil {
		return apitypes.Agent{}, fmt.Errorf("agent service is required")
	}
	recreated, err := c.agents.Recreate(ctx, id)
	if err != nil {
		return apitypes.Agent{}, err
	}
	return presentAgent(recreated), nil
}

func presentAgents(items []agent.Agent) []apitypes.Agent {
	out := make([]apitypes.Agent, 0, len(items))
	for _, item := range items {
		out = append(out, presentAgent(item))
	}
	return out
}

func presentAgent(item agent.Agent) apitypes.Agent {
	return apitypes.Agent{
		ID:           item.ID,
		Name:         item.Name,
		Description:  item.Description,
		Instructions: item.Instructions,
		RuntimeID:    item.RuntimeID,
		RuntimeKind:  item.RuntimeKind,
		Image:        item.Image,
		Avatar:       item.Avatar,
		BoxID:        item.BoxID,
		Role:         item.Role,
		Status:       item.Status,
		CreatedAt:    item.CreatedAt,
		Profile:      item.Profile,
	}
}
